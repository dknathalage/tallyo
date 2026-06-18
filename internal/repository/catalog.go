package repository

// CatalogRepo — the GLOBAL NDIS Support Catalogue (catalog_versions,
// support_items, support_item_prices). NOT tenant-scoped (shared reference
// data per spec §3.1/§4.3), so its methods take NO tenantID. It exposes the
// resolution helpers the validation engine (J10) needs.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// CatalogRepo — GLOBAL NDIS Support Catalogue (NOT tenant-scoped).
// ---------------------------------------------------------------------------

// CatalogVersion is the domain view of a row in catalog_versions.
type CatalogVersion struct {
	ID             int64  `json:"id"`
	UUID           string `json:"uuid"`
	Label          string `json:"label"`
	EffectiveFrom  string `json:"effectiveFrom"`
	EffectiveTo    string `json:"effectiveTo"`
	SourceFilename string `json:"sourceFilename"`
	CreatedAt      string `json:"createdAt"`
}

// SupportItem is the domain view of a row in support_items.
type SupportItem struct {
	ID                int64  `json:"id"`
	UUID              string `json:"uuid"`
	CatalogVersionID  int64  `json:"catalogVersionId"`
	Code              string `json:"code"`
	Name              string `json:"name"`
	Unit              string `json:"unit"`
	SupportCategory   string `json:"supportCategory"`
	RegistrationGroup string `json:"registrationGroup"`
	ClaimType         string `json:"claimType"`
	GstFree           bool   `json:"gstFree"`
	Metadata          string `json:"metadata"`
}

// SupportItemPrice is the domain view of a row in support_item_prices. PriceCap
// is nil for quotable items (no fixed cap) — the validation engine skips the
// over-cap assertion when nil (spec §6 step 4).
type SupportItemPrice struct {
	ID            int64    `json:"id"`
	SupportItemID int64    `json:"supportItemId"`
	Zone          string   `json:"zone"`
	PriceCap      *float64 `json:"priceCap"`
}

// CatalogRepo reads (and, for the platform-admin ingest, writes) the global NDIS
// Support Catalogue. It is intentionally NOT tenant-scoped.
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

// ListVersions returns all catalogue versions, newest effective_from first.
func (r *CatalogRepo) ListVersions(ctx context.Context) ([]*CatalogVersion, error) {
	rows, err := gen.New(r.db).ListCatalogVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list catalog versions: %w", err)
	}
	out := make([]*CatalogVersion, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, toCatalogVersion(rows[i]))
	}
	return out, nil
}

// GetVersion returns the version by id, or (nil, nil) when absent.
func (r *CatalogRepo) GetVersion(ctx context.Context, id int64) (*CatalogVersion, error) {
	row, err := gen.New(r.db).GetCatalogVersion(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get catalog version: %w", err)
	}
	return toCatalogVersion(row), nil
}

// ResolveVersionForDate returns the version whose [effective_from, effective_to]
// window contains serviceDate (spec §6 step 1), or (nil, nil) when none applies.
func (r *CatalogRepo) ResolveVersionForDate(ctx context.Context, serviceDate string) (*CatalogVersion, error) {
	if serviceDate == "" {
		return nil, errors.New("resolve catalog version: service date required")
	}
	row, err := gen.New(r.db).ResolveCatalogVersionForDate(ctx, serviceDate)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve catalog version for date: %w", err)
	}
	return toCatalogVersion(row), nil
}

// ListSupportItems returns all support items in a catalogue version, by code.
func (r *CatalogRepo) ListSupportItems(ctx context.Context, versionID int64) ([]*SupportItem, error) {
	rows, err := gen.New(r.db).ListSupportItems(ctx, versionID)
	if err != nil {
		return nil, fmt.Errorf("list support items: %w", err)
	}
	out := make([]*SupportItem, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, toSupportItem(rows[i]))
	}
	return out, nil
}

// SearchSupportItems returns the support items in a version whose code or name
// matches the query (LIKE, case-insensitive on the SQLite default collation),
// by code. An empty query matches everything in the version.
func (r *CatalogRepo) SearchSupportItems(ctx context.Context, versionID int64, query string) ([]*SupportItem, error) {
	like := "%" + escapeLike(query) + "%"
	rows, err := gen.New(r.db).SearchSupportItems(ctx, gen.SearchSupportItemsParams{
		CatalogVersionID: versionID,
		Code:             like,
		Name:             like,
	})
	if err != nil {
		return nil, fmt.Errorf("search support items: %w", err)
	}
	out := make([]*SupportItem, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, toSupportItem(rows[i]))
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

// GetSupportItemByCode finds a support item by code within a version (spec §6
// step 2), or (nil, nil) when none matches.
func (r *CatalogRepo) GetSupportItemByCode(ctx context.Context, versionID int64, code string) (*SupportItem, error) {
	if code == "" {
		return nil, errors.New("get support item by code: code required")
	}
	row, err := gen.New(r.db).GetSupportItemByCode(ctx, gen.GetSupportItemByCodeParams{
		CatalogVersionID: versionID,
		Code:             code,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get support item by code: %w", err)
	}
	return toSupportItem(row), nil
}

// ResolveZonePrice returns the price row for a code+zone within a version (spec
// §6 step 3), or (nil, nil) when no price row exists. A returned row with a nil
// PriceCap denotes a quotable item.
func (r *CatalogRepo) ResolveZonePrice(ctx context.Context, versionID int64, code, zone string) (*SupportItemPrice, error) {
	if code == "" || zone == "" {
		return nil, errors.New("resolve zone price: code and zone required")
	}
	row, err := gen.New(r.db).ResolveZonePrice(ctx, gen.ResolveZonePriceParams{
		CatalogVersionID: versionID,
		Code:             code,
		Zone:             zone,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve zone price: %w", err)
	}
	return toSupportItemPrice(row), nil
}

// ListPrices returns all zone price rows for a support item, by zone.
func (r *CatalogRepo) ListPrices(ctx context.Context, supportItemID int64) ([]*SupportItemPrice, error) {
	rows, err := gen.New(r.db).ListSupportItemPrices(ctx, supportItemID)
	if err != nil {
		return nil, fmt.Errorf("list support item prices: %w", err)
	}
	out := make([]*SupportItemPrice, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, toSupportItemPrice(rows[i]))
	}
	return out, nil
}

// IngestItem is one parsed support-item row destined for the ingest. Prices maps
// zone → cap; a nil cap denotes a quotable item (no fixed cap). Zones absent
// from the map get no price row.
type IngestItem struct {
	Code              string
	Name              string
	Unit              string
	SupportCategory   string
	RegistrationGroup string
	ClaimType         string
	GstFree           bool
	Prices            map[string]*float64 // zone → cap (nil = quotable)
}

// IngestResult summarises a completed catalogue ingest.
type IngestResult struct {
	Version    *CatalogVersion
	ItemCount  int
	PriceCount int
}

// Ingest creates a new catalog_version and bulk-upserts every support_item +
// its per-zone price rows in ONE audited transaction. Any error rolls the whole
// thing back (no partial-version state, spec §5). The version-create audit row is
// written inside the same tx. Returns the created version and counts.
func (r *CatalogRepo) Ingest(ctx context.Context, label, effectiveFrom, sourceFilename string, items []IngestItem) (*IngestResult, error) {
	if label == "" {
		return nil, errors.New("ingest catalogue: label required")
	}
	if effectiveFrom == "" {
		return nil, errors.New("ingest catalogue: effective_from required")
	}
	if len(items) == 0 {
		return nil, errors.New("ingest catalogue: no data rows")
	}

	var result IngestResult
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		now := time.Now().UTC().Format(time.RFC3339)
		ver, e := q.CreateCatalogVersion(ctx, gen.CreateCatalogVersionParams{
			Uuid:           uuid.NewString(),
			Label:          label,
			EffectiveFrom:  effectiveFrom,
			EffectiveTo:    sql.NullString{},
			SourceFilename: nzMaybe(sourceFilename),
			CreatedAt:      now,
		})
		if e != nil {
			return fmt.Errorf("create version: %w", e)
		}

		priceCount := 0
		for i := range items { // bounded by len(items)
			it := items[i]
			si, e := q.UpsertSupportItem(ctx, gen.UpsertSupportItemParams{
				Uuid:              uuid.NewString(),
				CatalogVersionID:  ver.ID,
				Code:              it.Code,
				Name:              it.Name,
				Unit:              nzMaybe(it.Unit),
				SupportCategory:   nzMaybe(it.SupportCategory),
				RegistrationGroup: nzMaybe(it.RegistrationGroup),
				ClaimType:         nzMaybe(it.ClaimType),
				GstFree:           b2i(it.GstFree),
				Metadata:          sql.NullString{String: "{}", Valid: true},
			})
			if e != nil {
				return fmt.Errorf("upsert item %q: %w", it.Code, e)
			}
			for zone, capPtr := range it.Prices { // bounded by len(it.Prices)
				cap := sql.NullFloat64{}
				if capPtr != nil {
					cap = sql.NullFloat64{Float64: *capPtr, Valid: true}
				}
				if e := q.UpsertSupportItemPrice(ctx, gen.UpsertSupportItemPriceParams{
					SupportItemID: si.ID,
					Zone:          zone,
					PriceCap:      cap,
				}); e != nil {
					return fmt.Errorf("upsert price %q/%s: %w", it.Code, zone, e)
				}
				priceCount++
			}
		}

		result = IngestResult{Version: toCatalogVersion(ver), ItemCount: len(items), PriceCount: priceCount}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "catalog_version",
			EntityID:   ver.ID,
			Action:     "ingest",
			Changes:    audit.Changes(map[string]any{"label": label, "items": len(items), "prices": priceCount}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("ingest catalogue: %w", err)
	}
	return &result, nil
}

func toCatalogVersion(row gen.CatalogVersion) *CatalogVersion {
	return &CatalogVersion{
		ID:             row.ID,
		UUID:           row.Uuid,
		Label:          row.Label,
		EffectiveFrom:  row.EffectiveFrom,
		EffectiveTo:    row.EffectiveTo.String,
		SourceFilename: row.SourceFilename.String,
		CreatedAt:      row.CreatedAt,
	}
}

func toSupportItem(row gen.SupportItem) *SupportItem {
	return &SupportItem{
		ID:                row.ID,
		UUID:              row.Uuid,
		CatalogVersionID:  row.CatalogVersionID,
		Code:              row.Code,
		Name:              row.Name,
		Unit:              row.Unit.String,
		SupportCategory:   row.SupportCategory.String,
		RegistrationGroup: row.RegistrationGroup.String,
		ClaimType:         row.ClaimType.String,
		GstFree:           row.GstFree == 1,
		Metadata:          row.Metadata.String,
	}
}

func toSupportItemPrice(row gen.SupportItemPrice) *SupportItemPrice {
	var cap *float64
	if row.PriceCap.Valid {
		v := row.PriceCap.Float64
		cap = &v
	}
	return &SupportItemPrice{
		ID:            row.ID,
		SupportItemID: row.SupportItemID,
		Zone:          row.Zone,
		PriceCap:      cap,
	}
}
