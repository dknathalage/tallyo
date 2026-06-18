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
	r.Get("/invoices/{id}/payments", h.ListForInvoice)
	r.Post("/invoices/{id}/payments", h.Create)
	r.Delete("/payments/{id}", h.Delete)
}

// paymentRequest is the write payload for recording a payment. The invoice id
// comes from the path, not the body.
type paymentRequest struct {
	Amount      float64 `json:"amount"`
	PaymentDate string  `json:"paymentDate"`
	Method      string  `json:"method"`
	Notes       string  `json:"notes"`
}

// ListForInvoice returns one invoice's payments. The {id} path param is the
// invoice id.
func (h *PaymentHandler) ListForInvoice(w http.ResponseWriter, r *http.Request) {
	invoiceID, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	payments, err := h.svc.ListForInvoice(r.Context(), invoiceID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, payments)
}

// Create records a payment against an invoice. The {id} path param is the
// invoice id. A non-positive amount → 400.
func (h *PaymentHandler) Create(w http.ResponseWriter, r *http.Request) {
	invoiceID, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
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

// Delete removes a payment. The {id} path param is the payment id. A missing
// payment → 404.
func (h *PaymentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	err := h.svc.Delete(r.Context(), id)
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
