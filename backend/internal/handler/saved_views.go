package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/choiceoh/phaeton/backend/internal/middleware"
	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// SavedViewHandler serves the Saved View API.
type SavedViewHandler struct {
	store *schema.Store
}

func NewSavedViewHandler(store *schema.Store) *SavedViewHandler {
	return &SavedViewHandler{store: store}
}

// ListSavedViews returns public views + the caller's private views for a collection.
func (h *SavedViewHandler) ListSavedViews(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "id")
	user, _ := middleware.GetUser(r.Context())

	views, err := h.store.ListSavedViews(r.Context(), collectionID, user.UserID)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	if views == nil {
		views = []schema.SavedView{}
	}

	// Apply pagination.
	total := int64(len(views))
	page, limit, offset := ParsePagination(r.URL.Query())
	if offset >= len(views) {
		views = []schema.SavedView{}
	} else {
		end := offset + limit
		if end > len(views) {
			end = len(views)
		}
		views = views[offset:end]
	}
	writeList(w, views, total, page, limit)
}

// CreateSavedView creates a new saved view for a collection.
func (h *SavedViewHandler) CreateSavedView(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "id")
	user, _ := middleware.GetUser(r.Context())

	var req schema.CreateSavedViewReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	view, err := h.store.CreateSavedView(r.Context(), collectionID, user.UserID, &req)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

// UpdateSavedView patches an existing saved view.
func (h *SavedViewHandler) UpdateSavedView(w http.ResponseWriter, r *http.Request) {
	viewID := chi.URLParam(r, "savedViewId")
	var req schema.UpdateSavedViewReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	view, err := h.store.UpdateSavedView(r.Context(), viewID, &req)
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

// DeleteSavedView removes a saved view.
func (h *SavedViewHandler) DeleteSavedView(w http.ResponseWriter, r *http.Request) {
	viewID := chi.URLParam(r, "savedViewId")
	if err := h.store.DeleteSavedView(r.Context(), viewID); err != nil {
		if errors.Is(err, schema.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
