package taxrate

import (
	"context"
	"github.com/dknathalage/tallyo/internal/db"

	"github.com/dknathalage/tallyo/internal/apperr"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// Service orchestrates tax-rate reads/writes.
type Service struct {
	repo *TaxRatesRepo
}

// NewService constructs the tax-rate service.
func NewService(db db.Executor) *Service {
	return &Service{repo: NewTaxRates(db)}
}

func (s *Service) List(ctx context.Context) ([]*TaxRate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.List(ctx, tenantID)
}

// Query returns a page of tax rates for the given listquery clause. Rows is
// never null in JSON.
func (s *Service) Query(ctx context.Context, c listquery.Clause) (listquery.Result[*TaxRate], error) {
	tenantID := reqctx.MustTenant(ctx)
	rows, total, err := s.repo.Query(ctx, tenantID, c)
	if err != nil {
		return listquery.Result[*TaxRate]{}, err
	}
	if rows == nil {
		rows = []*TaxRate{}
	}
	return listquery.Result[*TaxRate]{Rows: rows, Total: total}, nil
}

func (s *Service) Get(ctx context.Context, uuid string) (*TaxRate, error) {
	tenantID := reqctx.MustTenant(ctx)
	t, err := s.repo.Get(ctx, tenantID, uuid)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, apperr.ErrNotFound
	}
	return t, nil
}

func (s *Service) GetDefault(ctx context.Context) (*TaxRate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.GetDefault(ctx, tenantID)
}

// Create inserts a tax rate.
func (s *Service) Create(ctx context.Context, in TaxRateInput) (*TaxRate, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	tenantID := reqctx.MustTenant(ctx)
	t, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// Update mutates a tax rate. A missing row surfaces as apperr.ErrNotFound from
// the repo and is propagated.
func (s *Service) Update(ctx context.Context, uuid string, in TaxRateInput) (*TaxRate, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	tenantID := reqctx.MustTenant(ctx)
	t, err := s.repo.Update(ctx, tenantID, uuid, in)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// Delete removes a tax rate by uuid. A missing row surfaces as apperr.ErrNotFound
// from the repo and is propagated.
func (s *Service) Delete(ctx context.Context, uuid string) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Delete(ctx, tenantID, uuid); err != nil {
		return err
	}
	return nil
}
