package invoice

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/go-chi/chi/v5"
)

// PaymentHandler serves the per-invoice payment list and create routes plus
// payment deletion.
type PaymentHandler struct {
	svc *PaymentService
}

// NewPaymentHandler constructs the handler. A nil svc is a programmer error.
func NewPaymentHandler(svc *PaymentService) *PaymentHandler {
	if svc == nil {
		panic("invoice.NewPaymentHandler: nil svc")
	}
	return &PaymentHandler{svc: svc}
}

// Routes registers the payment routes on r. Mounted inside the authenticated
// /api group by the composition root.
func (h *PaymentHandler) Routes(r chi.Router) {
	r.Get("/invoices/{invoiceUUID}/payments", h.ListForInvoice)
	r.Post("/invoices/{invoiceUUID}/payments", h.Create)
	r.Delete("/invoices/{invoiceUUID}/payments/{paymentUUID}", h.Delete)
}

// paymentRequest is the write payload for recording a payment. The invoice id
// comes from the path, not the body.
type paymentRequest struct {
	Amount      float64 `json:"amount"`
	PaymentDate string  `json:"paymentDate"`
	Method      string  `json:"method"`
	Notes       string  `json:"notes"`
}

// resolveInvoice translates the {invoiceUUID} path param into the invoice int
// PK. Writes a 400 (bad uuid) or 404 (unknown) and returns ok=false on failure.
func (h *PaymentHandler) resolveInvoice(w http.ResponseWriter, r *http.Request) (string, bool) {
	invoiceUUID, ok := httpx.ParseUUID(r, "invoiceUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return "", false
	}
	invoiceID, err := h.svc.ResolveInvoiceID(r.Context(), invoiceUUID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return "", false
	}
	if invoiceID == "" {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return "", false
	}
	return invoiceID, true
}

// ListForInvoice returns one invoice's payments. The {invoiceUUID} path param is
// the invoice uuid (resolved to the int FK).
func (h *PaymentHandler) ListForInvoice(w http.ResponseWriter, r *http.Request) {
	invoiceID, ok := h.resolveInvoice(w, r)
	if !ok {
		return
	}
	payments, err := h.svc.ListForInvoice(r.Context(), invoiceID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, payments)
}

// Create records a payment against an invoice. The {invoiceUUID} path param is
// the invoice uuid (resolved to the int FK). A non-positive amount → 400.
func (h *PaymentHandler) Create(w http.ResponseWriter, r *http.Request) {
	invoiceID, ok := h.resolveInvoice(w, r)
	if !ok {
		return
	}
	var req paymentRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.Amount <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "amount must be positive")
		return
	}
	p, err := h.svc.Create(r.Context(), PaymentInput{
		InvoiceID: invoiceID,
		Amount:    req.Amount,
		PaidAt:    req.PaymentDate,
		Method:    req.Method,
		Notes:     req.Notes,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, p)
}

// Delete removes a payment addressed by {paymentUUID} under its invoice
// {invoiceUUID}. A missing payment (or one belonging to another invoice) → 404.
func (h *PaymentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	invoiceID, ok := h.resolveInvoice(w, r)
	if !ok {
		return
	}
	paymentUUID, ok := httpx.ParseUUID(r, "paymentUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid payment id")
		return
	}
	err := h.svc.DeleteByUUID(r.Context(), invoiceID, paymentUUID)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
