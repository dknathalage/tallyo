package service

import (
	"context"
	"database/sql"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// RecurringService orchestrates recurring-template reads/writes and invoice
// generation, publishing change events after a successful commit.
type RecurringService struct {
	repo *repository.RecurringRepo
	hub  *realtime.Hub
}

// NewRecurringService constructs the service. A nil hub is a programmer error.
func NewRecurringService(db *sql.DB, hub *realtime.Hub) *RecurringService {
	if hub == nil {
		panic("NewRecurringService: nil hub")
	}
	return &RecurringService{repo: repository.NewRecurring(db), hub: hub}
}

// List returns templates (all, or active only).
func (s *RecurringService) List(ctx context.Context, activeOnly bool) ([]*repository.RecurringTemplate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.List(ctx, tenantID, activeOnly)
}

// Get returns a single template, or (nil, nil) when absent.
func (s *RecurringService) Get(ctx context.Context, id int64) (*repository.RecurringTemplate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, id)
}

// Create inserts a template, then broadcasts on success.
func (s *RecurringService) Create(ctx context.Context, in repository.RecurringInput) (*repository.RecurringTemplate, error) {
	tenantID := reqctx.MustTenant(ctx)
	tpl, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{Entity: "recurring_template", ID: tpl.ID, Action: "create"})
	return tpl, nil
}

// Update rewrites a template. A nil result means the row was not found, in
// which case no event is published.
func (s *RecurringService) Update(ctx context.Context, id int64, in repository.RecurringInput) (*repository.RecurringTemplate, error) {
	tenantID := reqctx.MustTenant(ctx)
	tpl, err := s.repo.Update(ctx, tenantID, id, in)
	if err != nil {
		return nil, err
	}
	if tpl == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{Entity: "recurring_template", ID: id, Action: "update"})
	return tpl, nil
}

// Delete removes a template, then broadcasts on success.
func (s *RecurringService) Delete(ctx context.Context, id int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{Entity: "recurring_template", ID: id, Action: "delete"})
	return nil
}

// GenerateOne creates a draft invoice from the template and advances its
// next_due. A nil invoice means the template was missing (no events). On
// success it broadcasts both a template "generate" and an invoice "create".
func (s *RecurringService) GenerateOne(ctx context.Context, id int64) (*repository.Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	inv, err := s.repo.GenerateOne(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if inv == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{Entity: "recurring_template", ID: id, Action: "generate"})
	s.hub.Broadcast(realtime.Event{Entity: "invoice", ID: inv.ID, Action: "create"})
	return inv, nil
}

// GenerateDue generates one invoice per due template across ALL tenants (the
// sweep path). When any were generated it broadcasts a single sweep event so
// subscribers resync.
//
// TODO(J11): per-tenant SSE scoping — the sweep currently broadcasts globally.
func (s *RecurringService) GenerateDue(ctx context.Context) ([]repository.GeneratedInvoice, error) {
	gens, err := s.repo.GenerateDue(ctx)
	if err != nil {
		return nil, err
	}
	if len(gens) > 0 {
		s.hub.Broadcast(realtime.Event{Entity: "invoice", ID: 0, Action: "recurring_sweep"})
	}
	return gens, nil
}
