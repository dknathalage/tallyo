package estimate

import (
	"context"
	"github.com/dknathalage/tallyo/internal/db"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// Service orchestrates estimate reads/writes and publishes change events
// after a successful commit. Unlike invoices it has no overdue sweep, but it adds
// a Convert action that turns an accepted estimate into an invoice.
type Service struct {
	repo      *EstimatesRepo
	validator *billing.LineValidator
	hub       *realtime.Hub
}

// NewService constructs the estimate service. A nil hub is a programmer error.
func NewService(db, control db.Executor, hub *realtime.Hub) *Service {
	if hub == nil {
		panic("estimate.NewService: nil hub")
	}
	return &Service{repo: NewEstimates(db), validator: billing.NewLineValidator(db, control), hub: hub}
}

func (s *Service) List(ctx context.Context) ([]*Estimate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.List(ctx, tenantID)
}

// Query returns a page of estimates for the given listquery clause. Rows is
// never nil so it serializes as [] not null.
func (s *Service) Query(ctx context.Context, c listquery.Clause) (listquery.Result[*Estimate], error) {
	tenantID := reqctx.MustTenant(ctx)
	rows, total, err := s.repo.Query(ctx, tenantID, c)
	if err != nil {
		return listquery.Result[*Estimate]{}, err
	}
	if rows == nil {
		rows = []*Estimate{}
	}
	return listquery.Result[*Estimate]{Rows: rows, Total: total}, nil
}

func (s *Service) ListByStatus(ctx context.Context, status string) ([]*Estimate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ListByStatus(ctx, tenantID, status)
}

func (s *Service) ListClientEstimates(ctx context.Context, clientID int64) ([]*Estimate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ListClientEstimates(ctx, tenantID, clientID)
}

func (s *Service) Get(ctx context.Context, id int64) (*Estimate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, id)
}

// GetByUUID returns an estimate by uuid, or (nil, nil) when absent. Public HTTP read.
func (s *Service) GetByUUID(ctx context.Context, estimateUUID string) (*Estimate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.GetByUUID(ctx, tenantID, estimateUUID)
}

// ResolveClient translates a client uuid into its int FK for the
// tenant. Returns (0, nil) when no client matches (caller 400s).
func (s *Service) ResolveClient(ctx context.Context, clientUUID string) (int64, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ResolveClientID(ctx, tenantID, clientUUID)
}

// ResolvePayer translates a payer uuid into its int FK for the
// tenant. Returns (0, nil) when no payer matches (caller 400s).
func (s *Service) ResolvePayer(ctx context.Context, payerUUID string) (int64, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ResolvePayerID(ctx, tenantID, payerUUID)
}

// ResolveEstimateIDs translates a list of estimate uuids into their int PKs for
// the tenant (preserving order). An unknown uuid surfaces as an error so the
// caller can 400 — bulk operations must not silently drop a member.
func (s *Service) ResolveEstimateIDs(ctx context.Context, estimateUUIDs []string) ([]int64, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ResolveEstimateIDs(ctx, tenantID, estimateUUIDs)
}

// UpdateByUUID resolves the estimate uuid → int PK, then rewrites the estimate.
// Returns (nil, nil) when no estimate matches the uuid so the handler can 404.
func (s *Service) UpdateByUUID(ctx context.Context, estimateUUID string, in EstimateInput, items []billing.LineItemInput) (*Estimate, error) {
	tenantID := reqctx.MustTenant(ctx)
	id, err := s.repo.ResolveEstimateID(ctx, tenantID, estimateUUID)
	if err != nil {
		return nil, err
	}
	if id == 0 {
		return nil, nil
	}
	return s.Update(ctx, id, in, items)
}

// DeleteByUUID resolves the estimate uuid → int PK, then deletes the estimate.
// A no-match uuid is a no-op (the int Delete is idempotent).
func (s *Service) DeleteByUUID(ctx context.Context, estimateUUID string) error {
	tenantID := reqctx.MustTenant(ctx)
	id, err := s.repo.ResolveEstimateID(ctx, tenantID, estimateUUID)
	if err != nil {
		return err
	}
	if id == 0 {
		return nil
	}
	return s.Delete(ctx, id)
}

// UpdateStatusByUUID resolves the estimate uuid → int PK, then flips its status.
// A no-match uuid is a no-op.
func (s *Service) UpdateStatusByUUID(ctx context.Context, estimateUUID, status string) error {
	tenantID := reqctx.MustTenant(ctx)
	id, err := s.repo.ResolveEstimateID(ctx, tenantID, estimateUUID)
	if err != nil {
		return err
	}
	if id == 0 {
		return nil
	}
	return s.UpdateStatus(ctx, id, status)
}

// DuplicateByUUID resolves the estimate uuid → int PK, then duplicates it.
// Returns (nil, nil) when no estimate matches the uuid so the handler can 404.
func (s *Service) DuplicateByUUID(ctx context.Context, estimateUUID string) (*Estimate, error) {
	tenantID := reqctx.MustTenant(ctx)
	id, err := s.repo.ResolveEstimateID(ctx, tenantID, estimateUUID)
	if err != nil {
		return nil, err
	}
	if id == 0 {
		return nil, nil
	}
	return s.Duplicate(ctx, id)
}

// ConvertByUUID resolves the estimate uuid → int PK, then converts it to an
// invoice. Returns (nil, nil) when no estimate matches the uuid so the handler
// can 404; ErrNotAccepted/ErrAlreadyConverted are propagated unchanged.
func (s *Service) ConvertByUUID(ctx context.Context, estimateUUID string) (*ConvertResult, error) {
	tenantID := reqctx.MustTenant(ctx)
	id, err := s.repo.ResolveEstimateID(ctx, tenantID, estimateUUID)
	if err != nil {
		return nil, err
	}
	if id == 0 {
		return nil, nil
	}
	return s.Convert(ctx, id)
}

// Create inserts an estimate + line items, then broadcasts on success.
func (s *Service) Create(ctx context.Context, in EstimateInput, items []billing.LineItemInput) (*Estimate, error) {
	tenantID := reqctx.MustTenant(ctx)
	res, err := s.validator.Validate(ctx, tenantID, in.ClientID, items)
	if err != nil {
		return nil, err
	}
	in.Tax = res.Tax
	est, err := s.repo.Create(ctx, tenantID, in, res.Items)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "estimate", UUID: est.UUID, Action: "create"})
	return est, nil
}

// Update rewrites an estimate. A nil result means the row was not found, in which
// case no event is published.
func (s *Service) Update(ctx context.Context, id int64, in EstimateInput, items []billing.LineItemInput) (*Estimate, error) {
	tenantID := reqctx.MustTenant(ctx)
	res, err := s.validator.Validate(ctx, tenantID, in.ClientID, items)
	if err != nil {
		return nil, err
	}
	in.Tax = res.Tax
	est, err := s.repo.Update(ctx, tenantID, id, in, res.Items)
	if err != nil {
		return nil, err
	}
	if est == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "estimate", UUID: est.UUID, Action: "update"})
	return est, nil
}

// UpdateStatus sets the estimate status, then broadcasts on success. The row's
// uuid is resolved first so the post-commit event carries the public id.
func (s *Service) UpdateStatus(ctx context.Context, id int64, status string) error {
	tenantID := reqctx.MustTenant(ctx)
	est, err := s.repo.Get(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if est == nil {
		return nil
	}
	if err := s.repo.UpdateStatus(ctx, tenantID, id, status); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "estimate", UUID: est.UUID, Action: "status"})
	return nil
}

// Delete removes an estimate, then broadcasts on success. The row's uuid is
// resolved first so the post-commit event carries the public id.
func (s *Service) Delete(ctx context.Context, id int64) error {
	tenantID := reqctx.MustTenant(ctx)
	est, err := s.repo.Get(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if est == nil {
		return nil
	}
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "estimate", UUID: est.UUID, Action: "delete"})
	return nil
}

// Duplicate copies an estimate, then broadcasts a create for the new id.
func (s *Service) Duplicate(ctx context.Context, id int64) (*Estimate, error) {
	tenantID := reqctx.MustTenant(ctx)
	est, err := s.repo.Duplicate(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "estimate", UUID: est.UUID, Action: "create"})
	return est, nil
}

// BulkDelete removes several estimates, then broadcasts a single bulk event.
func (s *Service) BulkDelete(ctx context.Context, ids []int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.BulkDelete(ctx, tenantID, ids); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "estimate", UUID: "", Action: "bulk_delete"})
	return nil
}

// BulkUpdateStatus sets several estimates' status, then broadcasts a bulk event.
func (s *Service) BulkUpdateStatus(ctx context.Context, ids []int64, status string) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.BulkUpdateStatus(ctx, tenantID, ids, status); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "estimate", UUID: "", Action: "bulk_status"})
	return nil
}

// Convert turns an accepted estimate into an invoice. On success it broadcasts an
// estimate "convert" event and an invoice "create" event for the new invoice, then
// returns the result. ErrNotAccepted/ErrAlreadyConverted are propagated unchanged.
func (s *Service) Convert(ctx context.Context, id int64) (*ConvertResult, error) {
	tenantID := reqctx.MustTenant(ctx)
	est, err := s.repo.Get(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if est == nil {
		return nil, nil
	}
	res, err := s.repo.Convert(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "estimate", UUID: est.UUID, Action: "convert"})
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", UUID: res.InvoiceUUID, Action: "create"})
	return res, nil
}
