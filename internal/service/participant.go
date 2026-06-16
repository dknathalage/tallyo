package service

import (
	"context"
	"database/sql"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// ParticipantService orchestrates participant reads/writes and publishes change
// events after a successful commit. It resolves the caller's tenant from the
// request context and passes it into the tenant-scoped repository.
type ParticipantService struct {
	repo *repository.ParticipantsRepo
	hub  *realtime.Hub
}

func NewParticipantService(db *sql.DB, hub *realtime.Hub) *ParticipantService {
	if hub == nil {
		panic("NewParticipantService: nil hub")
	}
	return &ParticipantService{repo: repository.NewParticipants(db), hub: hub}
}

func (s *ParticipantService) List(ctx context.Context, search string) ([]*repository.Participant, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.List(ctx, tenantID, search)
}

func (s *ParticipantService) Get(ctx context.Context, id int64) (*repository.Participant, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, id)
}

// Create inserts a participant, then broadcasts AFTER the commit succeeds.
func (s *ParticipantService) Create(ctx context.Context, in repository.ParticipantInput) (*repository.Participant, error) {
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
func (s *ParticipantService) Update(ctx context.Context, id int64, in repository.ParticipantInput) (*repository.Participant, error) {
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
func (s *ParticipantService) Delete(ctx context.Context, id int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "participant", ID: id, Action: "delete"})
	return nil
}

// BulkDelete removes multiple participants, then broadcasts a single bulk_delete
// event on success.
func (s *ParticipantService) BulkDelete(ctx context.Context, ids []int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.BulkDelete(ctx, tenantID, ids); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "participant", ID: 0, Action: "bulk_delete"})
	return nil
}
