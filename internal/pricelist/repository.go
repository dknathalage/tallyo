package pricelist

// ItemsRepo — the tenant-owned price list (price_list_versions, items,
// item_prices). It exposes the resolution helpers the validation engine needs.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/dknathalage/tallyo/internal/db"
	"strings"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// ItemsRepo — tenant-owned price list.
// ---------------------------------------------------------------------------

// PriceListVersion is the domain view of a row in price_list_versions. The
// public identifier (`id`) is the uuid; the int PK stays internal-only (json:"-").
type PriceListVersion struct {
	ID             int64  `json:"-"`
	UUID           string `json:"id"`
	Label          string `json:"label"`
	EffectiveFrom  string `json:"effectiveFrom"`
	EffectiveTo    string `json:"effectiveTo"`
	SourceFilename string `json:"sourceFilename"`
	CreatedAt      string `json:"createdAt"`
}

// Item is the domain view of a row in items. The public identifier (`id`) is the
// uuid; the int PK stays internal-only. The owning version is exposed as its uuid
// under `priceListVersionId` (items are listed under a version, so the SPA links
// item→version by uuid). UnitPrice is nil when no generic per-unit price is set.
type Item struct {
	ID                  int64    `json:"-"`
	UUID                string   `json:"id"`
	PriceListVersionID  int64    `json:"-"`
	PriceListVersionUID string   `json:"priceListVersionId"`
	Code                string   `json:"code"`
	Name                string   `json:"name"`
	Unit                string   `json:"unit"`
	Category            string   `json:"category"`
	UnitPrice           *float64 `json:"unitPrice"`
	Taxable             bool     `json:"taxable"`
	Metadata            string   `json:"metadata"`
}

// ItemPrice is the domain view of a row in item_prices. PriceCap is nil for
// quotable items (no fixed cap) — the validation engine skips the over-cap
// assertion when nil. A price is always fetched under its item, so neither the
// int PK nor the item FK crosses the API.
type ItemPrice struct {
	ID       int64    `json:"-"`
	ItemID   int64    `json:"-"`
	Zone     string   `json:"zone"`
	PriceCap *float64 `json:"priceCap"`
}

// ItemsRepo reads (and, for the owner/admin ingest, writes) the tenant-owned
// price list.
type ItemsRepo struct {
	db db.Executor
}

// NewItems constructs a repository. A nil db is a programmer error.
func NewItems(db db.Executor) *ItemsRepo {
	if db == nil {
		panic("pricelist: NewItems requires a non-nil *sql.DB")
	}
	return &ItemsRepo{db: db}
}

// ListVersions returns all price-list versions, newest effective_from first.
func (r *ItemsRepo) ListVersions(ctx context.Context) ([]*PriceListVersion, error) {
	rows, err := gen.New(r.db).ListPriceListVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list price-list versions: %w", err)
	}
	out := make([]*PriceListVersion, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, toPriceListVersion(rows[i]))
	}
	return out, nil
}

// GetVersion returns the version by id, or (nil, nil) when absent.
func (r *ItemsRepo) GetVersion(ctx context.Context, id int64) (*PriceListVersion, error) {
	row, err := gen.New(r.db).GetPriceListVersion(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get price-list version: %w", err)
	}
	return toPriceListVersion(row), nil
}

// GetVersionByUUID returns the version by its UUID, or (nil, nil) when absent.
// Used to resolve a tenant line's pinned price_list_version_id (a tenant UUID).
func (r *ItemsRepo) GetVersionByUUID(ctx context.Context, uuid string) (*PriceListVersion, error) {
	row, err := gen.New(r.db).GetPriceListVersionByUUID(ctx, uuid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get price-list version by uuid: %w", err)
	}
	return toPriceListVersion(row), nil
}

// ResolveVersionForDate returns the version whose [effective_from, effective_to]
// window contains serviceDate, or (nil, nil) when none applies.
func (r *ItemsRepo) ResolveVersionForDate(ctx context.Context, serviceDate string) (*PriceListVersion, error) {
	if serviceDate == "" {
		return nil, errors.New("resolve price-list version: service date required")
	}
	row, err := gen.New(r.db).ResolvePriceListVersionForDate(ctx, serviceDate)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve price-list version for date: %w", err)
	}
	return toPriceListVersion(row), nil
}

// ListItems returns all items in a price-list version, by code.
func (r *ItemsRepo) ListItems(ctx context.Context, versionID int64) ([]*Item, error) {
	rows, err := gen.New(r.db).ListItems(ctx, versionID)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	out := make([]*Item, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, toItem(rows[i]))
	}
	return out, nil
}

// ResolveVersionIDByUUID maps a price-list-version uuid to its int PK, returning
// (0, nil) when no version carries that uuid. Used to translate a public version
// uuid path param to the internal FK before filtering items.
func (r *ItemsRepo) ResolveVersionIDByUUID(ctx context.Context, versionUUID string) (int64, error) {
	if versionUUID == "" {
		return 0, errors.New("resolve version id: uuid required")
	}
	id, err := gen.New(r.db).GetPriceListVersionIDByUUID(ctx, versionUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("resolve version id by uuid: %w", err)
	}
	return id, nil
}

// ResolveItemIDByUUID maps an item uuid to its int PK, returning (0, nil) when no
// item carries that uuid. Used to translate a public item uuid path param to the
// internal FK before filtering item_prices.
func (r *ItemsRepo) ResolveItemIDByUUID(ctx context.Context, itemUUID string) (int64, error) {
	if itemUUID == "" {
		return 0, errors.New("resolve item id: uuid required")
	}
	id, err := gen.New(r.db).GetItemIDByUUID(ctx, itemUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("resolve item id by uuid: %w", err)
	}
	return id, nil
}

// SearchItems returns the items in a version whose code or name matches the query
// (LIKE, case-insensitive on the SQLite default collation), by code. An empty
// query matches everything in the version.
func (r *ItemsRepo) SearchItems(ctx context.Context, versionID int64, query string) ([]*Item, error) {
	like := "%" + escapeLike(query) + "%"
	rows, err := gen.New(r.db).SearchItems(ctx, gen.SearchItemsParams{
		PriceListVersionID: versionID,
		Code:               like,
		Name:               like,
	})
	if err != nil {
		return nil, fmt.Errorf("search items: %w", err)
	}
	out := make([]*Item, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, toItem(rows[i]))
	}
	return out, nil
}

// escapeLike backslash-escapes the LIKE metacharacters (\, %, _) in a user query
// so they match literally. The escape order matters: backslash itself is escaped
// first, otherwise the escapes added for % and _ would be double-escaped. Callers
// must pair this with `ESCAPE '\'` in the SQL LIKE clause.
func escapeLike(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
	)
	return r.Replace(s)
}

// GetItemByCode finds an item by code within a version, or (nil, nil) when none
// matches.
func (r *ItemsRepo) GetItemByCode(ctx context.Context, versionID int64, code string) (*Item, error) {
	if code == "" {
		return nil, errors.New("get item by code: code required")
	}
	row, err := gen.New(r.db).GetItemByCode(ctx, gen.GetItemByCodeParams{
		PriceListVersionID: versionID,
		Code:               code,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get item by code: %w", err)
	}
	return toItem(row), nil
}

// ResolveZonePrice returns the price row for a code+zone within a version, or
// (nil, nil) when no price row exists. A returned row with a nil PriceCap denotes
// a quotable item.
func (r *ItemsRepo) ResolveZonePrice(ctx context.Context, versionID int64, code, zone string) (*ItemPrice, error) {
	if code == "" || zone == "" {
		return nil, errors.New("resolve zone price: code and zone required")
	}
	row, err := gen.New(r.db).ResolveZonePrice(ctx, gen.ResolveZonePriceParams{
		PriceListVersionID: versionID,
		Code:               code,
		Zone:               zone,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve zone price: %w", err)
	}
	return toItemPrice(row), nil
}

// ListPrices returns all zone price rows for an item, by zone.
func (r *ItemsRepo) ListPrices(ctx context.Context, itemID int64) ([]*ItemPrice, error) {
	rows, err := gen.New(r.db).ListItemPrices(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("list item prices: %w", err)
	}
	out := make([]*ItemPrice, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, toItemPrice(rows[i]))
	}
	return out, nil
}

// ImportItem is one parsed item row destined for the ingest. Prices maps
// zone → cap; a nil cap denotes a quotable item (no fixed cap). Zones absent
// from the map get no price row.
type ImportItem struct {
	Code     string
	Name     string
	Unit     string
	Category string
	Taxable  bool
	Prices   map[string]*float64 // zone → cap (nil = quotable)
}

// IngestResult summarises a completed price-list ingest.
type IngestResult struct {
	Version    *PriceListVersion
	ItemCount  int
	PriceCount int
}

// Ingest creates a new price_list_version and bulk-upserts every item + its
// per-zone price rows in ONE audited transaction. Any error rolls the whole thing
// back (no partial-version state). The version-create audit row is written inside
// the same tx. Returns the created version and counts.
func (r *ItemsRepo) Ingest(ctx context.Context, label, effectiveFrom, sourceFilename string, items []ImportItem) (*IngestResult, error) {
	if label == "" {
		return nil, errors.New("ingest price list: label required")
	}
	if effectiveFrom == "" {
		return nil, errors.New("ingest price list: effective_from required")
	}
	if len(items) == 0 {
		return nil, errors.New("ingest price list: no data rows")
	}

	var result IngestResult
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		now := time.Now().UTC().Format(time.RFC3339)
		// Close any currently-open version the day before the new one takes effect
		// so date-windows never overlap: historical service dates keep resolving to
		// the version effective then, and only the new version stays open-ended.
		// Existing invoices are unaffected — their prices are pinned per line.
		if prevTo := dayBefore(effectiveFrom); prevTo != "" {
			if e := q.CloseOpenPriceListVersions(ctx, sql.NullString{String: prevTo, Valid: true}); e != nil {
				return fmt.Errorf("close prior versions: %w", e)
			}
		}
		ver, e := q.CreatePriceListVersion(ctx, gen.CreatePriceListVersionParams{
			Uuid:           uuid.NewString(),
			Label:          label,
			EffectiveFrom:  effectiveFrom,
			EffectiveTo:    sql.NullString{},
			SourceFilename: db.NzMaybe(sourceFilename),
			CreatedAt:      now,
		})
		if e != nil {
			return fmt.Errorf("create version: %w", e)
		}

		priceCount := 0
		for i := range items { // bounded by len(items)
			it := items[i]
			item, e := q.UpsertItem(ctx, gen.UpsertItemParams{
				Uuid:               uuid.NewString(),
				PriceListVersionID: ver.ID,
				Code:               it.Code,
				Name:               it.Name,
				Unit:               db.NzMaybe(it.Unit),
				Category:           db.NzMaybe(it.Category),
				UnitPrice:          sql.NullFloat64{},
				Taxable:            db.B2i(it.Taxable),
				Metadata:           sql.NullString{String: "{}", Valid: true},
			})
			if e != nil {
				return fmt.Errorf("upsert item %q: %w", it.Code, e)
			}
			for zone, capPtr := range it.Prices { // bounded by len(it.Prices)
				cap := sql.NullFloat64{}
				if capPtr != nil {
					cap = sql.NullFloat64{Float64: *capPtr, Valid: true}
				}
				if e := q.UpsertItemPrice(ctx, gen.UpsertItemPriceParams{
					ItemID:   item.ID,
					Zone:     zone,
					PriceCap: cap,
				}); e != nil {
					return fmt.Errorf("upsert price %q/%s: %w", it.Code, zone, e)
				}
				priceCount++
			}
		}

		result = IngestResult{Version: toPriceListVersion(ver), ItemCount: len(items), PriceCount: priceCount}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "price_list_version",
			EntityID:   ver.ID,
			Action:     "ingest",
			Changes:    audit.Changes(map[string]any{"label": label, "items": len(items), "prices": priceCount}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("ingest price list: %w", err)
	}
	return &result, nil
}

// dayBefore returns the ISO date one day before the given YYYY-MM-DD date, or ""
// when the input is not a parseable date (in which case the caller skips closing
// prior versions rather than writing a bad boundary).
func dayBefore(isoDate string) string {
	t, err := time.Parse("2006-01-02", isoDate)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, -1).Format("2006-01-02")
}

func toPriceListVersion(row gen.PriceListVersion) *PriceListVersion {
	return &PriceListVersion{
		ID:             row.ID,
		UUID:           row.Uuid,
		Label:          row.Label,
		EffectiveFrom:  row.EffectiveFrom,
		EffectiveTo:    row.EffectiveTo.String,
		SourceFilename: row.SourceFilename.String,
		CreatedAt:      row.CreatedAt,
	}
}

func toItem(row gen.Item) *Item {
	var unitPrice *float64
	if row.UnitPrice.Valid {
		v := row.UnitPrice.Float64
		unitPrice = &v
	}
	return &Item{
		ID:                 row.ID,
		UUID:               row.Uuid,
		PriceListVersionID: row.PriceListVersionID,
		Code:               row.Code,
		Name:               row.Name,
		Unit:               row.Unit.String,
		Category:           row.Category.String,
		UnitPrice:          unitPrice,
		Taxable:            row.Taxable == 1,
		Metadata:           row.Metadata.String,
	}
}

func toItemPrice(row gen.ItemPrice) *ItemPrice {
	var cap *float64
	if row.PriceCap.Valid {
		v := row.PriceCap.Float64
		cap = &v
	}
	return &ItemPrice{
		ID:       row.ID,
		ItemID:   row.ItemID,
		Zone:     row.Zone,
		PriceCap: cap,
	}
}
