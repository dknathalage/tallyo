package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// PaymentService records and removes invoice payments, publishing both a
// payment event and an invoice update (so the invoice balance refreshes for
// subscribers) after a successful commit.
type PaymentService struct {
	repo *repository.PaymentsRepo
	hub  *realtime.Hub
}

func NewPaymentService(db *sql.DB, hub *realtime.Hub) *PaymentService {
	if hub == nil {
		panic("NewPaymentService: nil hub")
	}
	return &PaymentService{repo: repository.NewPayments(db), hub: hub}
}

// ListForInvoice returns one invoice's payments.
func (s *PaymentService) ListForInvoice(ctx context.Context, invoiceID int64) ([]*repository.Payment, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ListForInvoice(ctx, tenantID, invoiceID)
}

// Create records a payment, then broadcasts a payment create plus an invoice
// update so the invoice's balance refreshes.
func (s *PaymentService) Create(ctx context.Context, in repository.PaymentInput) (*repository.Payment, error) {
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
