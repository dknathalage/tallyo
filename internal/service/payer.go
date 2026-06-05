package service

import (
	"context"
	"database/sql"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
)

// PayerService orchestrates payer reads/writes and publishes change events
// after a successful commit.
type PayerService struct {
	repo *repository.PayersRepo
	hub  *realtime.Hub
}

func NewPayerService(db *sql.DB, hub *realtime.Hub) *PayerService {
	if hub == nil {
		panic("NewPayerService: nil hub")
	}
	return &PayerService{repo: repository.NewPayers(db), hub: hub}
}

func (s *PayerService) List(ctx context.Context, search string) ([]*repository.Payer, error) {
	return s.repo.List(ctx, search)
}

func (s *PayerService) Get(ctx context.Context, id int64) (*repository.Payer, error) {
	return s.repo.Get(ctx, id)
}

// Create inserts a payer, then broadcasts AFTER the commit succeeds.
func (s *PayerService) Create(ctx context.Context, in repository.PayerInput) (*repository.Payer, error) {
	p, err := s.repo.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{Entity: "payer", ID: p.ID, Action: "create"})
	return p, nil
}

// Update mutates a payer, then broadcasts on success. A nil result means the
// row was not found, in which case no event is published.
func (s *PayerService) Update(ctx context.Context, id int64, in repository.PayerInput) (*repository.Payer, error) {
	p, err := s.repo.Update(ctx, id, in)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{Entity: "payer", ID: id, Action: "update"})
	return p, nil
}

// Delete removes a payer, then broadcasts on success.
func (s *PayerService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{Entity: "payer", ID: id, Action: "delete"})
	return nil
}

// BulkDelete removes multiple payers, then broadcasts a single bulk_delete
// event on success.
func (s *PayerService) BulkDelete(ctx context.Context, ids []int64) error {
	if err := s.repo.BulkDelete(ctx, ids); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{Entity: "payer", ID: 0, Action: "bulk_delete"})
	return nil
}
