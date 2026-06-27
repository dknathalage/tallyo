package estimate

import (
	"context"
	"github.com/dknathalage/tallyo/internal/db"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// Service orchestrates estimate reads/writes. Unlike invoices it has no overdue
// sweep, but it adds a Convert action that turns an accepted estimate into an
// invoice.
type Service struct {
	repo      *EstimatesRepo
	validator *billing.LineValidator
}

// NewService constructs the estimate service.
func NewService(db db.Executor) *Service {
	return &Service{repo: NewEstimates(db), validator: billing.NewLineValidator(db)}
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

func (s *Service) ListClientEstimates(ctx context.Context, clientID string) ([]*Estimate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ListClientEstimates(ctx, tenantID, clientID)
}

func (s *Service) Get(ctx context.Context, id string) (*Estimate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, id)
}

// GetByUUID returns an estimate by uuid, or (nil, nil) when absent. Public HTTP read.
func (s *Service) GetByUUID(ctx context.Context, estimateUUID string) (*Estimate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.GetByUUID(ctx, tenantID, estimateUUID)
}

// ResolveClient resolves a client uuid to its row id (uuid) for the
// tenant. Returns ("", nil) when no client matches (caller 400s).
func (s *Service) ResolveClient(ctx context.Context, clientUUID string) (string, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ResolveClientID(ctx, tenantID, clientUUID)
}

// ResolvePayer resolves a payer uuid to its row id (uuid) for the
// tenant. Returns ("", nil) when no payer matches (caller 400s).
func (s *Service) ResolvePayer(ctx context.Context, payerUUID string) (string, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ResolvePayerID(ctx, tenantID, payerUUID)
}

// ResolveEstimateIDs resolves a list of estimate uuids to their row ids (uuid) for
// the tenant (preserving order). An unknown uuid surfaces as an error so the
// caller can 400 — bulk operations must not silently drop a member.
func (s *Service) ResolveEstimateIDs(ctx context.Context, estimateUUIDs []string) ([]string, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ResolveEstimateIDs(ctx, tenantID, estimateUUIDs)
}

// UpdateByUUID resolves the estimate uuid → row, then rewrites the estimate.
// Returns (nil, nil) when no estimate matches the uuid so the handler can 404.
func (s *Service) UpdateByUUID(ctx context.Context, estimateUUID string, in EstimateInput, items []billing.LineItemInput) (*Estimate, error) {
	tenantID := reqctx.MustTenant(ctx)
	id, err := s.repo.ResolveEstimateID(ctx, tenantID, estimateUUID)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil
	}
	return s.Update(ctx, id, in, items)
}

// DeleteByUUID resolves the estimate uuid → row, then deletes the estimate.
// A no-match uuid is a no-op (Delete is idempotent).
func (s *Service) DeleteByUUID(ctx context.Context, estimateUUID string) error {
	tenantID := reqctx.MustTenant(ctx)
	id, err := s.repo.ResolveEstimateID(ctx, tenantID, estimateUUID)
	if err != nil {
		return err
	}
	if id == "" {
		return nil
	}
	return s.Delete(ctx, id)
}

// UpdateStatusByUUID resolves the estimate uuid → row, then flips its status.
// A no-match uuid is a no-op.
func (s *Service) UpdateStatusByUUID(ctx context.Context, estimateUUID, status string) error {
	tenantID := reqctx.MustTenant(ctx)
	id, err := s.repo.ResolveEstimateID(ctx, tenantID, estimateUUID)
	if err != nil {
		return err
	}
	if id == "" {
		return nil
	}
	return s.UpdateStatus(ctx, id, status)
}

// DuplicateByUUID resolves the estimate uuid → row, then duplicates it.
// Returns (nil, nil) when no estimate matches the uuid so the handler can 404.
func (s *Service) DuplicateByUUID(ctx context.Context, estimateUUID string) (*Estimate, error) {
	tenantID := reqctx.MustTenant(ctx)
	id, err := s.repo.ResolveEstimateID(ctx, tenantID, estimateUUID)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil
	}
	return s.Duplicate(ctx, id)
}

// ConvertByUUID resolves the estimate uuid → row, then converts it to an
// invoice. Returns (nil, nil) when no estimate matches the uuid so the handler
// can 404; ErrNotAccepted/ErrAlreadyConverted are propagated unchanged.
func (s *Service) ConvertByUUID(ctx context.Context, estimateUUID string) (*ConvertResult, error) {
	tenantID := reqctx.MustTenant(ctx)
	id, err := s.repo.ResolveEstimateID(ctx, tenantID, estimateUUID)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil
	}
	return s.Convert(ctx, id)
}

// Create inserts an estimate + line items.
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
	return est, nil
}

// Update rewrites an estimate. A nil result means the row was not found.
func (s *Service) Update(ctx context.Context, id string, in EstimateInput, items []billing.LineItemInput) (*Estimate, error) {
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
	return est, nil
}

// UpdateStatus sets the estimate status.
func (s *Service) UpdateStatus(ctx context.Context, id string, status string) error {
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
	return nil
}

// Delete removes an estimate.
func (s *Service) Delete(ctx context.Context, id string) error {
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
	return nil
}

// Duplicate copies an estimate.
func (s *Service) Duplicate(ctx context.Context, id string) (*Estimate, error) {
	tenantID := reqctx.MustTenant(ctx)
	est, err := s.repo.Duplicate(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	return est, nil
}

// BulkDelete removes several estimates.
func (s *Service) BulkDelete(ctx context.Context, ids []string) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.BulkDelete(ctx, tenantID, ids); err != nil {
		return err
	}
	return nil
}

// BulkUpdateStatus sets several estimates' status.
func (s *Service) BulkUpdateStatus(ctx context.Context, ids []string, status string) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.BulkUpdateStatus(ctx, tenantID, ids, status); err != nil {
		return err
	}
	return nil
}

// Convert turns an accepted estimate into an invoice. ErrNotAccepted/
// ErrAlreadyConverted are propagated unchanged.
func (s *Service) Convert(ctx context.Context, id string) (*ConvertResult, error) {
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
	return res, nil
}
