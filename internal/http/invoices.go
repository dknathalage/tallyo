package httpapi

import (
	"log"
	"net/http"
	"strconv"

	"github.com/dknathalage/tallyo/internal/pdf"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/service"
)

// InvoiceHandler serves the invoice CRUD, status, duplicate, bulk and
// per-client stats routes.
type InvoiceHandler struct {
	svc *service.InvoiceService
}

// NewInvoiceHandler constructs the handler. A nil svc is a programmer error.
func NewInvoiceHandler(svc *service.InvoiceService) *InvoiceHandler {
	if svc == nil {
		panic("NewInvoiceHandler: nil svc")
	}
	return &InvoiceHandler{svc: svc}
}

// invoiceRequest is the flat write payload: every InvoiceInput field (embedded so
// its json tags flatten) plus the line items. The frontend posts this shape.
type invoiceRequest struct {
	repository.InvoiceInput
	LineItems []repository.LineItemInput `json:"lineItems"`
}

// List performs a read-time overdue sweep, then returns invoices filtered by the
// optional ?clientId= or ?status= query params.
func (h *InvoiceHandler) List(w http.ResponseWriter, r *http.Request) {
	if _, err := h.svc.MarkOverdue(r.Context()); err != nil {
		log.Printf("httpapi: overdue sweep on list: %v", err)
	}
	q := r.URL.Query()
	if pid := q.Get("participantId"); pid != "" {
		participantID, err := strconv.ParseInt(pid, 10, 64)
		if err != nil || participantID <= 0 {
			WriteError(w, http.StatusBadRequest, "invalid participantId")
			return
		}
		invs, err := h.svc.ListParticipantInvoices(r.Context(), participantID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		WriteJSON(w, http.StatusOK, invs)
		return
	}
	if status := q.Get("status"); status != "" {
		invs, err := h.svc.ListByStatus(r.Context(), status)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		WriteJSON(w, http.StatusOK, invs)
		return
	}
	invs, err := h.svc.List(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, invs)
}

// Get returns a single invoice (with line items), or 404 when not found.
func (h *InvoiceHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	inv, err := h.svc.Get(r.Context(), id)
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

// Create inserts an invoice. A missing participant or empty line items → 400.
func (h *InvoiceHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req invoiceRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.ParticipantID == 0 || len(req.LineItems) == 0 {
		WriteError(w, http.StatusBadRequest, "participant and at least one line item are required")
		return
	}
	inv, err := h.svc.Create(r.Context(), req.InvoiceInput, req.LineItems)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusCreated, inv)
}

// Update rewrites an invoice. Missing client/items → 400; unknown id → 404.
func (h *InvoiceHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req invoiceRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.ParticipantID == 0 || len(req.LineItems) == 0 {
		WriteError(w, http.StatusBadRequest, "participant and at least one line item are required")
		return
	}
	inv, err := h.svc.Update(r.Context(), id, req.InvoiceInput, req.LineItems)
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

// Delete removes an invoice by id.
func (h *InvoiceHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

// Status flips just the invoice status. An empty status → 400.
func (h *InvoiceHandler) Status(w http.ResponseWriter, r *http.Request) {
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

// BulkDelete removes every invoice whose id is in the request body.
func (h *InvoiceHandler) BulkDelete(w http.ResponseWriter, r *http.Request) {
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

// BulkStatus sets the status of every invoice whose id is in the request body.
func (h *InvoiceHandler) BulkStatus(w http.ResponseWriter, r *http.Request) {
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

// ParticipantStats returns the count and summed total of one participant's
// invoices. The {id} path param is the participant id.
func (h *InvoiceHandler) ParticipantStats(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	stats, err := h.svc.ParticipantStats(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, stats)
}

// Pdf renders an invoice to PDF and returns it as a file download.
func (h *InvoiceHandler) Pdf(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	inv, err := h.svc.Get(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if inv == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	b, err := pdf.RenderInvoice(inv)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "pdf render failed")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+inv.Number+`.pdf"`)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(b); err != nil {
		// client gone; nothing to do
	}
}
