package taxrate

import (
	"context"
	"database/sql"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// Service orchestrates tax-rate reads/writes and publishes change events after
// a successful commit.
type Service struct {
	repo *TaxRatesRepo
	hub  *realtime.Hub
}

// NewService constructs the tax-rate service. A nil hub is a programmer error.
func NewService(db *sql.DB, hub *realtime.Hub) *Service {
	if hub == nil {
		panic("taxrate.NewService: nil hub")
	}
	return &Service{repo: NewTaxRates(db), hub: hub}
}

func (s *Service) List(ctx context.Context) ([]*TaxRate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.List(ctx, tenantID)
}

func (s *Service) Get(ctx context.Context, id int64) (*TaxRate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, id)
}

func (s *Service) GetDefault(ctx context.Context) (*TaxRate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.GetDefault(ctx, tenantID)
}

// Create inserts a tax rate, then broadcasts AFTER the commit succeeds.
func (s *Service) Create(ctx context.Context, in TaxRateInput) (*TaxRate, error) {
	tenantID := reqctx.MustTenant(ctx)
	t, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "tax_rate", ID: t.ID, Action: "create"})
	return t, nil
}

// Update mutates a tax rate, then broadcasts on success. A nil result means the
// row was not found, in which case no event is published.
func (s *Service) Update(ctx context.Context, id int64, in TaxRateInput) (*TaxRate, error) {
	tenantID := reqctx.MustTenant(ctx)
	t, err := s.repo.Update(ctx, tenantID, id, in)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "tax_rate", ID: id, Action: "update"})
	return t, nil
}

// Delete removes a tax rate, then broadcasts on success.
func (s *Service) Delete(ctx context.Context, id int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "tax_rate", ID: id, Action: "delete"})
	return nil
}
