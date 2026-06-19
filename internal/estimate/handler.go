package estimate

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/dknathalage/tallyo/internal/pdf"
	"github.com/go-chi/chi/v5"
)

// Handler serves the estimate CRUD, status, duplicate, bulk and
// convert-to-invoice routes.
type Handler struct {
	svc *Service
}

// NewHandler constructs the handler. A nil svc is a programmer error.
func NewHandler(svc *Service) *Handler {
	if svc == nil {
		panic("estimate.NewHandler: nil svc")
	}
	return &Handler{svc: svc}
}

// Routes registers the estimate routes on r. Mounted inside the authenticated
// /api group by the composition root.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/estimates", h.List)
	r.Post("/estimates", h.Create)
	r.Post("/estimates/bulk-delete", h.BulkDelete)
	r.Post("/estimates/bulk-status", h.BulkStatus)
	r.Get("/estimates/{id}", h.Get)
	r.Put("/estimates/{id}", h.Update)
	r.Delete("/estimates/{id}", h.Delete)
	r.Post("/estimates/{id}/status", h.Status)
	r.Post("/estimates/{id}/duplicate", h.Duplicate)
	r.Get("/estimates/{id}/pdf", h.Pdf)
	r.Post("/estimates/{id}/convert", h.Convert)
}

// estimateRequest is the flat write payload: every EstimateInput field (embedded so
// its json tags flatten) plus the line items. The frontend posts this shape.
type estimateRequest struct {
	EstimateInput
	LineItems []billing.LineItemInput `json:"lineItems"`
}

// writeValidationError, when err is (or wraps) a *billing.ValidationError,
// writes a 422 envelope and returns true. Otherwise it writes nothing and
// returns false so callers can fall through to generic error handling.
func writeValidationError(w http.ResponseWriter, err error) bool {
	ve, ok := billing.AsValidationError(err)
	if !ok || ve == nil {
		return false
	}
	httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
		"error":   "validation failed",
		"details": ve.Errors,
	})
	return true
}

// parseIDFromString parses a decimal int64 from s. Used by the List handler
// to parse query-string ids (e.g. ?participantId=123).
func parseIDFromString(s string) (int64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// List returns estimates filtered by the optional ?participantId= or ?status= query
// params. Unlike invoices there is no read-time overdue sweep.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if listquery.IsListQuery(q) {
		c := listquery.Build(q, EstimateCols)
		res, err := h.svc.Query(r.Context(), c)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, res)
		return
	}
	if pid := q.Get("participantId"); pid != "" {
		participantID, ok := parseIDFromString(pid)
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "invalid participantId")
			return
		}
		ests, err := h.svc.ListParticipantEstimates(r.Context(), participantID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, ests)
		return
	}
	if status := q.Get("status"); status != "" {
		ests, err := h.svc.ListByStatus(r.Context(), status)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, ests)
		return
	}
	ests, err := h.svc.List(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ests)
}

// Get returns a single estimate (with line items), or 404 when not found.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	est, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if est == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, est)
}

// Create inserts an estimate. A missing participant or empty line items → 400.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req estimateRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.ParticipantID == 0 || len(req.LineItems) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "participant and at least one line item are required")
		return
	}
	est, err := h.svc.Create(r.Context(), req.EstimateInput, req.LineItems)
	if err != nil {
		if writeValidationError(w, err) {
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, est)
}

// Update rewrites an estimate. Missing participant/items → 400; unknown id → 404.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req estimateRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.ParticipantID == 0 || len(req.LineItems) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "participant and at least one line item are required")
		return
	}
	est, err := h.svc.Update(r.Context(), id, req.EstimateInput, req.LineItems)
	if err != nil {
		if writeValidationError(w, err) {
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if est == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, est)
}

// Delete removes an estimate by id.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Status flips just the estimate status. An empty status → 400.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.Status == "" {
		httpx.WriteError(w, http.StatusBadRequest, "status required")
		return
	}
	if err := h.svc.UpdateStatus(r.Context(), id, body.Status); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": body.Status})
}

// Duplicate copies an estimate into a new draft and returns it.
func (h *Handler) Duplicate(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	est, err := h.svc.Duplicate(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, est)
}

// BulkDelete removes every estimate whose id is in the request body.
func (h *Handler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ids []int64 `json:"ids"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := h.svc.BulkDelete(r.Context(), body.Ids); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BulkStatus sets the status of every estimate whose id is in the request body.
func (h *Handler) BulkStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ids    []int64 `json:"ids"`
		Status string  `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := h.svc.BulkUpdateStatus(r.Context(), body.Ids, body.Status); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Convert turns an accepted estimate into an invoice. A non-accepted or
// already-converted estimate → 409; an unknown id → 404.
func (h *Handler) Convert(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	res, err := h.svc.Convert(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotAccepted) {
			httpx.WriteError(w, http.StatusConflict, "only accepted estimates can be converted")
			return
		}
		if errors.Is(err, ErrAlreadyConverted) {
			httpx.WriteError(w, http.StatusConflict, "estimate already converted")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if res == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

// Pdf renders an estimate to PDF and returns it as a file download.
func (h *Handler) Pdf(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	est, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if est == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
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
		httpx.WriteError(w, http.StatusInternalServerError, "pdf render failed")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+est.Number+`.pdf"`)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(b); err != nil {
		// client gone; nothing to do
	}
}
