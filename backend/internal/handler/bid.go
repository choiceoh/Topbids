package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/bid"
	"github.com/choiceoh/phaeton/backend/internal/infra/apierr"
	"github.com/choiceoh/phaeton/backend/internal/middleware"
	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// BidHandler serves Topbids domain endpoints: award, PO distribution, etc.
// Read CRUD for rfqs/bids/suppliers uses the generic DynHandler.
type BidHandler struct {
	pool  *pgxpool.Pool
	cache *schema.Cache
}

func NewBidHandler(pool *pgxpool.Pool, cache *schema.Cache) *BidHandler {
	return &BidHandler{pool: pool, cache: cache}
}

// Award handles POST /api/bid/rfqs/{rfqId}/award.
//
// Selects the winning bid, transitions RFQ/bid statuses, and chains PO
// distribution. Idempotent: calling twice on an already-awarded RFQ returns
// the original winner with idempotent=true and takes no mutating action.
//
// RBAC: router-level `RequireRole("director","pm")` + defensive in-handler
// check so a routing misconfiguration can't silently grant access.
func (h *BidHandler) Award(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUser(r.Context())
	if !ok {
		apierr.Unauthorized("not authenticated").Write(w)
		return
	}
	// Defence in depth: reject non-admin even if the router middleware is
	// ever misconfigured. Suppliers must never reach this endpoint.
	if user.Role != RoleDirector && user.Role != RolePM {
		apierr.Forbidden("only director or pm can award RFQs").Write(w)
		return
	}

	rfqID := chi.URLParam(r, "rfqId")
	if rfqID == "" {
		apierr.BadRequest("rfqId is required").Write(w)
		return
	}

	actor := bid.Actor{UserID: user.UserID, Name: user.Name, IP: clientIP(r)}
	result, err := bid.AwardRFQ(r.Context(), h.pool, rfqID, actor)
	if err != nil {
		writeAwardErr(w, r, rfqID, err)
		return
	}

	slog.Info("bid awarded",
		"rfq_id", result.RFQID,
		"winner_bid_id", result.WinnerBidID,
		"eval_method", result.EvalMethod,
		"total_bids", result.TotalBids,
		"idempotent", result.Idempotent)

	writeJSON(w, http.StatusOK, result)
}

// AuditEntry is the wire shape of a _meta.bid_audit_log row. Lists are
// paginated and sorted newest-first; the JSON envelope matches the standard
// list response used by the dynamic data API for consistency.
type auditLogRow struct {
	ID        string         `json:"id"`
	ActorID   *string        `json:"actor_id,omitempty"`
	ActorName string         `json:"actor_name"`
	Action    string         `json:"action"`
	AppSlug   string         `json:"app_slug"`
	RowID     *string        `json:"row_id,omitempty"`
	IP        *string        `json:"ip,omitempty"`
	Detail    map[string]any `json:"detail"`
	CreatedAt string         `json:"created_at"`
}

// AuditLog handles GET /api/bid/audit.
//
// Returns a paginated list of audit rows filtered by action, app_slug,
// row_id, actor_id, and an optional created_at range. Director only —
// the audit log contains attribution data and shouldn't leak to line
// operators (pm) or anyone else.
//
// Query params:
//
//	?action=submit|read_sealed|read_opened|open|award|distribute
//	?app_slug=rfqs|bids|suppliers|purchase_orders
//	?row_id=<uuid>
//	?actor_id=<uuid>
//	?from=<RFC3339>  &to=<RFC3339>   (inclusive)
//	?page=1 &limit=50                (limit max 500)
func (h *BidHandler) AuditLog(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUser(r.Context())
	if !ok {
		apierr.Unauthorized("not authenticated").Write(w)
		return
	}
	if user.Role != RoleDirector {
		apierr.Forbidden("director only").Write(w)
		return
	}

	q := r.URL.Query()
	page, limit, offset := ParsePagination(q)
	if limit > 500 {
		limit = 500
	}

	wheres := []string{"1=1"}
	args := []any{}
	n := 1

	if v := q.Get("action"); v != "" {
		wheres = append(wheres, fmt.Sprintf("action = $%d", n))
		args = append(args, v)
		n++
	}
	if v := q.Get("app_slug"); v != "" {
		wheres = append(wheres, fmt.Sprintf("app_slug = $%d", n))
		args = append(args, v)
		n++
	}
	if v := q.Get("row_id"); v != "" {
		wheres = append(wheres, fmt.Sprintf("row_id = $%d", n))
		args = append(args, v)
		n++
	}
	if v := q.Get("actor_id"); v != "" {
		wheres = append(wheres, fmt.Sprintf("actor_id = $%d", n))
		args = append(args, v)
		n++
	}
	if v := q.Get("from"); v != "" {
		wheres = append(wheres, fmt.Sprintf("created_at >= $%d", n))
		args = append(args, v)
		n++
	}
	if v := q.Get("to"); v != "" {
		wheres = append(wheres, fmt.Sprintf("created_at <= $%d", n))
		args = append(args, v)
		n++
	}

	whereSQL := strings.Join(wheres, " AND ")

	var total int64
	if err := h.pool.QueryRow(r.Context(),
		"SELECT COUNT(*) FROM _meta.bid_audit_log WHERE "+whereSQL, args...,
	).Scan(&total); err != nil {
		slog.Error("audit count", "error", err)
		apierr.WrapInternal("audit log count", err).Write(w)
		return
	}

	sql := "SELECT id::text, actor_id::text, actor_name, action, app_slug, " +
		"row_id::text, ip::text, detail, created_at " +
		"FROM _meta.bid_audit_log WHERE " + whereSQL +
		fmt.Sprintf(" ORDER BY created_at DESC LIMIT %d OFFSET %d", limit, offset)

	rows, err := h.pool.Query(r.Context(), sql, args...)
	if err != nil {
		slog.Error("audit list", "error", err)
		apierr.WrapInternal("audit log list", err).Write(w)
		return
	}
	defer rows.Close()

	out := make([]auditLogRow, 0, limit)
	for rows.Next() {
		var (
			row       auditLogRow
			actor     *string
			rowID     *string
			ip        *string
			detailRaw []byte
			createdAt time.Time
		)
		if err := rows.Scan(&row.ID, &actor, &row.ActorName, &row.Action, &row.AppSlug,
			&rowID, &ip, &detailRaw, &createdAt); err != nil {
			slog.Warn("audit scan", "error", err)
			continue
		}
		row.ActorID = nilIfEmpty(actor)
		row.RowID = nilIfEmpty(rowID)
		row.IP = nilIfEmpty(ip)
		row.CreatedAt = createdAt.Format(time.RFC3339)
		if len(detailRaw) > 0 {
			if err := json.Unmarshal(detailRaw, &row.Detail); err != nil {
				// Keep going — a corrupt detail row shouldn't fail the whole list.
				row.Detail = nil
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		slog.Error("audit rows err", "error", err)
		apierr.WrapInternal("audit log iterate", err).Write(w)
		return
	}

	totalPages := int((total + int64(limit) - 1) / int64(limit))
	writeJSON(w, http.StatusOK, map[string]any{
		"data":        out,
		"total":       total,
		"page":        page,
		"limit":       limit,
		"total_pages": totalPages,
	})
}

// nilIfEmpty returns nil when the pointer is nil or empty string, so JSON
// omitempty fires rather than emitting "".
func nilIfEmpty(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}

// writeAwardErr maps the semantic errors from bid.AwardRFQ to apierr
// responses so clients see structured `{code, message}` bodies instead
// of ad-hoc strings. Unknown errors fall through as 500.
func writeAwardErr(w http.ResponseWriter, r *http.Request, rfqID string, err error) {
	switch {
	case errors.Is(err, bid.ErrRFQNotFound):
		apierr.New(http.StatusNotFound, "RFQ_NOT_FOUND", err.Error()).Write(w)
	case errors.Is(err, bid.ErrRFQIneligible):
		apierr.New(http.StatusBadRequest, "RFQ_INELIGIBLE", err.Error()).Write(w)
	case errors.Is(err, bid.ErrNoBids):
		apierr.New(http.StatusBadRequest, "NO_ELIGIBLE_BIDS", err.Error()).Write(w)
	case errors.Is(err, bid.ErrTechScoreMissing):
		apierr.New(http.StatusBadRequest, "TECH_SCORE_MISSING", err.Error()).Write(w)
	default:
		slog.Error("awardRFQ", "rfq_id", rfqID, "error", err, "path", r.URL.Path)
		apierr.WrapInternal("award failed", err).Write(w)
	}
}
