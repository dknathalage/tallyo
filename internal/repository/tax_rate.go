package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/google/uuid"
)

// TaxRate is the domain view of a row in the tax_rates table.
type TaxRate struct {
	ID        int64   `json:"id"`
	UUID      string  `json:"uuid"`
	Name      string  `json:"name"`
	Rate      float64 `json:"rate"`
	IsDefault bool    `json:"isDefault"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

// TaxRateInput is the writable subset of a tax rate.
type TaxRateInput struct {
	Name      string  `json:"name"`
	Rate      float64 `json:"rate"`
	IsDefault bool    `json:"isDefault"`
}

// errTaxNotFound is an internal sentinel returned from inside a tx so the outer
// code can map a missing row to a (nil, nil) result instead of an error.
var errTaxNotFound = errors.New("not found")

// TaxRatesRepo reads and writes the tax_rates table with audited mutations and
// exclusive-default semantics: at most one row per tenant may have is_default=1.
type TaxRatesRepo struct {
	db *sql.DB
}

// NewTaxRates constructs a repository. A nil db is a programmer error.
func NewTaxRates(db *sql.DB) *TaxRatesRepo {
	if db == nil {
		panic("repository: NewTaxRates requires a non-nil *sql.DB")
	}
	return &TaxRatesRepo{db: db}
}

// List returns the tenant's tax rates ordered by is_default desc then name.
func (r *TaxRatesRepo) List(ctx context.Context, tenantID int64) ([]*TaxRate, error) {
	rows, err := gen.New(r.db).ListTaxRates(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list tax rates: %w", err)
	}
	out := make([]*TaxRate, 0, len(rows))
	for i := range rows {
		out = append(out, toTaxRate(rows[i]))
	}
	return out, nil
}

// Get returns the tenant's tax rate, or (nil, nil) when none matches.
func (r *TaxRatesRepo) Get(ctx context.Context, tenantID, id int64) (*TaxRate, error) {
	row, err := gen.New(r.db).GetTaxRate(ctx, gen.GetTaxRateParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tax rate: %w", err)
	}
	return toTaxRate(row), nil
}

// GetDefault returns the tenant's default tax rate, or (nil, nil) when unset.
func (r *TaxRatesRepo) GetDefault(ctx context.Context, tenantID int64) (*TaxRate, error) {
	row, err := gen.New(r.db).GetDefaultTaxRate(ctx, tenantID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get default tax rate: %w", err)
	}
	return toTaxRate(row), nil
}

// Create inserts a tax rate and writes one audit row, atomically. When
// in.IsDefault is true, the tenant's other rows' is_default are cleared in the
// same tx first, preserving the one-default-per-tenant invariant.
func (r *TaxRatesRepo) Create(ctx context.Context, tenantID int64, in TaxRateInput) (*TaxRate, error) {
	if tenantID == 0 {
		return nil, errors.New("create tax rate: tenant id required")
	}
	if in.Name == "" {
		return nil, errors.New("create tax rate: name is required")
	}

	var created gen.TaxRate
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		if in.IsDefault {
			if e := q.ClearDefaultTaxRates(ctx, tenantID); e != nil {
				return fmt.Errorf("clear defaults: %w", e)
			}
		}
		now := time.Now().UTC().Format(time.RFC3339)
		t, e := q.CreateTaxRate(ctx, gen.CreateTaxRateParams{
			Uuid:      uuid.NewString(),
			TenantID:  tenantID,
			Name:      in.Name,
			Rate:      in.Rate,
			IsDefault: b2i(in.IsDefault),
			CreatedAt: now,
			UpdatedAt: now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		created = t
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "tax_rate",
			EntityID:   t.ID,
			Action:     "create",
			Changes:    audit.Changes(map[string]any{"name": in.Name}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create tax rate: %w", err)
	}
	return toTaxRate(created), nil
}

// Update writes the tax rate's fields and one audit row, atomically. When
// in.IsDefault is true, the tenant's other rows are cleared in the same tx
// first. Returns (nil, nil) when the row does not exist so the caller can 404.
func (r *TaxRatesRepo) Update(ctx context.Context, tenantID, id int64, in TaxRateInput) (*TaxRate, error) {
	if in.Name == "" {
		return nil, errors.New("update tax rate: name is required")
	}

	var updated gen.TaxRate
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		if in.IsDefault {
			if e := q.ClearDefaultTaxRates(ctx, tenantID); e != nil {
				return fmt.Errorf("clear defaults: %w", e)
			}
		}
		now := time.Now().UTC().Format(time.RFC3339)
		t, e := q.UpdateTaxRate(ctx, gen.UpdateTaxRateParams{
			Name:      in.Name,
			Rate:      in.Rate,
			IsDefault: b2i(in.IsDefault),
			UpdatedAt: now,
			TenantID:  tenantID,
			ID:        id,
		})
		if errors.Is(e, sql.ErrNoRows) {
			return errTaxNotFound
		}
		if e != nil {
			return fmt.Errorf("update: %w", e)
		}
		updated = t
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "tax_rate",
			EntityID:   t.ID,
			Action:     "update",
			Changes:    audit.Changes(map[string]any{"name": in.Name}),
		})
	})
	if errors.Is(err, errTaxNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update tax rate: %w", err)
	}
	return toTaxRate(updated), nil
}

// Delete removes a tax rate and writes one audit row, atomically.
func (r *TaxRatesRepo) Delete(ctx context.Context, tenantID, id int64) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "tax_rate",
		EntityID:   id,
		Action:     "delete",
	}, func(tx *sql.Tx) error {
		if e := gen.New(tx).DeleteTaxRate(ctx, gen.DeleteTaxRateParams{TenantID: tenantID, ID: id}); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		return nil
	})
}

// b2i maps a bool to the int64 column convention (true -> 1, false -> 0).
func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// toTaxRate maps a generated row to the domain TaxRate.
func toTaxRate(row gen.TaxRate) *TaxRate {
	return &TaxRate{
		ID:        row.ID,
		UUID:      row.Uuid,
		Name:      row.Name,
		Rate:      row.Rate,
		IsDefault: row.IsDefault == 1,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
