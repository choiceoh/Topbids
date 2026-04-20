package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// ViewHandler serves the View API (/api/schema/collections/{id}/views/...).
type ViewHandler struct {
	store *schema.Store
}

func NewViewHandler(store *schema.Store) *ViewHandler {
	return &ViewHandler{store: store}
}

// ListViews returns all views for a collection.
func (h *ViewHandler) ListViews(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "id")
	views, err := h.store.ListViews(r.Context(), collectionID)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	if views == nil {
		views = []schema.View{}
	}
	writeJSON(w, http.StatusOK, views)
}

// CreateView creates a new view for a collection.
func (h *ViewHandler) CreateView(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "id")
	var req schema.CreateViewReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.ViewType == "" {
		req.ViewType = "list"
	}

	view, err := h.store.CreateView(r.Context(), collectionID, &req)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

// UpdateView patches an existing view.
func (h *ViewHandler) UpdateView(w http.ResponseWriter, r *http.Request) {
	viewID := chi.URLParam(r, "viewId")
	var req schema.UpdateViewReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	view, err := h.store.UpdateView(r.Context(), viewID, &req)
	if err != nil {
		if errors.Is(err, schema.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// DeleteView removes a view.
func (h *ViewHandler) DeleteView(w http.ResponseWriter, r *http.Request) {
	viewID := chi.URLParam(r, "viewId")
	if err := h.store.DeleteView(r.Context(), viewID); err != nil {
		if errors.Is(err, schema.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
