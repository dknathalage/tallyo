package client

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dknathalage/tallyo/internal/db"

	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// errInvalidType is the sentinel returned when an inbound client type is not one
// of the allowed enum values. Handlers map it to a 400 (mirroring
// errPayerNotFound), so a bad type never reaches the DB CHECK as a 500.
var errInvalidType = errors.New("invalid client type")

// FieldError is one structured, field-level validation failure on a client
// payload (e.g. a required NDIS field missing). The HTTP layer surfaces these
// inline; the slice keeps its own type to avoid importing a billing dependency.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError aggregates field-level client validation failures. Returning
// it (rather than a bare error) lets the handler respond 400 with per-field
// detail while still being matchable via errors.As.
type ValidationError struct {
	Errors []FieldError `json:"errors"`
}

// Error renders the aggregated failures as a single string (the error
// interface). The structured slice in Errors is what callers should surface.
func (e *ValidationError) Error() string {
	if e == nil || len(e.Errors) == 0 {
		return "client validation failed"
	}
	parts := make([]string, 0, len(e.Errors))
	for i := range e.Errors { // bounded by len(e.Errors)
		parts = append(parts, e.Errors[i].Field+": "+e.Errors[i].Message)
	}
	return "client validation failed: " + strings.Join(parts, "; ")
}

// validateClientInput enforces type-driven field gating (Phase 6): the type must
// be one of the allowed enum values (blank defaults to standard, handled at the
// repository), and an ndis client must carry plan dates + a management type.
// Non-ndis clients leave those fields optional. Returns errInvalidType for a bad
// enum value, or a *ValidationError listing every missing NDIS field.
func validateClientInput(in ClientInput) error {
	t := strings.TrimSpace(in.Type)
	if t == "" {
		t = "standard"
	}
	if t != "standard" && t != "ndis" {
		return fmt.Errorf("%w: %q", errInvalidType, in.Type)
	}
	if t != "ndis" {
		return nil
	}
	var ve ValidationError
	if strings.TrimSpace(in.PlanStart) == "" {
		ve.Errors = append(ve.Errors, FieldError{Field: "planStart", Message: "plan start is required for an NDIS client"})
	}
	if strings.TrimSpace(in.PlanEnd) == "" {
		ve.Errors = append(ve.Errors, FieldError{Field: "planEnd", Message: "plan end is required for an NDIS client"})
	}
	if strings.TrimSpace(in.MgmtType) == "" {
		ve.Errors = append(ve.Errors, FieldError{Field: "mgmtType", Message: "management type is required for an NDIS client"})
	}
	if len(ve.Errors) > 0 {
		return &ve
	}
	return nil
}

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
	repo *ClientsRepo
	hub  *realtime.Hub
}

// NewService constructs the service. A nil hub is a programmer error.
func NewService(db db.Executor, hub *realtime.Hub) *Service {
	if hub == nil {
		panic("client.NewService: nil hub")
	}
	return &Service{repo: NewClients(db), hub: hub}
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

// Get returns a single client by uuid, or (nil, nil) when not found.
func (s *Service) Get(ctx context.Context, uuid string) (*Client, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, uuid)
}

// Create inserts a client, then broadcasts AFTER the commit succeeds.
func (s *Service) Create(ctx context.Context, in ClientInput) (*Client, error) {
	tenantID := reqctx.MustTenant(ctx)
	if err := validateClientInput(in); err != nil {
		return nil, err
	}
	c, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "client", UUID: c.UUID, Action: "create"})
	return c, nil
}

// Update mutates a client by uuid, then broadcasts on success. A nil result
// means the row was not found, in which case no event is published. The SSE
// event carries the row's uuid.
func (s *Service) Update(ctx context.Context, uuid string, in ClientInput) (*Client, error) {
	tenantID := reqctx.MustTenant(ctx)
	if err := validateClientInput(in); err != nil {
		return nil, err
	}
	c, err := s.repo.Update(ctx, tenantID, uuid, in)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "client", UUID: c.UUID, Action: "update"})
	return c, nil
}

// Delete removes a client by uuid, then broadcasts on success. The row is
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
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "client", UUID: p.UUID, Action: "delete"})
	return nil
}

// ResolveClientIDs translates a list of client uuids into their int
// PKs for the tenant (preserving order). An unknown uuid surfaces as an error so
// the caller can 400 — bulk operations must not silently drop a member.
func (s *Service) ResolveClientIDs(ctx context.Context, clientUUIDs []string) ([]int64, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ResolveClientIDs(ctx, tenantID, clientUUIDs)
}

// BulkDelete removes multiple clients, then broadcasts a single bulk_delete
// event on success.
func (s *Service) BulkDelete(ctx context.Context, ids []int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.BulkDelete(ctx, tenantID, ids); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "client", UUID: "", Action: "bulk_delete"})
	return nil
}
