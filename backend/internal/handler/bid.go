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

// Cancel handles POST /api/bid/rfqs/{rfqId}/cancel.
//
// Abandons an RFQ that was issued in error (wrong category, duplicate, etc.)
// and rejects any outstanding bids so suppliers see a definitive "탈락"
// outcome rather than a stuck "제출됨" badge. Director/pm only.
//
// Body (optional): {"reason": "..."} — recorded in the audit log so the
// team can review why an RFQ was pulled.
func (h *BidHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUser(r.Context())
	if !ok {
		apierr.Unauthorized("not authenticated").Write(w)
		return
	}
	if user.Role != RoleDirector && user.Role != RolePM {
		apierr.Forbidden("only director or pm can cancel RFQs").Write(w)
		return
	}

	rfqID := chi.URLParam(r, "rfqId")
	if rfqID == "" {
		apierr.BadRequest("rfqId is required").Write(w)
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	// Empty body is fine — reason is optional.
	_ = json.NewDecoder(r.Body).Decode(&body)

	actor := bid.Actor{UserID: user.UserID, Name: user.Name, IP: clientIP(r)}
	err := bid.CancelRFQ(r.Context(), h.pool, rfqID, strings.TrimSpace(body.Reason), actor)
	if err != nil {
		switch {
		case errors.Is(err, bid.ErrRFQNotFound):
			apierr.New(http.StatusNotFound, "RFQ_NOT_FOUND", err.Error()).Write(w)
		case errors.Is(err, bid.ErrRFQNotCancellable):
			apierr.New(http.StatusBadRequest, "RFQ_NOT_CANCELLABLE", err.Error()).Write(w)
		default:
			slog.Error("cancelRFQ", "rfq_id", rfqID, "error", err)
			apierr.WrapInternal("cancel failed", err).Write(w)
		}
		return
	}

	slog.Info("rfq cancelled", "rfq_id", rfqID, "actor", user.UserID, "reason", body.Reason)
	writeJSON(w, http.StatusOK, map[string]any{"rfq_id": rfqID, "status": "cancelled"})
}

// Withdraw handles POST /api/bid/bids/{bidId}/withdraw.
//
// Permits a supplier to retract their own bid while the parent RFQ is still
// accepting changes. Admins (director/pm) may also withdraw on behalf of a
// supplier — useful when a company calls to say they made a mistake.
//
// The per-row write guard (EnforceBidWriteOwnership) runs first so a supplier
// can't withdraw someone else's bid.
func (h *BidHandler) Withdraw(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUser(r.Context())
	if !ok {
		apierr.Unauthorized("not authenticated").Write(w)
		return
	}
	// Admins or the bid owner only. The guard below enforces ownership for
	// supplier callers; admins pass through unconditionally.
	switch user.Role {
	case RoleDirector, RolePM, "supplier":
	default:
		apierr.Forbidden("not permitted to withdraw bids").Write(w)
		return
	}

	bidID := chi.URLParam(r, "bidId")
	if bidID == "" {
		apierr.BadRequest("bidId is required").Write(w)
		return
	}

	// Row-ownership gate for suppliers. Also refuses if the parent RFQ has
	// already closed — identical policy to edit/delete.
	if user.Role == "supplier" {
		if err := bid.EnforceBidWriteOwnership(r.Context(), h.pool, user.Role, user.SupplierID,
			bid.OpUpdate, bidID, nil); err != nil {
			switch {
			case errors.Is(err, bid.ErrNotBidOwner):
				apierr.Forbidden(err.Error()).Write(w)
			case errors.Is(err, bid.ErrSupplierNotLinked):
				apierr.Forbidden(err.Error()).Write(w)
			case errors.Is(err, bid.ErrRFQNotAcceptingBids):
				apierr.Forbidden(err.Error()).Write(w)
			default:
				apierr.WrapInternal("withdraw guard", err).Write(w)
			}
			return
		}
	}

	actor := bid.Actor{UserID: user.UserID, Name: user.Name, IP: clientIP(r)}
	if err := bid.WithdrawBid(r.Context(), h.pool, bidID, actor); err != nil {
		switch {
		case errors.Is(err, bid.ErrBidNotFound):
			apierr.New(http.StatusNotFound, "BID_NOT_FOUND", err.Error()).Write(w)
		case errors.Is(err, bid.ErrBidNotWithdrawable):
			apierr.New(http.StatusBadRequest, "BID_NOT_WITHDRAWABLE", err.Error()).Write(w)
		default:
			slog.Error("withdrawBid", "bid_id", bidID, "error", err)
			apierr.WrapInternal("withdraw failed", err).Write(w)
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"bid_id": bidID, "status": bid.BidStatusWithdrawn})
}

// Publish handles POST /api/bid/rfqs/{rfqId}/publish.
//
// Moves a draft RFQ to 'published' and, when reserve_method='multiple',
// stamps the 15 예비가격 candidates. This is the primary "make it live"
// endpoint — raw PATCH on status is intentionally not used because it
// can't enforce the reserves invariant.
func (h *BidHandler) Publish(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUser(r.Context())
	if !ok {
		apierr.Unauthorized("not authenticated").Write(w)
		return
	}
	if user.Role != RoleDirector && user.Role != RolePM {
		apierr.Forbidden("only director or pm can publish RFQs").Write(w)
		return
	}
	rfqID := chi.URLParam(r, "rfqId")
	if rfqID == "" {
		apierr.BadRequest("rfqId is required").Write(w)
		return
	}
	actor := bid.Actor{UserID: user.UserID, Name: user.Name, IP: clientIP(r)}
	if err := bid.PublishRFQ(r.Context(), h.pool, rfqID, actor); err != nil {
		switch {
		case errors.Is(err, bid.ErrRFQNotFound):
			apierr.New(http.StatusNotFound, "RFQ_NOT_FOUND", err.Error()).Write(w)
		case errors.Is(err, bid.ErrRFQIneligible):
			apierr.New(http.StatusBadRequest, "RFQ_INELIGIBLE", err.Error()).Write(w)
		default:
			slog.Error("publishRFQ", "rfq_id", rfqID, "error", err)
			apierr.WrapInternal("publish failed", err).Write(w)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rfq_id": rfqID, "status": "published"})
}

// Amend handles POST /api/bid/rfqs/{rfqId}/amend.
//
// Body: {"note": "specs updated; deadline extended"} — the note is stored
// on the RFQ and broadcast to current bidders via in-app notifications.
func (h *BidHandler) Amend(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUser(r.Context())
	if !ok {
		apierr.Unauthorized("not authenticated").Write(w)
		return
	}
	if user.Role != RoleDirector && user.Role != RolePM {
		apierr.Forbidden("only director or pm can amend RFQs").Write(w)
		return
	}
	rfqID := chi.URLParam(r, "rfqId")
	if rfqID == "" {
		apierr.BadRequest("rfqId is required").Write(w)
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if strings.TrimSpace(body.Note) == "" {
		apierr.BadRequest("note is required").Write(w)
		return
	}

	actor := bid.Actor{UserID: user.UserID, Name: user.Name, IP: clientIP(r)}
	if err := bid.AmendRFQ(r.Context(), h.pool, rfqID, body.Note, actor); err != nil {
		switch {
		case errors.Is(err, bid.ErrRFQNotFound):
			apierr.New(http.StatusNotFound, "RFQ_NOT_FOUND", err.Error()).Write(w)
		case errors.Is(err, bid.ErrAmendOnTerminalRFQ):
			apierr.New(http.StatusBadRequest, "RFQ_TERMINAL", err.Error()).Write(w)
		default:
			slog.Error("amendRFQ", "rfq_id", rfqID, "error", err)
			apierr.WrapInternal("amend failed", err).Write(w)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rfq_id": rfqID, "status": "amended"})
}

// Clone handles POST /api/bid/rfqs/{rfqId}/clone.
//
// Duplicates the RFQ into a fresh draft and returns the new id. Used both
// for "duplicate this RFQ" and for spawning a draft from a template shell
// (is_template=true). Director/pm only.
func (h *BidHandler) Clone(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUser(r.Context())
	if !ok {
		apierr.Unauthorized("not authenticated").Write(w)
		return
	}
	if user.Role != RoleDirector && user.Role != RolePM {
		apierr.Forbidden("only director or pm can clone RFQs").Write(w)
		return
	}
	rfqID := chi.URLParam(r, "rfqId")
	if rfqID == "" {
		apierr.BadRequest("rfqId is required").Write(w)
		return
	}
	actor := bid.Actor{UserID: user.UserID, Name: user.Name, IP: clientIP(r)}
	newID, err := bid.CloneRFQ(r.Context(), h.pool, rfqID, actor)
	if err != nil {
		if errors.Is(err, bid.ErrRFQNotFound) {
			apierr.New(http.StatusNotFound, "RFQ_NOT_FOUND", err.Error()).Write(w)
			return
		}
		slog.Error("cloneRFQ", "rfq_id", rfqID, "error", err)
		apierr.WrapInternal("clone failed", err).Write(w)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"rfq_id": newID})
}

// Evaluate handles POST /api/bid/rfqs/{rfqId}/evaluate.
//
// Moves an RFQ from 'opened' to 'evaluating' — the formal 적격심사 start.
// Kept as a distinct endpoint (rather than bundling into award) so a team
// can pause mid-review without pressure to finalise immediately.
func (h *BidHandler) Evaluate(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUser(r.Context())
	if !ok {
		apierr.Unauthorized("not authenticated").Write(w)
		return
	}
	if user.Role != RoleDirector && user.Role != RolePM {
		apierr.Forbidden("only director or pm can move to evaluation").Write(w)
		return
	}
	rfqID := chi.URLParam(r, "rfqId")
	if rfqID == "" {
		apierr.BadRequest("rfqId is required").Write(w)
		return
	}
	actor := bid.Actor{UserID: user.UserID, Name: user.Name, IP: clientIP(r)}
	if err := bid.MoveToEvaluating(r.Context(), h.pool, rfqID, actor); err != nil {
		if errors.Is(err, bid.ErrRFQIneligible) {
			apierr.New(http.StatusBadRequest, "RFQ_INELIGIBLE", err.Error()).Write(w)
			return
		}
		slog.Error("moveToEvaluating", "rfq_id", rfqID, "error", err)
		apierr.WrapInternal("move to evaluating failed", err).Write(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rfq_id": rfqID, "status": "evaluating"})
}

// AuctionBid is a single row of the auction leaderboard. Supplier name is
// denormalised into the response so the portal can render without a second
// round-trip. For suppliers we mask competitors' company names (show an
// anonymised "공급사 #N") — the live price is what they need to undercut,
// not who to target.
type AuctionBid struct {
	ID           string  `json:"id"`
	SupplierName string  `json:"supplier_name"`
	TotalAmount  float64 `json:"total_amount"`
	SubmittedAt  string  `json:"submitted_at"`
	IsMine       bool    `json:"is_mine,omitempty"`
}

// AuctionBids handles GET /api/bid/rfqs/{rfqId}/auction-bids.
//
// Returns the live leaderboard for a reverse-auction RFQ, sorted ascending
// by price (lowest = leading). Only accessible when the RFQ is in mode='auction'
// and status='published'; other states keep the regular sealed-bid access path.
//
// Authorization:
//   - buyer staff (director/pm/engineer/viewer) always pass
//   - supplier callers must have at least one bid on the RFQ (skin in the
//     game — otherwise any supplier could spy on unrelated auctions)
//   - competitors' names are anonymised to "공급사 #N" for supplier callers
//     to protect identity while still showing the ranking
func (h *BidHandler) AuctionBids(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUser(r.Context())
	if !ok {
		apierr.Unauthorized("not authenticated").Write(w)
		return
	}
	rfqID := chi.URLParam(r, "rfqId")
	if rfqID == "" {
		apierr.BadRequest("rfqId is required").Write(w)
		return
	}

	var mode, status string
	err := h.pool.QueryRow(r.Context(),
		`SELECT mode, status FROM data.rfqs WHERE id = $1 AND deleted_at IS NULL`,
		rfqID,
	).Scan(&mode, &status)
	if err != nil {
		apierr.New(http.StatusNotFound, "RFQ_NOT_FOUND", "rfq not found").Write(w)
		return
	}
	if mode != "auction" {
		apierr.New(http.StatusBadRequest, "NOT_AUCTION",
			"this endpoint only serves auction-mode RFQs").Write(w)
		return
	}

	// Supplier skin-in-the-game check.
	if user.Role == "supplier" {
		var mineCount int
		_ = h.pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM data.bids WHERE rfq=$1 AND supplier=$2 AND deleted_at IS NULL`,
			rfqID, user.SupplierID,
		).Scan(&mineCount)
		if mineCount == 0 {
			apierr.Forbidden("submit a bid first to view the auction").Write(w)
			return
		}
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT b.id::text,
		       COALESCE(s.name,'') AS supplier_name,
		       b.total_amount,
		       b.submitted_at,
		       b.supplier::text
		  FROM data.bids b
		  LEFT JOIN data.suppliers s ON s.id = b.supplier
		 WHERE b.rfq = $1
		   AND b.status IN ('submitted','opened','draft')
		   AND b.deleted_at IS NULL
		 ORDER BY b.total_amount ASC, b.submitted_at ASC`,
		rfqID,
	)
	if err != nil {
		apierr.WrapInternal("list auction bids", err).Write(w)
		return
	}
	defer rows.Close()

	out := make([]AuctionBid, 0, 16)
	for rows.Next() {
		var (
			id, name, supID string
			amount          float64
			submittedAt     *time.Time
		)
		if err := rows.Scan(&id, &name, &amount, &submittedAt, &supID); err != nil {
			continue
		}
		displayName := name
		if user.Role == "supplier" && supID != user.SupplierID {
			displayName = fmt.Sprintf("공급사 #%d", len(out)+1)
		}
		var stamped string
		if submittedAt != nil {
			stamped = submittedAt.Format(time.RFC3339)
		}
		out = append(out, AuctionBid{
			ID:           id,
			SupplierName: displayName,
			TotalAmount:  amount,
			SubmittedAt:  stamped,
			IsMine:       user.Role == "supplier" && supID == user.SupplierID,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

// AnswerClarification handles POST /api/bid/clarifications/{id}/answer.
//
// Posts a buyer answer, flips the row to 'answered', and broadcasts in-app
// notifications to every bidder. Distinct from a raw PATCH so the
// notification side effect is idempotent and audited.
func (h *BidHandler) AnswerClarification(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUser(r.Context())
	if !ok {
		apierr.Unauthorized("not authenticated").Write(w)
		return
	}
	if user.Role != RoleDirector && user.Role != RolePM && user.Role != RoleEngineer {
		apierr.Forbidden("only buyer staff can answer clarifications").Write(w)
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		apierr.BadRequest("id is required").Write(w)
		return
	}
	var body struct {
		Answer string `json:"answer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierr.BadRequest("invalid body").Write(w)
		return
	}
	actor := bid.Actor{UserID: user.UserID, Name: user.Name, IP: clientIP(r)}
	if err := bid.AnswerClarification(r.Context(), h.pool, id, body.Answer, actor); err != nil {
		if strings.Contains(err.Error(), "not found") {
			apierr.New(http.StatusNotFound, "CLARIFICATION_NOT_FOUND", err.Error()).Write(w)
			return
		}
		if strings.Contains(err.Error(), "required") {
			apierr.BadRequest(err.Error()).Write(w)
			return
		}
		slog.Error("answerClarification", "id", id, "error", err)
		apierr.WrapInternal("answer failed", err).Write(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "answered"})
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
	case errors.Is(err, bid.ErrSupplierNotQualified):
		apierr.New(http.StatusBadRequest, "SUPPLIER_NOT_QUALIFIED", err.Error()).Write(w)
	default:
		slog.Error("awardRFQ", "rfq_id", rfqID, "error", err, "path", r.URL.Path)
		apierr.WrapInternal("award failed", err).Write(w)
	}
}
