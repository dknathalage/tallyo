package customitem

import (
	"context"
	"github.com/dknathalage/tallyo/internal/db"

	"github.com/dknathalage/tallyo/internal/apperr"
	"github.com/dknathalage/tallyo/internal/events"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// Service orchestrates per-tenant custom line-item reads/writes and
// publishes change events after a successful commit.
type Service struct {
	repo   *Repo
	hub    *realtime.Hub
	events events.Notifier
}

// NewService constructs a Service. A nil hub is a programmer error.
func NewService(db db.Executor, hub *realtime.Hub) *Service {
	if hub == nil {
		panic("customitem.NewService: nil hub")
	}
	return &Service{repo: NewRepo(db), hub: hub, events: events.New(hub, "custom_item")}
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
	item, err := s.repo.Get(ctx, tenantID, uuid)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, apperr.ErrNotFound
	}
	return item, nil
}

// Create inserts a custom item, then broadcasts AFTER the commit succeeds.
func (s *Service) Create(ctx context.Context, in CustomItemInput) (*CustomItem, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	tenantID := reqctx.MustTenant(ctx)
	item, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	s.events.Created(tenantID, item.ID)
	return item, nil
}

// Update mutates a custom item, then broadcasts on success. A missing row
// surfaces as apperr.ErrNotFound from the repo and is propagated.
func (s *Service) Update(ctx context.Context, uuid string, in CustomItemInput) (*CustomItem, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	tenantID := reqctx.MustTenant(ctx)
	item, err := s.repo.Update(ctx, tenantID, uuid, in)
	if err != nil {
		return nil, err
	}
	s.events.Updated(tenantID, item.ID)
	return item, nil
}

// Delete removes a custom item by uuid, then broadcasts on success. A missing
// row surfaces as apperr.ErrNotFound from the repo and is propagated.
func (s *Service) Delete(ctx context.Context, uuid string) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Delete(ctx, tenantID, uuid); err != nil {
		return err
	}
	s.events.Deleted(tenantID, uuid)
	return nil
}

// ResolveCustomItemIDs resolves a list of custom-item uuids to their row ids (uuid)
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
