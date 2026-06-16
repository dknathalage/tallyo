package service

import (
	"context"
	"database/sql"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// CustomItemService orchestrates per-tenant custom line-item reads/writes and
// publishes change events after a successful commit. These are the tenant-scoped
// successor to the old generic catalog items.
type CustomItemService struct {
	repo *repository.CustomItemsRepo
	hub  *realtime.Hub
}

func NewCustomItemService(db *sql.DB, hub *realtime.Hub) *CustomItemService {
	if hub == nil {
		panic("NewCustomItemService: nil hub")
	}
	return &CustomItemService{repo: repository.NewCustomItems(db), hub: hub}
}

func (s *CustomItemService) List(ctx context.Context) ([]*repository.CustomItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.List(ctx, tenantID)
}

func (s *CustomItemService) Search(ctx context.Context, q string) ([]*repository.CustomItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Search(ctx, tenantID, q)
}

func (s *CustomItemService) Get(ctx context.Context, id int64) (*repository.CustomItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, id)
}

// Create inserts a custom item, then broadcasts AFTER the commit succeeds.
func (s *CustomItemService) Create(ctx context.Context, in repository.CustomItemInput) (*repository.CustomItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	item, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{Entity: "custom_item", ID: item.ID, Action: "create"})
	return item, nil
}

// Update mutates a custom item, then broadcasts on success. A nil result means
// the row was not found, in which case no event is published.
func (s *CustomItemService) Update(ctx context.Context, id int64, in repository.CustomItemInput) (*repository.CustomItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	item, err := s.repo.Update(ctx, tenantID, id, in)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{Entity: "custom_item", ID: id, Action: "update"})
	return item, nil
}

// Delete removes a custom item, then broadcasts on success.
func (s *CustomItemService) Delete(ctx context.Context, id int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{Entity: "custom_item", ID: id, Action: "delete"})
	return nil
}

// BulkDelete removes multiple custom items, then broadcasts a single bulk_delete
// event on success.
func (s *CustomItemService) BulkDelete(ctx context.Context, ids []int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.BulkDelete(ctx, tenantID, ids); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{Entity: "custom_item", ID: 0, Action: "bulk_delete"})
	return nil
}

// SupportCatalogService exposes read access to the GLOBAL NDIS Support
// Catalogue (catalog_versions / support_items / support_item_prices). It is NOT
// tenant-scoped: the catalogue is shared reference data (spec §3.1/§4.3).
//
// TODO(J7): catalogue ingest (platform-admin XLSX upload) writes are owned by J7.
type SupportCatalogService struct {
	repo *repository.CatalogRepo
}

func NewSupportCatalogService(db *sql.DB) *SupportCatalogService {
	return &SupportCatalogService{repo: repository.NewCatalog(db)}
}

// ListVersions returns all catalogue versions.
func (s *SupportCatalogService) ListVersions(ctx context.Context) ([]*repository.CatalogVersion, error) {
	return s.repo.ListVersions(ctx)
}

// GetVersion returns a catalogue version by id, or (nil, nil) when absent.
func (s *SupportCatalogService) GetVersion(ctx context.Context, id int64) (*repository.CatalogVersion, error) {
	return s.repo.GetVersion(ctx, id)
}

// ListSupportItems returns the support items in a catalogue version.
func (s *SupportCatalogService) ListSupportItems(ctx context.Context, versionID int64) ([]*repository.SupportItem, error) {
	return s.repo.ListSupportItems(ctx, versionID)
}

// ListPrices returns the zone prices for a support item.
func (s *SupportCatalogService) ListPrices(ctx context.Context, supportItemID int64) ([]*repository.SupportItemPrice, error) {
	return s.repo.ListPrices(ctx, supportItemID)
}
