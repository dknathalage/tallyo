package customitem

import (
	"context"
	"github.com/dknathalage/tallyo/internal/db"

	"github.com/dknathalage/tallyo/internal/listquery"
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
func NewService(db db.Executor, hub *realtime.Hub) *Service {
	if hub == nil {
		panic("customitem.NewService: nil hub")
	}
	return &Service{repo: NewRepo(db), hub: hub}
}

func (s *Service) List(ctx context.Context) ([]*CustomItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.List(ctx, tenantID)
}

// Query returns a page of custom items for the given listquery clause. Rows is
// always non-nil so it serializes as [] not null.
func (s *Service) Query(ctx context.Context, c listquery.Clause) (listquery.Result[*CustomItem], error) {
	tenantID := reqctx.MustTenant(ctx)
	rows, total, err := s.repo.Query(ctx, tenantID, c)
	if err != nil {
		return listquery.Result[*CustomItem]{}, err
	}
	if rows == nil {
		rows = []*CustomItem{}
	}
	return listquery.Result[*CustomItem]{Rows: rows, Total: total}, nil
}

func (s *Service) Search(ctx context.Context, q string) ([]*CustomItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Search(ctx, tenantID, q)
}

func (s *Service) Get(ctx context.Context, uuid string) (*CustomItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, uuid)
}

// Create inserts a custom item, then broadcasts AFTER the commit succeeds.
func (s *Service) Create(ctx context.Context, in CustomItemInput) (*CustomItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	item, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "custom_item", UUID: item.ID, Action: "create"})
	return item, nil
}

// Update mutates a custom item, then broadcasts on success. A nil result means
// the row was not found, in which case no event is published.
func (s *Service) Update(ctx context.Context, uuid string, in CustomItemInput) (*CustomItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	item, err := s.repo.Update(ctx, tenantID, uuid, in)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "custom_item", UUID: item.ID, Action: "update"})
	return item, nil
}

// Delete removes a custom item by uuid, then broadcasts on success. The row is
// resolved first so the post-commit event still carries the int PK.
func (s *Service) Delete(ctx context.Context, uuid string) error {
	tenantID := reqctx.MustTenant(ctx)
	item, err := s.repo.Get(ctx, tenantID, uuid)
	if err != nil {
		return err
	}
	if item == nil {
		return nil
	}
	if err := s.repo.Delete(ctx, tenantID, uuid); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "custom_item", UUID: item.ID, Action: "delete"})
	return nil
}

// ResolveCustomItemIDs translates a list of custom-item uuids into their int PKs
// for the tenant (preserving order). An unknown uuid surfaces as an error so the
// caller can 400 — bulk operations must not silently drop a member.
func (s *Service) ResolveCustomItemIDs(ctx context.Context, itemUUIDs []string) ([]string, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ResolveCustomItemIDs(ctx, tenantID, itemUUIDs)
}

// BulkDelete removes multiple custom items, then broadcasts a single bulk_delete
// event on success.
func (s *Service) BulkDelete(ctx context.Context, ids []string) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.BulkDelete(ctx, tenantID, ids); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "custom_item", UUID: "", Action: "bulk_delete"})
	return nil
}
