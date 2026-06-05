package httpapi

import (
	"net/http"

	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/service"
)

// ColumnMappingHandler serves the column-mapping CRUD routes.
type ColumnMappingHandler struct {
	svc *service.ColumnMappingService
}

// NewColumnMappingHandler constructs the handler. A nil svc is a programmer error.
func NewColumnMappingHandler(svc *service.ColumnMappingService) *ColumnMappingHandler {
	if svc == nil {
		panic("NewColumnMappingHandler: nil svc")
	}
	return &ColumnMappingHandler{svc: svc}
}

// List returns column mappings, optionally filtered by ?entityType=. Always a
// JSON array, never null.
func (h *ColumnMappingHandler) List(w http.ResponseWriter, r *http.Request) {
	entityType := r.URL.Query().Get("entityType")
	mappings, err := h.svc.List(r.Context(), entityType)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, mappings)
}

// Get returns a single mapping by id, or 404 when not found.
func (h *ColumnMappingHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	m, err := h.svc.Get(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if m == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, m)
}

// Create inserts a mapping. An empty name is rejected with 400.
func (h *ColumnMappingHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in repository.ColumnMappingInput
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Name == "" {
		WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	m, err := h.svc.Create(r.Context(), in)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusCreated, m)
}

// Update mutates a mapping. Empty name → 400; unknown id → 404.
func (h *ColumnMappingHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in repository.ColumnMappingInput
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Name == "" {
		WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	m, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if m == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, m)
}

// Delete removes a mapping by id.
func (h *ColumnMappingHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
