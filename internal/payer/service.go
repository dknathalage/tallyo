package payer

import (
	"context"
	"github.com/dknathalage/tallyo/internal/db"

	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// Service orchestrates payer reads/writes and publishes change events
// after a successful commit.
type Service struct {
	repo *PayersRepo
	hub  *realtime.Hub
}

// NewService constructs the payer service. A nil hub is a programmer error.
func NewService(db db.Executor, hub *realtime.Hub) *Service {
	if hub == nil {
		panic("payer.NewService: nil hub")
	}
	return &Service{repo: NewPayers(db), hub: hub}
}

func (s *Service) List(ctx context.Context, search string) ([]*Payer, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.List(ctx, tenantID, search)
}

// Query returns a page of payers for the given listquery clause. Rows is
// never null so it serializes as [] not null.
func (s *Service) Query(ctx context.Context, c listquery.Clause) (listquery.Result[*Payer], error) {
	tenantID := reqctx.MustTenant(ctx)
	rows, total, err := s.repo.Query(ctx, tenantID, c)
	if err != nil {
		return listquery.Result[*Payer]{}, err
	}
	if rows == nil {
		rows = []*Payer{}
	}
	return listquery.Result[*Payer]{Rows: rows, Total: total}, nil
}

func (s *Service) Get(ctx context.Context, uuid string) (*Payer, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, uuid)
}

// Create inserts a payer, then broadcasts AFTER the commit succeeds.
func (s *Service) Create(ctx context.Context, in PayerInput) (*Payer, error) {
	tenantID := reqctx.MustTenant(ctx)
	p, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "payer", UUID: p.ID, Action: "create"})
	return p, nil
}

// Update mutates a payer, then broadcasts on success. A nil result means
// the row was not found, in which case no event is published.
func (s *Service) Update(ctx context.Context, uuid string, in PayerInput) (*Payer, error) {
	tenantID := reqctx.MustTenant(ctx)
	p, err := s.repo.Update(ctx, tenantID, uuid, in)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "payer", UUID: p.ID, Action: "update"})
	return p, nil
}

// Delete removes a payer by uuid, then broadcasts on success. The row is
// resolved first so the post-commit event still carries the int PK.
func (s *Service) Delete(ctx context.Context, uuid string) error {
	tenantID := reqctx.MustTenant(ctx)
	p, err := s.repo.Get(ctx, tenantID, uuid)
	if err != nil {
		return err
	}
	if p == nil {
		return nil
	}
	if err := s.repo.Delete(ctx, tenantID, uuid); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "payer", UUID: p.ID, Action: "delete"})
	return nil
}

// ResolvePayerIDs translates a list of payer uuids into their int
// PKs for the tenant (preserving order). An unknown uuid surfaces as an error so
// the caller can 400 — bulk operations must not silently drop a member.
func (s *Service) ResolvePayerIDs(ctx context.Context, pmUUIDs []string) ([]string, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ResolvePayerIDs(ctx, tenantID, pmUUIDs)
}

// BulkDelete removes multiple payers, then broadcasts a single
// bulk_delete event on success.
func (s *Service) BulkDelete(ctx context.Context, ids []string) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.BulkDelete(ctx, tenantID, ids); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "payer", UUID: "", Action: "bulk_delete"})
	return nil
}
