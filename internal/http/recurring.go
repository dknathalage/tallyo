package httpapi

import (
	"net/http"

	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/service"
)

// RecurringHandler serves the recurring-template CRUD plus the generate route.
type RecurringHandler struct {
	svc *service.RecurringService
}

// NewRecurringHandler constructs the handler. A nil svc is a programmer error.
func NewRecurringHandler(svc *service.RecurringService) *RecurringHandler {
	if svc == nil {
		panic("NewRecurringHandler: nil svc")
	}
	return &RecurringHandler{svc: svc}
}

// List returns templates. ?active=true (or 1) returns only active templates.
func (h *RecurringHandler) List(w http.ResponseWriter, r *http.Request) {
	active := r.URL.Query().Get("active")
	activeOnly := active == "true" || active == "1"
	tpls, err := h.svc.List(r.Context(), activeOnly)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, tpls)
}

// Get returns a single template, or 404 when not found.
func (h *RecurringHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	tpl, err := h.svc.Get(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tpl == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, tpl)
}

// validateRecurring checks the writable fields at the request boundary.
func validateRecurring(in repository.RecurringInput) (string, bool) {
	if in.Name == "" {
		return "name required", false
	}
	if in.ParticipantID == nil || *in.ParticipantID == 0 {
		return "participant required", false
	}
	if in.Frequency == "" {
		return "frequency required", false
	}
	return "", true
}

// Create inserts a template after validating name, client and frequency.
func (h *RecurringHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in repository.RecurringInput
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if msg, ok := validateRecurring(in); !ok {
		WriteError(w, http.StatusBadRequest, msg)
		return
	}
	tpl, err := h.svc.Create(r.Context(), in)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusCreated, tpl)
}

// Update rewrites a template. Validation fails → 400; unknown id → 404.
func (h *RecurringHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in repository.RecurringInput
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if msg, ok := validateRecurring(in); !ok {
		WriteError(w, http.StatusBadRequest, msg)
		return
	}
	tpl, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tpl == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, tpl)
}

// Delete removes a template by id.
func (h *RecurringHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

// Generate produces a draft invoice from the template and advances its next_due.
// Unknown id → 404; otherwise the generated invoice is returned with 200.
func (h *RecurringHandler) Generate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	inv, err := h.svc.GenerateOne(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if inv == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, inv)
}
