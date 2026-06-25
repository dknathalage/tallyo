package taxrate

import (
	"context"
	"github.com/dknathalage/tallyo/internal/db"

	"github.com/dknathalage/tallyo/internal/apperr"
	"github.com/dknathalage/tallyo/internal/events"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// Service orchestrates tax-rate reads/writes and publishes change events after
// a successful commit.
type Service struct {
	repo   *TaxRatesRepo
	events events.Notifier
}

// NewService constructs the tax-rate service. A nil hub is a programmer error.
func NewService(db db.Executor, hub *realtime.Hub) *Service {
	if hub == nil {
		panic("taxrate.NewService: nil hub")
	}
	return &Service{repo: NewTaxRates(db), events: events.New(hub, "tax_rate")}
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

// Create inserts a tax rate, then broadcasts AFTER the commit succeeds.
func (s *Service) Create(ctx context.Context, in TaxRateInput) (*TaxRate, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	tenantID := reqctx.MustTenant(ctx)
	t, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	s.events.Created(tenantID, t.ID)
	return t, nil
}

// Update mutates a tax rate, then broadcasts on success. A missing row surfaces
// as apperr.ErrNotFound from the repo and is propagated (no event published).
func (s *Service) Update(ctx context.Context, uuid string, in TaxRateInput) (*TaxRate, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	tenantID := reqctx.MustTenant(ctx)
	t, err := s.repo.Update(ctx, tenantID, uuid, in)
	if err != nil {
		return nil, err
	}
	s.events.Updated(tenantID, t.ID)
	return t, nil
}

// Delete removes a tax rate by uuid, then broadcasts on success. A missing row
// surfaces as apperr.ErrNotFound from the repo and is propagated.
func (s *Service) Delete(ctx context.Context, uuid string) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Delete(ctx, tenantID, uuid); err != nil {
		return err
	}
	s.events.Deleted(tenantID, uuid)
	return nil
}
