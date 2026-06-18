package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/dknathalage/tallyo/internal/importer"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
)

// SupportCatalogService exposes read access to the GLOBAL NDIS Support
// Catalogue (catalog_versions / support_items / support_item_prices). It is NOT
// tenant-scoped: the catalogue is shared reference data (spec §3.1/§4.3).
//
// Platform-admin write access (XLSX ingest) lives in CatalogIngestService below.
type SupportCatalogService struct {
	repo *repository.CatalogRepo
}

func NewSupportCatalogService(db *sql.DB) *SupportCatalogService {
	return &SupportCatalogService{repo: repository.NewCatalog(db)}
}

// ListVersions returns all catalogue versions.
func (s *SupportCatalogService) ListVersions(ctx context.Context) ([]*repository.CatalogVersion, error) {
	return s.repo.ListVersions(ctx)
}

// GetVersion returns a catalogue version by id, or (nil, nil) when absent.
func (s *SupportCatalogService) GetVersion(ctx context.Context, id int64) (*repository.CatalogVersion, error) {
	return s.repo.GetVersion(ctx, id)
}

// ListSupportItems returns the support items in a catalogue version.
func (s *SupportCatalogService) ListSupportItems(ctx context.Context, versionID int64) ([]*repository.SupportItem, error) {
	return s.repo.ListSupportItems(ctx, versionID)
}

// ListPrices returns the zone prices for a support item.
func (s *SupportCatalogService) ListPrices(ctx context.Context, supportItemID int64) ([]*repository.SupportItemPrice, error) {
	return s.repo.ListPrices(ctx, supportItemID)
}

// CatalogMatch is one support item resolved for a service date, enriched with
// its price cap in the requested zone. PriceCap is nil for a quotable item (no
// fixed cap) or when no price row exists for the zone.
type CatalogMatch struct {
	Code      string   `json:"code"`
	Name      string   `json:"name"`
	Unit      string   `json:"unit"`
	GstFree   bool     `json:"gstFree"`
	Zone      string   `json:"zone"`
	PriceCap  *float64 `json:"priceCap"`
	Quotable  bool     `json:"quotable"`
	VersionID int64    `json:"catalogVersionId"`
}

// SearchForDate resolves the catalogue version effective on serviceDate, finds
// support items whose code or name matches query, and attaches each item's
// price cap for the given zone (default "national" when zone is empty). Returns
// an empty (non-nil) slice when no version is in effect or nothing matches —
// capped at limit results (limit ≤ 0 → a default of 25) to bound the payload.
func (s *SupportCatalogService) SearchForDate(ctx context.Context, query, serviceDate, zone string, limit int) ([]*CatalogMatch, error) {
	if serviceDate == "" {
		return nil, fmt.Errorf("catalogue search: service date is required")
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
	out := make([]*CatalogMatch, 0)
	if ver == nil {
		return out, nil
	}
	items, err := s.repo.SearchSupportItems(ctx, ver.ID, query)
	if err != nil {
		return nil, err
	}
	for i := range items { // bounded by len(items)
		if len(out) >= limit {
			break
		}
		it := items[i]
		m := &CatalogMatch{
			Code: it.Code, Name: it.Name, Unit: it.Unit, GstFree: it.GstFree,
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

// CatalogIngestService is the platform-admin WRITE path for the GLOBAL NDIS
// Support Catalogue: it parses an official Support Catalogue XLSX (fixed-format,
// keyed to known NDIA headers — no column-mapping wizard) and bulk-loads a new
// catalog_version + support_items + per-zone prices in one transaction (spec §5).
// It is NOT tenant-scoped.
type CatalogIngestService struct {
	repo *repository.CatalogRepo
	hub  *realtime.Hub
}

// NewCatalogIngestService constructs the ingest service. A nil hub is a
// programmer error (the ingest must broadcast after commit).
func NewCatalogIngestService(db *sql.DB, hub *realtime.Hub) *CatalogIngestService {
	if hub == nil {
		panic("NewCatalogIngestService: nil hub")
	}
	return &CatalogIngestService{repo: repository.NewCatalog(db), hub: hub}
}

// IngestSummary is the JSON-friendly result of a catalogue ingest.
type IngestSummary struct {
	VersionID     int64  `json:"versionId"`
	VersionUUID   string `json:"versionUuid"`
	Label         string `json:"label"`
	EffectiveFrom string `json:"effectiveFrom"`
	ItemCount     int    `json:"itemCount"`
	PriceCount    int    `json:"priceCount"`
}

// Canonical NDIS Support Catalogue column headers (normalised: lower-cased,
// internal whitespace collapsed). The official export uses these exact labels;
// adjust here if NDIA renames a column. The geographic price-limit columns map
// to our three zones.
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

// IngestXLSX parses fixed-format NDIS Support Catalogue XLSX bytes and loads a
// new catalogue version. The WHOLE upload is rejected (no partial state) when a
// required column is missing or zero data rows parse. Broadcasts an SSE event
// AFTER the commit succeeds (spec §5). Catalogue is GLOBAL so the event is
// broadcast to all subscribers.
func (s *CatalogIngestService) IngestXLSX(ctx context.Context, data []byte, label, effectiveFrom, sourceFilename string) (*IngestSummary, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("ingest: empty file")
	}
	if label == "" {
		return nil, fmt.Errorf("ingest: label required")
	}
	if effectiveFrom == "" {
		return nil, fmt.Errorf("ingest: effective_from required")
	}

	headers, rows, err := importer.ParseRows(data, "xlsx", "", 1)
	if err != nil {
		return nil, fmt.Errorf("ingest: parse: %w", err)
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
			return nil, fmt.Errorf("ingest: missing required column %q", required[i])
		}
	}

	items, err := buildIngestItems(rows, norm)
	if err != nil {
		return nil, fmt.Errorf("ingest: %w", err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("ingest: no data rows parsed")
	}

	res, err := s.repo.Ingest(ctx, label, effectiveFrom, sourceFilename, items)
	if err != nil {
		return nil, err
	}

	// The NDIS Support Catalogue is GLOBAL shared reference data (spec §4.3) with
	// no owning tenant: broadcast with the GlobalTenantID sentinel so the event
	// reaches every tenant's open SSE stream, not just one tenant's.
	s.hub.Broadcast(realtime.Event{TenantID: realtime.GlobalTenantID, Entity: "catalog_version", ID: res.Version.ID, Action: "ingest"})
	return &IngestSummary{
		VersionID:     res.Version.ID,
		VersionUUID:   res.Version.UUID,
		Label:         res.Version.Label,
		EffectiveFrom: res.Version.EffectiveFrom,
		ItemCount:     res.ItemCount,
		PriceCount:    res.PriceCount,
	}, nil
}

// buildIngestItems maps parsed rows to IngestItem values, skipping rows with a
// blank support-item code. Bounded by len(rows).
func buildIngestItems(rows []map[string]string, norm map[string]string) ([]repository.IngestItem, error) {
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
	out := make([]repository.IngestItem, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		row := rows[i]
		code := cell(row, colCode)
		if code == "" {
			continue // skip blank/spacer rows
		}
		it := repository.IngestItem{
			Code:              code,
			Name:              cell(row, colName),
			Unit:              cell(row, colUnit),
			SupportCategory:   cell(row, colCategory),
			RegistrationGroup: cell(row, colRegGroup),
			Prices:            zonePrices(row, norm),
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
