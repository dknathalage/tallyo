package httpapi

import (
	"net/http"

	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/service"
)

// NoteHandler serves the per-participant journal-note routes plus the billing
// link endpoint.
type NoteHandler struct {
	svc *service.NoteService
}

// NewNoteHandler constructs the handler. A nil svc is a programmer error.
func NewNoteHandler(svc *service.NoteService) *NoteHandler {
	if svc == nil {
		panic("NewNoteHandler: nil svc")
	}
	return &NoteHandler{svc: svc}
}

// ListForParticipant returns a participant's notes, optionally restricted to the
// ?from=&to= service-date window. The chi {id} path param is the participant id.
func (h *NoteHandler) ListForParticipant(w http.ResponseWriter, r *http.Request) {
	participantID, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	notes, err := h.svc.ListParticipant(r.Context(), participantID, from, to)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, notes)
}

// Get returns a single note by id, or 404 when not found.
func (h *NoteHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	n, err := h.svc.Get(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if n == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, n)
}

// Create inserts a note. A missing participant, service date, or body → 400.
func (h *NoteHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in repository.NoteInput
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.ParticipantID == 0 {
		WriteError(w, http.StatusBadRequest, "participant required")
		return
	}
	if in.ServiceDate == "" {
		WriteError(w, http.StatusBadRequest, "service date required")
		return
	}
	if in.Body == "" {
		WriteError(w, http.StatusBadRequest, "body required")
		return
	}
	n, err := h.svc.Create(r.Context(), in)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusCreated, n)
}

// Update mutates a note. Empty service date or body → 400; unknown id → 404.
func (h *NoteHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in repository.NoteInput
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.ServiceDate == "" {
		WriteError(w, http.StatusBadRequest, "service date required")
		return
	}
	if in.Body == "" {
		WriteError(w, http.StatusBadRequest, "body required")
		return
	}
	n, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if n == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, n)
}

// Delete removes a note by id.
func (h *NoteHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

// Bill links the given notes to an invoice. A zero invoice id or an empty note
// list → 400.
func (h *NoteHandler) Bill(w http.ResponseWriter, r *http.Request) {
	var body struct {
		InvoiceID int64   `json:"invoiceId"`
		NoteIDs   []int64 `json:"noteIds"`
	}
	if err := DecodeJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.InvoiceID == 0 {
		WriteError(w, http.StatusBadRequest, "invoice required")
		return
	}
	if len(body.NoteIDs) == 0 {
		WriteError(w, http.StatusBadRequest, "notes required")
		return
	}
	if err := h.svc.Bill(r.Context(), body.InvoiceID, body.NoteIDs); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
