package service

import (
	"context"
	"database/sql"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
)

// ClientService orchestrates client reads/writes and publishes change events
// after a successful commit.
type ClientService struct {
	repo *repository.ClientsRepo
	hub  *realtime.Hub
}

func NewClientService(db *sql.DB, hub *realtime.Hub) *ClientService {
	if hub == nil {
		panic("NewClientService: nil hub")
	}
	return &ClientService{repo: repository.NewClients(db), hub: hub}
}

func (s *ClientService) List(ctx context.Context, search string) ([]*repository.Client, error) {
	return s.repo.List(ctx, search)
}

func (s *ClientService) Get(ctx context.Context, id int64) (*repository.Client, error) {
	return s.repo.Get(ctx, id)
}

// Create inserts a client, then broadcasts AFTER the commit succeeds.
func (s *ClientService) Create(ctx context.Context, in repository.ClientInput) (*repository.Client, error) {
	c, err := s.repo.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{Entity: "client", ID: c.ID, Action: "create"})
	return c, nil
}

// Update mutates a client, then broadcasts on success. A nil result means the
// row was not found, in which case no event is published.
func (s *ClientService) Update(ctx context.Context, id int64, in repository.ClientInput) (*repository.Client, error) {
	c, err := s.repo.Update(ctx, id, in)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{Entity: "client", ID: id, Action: "update"})
	return c, nil
}

// Delete removes a client, then broadcasts on success.
func (s *ClientService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{Entity: "client", ID: id, Action: "delete"})
	return nil
}

// BulkDelete removes multiple clients, then broadcasts a single bulk_delete
// event on success.
func (s *ClientService) BulkDelete(ctx context.Context, ids []int64) error {
	if err := s.repo.BulkDelete(ctx, ids); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{Entity: "client", ID: 0, Action: "bulk_delete"})
	return nil
}
