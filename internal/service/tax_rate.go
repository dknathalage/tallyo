package service

import (
	"context"
	"database/sql"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// TaxRateService orchestrates tax-rate reads/writes and publishes change
// events after a successful commit.
type TaxRateService struct {
	repo *repository.TaxRatesRepo
	hub  *realtime.Hub
}

func NewTaxRateService(db *sql.DB, hub *realtime.Hub) *TaxRateService {
	if hub == nil {
		panic("NewTaxRateService: nil hub")
	}
	return &TaxRateService{repo: repository.NewTaxRates(db), hub: hub}
}

func (s *TaxRateService) List(ctx context.Context) ([]*repository.TaxRate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.List(ctx, tenantID)
}

func (s *TaxRateService) Get(ctx context.Context, id int64) (*repository.TaxRate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, id)
}

func (s *TaxRateService) GetDefault(ctx context.Context) (*repository.TaxRate, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.GetDefault(ctx, tenantID)
}

// Create inserts a tax rate, then broadcasts AFTER the commit succeeds.
func (s *TaxRateService) Create(ctx context.Context, in repository.TaxRateInput) (*repository.TaxRate, error) {
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
func (s *TaxRateService) Update(ctx context.Context, id int64, in repository.TaxRateInput) (*repository.TaxRate, error) {
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
func (s *TaxRateService) Delete(ctx context.Context, id int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "tax_rate", ID: id, Action: "delete"})
	return nil
}
