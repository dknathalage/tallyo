package client

import (
	"context"

	"github.com/dknathalage/tallyo/internal/db"

	"github.com/dknathalage/tallyo/internal/apperr"
	"github.com/dknathalage/tallyo/internal/events"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// QueryResult is one page of clients plus the total matching the filter
// (before pagination). rows is never null in JSON.
type QueryResult struct {
	Rows  []*Client `json:"rows"`
	Total int64     `json:"total"`
}

// Service orchestrates client reads/writes and publishes change events
// after a successful commit. It resolves the caller's tenant from the request
// context and passes it into the tenant-scoped repository.
type Service struct {
	repo   *ClientsRepo
	hub    *realtime.Hub
	events events.Notifier
}

// NewService constructs the service. A nil hub is a programmer error.
func NewService(db db.Executor, hub *realtime.Hub) *Service {
	if hub == nil {
		panic("client.NewService: nil hub")
	}
	return &Service{repo: NewClients(db), hub: hub, events: events.New(hub, "client")}
}

// List returns the tenant's clients, optionally filtered by search.
func (s *Service) List(ctx context.Context, search string) ([]*Client, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.List(ctx, tenantID, search)
}

// Query returns a page of clients for the given listquery clause.
func (s *Service) Query(ctx context.Context, c listquery.Clause) (QueryResult, error) {
	tenantID := reqctx.MustTenant(ctx)
	rows, total, err := s.repo.Query(ctx, tenantID, c)
	if err != nil {
		return QueryResult{}, err
	}
	if rows == nil {
		rows = []*Client{}
	}
	return QueryResult{Rows: rows, Total: total}, nil
}

// Get returns a single client by uuid, or apperr.ErrNotFound when not found.
func (s *Service) Get(ctx context.Context, uuid string) (*Client, error) {
	tenantID := reqctx.MustTenant(ctx)
	c, err := s.repo.Get(ctx, tenantID, uuid)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, apperr.ErrNotFound
	}
	return c, nil
}

// Create inserts a client, then broadcasts AFTER the commit succeeds.
func (s *Service) Create(ctx context.Context, in ClientInput) (*Client, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	tenantID := reqctx.MustTenant(ctx)
	c, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	s.events.Created(tenantID, c.ID)
	return c, nil
}

// Update mutates a client by uuid, then broadcasts on success. A missing row
// surfaces as apperr.ErrNotFound from the repo and is propagated (no event). The
// SSE event carries the row's uuid.
func (s *Service) Update(ctx context.Context, uuid string, in ClientInput) (*Client, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	tenantID := reqctx.MustTenant(ctx)
	c, err := s.repo.Update(ctx, tenantID, uuid, in)
	if err != nil {
		return nil, err
	}
	s.events.Updated(tenantID, c.ID)
	return c, nil
}

// Delete removes a client by uuid, then broadcasts on success. A missing row
// surfaces as apperr.ErrNotFound from the repo and is propagated.
func (s *Service) Delete(ctx context.Context, uuid string) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Delete(ctx, tenantID, uuid); err != nil {
		return err
	}
	s.events.Deleted(tenantID, uuid)
	return nil
}

// ResolveClientIDs resolves a list of client uuids to their row ids
// (uuid) for the tenant (preserving order). An unknown uuid surfaces as an error so
// the caller can 400 — bulk operations must not silently drop a member.
func (s *Service) ResolveClientIDs(ctx context.Context, clientUUIDs []string) ([]string, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ResolveClientIDs(ctx, tenantID, clientUUIDs)
}

// BulkDelete removes multiple clients, then broadcasts a single bulk_delete
// event on success.
func (s *Service) BulkDelete(ctx context.Context, ids []string) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.BulkDelete(ctx, tenantID, ids); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "client", UUID: "", Action: "bulk_delete"})
	return nil
}
