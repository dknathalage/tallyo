package httpapi

import (
	"errors"
	"net/http"

	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/service"
)

// RateTierHandler serves the rate-tier CRUD routes.
type RateTierHandler struct {
	svc *service.RateTierService
}

// NewRateTierHandler constructs the handler. A nil svc is a programmer error.
func NewRateTierHandler(svc *service.RateTierService) *RateTierHandler {
	if svc == nil {
		panic("NewRateTierHandler: nil svc")
	}
	return &RateTierHandler{svc: svc}
}

// List returns every rate tier (always a JSON array, never null).
func (h *RateTierHandler) List(w http.ResponseWriter, r *http.Request) {
	tiers, err := h.svc.List(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, tiers)
}

// Get returns a single tier by id, or 404 when not found.
func (h *RateTierHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	t, err := h.svc.Get(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if t == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, t)
}

// Create inserts a tier. An empty name is rejected with 400.
func (h *RateTierHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in repository.RateTierInput
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Name == "" {
		WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	t, err := h.svc.Create(r.Context(), in)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusCreated, t)
}

// Update mutates a tier. Empty name → 400; unknown id → 404.
func (h *RateTierHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in repository.RateTierInput
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Name == "" {
		WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	t, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if t == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, t)
}

// Delete removes a tier. Deleting the only remaining tier yields 409.
func (h *RateTierHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	err := h.svc.Delete(r.Context(), id)
	if errors.Is(err, repository.ErrLastTier) {
		WriteError(w, http.StatusConflict, "cannot delete the last rate tier")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
