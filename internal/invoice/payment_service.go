package invoice

import (
	"context"
	"database/sql"
	"errors"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// PaymentService records and removes invoice payments, publishing both a
// payment event and an invoice update (so the invoice balance refreshes for
// subscribers) after a successful commit.
type PaymentService struct {
	repo *PaymentsRepo
	hub  *realtime.Hub
}

// NewPaymentService constructs the payment service. A nil hub is a programmer error.
func NewPaymentService(db *sql.DB, hub *realtime.Hub) *PaymentService {
	if hub == nil {
		panic("invoice.NewPaymentService: nil hub")
	}
	return &PaymentService{repo: NewPayments(db), hub: hub}
}

// ListForInvoice returns one invoice's payments.
func (s *PaymentService) ListForInvoice(ctx context.Context, invoiceID int64) ([]*Payment, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ListForInvoice(ctx, tenantID, invoiceID)
}

// Create records a payment, then broadcasts a payment create plus an invoice
// update so the invoice's balance refreshes.
func (s *PaymentService) Create(ctx context.Context, in PaymentInput) (*Payment, error) {
	tenantID := reqctx.MustTenant(ctx)
	p, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "payment", ID: p.ID, Action: "create"})
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", ID: in.InvoiceID, Action: "update"})
	return p, nil
}

// Delete removes a payment. A missing payment surfaces sql.ErrNoRows so the
// handler can 404; on success it broadcasts a payment delete plus an invoice
// update so the balance refreshes.
func (s *PaymentService) Delete(ctx context.Context, id int64) error {
	tenantID := reqctx.MustTenant(ctx)
	invoiceID, err := s.repo.Delete(ctx, tenantID, id)
	if errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "payment", ID: id, Action: "delete"})
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", ID: invoiceID, Action: "update"})
	return nil
}
