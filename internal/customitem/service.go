package customitem

import (
	"context"
	"database/sql"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// Service orchestrates per-tenant custom line-item reads/writes and
// publishes change events after a successful commit.
type Service struct {
	repo *Repo
	hub  *realtime.Hub
}

// NewService constructs a Service. A nil hub is a programmer error.
func NewService(db *sql.DB, hub *realtime.Hub) *Service {
	if hub == nil {
		panic("customitem.NewService: nil hub")
	}
	return &Service{repo: NewRepo(db), hub: hub}
}

func (s *Service) List(ctx context.Context) ([]*CustomItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.List(ctx, tenantID)
}

func (s *Service) Search(ctx context.Context, q string) ([]*CustomItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Search(ctx, tenantID, q)
}

func (s *Service) Get(ctx context.Context, id int64) (*CustomItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, id)
}

// Create inserts a custom item, then broadcasts AFTER the commit succeeds.
func (s *Service) Create(ctx context.Context, in CustomItemInput) (*CustomItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	item, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "custom_item", ID: item.ID, Action: "create"})
	return item, nil
}

// Update mutates a custom item, then broadcasts on success. A nil result means
// the row was not found, in which case no event is published.
func (s *Service) Update(ctx context.Context, id int64, in CustomItemInput) (*CustomItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	item, err := s.repo.Update(ctx, tenantID, id, in)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "custom_item", ID: id, Action: "update"})
	return item, nil
}

// Delete removes a custom item, then broadcasts on success.
func (s *Service) Delete(ctx context.Context, id int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "custom_item", ID: id, Action: "delete"})
	return nil
}

// BulkDelete removes multiple custom items, then broadcasts a single bulk_delete
// event on success.
func (s *Service) BulkDelete(ctx context.Context, ids []int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.BulkDelete(ctx, tenantID, ids); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "custom_item", ID: 0, Action: "bulk_delete"})
	return nil
}
