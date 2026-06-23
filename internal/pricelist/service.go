package pricelist

import (
	"context"
	"fmt"
	"github.com/dknathalage/tallyo/internal/db"
	"strings"

	"github.com/dknathalage/tallyo/internal/importer"
	"github.com/dknathalage/tallyo/internal/realtime"
)

// Service exposes read access to the tenant-owned price list
// (price_list_versions / items / item_prices).
//
// Owner/admin write access (XLSX ingest) lives in IngestService below.
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

// IngestService is the owner/admin WRITE path for the tenant-owned price list: it
// parses a fixed-format XLSX (keyed to known headers — no column-mapping wizard)
// and bulk-loads a new price_list_version + items + per-zone prices in one
// transaction.
type IngestService struct {
	repo *ItemsRepo
	hub  *realtime.Hub
}

// NewIngestService constructs the ingest service. A nil hub is a programmer error.
func NewIngestService(db db.Executor, hub *realtime.Hub) *IngestService {
	if hub == nil {
		panic("pricelist.NewIngestService: nil hub")
	}
	return &IngestService{repo: NewItems(db), hub: hub}
}

// IngestSummary is the JSON-friendly result of a price-list ingest.
type IngestSummary struct {
	VersionID     int64  `json:"versionId"`
	VersionUUID   string `json:"versionUuid"`
	Label         string `json:"label"`
	EffectiveFrom string `json:"effectiveFrom"`
	ItemCount     int    `json:"itemCount"`
	PriceCount    int    `json:"priceCount"`
}

// Canonical Support Catalogue column headers (normalised: lower-cased, internal
// whitespace collapsed). DEFERRED: this XLSX parser is the legacy NDIS shape,
// retained for the per-tenant ingest path wired in a later phase. The geographic
// price-limit columns map to our three zones.
const (
	colCode       = "support item number"
	colName       = "support item name"
	colUnit       = "unit"
	colCategory   = "support category"
	colRegGroup   = "registration group name"
	colNational   = "national"
	colRemote     = "remote"
	colVeryRemote = "very remote"
)

// nationalPriceColumns are the headers the standard ("national") zone price is
// read from, in precedence order. The legacy catalogue has no single "National"
// column — the standard price lives in per-state columns that are all identical,
// so any present one is the national rate. "national" is tried first to stay
// forward-compatible if the source reintroduces that column.
var nationalPriceColumns = []string{
	colNational, "act", "nsw", "nt", "qld", "sa", "tas", "vic", "wa",
}

// IngestXLSX parses fixed-format XLSX bytes and loads a new price-list version.
// The WHOLE upload is rejected (no partial state) when a required column is
// missing or zero data rows parse. Broadcasts an SSE event AFTER the commit
// succeeds.
func (s *IngestService) IngestXLSX(ctx context.Context, data []byte, label, effectiveFrom, sourceFilename string) (*IngestSummary, error) {
	if label == "" {
		return nil, fmt.Errorf("ingest: label required")
	}
	if effectiveFrom == "" {
		return nil, fmt.Errorf("ingest: effective_from required")
	}

	items, err := ParseXLSX(data)
	if err != nil {
		return nil, fmt.Errorf("ingest: %w", err)
	}

	res, err := s.repo.Ingest(ctx, label, effectiveFrom, sourceFilename, items)
	if err != nil {
		return nil, err
	}

	// Broadcast with the GlobalTenantID sentinel so the event reaches every open
	// SSE stream (the ingest path has no request tenant in scope here).
	s.hub.Broadcast(realtime.Event{TenantID: realtime.GlobalTenantID, Entity: "price_list_version", UUID: res.Version.UUID, Action: "ingest"})
	return &IngestSummary{
		VersionID:     res.Version.ID,
		VersionUUID:   res.Version.UUID,
		Label:         res.Version.Label,
		EffectiveFrom: res.Version.EffectiveFrom,
		ItemCount:     res.ItemCount,
		PriceCount:    res.PriceCount,
	}, nil
}

// ParseXLSX parses fixed-format XLSX bytes into the ImportItem domain values that
// repo.Ingest persists. The whole upload is rejected (no partial state) when a
// required column is missing or zero data rows parse. Retained for the deferred
// per-tenant ingest path (IngestXLSX).
func ParseXLSX(data []byte) ([]ImportItem, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty file")
	}
	headers, rows, err := importer.ParseRows(data, "xlsx", "", 1)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	// Build a normalised header→original-key index so cell lookups tolerate
	// case/whitespace differences in the source file.
	norm := make(map[string]string, len(headers))
	for i := range headers { // bounded by len(headers)
		norm[normaliseHeader(headers[i])] = headers[i]
	}

	required := []string{colCode, colName}
	for i := range required { // bounded by len(required)
		if _, ok := norm[required[i]]; !ok {
			return nil, fmt.Errorf("missing required column %q", required[i])
		}
	}

	items, err := buildImportItems(rows, norm)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no data rows parsed")
	}
	return items, nil
}

// buildImportItems maps parsed rows to ImportItem values, skipping rows with a
// blank item code. Bounded by len(rows).
func buildImportItems(rows []map[string]string, norm map[string]string) ([]ImportItem, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("file has no data rows")
	}
	cell := func(row map[string]string, canonical string) string {
		key, ok := norm[canonical]
		if !ok {
			return ""
		}
		return strings.TrimSpace(row[key])
	}
	out := make([]ImportItem, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		row := rows[i]
		code := cell(row, colCode)
		if code == "" {
			continue // skip blank/spacer rows
		}
		it := ImportItem{
			Code:     code,
			Name:     cell(row, colName),
			Unit:     cell(row, colUnit),
			Category: cell(row, colCategory),
			// NDIS supports are GST-free by default, so taxable is false; the
			// standard catalogue export carries no taxable column.
			Taxable: false,
			Prices:  zonePrices(row, norm),
		}
		out = append(out, it)
	}
	return out, nil
}

// zonePrices reads the three geographic price-limit columns into a zone→cap map.
// A blank / non-numeric / "Quote" cell yields a nil cap (quotable item); a zone
// column absent from the sheet yields no entry for that zone.
func zonePrices(row map[string]string, norm map[string]string) map[string]*float64 {
	zones := [3]struct {
		zone   string
		header string
	}{
		{"national", colNational},
		{"remote", colRemote},
		{"very_remote", colVeryRemote},
	}
	out := make(map[string]*float64, 3)
	for i := range zones { // bounded: exactly 3 zones
		key, ok := norm[zones[i].header]
		if !ok {
			continue
		}
		raw := strings.TrimSpace(row[key])
		out[zones[i].zone] = parseCap(raw)
	}
	// The standard price has no single "national" column; read it from the first
	// available per-state column (all states carry the identical national rate).
	if _, have := out["national"]; !have {
		for i := range nationalPriceColumns { // bounded by len(nationalPriceColumns)
			key, ok := norm[nationalPriceColumns[i]]
			if !ok {
				continue
			}
			if raw := strings.TrimSpace(row[key]); raw != "" {
				out["national"] = parseCap(raw)
				break
			}
		}
	}
	return out
}

// parseCap returns a fixed cap, or nil for a quotable item. A blank cell, a
// non-numeric cell, or one containing "quote" is treated as quotable (nil).
func parseCap(raw string) *float64 {
	if raw == "" {
		return nil
	}
	if strings.Contains(strings.ToLower(raw), "quote") {
		return nil
	}
	v := importer.ParseFloat(raw)
	if v <= 0 {
		return nil // unparseable/zero → quotable
	}
	return &v
}

// normaliseHeader lower-cases a header and collapses internal whitespace so the
// fixed parser tolerates spacing/case noise in the source spreadsheet.
func normaliseHeader(h string) string {
	return strings.Join(strings.Fields(strings.ToLower(h)), " ")
}
