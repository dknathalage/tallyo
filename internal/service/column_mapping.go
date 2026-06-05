package service

import (
	"context"
	"database/sql"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
)

// ColumnMappingService orchestrates column-mapping reads/writes and publishes
// change events after a successful commit.
type ColumnMappingService struct {
	repo *repository.ColumnMappingsRepo
	hub  *realtime.Hub
}

func NewColumnMappingService(db *sql.DB, hub *realtime.Hub) *ColumnMappingService {
	if hub == nil {
		panic("NewColumnMappingService: nil hub")
	}
	return &ColumnMappingService{repo: repository.NewColumnMappings(db), hub: hub}
}

func (s *ColumnMappingService) List(ctx context.Context, entityType string) ([]*repository.ColumnMapping, error) {
	return s.repo.List(ctx, entityType)
}

func (s *ColumnMappingService) Get(ctx context.Context, id int64) (*repository.ColumnMapping, error) {
	return s.repo.Get(ctx, id)
}

// Create inserts a mapping, then broadcasts AFTER the commit succeeds.
func (s *ColumnMappingService) Create(ctx context.Context, in repository.ColumnMappingInput) (*repository.ColumnMapping, error) {
	m, err := s.repo.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{Entity: "column_mapping", ID: m.ID, Action: "create"})
	return m, nil
}

// Update mutates a mapping, then broadcasts on success. A nil result means the
// row was not found, in which case no event is published.
func (s *ColumnMappingService) Update(ctx context.Context, id int64, in repository.ColumnMappingInput) (*repository.ColumnMapping, error) {
	m, err := s.repo.Update(ctx, id, in)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{Entity: "column_mapping", ID: id, Action: "update"})
	return m, nil
}

// Delete removes a mapping, then broadcasts on success.
func (s *ColumnMappingService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{Entity: "column_mapping", ID: id, Action: "delete"})
	return nil
}
