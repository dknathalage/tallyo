package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/pdf"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/service"
)

// EstimateHandler serves the estimate CRUD, status, duplicate, bulk and
// convert-to-invoice routes.
type EstimateHandler struct {
	svc *service.EstimateService
}

// NewEstimateHandler constructs the handler. A nil svc is a programmer error.
func NewEstimateHandler(svc *service.EstimateService) *EstimateHandler {
	if svc == nil {
		panic("NewEstimateHandler: nil svc")
	}
	return &EstimateHandler{svc: svc}
}

// estimateRequest is the flat write payload: every EstimateInput field (embedded so
// its json tags flatten) plus the line items. The frontend posts this shape.
type estimateRequest struct {
	repository.EstimateInput
	LineItems []billing.LineItemInput `json:"lineItems"`
}

// List returns estimates filtered by the optional ?participantId= or ?status= query
// params. Unlike invoices there is no read-time overdue sweep.
func (h *EstimateHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if pid := q.Get("participantId"); pid != "" {
		participantID, err := strconv.ParseInt(pid, 10, 64)
		if err != nil || participantID <= 0 {
			WriteError(w, http.StatusBadRequest, "invalid participantId")
			return
		}
		ests, err := h.svc.ListParticipantEstimates(r.Context(), participantID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		WriteJSON(w, http.StatusOK, ests)
		return
	}
	if status := q.Get("status"); status != "" {
		ests, err := h.svc.ListByStatus(r.Context(), status)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		WriteJSON(w, http.StatusOK, ests)
		return
	}
	ests, err := h.svc.List(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, ests)
}

// Get returns a single estimate (with line items), or 404 when not found.
func (h *EstimateHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	est, err := h.svc.Get(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if est == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, est)
}

// Create inserts an estimate. A missing participant or empty line items → 400.
func (h *EstimateHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req estimateRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.ParticipantID == 0 || len(req.LineItems) == 0 {
		WriteError(w, http.StatusBadRequest, "participant and at least one line item are required")
		return
	}
	est, err := h.svc.Create(r.Context(), req.EstimateInput, req.LineItems)
	if err != nil {
		if WriteValidationError(w, err) {
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusCreated, est)
}

// Update rewrites an estimate. Missing participant/items → 400; unknown id → 404.
func (h *EstimateHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req estimateRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.ParticipantID == 0 || len(req.LineItems) == 0 {
		WriteError(w, http.StatusBadRequest, "participant and at least one line item are required")
		return
	}
	est, err := h.svc.Update(r.Context(), id, req.EstimateInput, req.LineItems)
	if err != nil {
		if WriteValidationError(w, err) {
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if est == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, est)
}

// Delete removes an estimate by id.
func (h *EstimateHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

// Status flips just the estimate status. An empty status → 400.
func (h *EstimateHandler) Status(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := DecodeJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.Status == "" {
		WriteError(w, http.StatusBadRequest, "status required")
		return
	}
	if err := h.svc.UpdateStatus(r.Context(), id, body.Status); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": body.Status})
}

// Duplicate copies an estimate into a new draft and returns it.
func (h *EstimateHandler) Duplicate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	est, err := h.svc.Duplicate(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusCreated, est)
}

// BulkDelete removes every estimate whose id is in the request body.
func (h *EstimateHandler) BulkDelete(w http.ResponseWriter, r *http.Request) {
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

// BulkStatus sets the status of every estimate whose id is in the request body.
func (h *EstimateHandler) BulkStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ids    []int64 `json:"ids"`
		Status string  `json:"status"`
	}
	if err := DecodeJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := h.svc.BulkUpdateStatus(r.Context(), body.Ids, body.Status); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Convert turns an accepted estimate into an invoice. A non-accepted or
// already-converted estimate → 409; an unknown id → 404.
func (h *EstimateHandler) Convert(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	res, err := h.svc.Convert(r.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotAccepted) {
			WriteError(w, http.StatusConflict, "only accepted estimates can be converted")
			return
		}
		if errors.Is(err, repository.ErrAlreadyConverted) {
			WriteError(w, http.StatusConflict, "estimate already converted")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if res == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, res)
}

// Pdf renders an estimate to PDF and returns it as a file download.
func (h *EstimateHandler) Pdf(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	est, err := h.svc.Get(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if est == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	estDoc := &pdf.EstimateDoc{
		Number:           est.Number,
		IssueDate:        est.IssueDate,
		ValidUntil:       est.ValidUntil,
		Status:           est.Status,
		BusinessSnapshot: est.BusinessSnapshot,
		ClientSnapshot:   est.ClientSnapshot,
		LineItems:        est.LineItems,
		Subtotal:         est.Subtotal,
		Tax:              est.Tax,
		Total:            est.Total,
		Notes:            est.Notes,
	}
	b, err := pdf.RenderEstimate(estDoc)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "pdf render failed")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+est.Number+`.pdf"`)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(b); err != nil {
		// client gone; nothing to do
	}
}
