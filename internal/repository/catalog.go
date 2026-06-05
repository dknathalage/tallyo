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

// CatalogItem is the domain view of a row in the catalog_items table. All
// nullable columns are unwrapped to plain strings ("" when absent).
type CatalogItem struct {
	ID        int64   `json:"id"`
	UUID      string  `json:"uuid"`
	Name      string  `json:"name"`
	Rate      float64 `json:"rate"`
	Unit      string  `json:"unit"`
	Category  string  `json:"category"`
	Sku       string  `json:"sku"`
	Metadata  string  `json:"metadata"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

// CatalogItemInput is the writable subset of a catalog item.
type CatalogItemInput struct {
	Name     string  `json:"name"`
	Rate     float64 `json:"rate"`
	Unit     string  `json:"unit"`
	Category string  `json:"category"`
	Sku      string  `json:"sku"`
	Metadata string  `json:"metadata"`
}

// CatalogItemRate is a per-tier rate override for a catalog item.
type CatalogItemRate struct {
	RateTierID int64   `json:"rateTierId"`
	Rate       float64 `json:"rate"`
}

// CatalogRepo reads and writes the catalog_items and catalog_item_rates tables
// with audited mutations.
type CatalogRepo struct {
	db *sql.DB
}

// NewCatalog constructs a repository. A nil db is a programmer error.
func NewCatalog(db *sql.DB) *CatalogRepo {
	if db == nil {
		panic("repository: NewCatalog requires a non-nil *sql.DB")
	}
	return &CatalogRepo{db: db}
}

// List returns all catalog items ordered by name.
func (r *CatalogRepo) List(ctx context.Context) ([]*CatalogItem, error) {
	rows, err := gen.New(r.db).ListCatalogItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("list catalog items: %w", err)
	}
	return mapCatalogItems(rows), nil
}

// Search filters items whose name, sku, or category matches the term
// (case-insensitive LIKE). An empty term matches everything.
func (r *CatalogRepo) Search(ctx context.Context, q string) ([]*CatalogItem, error) {
	like := "%" + q + "%"
	rows, err := gen.New(r.db).SearchCatalogItems(ctx, gen.SearchCatalogItemsParams{
		Name:     like,
		Sku:      nz(like),
		Category: nz(like),
	})
	if err != nil {
		return nil, fmt.Errorf("search catalog items: %w", err)
	}
	return mapCatalogItems(rows), nil
}

// Get returns the item, or (nil, nil) when none matches.
func (r *CatalogRepo) Get(ctx context.Context, id int64) (*CatalogItem, error) {
	row, err := gen.New(r.db).GetCatalogItem(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get catalog item: %w", err)
	}
	return toCatalogItem(row), nil
}

// Categories returns the distinct non-empty category strings. The slice is
// always non-nil.
func (r *CatalogRepo) Categories(ctx context.Context) ([]string, error) {
	rows, err := gen.New(r.db).ListCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	out := make([]string, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		if rows[i].Valid && rows[i].String != "" {
			out = append(out, rows[i].String)
		}
	}
	return out, nil
}

// Create inserts an item and writes one audit row, atomically.
func (r *CatalogRepo) Create(ctx context.Context, in CatalogItemInput) (*CatalogItem, error) {
	if in.Name == "" {
		return nil, errors.New("create catalog item: name is required")
	}
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}

	var created gen.CatalogItem
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		c, e := gen.New(tx).CreateCatalogItem(ctx, gen.CreateCatalogItemParams{
			Uuid:      uuid.NewString(),
			Name:      in.Name,
			Rate:      in.Rate,
			Unit:      nz(in.Unit),
			Category:  nz(in.Category),
			Sku:       nz(in.Sku),
			Metadata:  nz(metadata),
			CreatedAt: now,
			UpdatedAt: now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		created = c
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "catalog_item",
			EntityID:   c.ID,
			Action:     "create",
			Changes:    audit.Changes(map[string]any{"name": in.Name, "rate": in.Rate}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create catalog item: %w", err)
	}
	return toCatalogItem(created), nil
}

// Update writes the item's fields and one audit row, atomically. Returns
// (nil, nil) when the item does not exist so the caller can 404.
func (r *CatalogRepo) Update(ctx context.Context, id int64, in CatalogItemInput) (*CatalogItem, error) {
	if in.Name == "" {
		return nil, errors.New("update catalog item: name is required")
	}
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}

	var updated gen.CatalogItem
	var missing bool
	err := audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "catalog_item",
		EntityID:   id,
		Action:     "update",
		Changes:    audit.Changes(map[string]any{"name": in.Name}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		c, e := gen.New(tx).UpdateCatalogItem(ctx, gen.UpdateCatalogItemParams{
			Name:      in.Name,
			Rate:      in.Rate,
			Unit:      nz(in.Unit),
			Category:  nz(in.Category),
			Sku:       nz(in.Sku),
			Metadata:  nz(metadata),
			UpdatedAt: now,
			ID:        id,
		})
		if errors.Is(e, sql.ErrNoRows) {
			missing = true
			return e
		}
		if e != nil {
			return fmt.Errorf("update: %w", e)
		}
		updated = c
		return nil
	})
	if missing {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update catalog item: %w", err)
	}
	return toCatalogItem(updated), nil
}

// Delete removes an item and writes one audit row, atomically.
func (r *CatalogRepo) Delete(ctx context.Context, id int64) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "catalog_item",
		EntityID:   id,
		Action:     "delete",
	}, func(tx *sql.Tx) error {
		if err := gen.New(tx).DeleteCatalogItem(ctx, id); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		return nil
	})
}

// BulkDelete removes several items and writes one audit row, atomically. An
// empty id list is a no-op.
func (r *CatalogRepo) BulkDelete(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		for _, id := range ids { // bounded by len(ids)
			if err := q.DeleteCatalogItem(ctx, id); err != nil {
				return fmt.Errorf("delete %d: %w", id, err)
			}
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "catalog_item",
			EntityID:   0,
			Action:     "bulk_delete",
			Changes:    audit.Changes(map[string]any{"ids": ids}),
		})
	})
}

// SetRate upserts a per-tier rate override and writes one audit row, atomically.
func (r *CatalogRepo) SetRate(ctx context.Context, itemID, tierID int64, rate float64) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "catalog_item",
		EntityID:   itemID,
		Action:     "set_rate",
		Changes:    audit.Changes(map[string]any{"tierId": tierID, "rate": rate}),
	}, func(tx *sql.Tx) error {
		err := gen.New(tx).UpsertCatalogItemRate(ctx, gen.UpsertCatalogItemRateParams{
			CatalogItemID: itemID,
			RateTierID:    tierID,
			Rate:          rate,
		})
		if err != nil {
			return fmt.Errorf("upsert rate: %w", err)
		}
		return nil
	})
}

// GetRates returns the per-tier rate overrides for an item. The slice is always
// non-nil.
func (r *CatalogRepo) GetRates(ctx context.Context, itemID int64) ([]*CatalogItemRate, error) {
	rows, err := gen.New(r.db).ListRatesForItem(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("list rates for item: %w", err)
	}
	out := make([]*CatalogItemRate, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, &CatalogItemRate{RateTierID: rows[i].RateTierID, Rate: rows[i].Rate})
	}
	return out, nil
}

// EffectiveRate resolves the rate for an item and optional tier. With a tier it
// prefers the per-tier override, then falls back to the item's base rate. A nil
// tier (or missing item) yields the base rate (0 when the item is gone).
func (r *CatalogRepo) EffectiveRate(ctx context.Context, itemID int64, tierID *int64) (float64, error) {
	if tierID != nil {
		rate, err := gen.New(r.db).GetCatalogItemRate(ctx, gen.GetCatalogItemRateParams{
			CatalogItemID: itemID,
			RateTierID:    *tierID,
		})
		if err == nil {
			return rate.Rate, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("get catalog item rate: %w", err)
		}
	}
	item, err := r.Get(ctx, itemID)
	if err != nil {
		return 0, err
	}
	if item == nil {
		return 0, nil
	}
	return item.Rate, nil
}

// mapCatalogItems converts a slice of generated rows to a non-nil domain slice.
func mapCatalogItems(rows []gen.CatalogItem) []*CatalogItem {
	out := make([]*CatalogItem, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, toCatalogItem(rows[i]))
	}
	return out
}

// toCatalogItem maps a generated row to the domain CatalogItem, unwrapping
// NullStrings.
func toCatalogItem(row gen.CatalogItem) *CatalogItem {
	return &CatalogItem{
		ID:        row.ID,
		UUID:      row.Uuid,
		Name:      row.Name,
		Rate:      row.Rate,
		Unit:      row.Unit.String,
		Category:  row.Category.String,
		Sku:       row.Sku.String,
		Metadata:  row.Metadata.String,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
