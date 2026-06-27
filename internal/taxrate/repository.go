// Package taxrate is the tax-rate vertical slice: domain types, the audited
// repository over the tax_rates table, the service (with SSE broadcast), and the
// HTTP handler. It depends only on platform packages (db/gen, audit, reqctx,
// httpx), never on other domain slices.
package taxrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/dknathalage/tallyo/internal/db"
	"time"

	"github.com/dknathalage/tallyo/internal/apperr"
	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
	"github.com/dknathalage/tallyo/internal/listquery"
)

// taxRateListSelect is the base SELECT for paged tax-rate queries; listquery
// appends a safe WHERE/ORDER/LIMIT tail after the mandatory tenant filter.
const taxRateListSelect = `SELECT * FROM tax_rates WHERE tenant_id = ?`

// TaxRateCols is the listquery allowlist for tax rates. Keys match the JSON
// field names so the frontend column key drives filter, sort, and display with
// one identifier. "isDefault" is sort-only (None).
var TaxRateCols = listquery.Spec{
	"name":      {Col: "name", Filter: listquery.Text},
	"rate":      {Col: "rate", Filter: listquery.Number},
	"isDefault": {Col: "is_default", Filter: listquery.None},
}

// TaxRate is the domain view of a row in the tax_rates table.
type TaxRate struct {
	ID        string  `json:"id"`
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

// Validate checks the cheap required-field rules the service enforces before the
// repository runs. A failure is returned as an *apperr.ValidationError so the
// HTTP layer responds 422 with per-field detail. (taxrate cannot import billing
// — billing's tests import taxrate — so it uses the equivalent apperr type.)
func (in TaxRateInput) Validate() error {
	ve := &apperr.ValidationError{}
	if in.Name == "" {
		ve.Errors = append(ve.Errors, apperr.FieldError{Line: 0, Field: "name", Message: "required"})
	}
	if len(ve.Errors) > 0 {
		return ve
	}
	return nil
}

// TaxRatesRepo reads and writes the tax_rates table with audited mutations and
// exclusive-default semantics: at most one row per tenant may have is_default=1.
type TaxRatesRepo struct {
	db db.Executor
}

// NewTaxRates constructs a repository. A nil db is a programmer error.
func NewTaxRates(db db.Executor) *TaxRatesRepo {
	if db == nil {
		panic("taxrate: NewTaxRates requires a non-nil *sql.DB")
	}
	return &TaxRatesRepo{db: db}
}

// List returns the tenant's tax rates ordered by is_default desc then name.
func (r *TaxRatesRepo) List(ctx context.Context, tenantID string) ([]*TaxRate, error) {
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

// Query returns one page of tax rates plus the total row count for the filter
// (ignoring pagination). The clause is built by listquery from an allowlisted
// spec, so its Where/Order fragments are injection-safe.
func (r *TaxRatesRepo) Query(ctx context.Context, tenantID string, c listquery.Clause) ([]*TaxRate, int64, error) {
	if tenantID == "" {
		return nil, 0, errors.New("query tax rates: tenant id required")
	}
	var total int64
	countSQL := db.Rebind("SELECT count(*) FROM (" + taxRateListSelect + c.Where + ") AS sub")
	countArgs := append([]any{tenantID}, c.CountArgs()...)
	if err := r.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count tax rates: %w", err)
	}
	order := c.Order
	if order == "" {
		order = " ORDER BY is_default DESC, name"
	}
	sqlText := db.Rebind(taxRateListSelect + c.Where + order + c.Limit)
	pageArgs := append([]any{tenantID}, c.Args...)
	rows, err := r.db.QueryContext(ctx, sqlText, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query tax rates: %w", err)
	}
	defer rows.Close()
	out := make([]*TaxRate, 0, 50)
	for rows.Next() { // bounded by LIMIT in the query
		var t TaxRate
		var tenant string
		var isDefault int64
		if err := rows.Scan(&t.ID, &tenant, &t.Name, &t.Rate,
			&isDefault, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan tax rate: %w", err)
		}
		t.IsDefault = isDefault == 1
		out = append(out, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("query tax rates: %w", err)
	}
	return out, total, nil
}

// Get returns the tenant's tax rate by uuid, or (nil, nil) when none matches.
func (r *TaxRatesRepo) Get(ctx context.Context, tenantID string, uuid string) (*TaxRate, error) {
	row, err := gen.New(r.db).GetTaxRate(ctx, gen.GetTaxRateParams{TenantID: tenantID, ID: uuid})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tax rate: %w", err)
	}
	return toTaxRate(row), nil
}

// GetDefault returns the tenant's default tax rate, or (nil, nil) when unset.
func (r *TaxRatesRepo) GetDefault(ctx context.Context, tenantID string) (*TaxRate, error) {
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
func (r *TaxRatesRepo) Create(ctx context.Context, tenantID string, in TaxRateInput) (*TaxRate, error) {
	if tenantID == "" {
		return nil, errors.New("create tax rate: tenant id required")
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
			ID:        ids.New(),
			TenantID:  tenantID,
			Name:      in.Name,
			Rate:      in.Rate,
			IsDefault: db.B2i(in.IsDefault),
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
// first. Returns apperr.ErrNotFound when the row does not exist so the caller
// can 404.
func (r *TaxRatesRepo) Update(ctx context.Context, tenantID string, uuid string, in TaxRateInput) (*TaxRate, error) {
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
			IsDefault: db.B2i(in.IsDefault),
			UpdatedAt: now,
			TenantID:  tenantID,
			ID:        uuid,
		})
		if errors.Is(e, sql.ErrNoRows) {
			return apperr.ErrNotFound
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
	if errors.Is(err, apperr.ErrNotFound) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update tax rate: %w", err)
	}
	return toTaxRate(updated), nil
}

// Delete removes a tax rate by uuid and writes one audit row, atomically. The
// audit entry records the row's id, looked up by-uuid in the same tx.
func (r *TaxRatesRepo) Delete(ctx context.Context, tenantID string, uuid string) error {
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		row, e := q.GetTaxRate(ctx, gen.GetTaxRateParams{TenantID: tenantID, ID: uuid})
		if errors.Is(e, sql.ErrNoRows) {
			return apperr.ErrNotFound
		}
		if e != nil {
			return fmt.Errorf("lookup: %w", e)
		}
		if e := q.DeleteTaxRate(ctx, gen.DeleteTaxRateParams{TenantID: tenantID, ID: uuid}); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "tax_rate",
			EntityID:   row.ID,
			Action:     "delete",
		})
	})
}

// toTaxRate maps a generated row to the domain TaxRate.
func toTaxRate(row gen.TaxRate) *TaxRate {
	return &TaxRate{
		ID:        row.ID,
		Name:      row.Name,
		Rate:      row.Rate,
		IsDefault: row.IsDefault == 1,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
