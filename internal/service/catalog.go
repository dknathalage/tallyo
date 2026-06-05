package service

import (
	"context"
	"database/sql"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
)

// CatalogService orchestrates catalog-item reads/writes and publishes change
// events after a successful commit.
type CatalogService struct {
	repo *repository.CatalogRepo
	hub  *realtime.Hub
}

func NewCatalogService(db *sql.DB, hub *realtime.Hub) *CatalogService {
	if hub == nil {
		panic("NewCatalogService: nil hub")
	}
	return &CatalogService{repo: repository.NewCatalog(db), hub: hub}
}

func (s *CatalogService) List(ctx context.Context) ([]*repository.CatalogItem, error) {
	return s.repo.List(ctx)
}

func (s *CatalogService) Search(ctx context.Context, q string) ([]*repository.CatalogItem, error) {
	return s.repo.Search(ctx, q)
}

func (s *CatalogService) Get(ctx context.Context, id int64) (*repository.CatalogItem, error) {
	return s.repo.Get(ctx, id)
}

func (s *CatalogService) Categories(ctx context.Context) ([]string, error) {
	return s.repo.Categories(ctx)
}

func (s *CatalogService) GetRates(ctx context.Context, itemID int64) ([]*repository.CatalogItemRate, error) {
	return s.repo.GetRates(ctx, itemID)
}

func (s *CatalogService) EffectiveRate(ctx context.Context, itemID int64, tierID *int64) (float64, error) {
	return s.repo.EffectiveRate(ctx, itemID, tierID)
}

// Create inserts a catalog item, then broadcasts AFTER the commit succeeds.
func (s *CatalogService) Create(ctx context.Context, in repository.CatalogItemInput) (*repository.CatalogItem, error) {
	item, err := s.repo.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{Entity: "catalog_item", ID: item.ID, Action: "create"})
	return item, nil
}

// Update mutates a catalog item, then broadcasts on success. A nil result means
// the row was not found, in which case no event is published.
func (s *CatalogService) Update(ctx context.Context, id int64, in repository.CatalogItemInput) (*repository.CatalogItem, error) {
	item, err := s.repo.Update(ctx, id, in)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{Entity: "catalog_item", ID: id, Action: "update"})
	return item, nil
}

// Delete removes a catalog item, then broadcasts on success.
func (s *CatalogService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{Entity: "catalog_item", ID: id, Action: "delete"})
	return nil
}

// BulkDelete removes multiple catalog items, then broadcasts a single
// bulk_delete event on success.
func (s *CatalogService) BulkDelete(ctx context.Context, ids []int64) error {
	if err := s.repo.BulkDelete(ctx, ids); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{Entity: "catalog_item", ID: 0, Action: "bulk_delete"})
	return nil
}

// SetRate upserts a per-tier rate for an item, then broadcasts on success.
func (s *CatalogService) SetRate(ctx context.Context, itemID, tierID int64, rate float64) error {
	if err := s.repo.SetRate(ctx, itemID, tierID, rate); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{Entity: "catalog_item", ID: itemID, Action: "set_rate"})
	return nil
}
