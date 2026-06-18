package invoice

import (
	"log/slog"
	"net/http"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/pdf"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/go-chi/chi/v5"
)

// Handler serves the invoice CRUD, status, bulk and per-client stats routes.
type Handler struct {
	svc *Service
}

// NewHandler constructs the handler. A nil svc is a programmer error.
func NewHandler(svc *Service) *Handler {
	if svc == nil {
		panic("invoice.NewHandler: nil svc")
	}
	return &Handler{svc: svc}
}

// Routes registers the invoice routes on r. Mounted inside the authenticated
// /api group by the composition root.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/invoices", h.List)
	r.Post("/invoices", h.Create)
	r.Post("/invoices/bulk-delete", h.BulkDelete)
	r.Post("/invoices/bulk-status", h.BulkStatus)
	r.Get("/invoices/{id}", h.Get)
	r.Put("/invoices/{id}", h.Update)
	r.Delete("/invoices/{id}", h.Delete)
	r.Post("/invoices/{id}/status", h.Status)
	r.Get("/invoices/{id}/pdf", h.Pdf)
	r.Get("/participants/{id}/stats", h.ParticipantStats)
}

// invoiceRequest is the flat write payload: every InvoiceInput field (embedded so
// its json tags flatten) plus the line items. The frontend posts this shape.
type invoiceRequest struct {
	InvoiceInput
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

// List performs a read-time overdue sweep, then returns invoices filtered by the
// optional ?participantId= or ?status= query params.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if _, err := h.svc.MarkOverdueForTenant(r.Context(), reqctx.MustTenant(r.Context())); err != nil {
		httpx.LoggerFrom(r.Context()).Error("overdue sweep on list failed", slog.Any("error", err))
	}
	q := r.URL.Query()
	if pid := q.Get("participantId"); pid != "" {
		participantID, ok := parseIDFromString(pid)
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "invalid participantId")
			return
		}
		invs, err := h.svc.ListParticipantInvoices(r.Context(), participantID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, invs)
		return
	}
	if status := q.Get("status"); status != "" {
		invs, err := h.svc.ListByStatus(r.Context(), status)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, invs)
		return
	}
	invs, err := h.svc.List(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, invs)
}

// Get returns a single invoice (with line items), or 404 when not found.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	inv, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if inv == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, inv)
}

// Create inserts an invoice. A missing participant or empty line items → 400.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req invoiceRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.ParticipantID == 0 || len(req.LineItems) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "participant and at least one line item are required")
		return
	}
	inv, err := h.svc.Create(r.Context(), req.InvoiceInput, req.LineItems)
	if err != nil {
		if writeValidationError(w, err) {
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, inv)
}

// Update rewrites an invoice. Missing participant/items → 400; unknown id → 404.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req invoiceRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.ParticipantID == 0 || len(req.LineItems) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "participant and at least one line item are required")
		return
	}
	inv, err := h.svc.Update(r.Context(), id, req.InvoiceInput, req.LineItems)
	if err != nil {
		if writeValidationError(w, err) {
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if inv == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, inv)
}

// Delete removes an invoice by id.
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

// Status flips just the invoice status. An empty status → 400.
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

// BulkDelete removes every invoice whose id is in the request body.
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

// BulkStatus sets the status of every invoice whose id is in the request body.
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

// ParticipantStats returns the count and summed total of one participant's
// invoices. The {id} path param is the participant id.
func (h *Handler) ParticipantStats(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	stats, err := h.svc.ParticipantStats(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, stats)
}

// Pdf renders an invoice to PDF and returns it as a file download.
func (h *Handler) Pdf(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	inv, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if inv == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	doc := &pdf.InvoiceDoc{
		Number:           inv.Number,
		IssueDate:        inv.IssueDate,
		DueDate:          inv.DueDate,
		Status:           inv.Status,
		BusinessSnapshot: inv.BusinessSnapshot,
		ClientSnapshot:   inv.ClientSnapshot,
		LineItems:        inv.LineItems,
		Subtotal:         inv.Subtotal,
		Tax:              inv.Tax,
		Total:            inv.Total,
		Notes:            inv.Notes,
	}
	b, err := pdf.RenderInvoice(doc)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "pdf render failed")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+inv.Number+`.pdf"`)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(b); err != nil {
		// client gone; nothing to do
	}
}
