package service

import (
	"context"
	"database/sql"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// InvoiceService orchestrates invoice reads/writes and publishes change events
// after a successful commit. Line items pass through the NDIS validation engine
// (validator) on create/update before reaching the repository.
type InvoiceService struct {
	repo      *repository.InvoicesRepo
	validator *LineValidator
	hub       *realtime.Hub
}

func NewInvoiceService(db *sql.DB, hub *realtime.Hub) *InvoiceService {
	if hub == nil {
		panic("NewInvoiceService: nil hub")
	}
	return &InvoiceService{repo: repository.NewInvoices(db), validator: NewLineValidator(db), hub: hub}
}

func (s *InvoiceService) List(ctx context.Context) ([]*repository.Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.List(ctx, tenantID)
}

func (s *InvoiceService) ListByStatus(ctx context.Context, status string) ([]*repository.Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ListByStatus(ctx, tenantID, status)
}

func (s *InvoiceService) ListParticipantInvoices(ctx context.Context, participantID int64) ([]*repository.Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ListParticipantInvoices(ctx, tenantID, participantID)
}

func (s *InvoiceService) Get(ctx context.Context, id int64) (*repository.Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, id)
}

func (s *InvoiceService) ParticipantStats(ctx context.Context, participantID int64) (*repository.ParticipantStats, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ParticipantStats(ctx, tenantID, participantID)
}

// Create inserts an invoice + line items, then broadcasts on success.
//
// Every line passes through the NDIS validation engine (price-cap, plan-window,
// gst-free defaulting, snapshotting) first; tax is COMPUTED from the validated
// lines and overrides any client-supplied value (see validation.go tax note).
// A validation failure returns a *ValidationError with field-level detail.
func (s *InvoiceService) Create(ctx context.Context, in repository.InvoiceInput, items []repository.LineItemInput) (*repository.Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	res, err := s.validator.Validate(ctx, tenantID, in.ParticipantID, items)
	if err != nil {
		return nil, err
	}
	in.Tax = res.Tax
	inv, err := s.repo.Create(ctx, tenantID, in, res.Items)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{Entity: "invoice", ID: inv.ID, Action: "create"})
	return inv, nil
}

// Update rewrites an invoice. A nil result means the row was not found, in which
// case no event is published.
func (s *InvoiceService) Update(ctx context.Context, id int64, in repository.InvoiceInput, items []repository.LineItemInput) (*repository.Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	res, err := s.validator.Validate(ctx, tenantID, in.ParticipantID, items)
	if err != nil {
		return nil, err
	}
	in.Tax = res.Tax
	inv, err := s.repo.Update(ctx, tenantID, id, in, res.Items)
	if err != nil {
		return nil, err
	}
	if inv == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{Entity: "invoice", ID: id, Action: "update"})
	return inv, nil
}

// UpdateStatus sets the invoice status, then broadcasts on success.
func (s *InvoiceService) UpdateStatus(ctx context.Context, id int64, status string) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.UpdateStatus(ctx, tenantID, id, status); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{Entity: "invoice", ID: id, Action: "status"})
	return nil
}

// Delete removes an invoice, then broadcasts on success.
func (s *InvoiceService) Delete(ctx context.Context, id int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{Entity: "invoice", ID: id, Action: "delete"})
	return nil
}

// BulkDelete removes several invoices, then broadcasts a single bulk event.
func (s *InvoiceService) BulkDelete(ctx context.Context, ids []int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.BulkDelete(ctx, tenantID, ids); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{Entity: "invoice", ID: 0, Action: "bulk_delete"})
	return nil
}

// BulkUpdateStatus sets several invoices' status, then broadcasts a bulk event.
func (s *InvoiceService) BulkUpdateStatus(ctx context.Context, ids []int64, status string) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.BulkUpdateStatus(ctx, tenantID, ids, status); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{Entity: "invoice", ID: 0, Action: "bulk_status"})
	return nil
}

// MarkOverdue flips overdue invoices across ALL tenants (the sweep path) and,
// when any flipped, broadcasts a sweep event so subscribers resync.
//
// TODO(J11): per-tenant SSE scoping — the sweep currently broadcasts globally.
func (s *InvoiceService) MarkOverdue(ctx context.Context) ([]repository.OverdueInvoice, error) {
	rows, err := s.repo.MarkOverdue(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		s.hub.Broadcast(realtime.Event{Entity: "invoice", ID: 0, Action: "overdue_sweep"})
	}
	return rows, nil
}
