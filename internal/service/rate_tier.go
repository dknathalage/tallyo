package service

import (
	"context"
	"database/sql"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
)

// RateTierService orchestrates rate-tier reads/writes and publishes change
// events after a successful commit.
type RateTierService struct {
	repo *repository.RateTiersRepo
	hub  *realtime.Hub
}

func NewRateTierService(db *sql.DB, hub *realtime.Hub) *RateTierService {
	if hub == nil {
		panic("NewRateTierService: nil hub")
	}
	return &RateTierService{repo: repository.NewRateTiers(db), hub: hub}
}

func (s *RateTierService) List(ctx context.Context) ([]*repository.RateTier, error) {
	return s.repo.List(ctx)
}

func (s *RateTierService) Get(ctx context.Context, id int64) (*repository.RateTier, error) {
	return s.repo.Get(ctx, id)
}

func (s *RateTierService) GetDefault(ctx context.Context) (*repository.RateTier, error) {
	return s.repo.GetDefault(ctx)
}

// Create inserts a tier, then broadcasts AFTER the commit succeeds.
func (s *RateTierService) Create(ctx context.Context, in repository.RateTierInput) (*repository.RateTier, error) {
	t, err := s.repo.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{Entity: "rate_tier", ID: t.ID, Action: "create"})
	return t, nil
}

// Update mutates a tier, then broadcasts on success. A nil result means the
// row was not found, in which case no event is published.
func (s *RateTierService) Update(ctx context.Context, id int64, in repository.RateTierInput) (*repository.RateTier, error) {
	t, err := s.repo.Update(ctx, id, in)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{Entity: "rate_tier", ID: id, Action: "update"})
	return t, nil
}

// Delete removes a tier, then broadcasts on success. ErrLastTier (and any
// other error) propagates without an event.
func (s *RateTierService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{Entity: "rate_tier", ID: id, Action: "delete"})
	return nil
}
