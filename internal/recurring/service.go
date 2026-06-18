package recurring

import (
	"context"
	"database/sql"

	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// Service orchestrates recurring-template reads/writes and invoice
// generation, publishing change events after a successful commit.
type Service struct {
	repo *Repo
	hub  *realtime.Hub
}

// NewService constructs the service. A nil hub is a programmer error.
func NewService(db *sql.DB, hub *realtime.Hub) *Service {
	if hub == nil {
		panic("recurring.NewService: nil hub")
	}
	return &Service{repo: NewRepo(db), hub: hub}
}

// List returns templates (all, or active only).
func (s *Service) List(ctx context.Context, activeOnly bool) ([]*RecurringTemplate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.List(ctx, tenantID, activeOnly)
}

// Get returns a single template, or (nil, nil) when absent.
func (s *Service) Get(ctx context.Context, id int64) (*RecurringTemplate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, id)
}

// Create inserts a template, then broadcasts on success.
func (s *Service) Create(ctx context.Context, in RecurringInput) (*RecurringTemplate, error) {
	tenantID := reqctx.MustTenant(ctx)
	tpl, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "recurring_template", ID: tpl.ID, Action: "create"})
	return tpl, nil
}

// Update rewrites a template. A nil result means the row was not found, in
// which case no event is published.
func (s *Service) Update(ctx context.Context, id int64, in RecurringInput) (*RecurringTemplate, error) {
	tenantID := reqctx.MustTenant(ctx)
	tpl, err := s.repo.Update(ctx, tenantID, id, in)
	if err != nil {
		return nil, err
	}
	if tpl == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "recurring_template", ID: id, Action: "update"})
	return tpl, nil
}

// Delete removes a template, then broadcasts on success.
func (s *Service) Delete(ctx context.Context, id int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "recurring_template", ID: id, Action: "delete"})
	return nil
}

// GenerateOne creates a draft invoice from the template and advances its
// next_due. A nil invoice means the template was missing (no events). On
// success it broadcasts both a template "generate" and an invoice "create".
func (s *Service) GenerateOne(ctx context.Context, id int64) (*invoice.Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	inv, err := s.repo.GenerateOne(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if inv == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "recurring_template", ID: id, Action: "generate"})
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", ID: inv.ID, Action: "create"})
	return inv, nil
}

// GenerateDueForTenant generates one invoice per due template of ONE tenant (the
// per-tenant sweep path, spec §8). When any were generated it broadcasts a
// single sweep event SCOPED to that tenant so only its subscribers resync. ctx
// must carry the tenant (the sweep driver attaches it via reqctx.WithTenant).
//
// Validation-engine note (J10/J11 decision): generated invoices are produced
// DB-side in the repository (tx-scoped numbering + idempotent next_due advance
// in one transaction) and do NOT pass through the J10 LineValidator. Routing
// them through it was DEFERRED because recurring template lines carry no
// per-line service_date — the validator's version-resolution and plan-window
// checks are keyed on service_date, so they have nothing to validate against
// without first defining a service-date policy for generated lines. RISK: a
// generated line whose template unit_price exceeds the current price cap, or
// whose participant plan window has lapsed, is NOT blocked at generation time;
// it surfaces only when the invoice is next edited (which re-validates). This is
// acceptable for this scope: generated invoices are drafts, reviewed before
// being sent. Revisit when adding a service-date policy for recurring lines.
func (s *Service) GenerateDueForTenant(ctx context.Context, tenantID int64) ([]GeneratedInvoice, error) {
	gens, err := s.repo.GenerateDueForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if len(gens) > 0 {
		s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", ID: 0, Action: "recurring_sweep"})
	}
	return gens, nil
}
