package payer

import (
	"context"
	"github.com/dknathalage/tallyo/internal/db"

	"github.com/dknathalage/tallyo/internal/apperr"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// Service orchestrates payer reads/writes.
type Service struct {
	repo *PayersRepo
}

// NewService constructs the payer service.
func NewService(db db.Executor) *Service {
	return &Service{repo: NewPayers(db)}
}

func (s *Service) List(ctx context.Context, search string) ([]*Payer, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.List(ctx, tenantID, search)
}

// Query returns a page of payers for the given listquery clause. Rows is
// never null so it serializes as [] not null.
func (s *Service) Query(ctx context.Context, c listquery.Clause) (listquery.Result[*Payer], error) {
	tenantID := reqctx.MustTenant(ctx)
	rows, total, err := s.repo.Query(ctx, tenantID, c)
	if err != nil {
		return listquery.Result[*Payer]{}, err
	}
	if rows == nil {
		rows = []*Payer{}
	}
	return listquery.Result[*Payer]{Rows: rows, Total: total}, nil
}

func (s *Service) Get(ctx context.Context, uuid string) (*Payer, error) {
	tenantID := reqctx.MustTenant(ctx)
	p, err := s.repo.Get(ctx, tenantID, uuid)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, apperr.ErrNotFound
	}
	return p, nil
}

// Create inserts a payer.
func (s *Service) Create(ctx context.Context, in PayerInput) (*Payer, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	tenantID := reqctx.MustTenant(ctx)
	p, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Update mutates a payer. A missing row surfaces as apperr.ErrNotFound from the
// repo and is propagated.
func (s *Service) Update(ctx context.Context, uuid string, in PayerInput) (*Payer, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	tenantID := reqctx.MustTenant(ctx)
	p, err := s.repo.Update(ctx, tenantID, uuid, in)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Delete removes a payer by uuid. A missing row surfaces as apperr.ErrNotFound
// from the repo and is propagated.
func (s *Service) Delete(ctx context.Context, uuid string) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Delete(ctx, tenantID, uuid); err != nil {
		return err
	}
	return nil
}

// ResolvePayerIDs resolves a list of payer uuids to their row ids
// (uuid) for the tenant (preserving order). An unknown uuid surfaces as an error so
// the caller can 400 — bulk operations must not silently drop a member.
func (s *Service) ResolvePayerIDs(ctx context.Context, pmUUIDs []string) ([]string, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ResolvePayerIDs(ctx, tenantID, pmUUIDs)
}

// BulkDelete removes multiple payers.
func (s *Service) BulkDelete(ctx context.Context, ids []string) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.BulkDelete(ctx, tenantID, ids); err != nil {
		return err
	}
	return nil
}
