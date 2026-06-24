package recurring

import (
	"context"
	"github.com/dknathalage/tallyo/internal/db"

	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/listquery"
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
func NewService(db db.Executor, hub *realtime.Hub) *Service {
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

// Query returns a page of templates for the given listquery clause. Rows is
// never nil so it serializes as [] not null.
func (s *Service) Query(ctx context.Context, c listquery.Clause) (listquery.Result[*RecurringTemplate], error) {
	tenantID := reqctx.MustTenant(ctx)
	rows, total, err := s.repo.Query(ctx, tenantID, c)
	if err != nil {
		return listquery.Result[*RecurringTemplate]{}, err
	}
	if rows == nil {
		rows = []*RecurringTemplate{}
	}
	return listquery.Result[*RecurringTemplate]{Rows: rows, Total: total}, nil
}

// Get returns a single template by uuid, or (nil, nil) when absent.
func (s *Service) Get(ctx context.Context, uuid string) (*RecurringTemplate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, uuid)
}

// Create inserts a template, then broadcasts on success.
func (s *Service) Create(ctx context.Context, in RecurringInput) (*RecurringTemplate, error) {
	tenantID := reqctx.MustTenant(ctx)
	tpl, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "recurring_template", UUID: tpl.ID, Action: "create"})
	return tpl, nil
}

// Update rewrites a template by uuid. A nil result means the row was not found,
// in which case no event is published. The SSE event carries the row's id (uuid).
func (s *Service) Update(ctx context.Context, uuid string, in RecurringInput) (*RecurringTemplate, error) {
	tenantID := reqctx.MustTenant(ctx)
	tpl, err := s.repo.Update(ctx, tenantID, uuid, in)
	if err != nil {
		return nil, err
	}
	if tpl == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "recurring_template", UUID: tpl.ID, Action: "update"})
	return tpl, nil
}

// Delete removes a template by uuid, then broadcasts on success. The row is
// resolved first so the post-commit event still carries the row's id (uuid).
func (s *Service) Delete(ctx context.Context, uuid string) error {
	tenantID := reqctx.MustTenant(ctx)
	tpl, err := s.repo.Get(ctx, tenantID, uuid)
	if err != nil {
		return err
	}
	if tpl == nil {
		return nil
	}
	if err := s.repo.Delete(ctx, tenantID, uuid); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "recurring_template", UUID: tpl.ID, Action: "delete"})
	return nil
}

// GenerateOne creates a draft invoice from the template and advances its
// next_due. A nil invoice means the template was missing (no events). On
// success it broadcasts both a template "generate" and an invoice "create".
func (s *Service) GenerateOne(ctx context.Context, uuid string) (*invoice.Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	tpl, err := s.repo.Get(ctx, tenantID, uuid)
	if err != nil {
		return nil, err
	}
	if tpl == nil {
		return nil, nil
	}
	inv, err := s.repo.GenerateOne(ctx, tenantID, uuid)
	if err != nil {
		return nil, err
	}
	if inv == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "recurring_template", UUID: tpl.ID, Action: "generate"})
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", UUID: inv.ID, Action: "create"})
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
// whose client plan window has lapsed, is NOT blocked at generation time;
// it surfaces only when the invoice is next edited (which re-validates). This is
// acceptable for this scope: generated invoices are drafts, reviewed before
// being sent. Revisit when adding a service-date policy for recurring lines.
func (s *Service) GenerateDueForTenant(ctx context.Context, tenantID string) ([]GeneratedInvoice, error) {
	gens, err := s.repo.GenerateDueForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if len(gens) > 0 {
		s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", UUID: "", Action: "recurring_sweep"})
	}
	return gens, nil
}
