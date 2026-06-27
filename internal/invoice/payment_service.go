package invoice

import (
	"context"
	"database/sql"
	"errors"

	"github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// PaymentService records and removes invoice payments.
type PaymentService struct {
	repo *PaymentsRepo
}

// NewPaymentService constructs the payment service.
func NewPaymentService(db db.Executor) *PaymentService {
	return &PaymentService{repo: NewPayments(db)}
}

// ListForInvoice returns one invoice's payments.
func (s *PaymentService) ListForInvoice(ctx context.Context, invoiceID string) ([]*Payment, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ListForInvoice(ctx, tenantID, invoiceID)
}

// ResolveInvoiceID resolves an invoice uuid to its row id (uuid) for the tenant.
// Returns ("", nil) when no invoice matches (caller 404s).
func (s *PaymentService) ResolveInvoiceID(ctx context.Context, invoiceUUID string) (string, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ResolveInvoiceID(ctx, tenantID, invoiceUUID)
}

// Create records a payment.
func (s *PaymentService) Create(ctx context.Context, in PaymentInput) (*Payment, error) {
	tenantID := reqctx.MustTenant(ctx)
	p, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Delete removes a payment. A missing payment surfaces sql.ErrNoRows so the
// handler can 404.
func (s *PaymentService) Delete(ctx context.Context, id string) error {
	tenantID := reqctx.MustTenant(ctx)
	_, _, err := s.repo.Delete(ctx, tenantID, id)
	if errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err != nil {
		return err
	}
	return nil
}

// DeleteByUUID removes a payment addressed by its uuid under the given invoice
// row id. A missing payment (or one under another invoice) surfaces
// sql.ErrNoRows so the handler can 404.
func (s *PaymentService) DeleteByUUID(ctx context.Context, invoiceID string, paymentUUID string) error {
	tenantID := reqctx.MustTenant(ctx)
	_, err := s.repo.DeleteByUUID(ctx, tenantID, invoiceID, paymentUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err != nil {
		return err
	}
	return nil
}
