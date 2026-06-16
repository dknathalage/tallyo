package service

import (
	"context"
	"database/sql"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// PlanManagerService orchestrates plan-manager reads/writes and publishes change
// events after a successful commit.
type PlanManagerService struct {
	repo *repository.PlanManagersRepo
	hub  *realtime.Hub
}

func NewPlanManagerService(db *sql.DB, hub *realtime.Hub) *PlanManagerService {
	if hub == nil {
		panic("NewPlanManagerService: nil hub")
	}
	return &PlanManagerService{repo: repository.NewPlanManagers(db), hub: hub}
}

func (s *PlanManagerService) List(ctx context.Context, search string) ([]*repository.PlanManager, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.List(ctx, tenantID, search)
}

func (s *PlanManagerService) Get(ctx context.Context, id int64) (*repository.PlanManager, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, id)
}

// Create inserts a plan manager, then broadcasts AFTER the commit succeeds.
func (s *PlanManagerService) Create(ctx context.Context, in repository.PlanManagerInput) (*repository.PlanManager, error) {
	tenantID := reqctx.MustTenant(ctx)
	p, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "plan_manager", ID: p.ID, Action: "create"})
	return p, nil
}

// Update mutates a plan manager, then broadcasts on success. A nil result means
// the row was not found, in which case no event is published.
func (s *PlanManagerService) Update(ctx context.Context, id int64, in repository.PlanManagerInput) (*repository.PlanManager, error) {
	tenantID := reqctx.MustTenant(ctx)
	p, err := s.repo.Update(ctx, tenantID, id, in)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "plan_manager", ID: id, Action: "update"})
	return p, nil
}

// Delete removes a plan manager, then broadcasts on success.
func (s *PlanManagerService) Delete(ctx context.Context, id int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "plan_manager", ID: id, Action: "delete"})
	return nil
}

// BulkDelete removes multiple plan managers, then broadcasts a single
// bulk_delete event on success.
func (s *PlanManagerService) BulkDelete(ctx context.Context, ids []int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.BulkDelete(ctx, tenantID, ids); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "plan_manager", ID: 0, Action: "bulk_delete"})
	return nil
}
