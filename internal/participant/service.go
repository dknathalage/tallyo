package participant

import (
	"context"
	"database/sql"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// Service orchestrates participant reads/writes and publishes change events
// after a successful commit. It resolves the caller's tenant from the request
// context and passes it into the tenant-scoped repository.
type Service struct {
	repo *ParticipantsRepo
	hub  *realtime.Hub
}

// NewService constructs the service. A nil hub is a programmer error.
func NewService(db *sql.DB, hub *realtime.Hub) *Service {
	if hub == nil {
		panic("participant.NewService: nil hub")
	}
	return &Service{repo: NewParticipants(db), hub: hub}
}

// List returns the tenant's participants, optionally filtered by search.
func (s *Service) List(ctx context.Context, search string) ([]*Participant, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.List(ctx, tenantID, search)
}

// Get returns a single participant by id, or (nil, nil) when not found.
func (s *Service) Get(ctx context.Context, id int64) (*Participant, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, id)
}

// Create inserts a participant, then broadcasts AFTER the commit succeeds.
func (s *Service) Create(ctx context.Context, in ParticipantInput) (*Participant, error) {
	tenantID := reqctx.MustTenant(ctx)
	c, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "participant", ID: c.ID, Action: "create"})
	return c, nil
}

// Update mutates a participant, then broadcasts on success. A nil result means
// the row was not found, in which case no event is published.
func (s *Service) Update(ctx context.Context, id int64, in ParticipantInput) (*Participant, error) {
	tenantID := reqctx.MustTenant(ctx)
	c, err := s.repo.Update(ctx, tenantID, id, in)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "participant", ID: id, Action: "update"})
	return c, nil
}

// Delete removes a participant, then broadcasts on success.
func (s *Service) Delete(ctx context.Context, id int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "participant", ID: id, Action: "delete"})
	return nil
}

// BulkDelete removes multiple participants, then broadcasts a single bulk_delete
// event on success.
func (s *Service) BulkDelete(ctx context.Context, ids []int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.BulkDelete(ctx, tenantID, ids); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "participant", ID: 0, Action: "bulk_delete"})
	return nil
}
