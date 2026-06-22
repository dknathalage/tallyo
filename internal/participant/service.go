package participant

import (
	"context"
	"github.com/dknathalage/tallyo/internal/db"

	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// QueryResult is one page of participants plus the total matching the filter
// (before pagination). rows is never null in JSON.
type QueryResult struct {
	Rows  []*Participant `json:"rows"`
	Total int64          `json:"total"`
}

// Service orchestrates participant reads/writes and publishes change events
// after a successful commit. It resolves the caller's tenant from the request
// context and passes it into the tenant-scoped repository.
type Service struct {
	repo *ParticipantsRepo
	hub  *realtime.Hub
}

// NewService constructs the service. A nil hub is a programmer error.
func NewService(db db.Executor, hub *realtime.Hub) *Service {
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

// Query returns a page of participants for the given listquery clause.
func (s *Service) Query(ctx context.Context, c listquery.Clause) (QueryResult, error) {
	tenantID := reqctx.MustTenant(ctx)
	rows, total, err := s.repo.Query(ctx, tenantID, c)
	if err != nil {
		return QueryResult{}, err
	}
	if rows == nil {
		rows = []*Participant{}
	}
	return QueryResult{Rows: rows, Total: total}, nil
}

// Get returns a single participant by uuid, or (nil, nil) when not found.
func (s *Service) Get(ctx context.Context, uuid string) (*Participant, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, uuid)
}

// Create inserts a participant, then broadcasts AFTER the commit succeeds.
func (s *Service) Create(ctx context.Context, in ParticipantInput) (*Participant, error) {
	tenantID := reqctx.MustTenant(ctx)
	c, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "participant", UUID: c.UUID, Action: "create"})
	return c, nil
}

// Update mutates a participant by uuid, then broadcasts on success. A nil result
// means the row was not found, in which case no event is published. The SSE
// event carries the row's int PK (Phase 2.8 retypes the SSE payload).
func (s *Service) Update(ctx context.Context, uuid string, in ParticipantInput) (*Participant, error) {
	tenantID := reqctx.MustTenant(ctx)
	c, err := s.repo.Update(ctx, tenantID, uuid, in)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "participant", UUID: c.UUID, Action: "update"})
	return c, nil
}

// Delete removes a participant by uuid, then broadcasts on success. The row is
// resolved first so the post-commit event still carries the int PK.
func (s *Service) Delete(ctx context.Context, uuid string) error {
	tenantID := reqctx.MustTenant(ctx)
	p, err := s.repo.Get(ctx, tenantID, uuid)
	if err != nil {
		return err
	}
	if p == nil {
		return nil
	}
	if err := s.repo.Delete(ctx, tenantID, uuid); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "participant", UUID: p.UUID, Action: "delete"})
	return nil
}

// ResolveParticipantIDs translates a list of participant uuids into their int
// PKs for the tenant (preserving order). An unknown uuid surfaces as an error so
// the caller can 400 — bulk operations must not silently drop a member.
func (s *Service) ResolveParticipantIDs(ctx context.Context, participantUUIDs []string) ([]int64, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ResolveParticipantIDs(ctx, tenantID, participantUUIDs)
}

// BulkDelete removes multiple participants, then broadcasts a single bulk_delete
// event on success.
func (s *Service) BulkDelete(ctx context.Context, ids []int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.BulkDelete(ctx, tenantID, ids); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "participant", UUID: "", Action: "bulk_delete"})
	return nil
}
