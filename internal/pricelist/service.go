package pricelist

import (
	"context"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/importer"
	"github.com/dknathalage/tallyo/internal/realtime"
)

// Service exposes read access to the tenant-owned price list
// (price_list_versions / items / item_prices).
//
// Owner/admin write access (upload-and-map import) lives in ImportService below.
type Service struct {
	repo *ItemsRepo
}

// NewService constructs the read service.
func NewService(db db.Executor) *Service {
	return &Service{repo: NewItems(db)}
}

// ListVersions returns all price-list versions.
func (s *Service) ListVersions(ctx context.Context) ([]*PriceListVersion, error) {
	return s.repo.ListVersions(ctx)
}

// GetVersion returns a price-list version by id, or (nil, nil) when absent.
func (s *Service) GetVersion(ctx context.Context, id int64) (*PriceListVersion, error) {
	return s.repo.GetVersion(ctx, id)
}

// ErrNotFound is returned when a version/item uuid resolves to no row. The
// handler maps it to a 404.
var ErrNotFound = fmt.Errorf("price-list resource not found")

// ListItemsByVersionUUID returns the items in the price-list version identified
// by versionUUID. Returns ErrNotFound when no version carries that uuid. Each
// returned item's PriceListVersionUID is set to versionUUID so the SPA can link
// item→version by uuid.
func (s *Service) ListItemsByVersionUUID(ctx context.Context, versionUUID string) ([]*Item, error) {
	versionID, err := s.repo.ResolveVersionIDByUUID(ctx, versionUUID)
	if err != nil {
		return nil, err
	}
	if versionID == 0 {
		return nil, ErrNotFound
	}
	items, err := s.repo.ListItems(ctx, versionID)
	if err != nil {
		return nil, err
	}
	for i := range items { // bounded by len(items)
		items[i].PriceListVersionUID = versionUUID
	}
	return items, nil
}

// ListPricesByItemUUID returns the zone prices for the item identified by
// itemUUID. Returns ErrNotFound when no item carries that uuid.
func (s *Service) ListPricesByItemUUID(ctx context.Context, itemUUID string) ([]*ItemPrice, error) {
	itemID, err := s.repo.ResolveItemIDByUUID(ctx, itemUUID)
	if err != nil {
		return nil, err
	}
	if itemID == 0 {
		return nil, ErrNotFound
	}
	return s.repo.ListPrices(ctx, itemID)
}

// Match is one item resolved for a service date, enriched with its price cap in
// the requested zone. PriceCap is nil for a quotable item (no fixed cap) or when
// no price row exists for the zone.
type Match struct {
	Code      string   `json:"code"`
	Name      string   `json:"name"`
	Unit      string   `json:"unit"`
	Taxable   bool     `json:"taxable"`
	Zone      string   `json:"zone"`
	PriceCap  *float64 `json:"priceCap"`
	Quotable  bool     `json:"quotable"`
	VersionID int64    `json:"priceListVersionId"`
}

// SearchForDate resolves the price-list version effective on serviceDate, finds
// items whose code or name matches query, and attaches each item's price cap for
// the given zone (default "national" when zone is empty). Returns an empty
// (non-nil) slice when no version is in effect or nothing matches — capped at
// limit results (limit ≤ 0 → a default of 25) to bound the payload.
func (s *Service) SearchForDate(ctx context.Context, query, serviceDate, zone string, limit int) ([]*Match, error) {
	if serviceDate == "" {
		return nil, fmt.Errorf("price-list search: service date is required")
	}
	if zone == "" {
		zone = "national"
	}
	if limit <= 0 {
		limit = 25
	}
	ver, err := s.repo.ResolveVersionForDate(ctx, serviceDate)
	if err != nil {
		return nil, err
	}
	out := make([]*Match, 0)
	if ver == nil {
		return out, nil
	}
	items, err := s.repo.SearchItems(ctx, ver.ID, query)
	if err != nil {
		return nil, err
	}
	for i := range items { // bounded by len(items)
		if len(out) >= limit {
			break
		}
		it := items[i]
		m := &Match{
			Code: it.Code, Name: it.Name, Unit: it.Unit, Taxable: it.Taxable,
			Zone: zone, VersionID: ver.ID,
		}
		price, perr := s.repo.ResolveZonePrice(ctx, ver.ID, it.Code, zone)
		if perr != nil {
			return nil, perr
		}
		if price != nil {
			m.PriceCap = price.PriceCap
			m.Quotable = price.PriceCap == nil
		}
		out = append(out, m)
	}
	return out, nil
}

// ImportService is the owner/admin WRITE path for the tenant-owned price list. It
// is a generic two-step "upload a file and map columns" importer: Inspect reads
// the headers + a sample WITHOUT persisting, then ImportMapped applies a
// source-column→target-field map and bulk-loads a new price_list_version + items
// in ONE transaction.
type ImportService struct {
	repo *ItemsRepo
	hub  *realtime.Hub
}

// NewImportService constructs the import service. A nil hub is a programmer error.
func NewImportService(db db.Executor, hub *realtime.Hub) *ImportService {
	if hub == nil {
		panic("pricelist.NewImportService: nil hub")
	}
	return &ImportService{repo: NewItems(db), hub: hub}
}

// maxSampleRows caps the preview returned by Inspect so the payload stays small.
const maxSampleRows = 10

// InspectResult is the headers + a capped sample of data rows from an uploaded
// file. It is used by the SPA to render one mapping <select> per header and a
// preview of the mapped data. Inspect persists nothing.
type InspectResult struct {
	Headers    []string            `json:"headers"`
	SampleRows []map[string]string `json:"sampleRows"`
}

// Inspect parses an uploaded file and returns its headers plus a sample of up to
// maxSampleRows data rows, WITHOUT writing anything to the database. fileType is
// "csv" or "xlsx"; sheetName/headerRow are forwarded to importer.ParseRows.
func (s *ImportService) Inspect(data []byte, fileType, sheetName string, headerRow int) (*InspectResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("inspect: empty file")
	}
	headers, rows, err := importer.ParseRows(data, fileType, sheetName, headerRow)
	if err != nil {
		return nil, fmt.Errorf("inspect: %w", err)
	}
	sample := rows
	if len(sample) > maxSampleRows {
		sample = sample[:maxSampleRows]
	}
	return &InspectResult{Headers: headers, SampleRows: sample}, nil
}

// ImportSummary is the JSON-friendly result of a price-list import.
type ImportSummary struct {
	VersionID     int64  `json:"versionId"`
	VersionUUID   string `json:"versionUuid"`
	Label         string `json:"label"`
	EffectiveFrom string `json:"effectiveFrom"`
	ItemCount     int    `json:"itemCount"`
}

// ImportMapped parses an uploaded file, applies the source-column→target-field
// mapping, and loads a new price-list version + its items in ONE transaction. The
// WHOLE upload is rejected (no partial state) when the required "name" target is
// unmapped or zero data rows parse. The new version is effective from today.
// Broadcasts an SSE event AFTER the commit succeeds.
//
// ponytail: single price column; multi-zone NDIS cap import later — generic
// import sets unit_price + category and writes no item_prices.
func (s *ImportService) ImportMapped(ctx context.Context, data []byte, fileType, sheetName string, headerRow int, mapping map[string]string, label string) (*ImportSummary, error) {
	if label == "" {
		return nil, fmt.Errorf("import: label required")
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("import: empty file")
	}
	headers, rows, err := importer.ParseRows(data, fileType, sheetName, headerRow)
	if err != nil {
		return nil, fmt.Errorf("import: %w", err)
	}
	parsed, err := importer.ApplyMapping(headers, rows, mapping)
	if err != nil {
		return nil, fmt.Errorf("import: %w", err)
	}

	items := make([]ImportItem, 0, len(parsed))
	for i := range parsed { // bounded by len(parsed)
		p := parsed[i]
		var unitPrice *float64
		if p.UnitPrice != 0 {
			v := p.UnitPrice
			unitPrice = &v
		}
		// A generic import has no item code column requirement; fall back to the
		// name as the code so the (version, code) uniqueness key stays populated.
		code := p.Code
		if code == "" {
			code = p.Name
		}
		items = append(items, ImportItem{
			Code:      code,
			Name:      p.Name,
			Unit:      p.Unit,
			Category:  p.Category,
			UnitPrice: unitPrice,
			Taxable:   p.Taxable,
			// Prices intentionally empty: generic import writes no zone caps.
		})
	}

	effectiveFrom := time.Now().UTC().Format("2006-01-02")
	res, err := s.repo.Ingest(ctx, label, effectiveFrom, "", items)
	if err != nil {
		return nil, err
	}

	// Broadcast with the GlobalTenantID sentinel so the event reaches every open
	// SSE stream (the import path has no request tenant in scope here).
	s.hub.Broadcast(realtime.Event{TenantID: realtime.GlobalTenantID, Entity: "price_list_version", UUID: res.Version.UUID, Action: "import"})
	return &ImportSummary{
		VersionID:     res.Version.ID,
		VersionUUID:   res.Version.UUID,
		Label:         res.Version.Label,
		EffectiveFrom: res.Version.EffectiveFrom,
		ItemCount:     res.ItemCount,
	}, nil
}
