package httpapi

import (
	"net/http"

	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/service"
)

// ParticipantHandler serves the participant CRUD plus bulk-delete routes.
type ParticipantHandler struct {
	svc *service.ParticipantService
}

// NewParticipantHandler constructs the handler. A nil svc is a programmer error.
func NewParticipantHandler(svc *service.ParticipantService) *ParticipantHandler {
	if svc == nil {
		panic("NewParticipantHandler: nil svc")
	}
	return &ParticipantHandler{svc: svc}
}

// List returns participants, optionally filtered by the ?search= query param.
func (h *ParticipantHandler) List(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	participants, err := h.svc.List(r.Context(), search)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, participants)
}

// Get returns a single participant by id, or 404 when not found.
func (h *ParticipantHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	p, err := h.svc.Get(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if p == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, p)
}

// Create inserts a participant. An empty name is rejected with 400.
func (h *ParticipantHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in repository.ParticipantInput
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Name == "" {
		WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	p, err := h.svc.Create(r.Context(), in)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusCreated, p)
}

// Update mutates a participant. Empty name → 400; unknown id → 404.
func (h *ParticipantHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in repository.ParticipantInput
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Name == "" {
		WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	p, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if p == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, p)
}

// Delete removes a participant by id.
func (h *ParticipantHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

// BulkDelete removes every participant whose id is in the request body.
func (h *ParticipantHandler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ids []int64 `json:"ids"`
	}
	if err := DecodeJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := h.svc.BulkDelete(r.Context(), body.Ids); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
