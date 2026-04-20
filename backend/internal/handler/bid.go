package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/bid"
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
// Selects a winning bid, transitions statuses, returns AwardResult JSON.
// RBAC is applied at the router level (director/pm only).
func (h *BidHandler) Award(w http.ResponseWriter, r *http.Request) {
	rfqID := chi.URLParam(r, "rfqId")
	if rfqID == "" {
		writeError(w, http.StatusBadRequest, "rfqId is required")
		return
	}

	result, err := bid.AwardRFQ(r.Context(), h.pool, rfqID)
	if err != nil {
		switch {
		case errors.Is(err, bid.ErrRFQNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, bid.ErrRFQIneligible),
			errors.Is(err, bid.ErrNoBids),
			errors.Is(err, bid.ErrTechScoreMissing):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			slog.Error("awardRFQ", "rfq_id", rfqID, "error", err)
			writeError(w, http.StatusInternalServerError, "award failed")
		}
		return
	}

	slog.Info("bid awarded",
		"rfq_id", result.RFQID,
		"winner_bid_id", result.WinnerBidID,
		"eval_method", result.EvalMethod,
		"total_bids", result.TotalBids)

	writeJSON(w, http.StatusOK, result)
}
