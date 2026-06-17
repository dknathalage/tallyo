package httpapi

import (
	"net/http"

	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/service"
)

// ShiftHandler serves the shift lifecycle routes: per-participant listing,
// tenant-wide listing, the billing-suggestion and to-record prompts, plus shift
// CRUD and the status-transition endpoint.
type ShiftHandler struct {
	svc *service.ShiftService
}

// NewShiftHandler constructs the handler. A nil svc is a programmer error.
func NewShiftHandler(svc *service.ShiftService) *ShiftHandler {
	if svc == nil {
		panic("NewShiftHandler: nil svc")
	}
	return &ShiftHandler{svc: svc}
}

// ListForParticipant returns a participant's shifts, optionally restricted to the
// ?from=&to= service-date window and a ?status= filter. The chi {id} path param
// is the participant id.
func (h *ShiftHandler) ListForParticipant(w http.ResponseWriter, r *http.Request) {
	participantID, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	status := r.URL.Query().Get("status")
	shifts, err := h.svc.ListParticipant(r.Context(), participantID, from, to)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if status != "" {
		filtered := make([]*repository.Shift, 0, len(shifts))
		for i := range shifts { // bounded by len(shifts)
			if shifts[i].Status == status {
				filtered = append(filtered, shifts[i])
			}
		}
		shifts = filtered
	}
	WriteJSON(w, http.StatusOK, shifts)
}

// List returns every shift for the acting tenant, optionally restricted to a
// ?status= filter (used to populate the shifts table).
func (h *ShiftHandler) List(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	shifts, err := h.svc.List(r.Context(), status)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, shifts)
}

// Suggestions returns each participant's recorded-but-unbilled billing prompt.
func (h *ShiftHandler) Suggestions(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Suggestions(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, out)
}

// ToRecord returns the tenant's scheduled shifts still awaiting a record.
func (h *ShiftHandler) ToRecord(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ToRecord(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, out)
}

// Get returns a single shift by id, or 404 when not found.
func (h *ShiftHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	sh, err := h.svc.Get(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if sh == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, sh)
}

// Create inserts a shift. A missing participant or service date → 400.
func (h *ShiftHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in repository.ShiftInput
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
	sh, err := h.svc.Create(r.Context(), in)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusCreated, sh)
}

// Update mutates a shift. Empty service date → 400; unknown id → 404.
func (h *ShiftHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in repository.ShiftInput
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.ServiceDate == "" {
		WriteError(w, http.StatusBadRequest, "service date required")
		return
	}
	sh, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if sh == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, sh)
}

// Delete removes a shift by id.
func (h *ShiftHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

// statusRequest is the body of UpdateStatus: the target lifecycle status.
type statusRequest struct {
	Status string `json:"status"`
}

// UpdateStatus advances a shift's lifecycle status. An empty status → 400.
func (h *ShiftHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req statusRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.Status == "" {
		WriteError(w, http.StatusBadRequest, "status required")
		return
	}
	if err := h.svc.UpdateStatus(r.Context(), id, req.Status); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
