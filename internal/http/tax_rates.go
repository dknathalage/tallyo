package httpapi

import (
	"net/http"

	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/service"
)

// TaxRateHandler serves the tax-rate CRUD routes.
type TaxRateHandler struct {
	svc *service.TaxRateService
}

// NewTaxRateHandler constructs the handler. A nil svc is a programmer error.
func NewTaxRateHandler(svc *service.TaxRateService) *TaxRateHandler {
	if svc == nil {
		panic("NewTaxRateHandler: nil svc")
	}
	return &TaxRateHandler{svc: svc}
}

// List returns every tax rate (always a JSON array, never null).
func (h *TaxRateHandler) List(w http.ResponseWriter, r *http.Request) {
	rates, err := h.svc.List(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, rates)
}

// Get returns a single tax rate by id, or 404 when not found.
func (h *TaxRateHandler) Get(w http.ResponseWriter, r *http.Request) {
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

// Create inserts a tax rate. An empty name is rejected with 400.
func (h *TaxRateHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in repository.TaxRateInput
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

// Update mutates a tax rate. Empty name → 400; unknown id → 404.
func (h *TaxRateHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in repository.TaxRateInput
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

// Delete removes a tax rate by id.
func (h *TaxRateHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
