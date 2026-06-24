package invoice

import (
	"context"
	"database/sql"
	"errors"

	"github.com/dknathalage/tallyo/internal/db"
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
func NewPaymentService(db db.Executor, hub *realtime.Hub) *PaymentService {
	if hub == nil {
		panic("invoice.NewPaymentService: nil hub")
	}
	return &PaymentService{repo: NewPayments(db), hub: hub}
}

// ListForInvoice returns one invoice's payments.
func (s *PaymentService) ListForInvoice(ctx context.Context, invoiceID string) ([]*Payment, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ListForInvoice(ctx, tenantID, invoiceID)
}

// ResolveInvoiceID translates an invoice uuid into its int PK for the tenant.
// Returns (0, nil) when no invoice matches (caller 404s).
func (s *PaymentService) ResolveInvoiceID(ctx context.Context, invoiceUUID string) (string, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ResolveInvoiceID(ctx, tenantID, invoiceUUID)
}

// Create records a payment, then broadcasts a payment create plus an invoice
// update so the invoice's balance refreshes.
func (s *PaymentService) Create(ctx context.Context, in PaymentInput) (*Payment, error) {
	tenantID := reqctx.MustTenant(ctx)
	p, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	invoiceUUID, err := s.repo.InvoiceUUID(ctx, tenantID, in.InvoiceID)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "payment", UUID: p.ID, Action: "create"})
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", UUID: invoiceUUID, Action: "update"})
	return p, nil
}

// Delete removes a payment. A missing payment surfaces sql.ErrNoRows so the
// handler can 404; on success it broadcasts a payment delete plus an invoice
// update so the balance refreshes.
func (s *PaymentService) Delete(ctx context.Context, id string) error {
	tenantID := reqctx.MustTenant(ctx)
	paymentUUID, invoiceID, err := s.repo.Delete(ctx, tenantID, id)
	if errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err != nil {
		return err
	}
	invoiceUUID, err := s.repo.InvoiceUUID(ctx, tenantID, invoiceID)
	if err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "payment", UUID: paymentUUID, Action: "delete"})
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", UUID: invoiceUUID, Action: "update"})
	return nil
}

// DeleteByUUID removes a payment addressed by its uuid under the given invoice
// int id. A missing payment (or one under another invoice) surfaces
// sql.ErrNoRows so the handler can 404; on success it broadcasts a payment
// delete plus an invoice update so the balance refreshes. The SSE events carry
// the payment uuid and the invoice uuid — no int PK crosses the API.
func (s *PaymentService) DeleteByUUID(ctx context.Context, invoiceID string, paymentUUID string) error {
	tenantID := reqctx.MustTenant(ctx)
	deletedInvoiceID, err := s.repo.DeleteByUUID(ctx, tenantID, invoiceID, paymentUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err != nil {
		return err
	}
	invoiceUUID, err := s.repo.InvoiceUUID(ctx, tenantID, deletedInvoiceID)
	if err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "payment", UUID: paymentUUID, Action: "delete"})
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", UUID: invoiceUUID, Action: "update"})
	return nil
}
