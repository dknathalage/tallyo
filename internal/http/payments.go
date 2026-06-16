package httpapi

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/service"
)

// PaymentHandler serves the per-invoice payment list and create routes plus
// payment deletion.
type PaymentHandler struct {
	svc *service.PaymentService
}

// NewPaymentHandler constructs the handler. A nil svc is a programmer error.
func NewPaymentHandler(svc *service.PaymentService) *PaymentHandler {
	if svc == nil {
		panic("NewPaymentHandler: nil svc")
	}
	return &PaymentHandler{svc: svc}
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
	invoiceID, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	payments, err := h.svc.ListForInvoice(r.Context(), invoiceID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, payments)
}

// Create records a payment against an invoice. The {id} path param is the
// invoice id. A non-positive amount → 400.
func (h *PaymentHandler) Create(w http.ResponseWriter, r *http.Request) {
	invoiceID, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req paymentRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.Amount <= 0 {
		WriteError(w, http.StatusBadRequest, "amount must be positive")
		return
	}
	p, err := h.svc.Create(r.Context(), repository.PaymentInput{
		InvoiceID: invoiceID,
		Amount:    req.Amount,
		PaidAt:    req.PaymentDate,
		Method:    req.Method,
		Notes:     req.Notes,
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusCreated, p)
}

// Delete removes a payment. The {id} path param is the payment id. A missing
// payment → 404.
func (h *PaymentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	err := h.svc.Delete(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
