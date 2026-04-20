package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/choiceoh/phaeton/backend/internal/middleware"
	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// ChartHandler serves the Charts API.
type ChartHandler struct {
	store *schema.Store
}

func NewChartHandler(store *schema.Store) *ChartHandler {
	return &ChartHandler{store: store}
}

func (h *ChartHandler) List(w http.ResponseWriter, r *http.Request) {
	colID := chi.URLParam(r, "id")
	charts, err := h.store.ListCharts(r.Context(), colID)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	if charts == nil {
		charts = []schema.Chart{}
	}

	// Apply pagination.
	total := int64(len(charts))
	page, limit, offset := ParsePagination(r.URL.Query())
	if offset >= len(charts) {
		charts = []schema.Chart{}
	} else {
		end := offset + limit
		if end > len(charts) {
			end = len(charts)
		}
		charts = charts[offset:end]
	}
	writeList(w, charts, total, page, limit)
}

func (h *ChartHandler) Create(w http.ResponseWriter, r *http.Request) {
	colID := chi.URLParam(r, "id")
	var req schema.CreateChartReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.ChartType == "" {
		writeError(w, http.StatusBadRequest, "chart_type is required")
		return
	}
	validChartTypes := map[string]bool{"bar": true, "line": true, "pie": true, "doughnut": true, "area": true}
	if !validChartTypes[req.ChartType] {
		writeError(w, http.StatusBadRequest, "invalid chart_type; allowed: bar, line, pie, doughnut, area")
		return
	}

	user, _ := middleware.GetUser(r.Context())
	chart, err := h.store.CreateChart(r.Context(), colID, req, user.UserID)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, chart)
}

func (h *ChartHandler) Update(w http.ResponseWriter, r *http.Request) {
	chartID := chi.URLParam(r, "chartId")
	var req schema.UpdateChartReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	chart, err := h.store.UpdateChart(r.Context(), chartID, req)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, chart)
}

func (h *ChartHandler) Delete(w http.ResponseWriter, r *http.Request) {
	chartID := chi.URLParam(r, "chartId")
	if err := h.store.DeleteChart(r.Context(), chartID); err != nil {
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
