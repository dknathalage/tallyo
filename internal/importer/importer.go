// Package importer parses catalog files (CSV/XLSX), applies a column mapping,
// diffs the result against the existing catalog by SKU, and commits new and
// updated items. It mirrors the legacy TypeScript import flow (parse → map →
// diff → commit) but is pure-Go (encoding/csv + excelize, both cgo-free).
package importer

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

// MappedRow is one catalog row after the column mapping has been applied.
type MappedRow struct {
	Name      string             `json:"name"`
	Sku       string             `json:"sku"`
	Unit      string             `json:"unit"`
	Category  string             `json:"category"`
	Rate      float64            `json:"rate"`
	Metadata  map[string]string  `json:"metadata"`
	TierRates map[string]float64 `json:"tierRates"`
}

// RowError records a row that could not be mapped (1-based index into the input
// rows) along with a human-readable reason.
type RowError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

// UpdatedItem pairs an existing catalog item with the incoming row that differs
// from it.
type UpdatedItem struct {
	Existing *repository.CatalogItem `json:"existing"`
	Incoming MappedRow               `json:"incoming"`
}

// Summary is the diff's count breakdown.
type Summary struct {
	Total     int `json:"total"`
	New       int `json:"new"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Errors    int `json:"errors"`
}

// DiffResult is the outcome of diffing mapped rows against the catalog.
type DiffResult struct {
	New            []MappedRow   `json:"new"`
	Updated        []UpdatedItem `json:"updated"`
	UnchangedCount int           `json:"unchangedCount"`
	Summary        Summary       `json:"summary"`
}

// CommitResult reports how many items were inserted and updated.
type CommitResult struct {
	Inserted int    `json:"inserted"`
	Updated  int    `json:"updated"`
	BatchID  string `json:"batchId"`
}

// Mapping is the transient, per-import column mapping built from the request —
// there is no persisted mapping. Fields maps header -> name|sku|unit|category|
// rate. TierCols maps a header -> the tier NAME its values feed (only columns
// the user kept). FileType/SheetName/HeaderRow steer parsing.
type Mapping struct {
	Fields    map[string]string `json:"fields"`
	TierCols  map[string]string `json:"tierCols"`
	FileType  string            `json:"fileType"`
	SheetName string            `json:"sheetName"`
	HeaderRow int               `json:"headerRow"`
}

// ParseRows reads a CSV or XLSX file into headers plus a slice of
// header→cell maps. headerRow is 1-based; the row at index headerRow-1 is the
// header and subsequent rows become data. A headerRow < 1 defaults to 1.
func ParseRows(data []byte, fileType, sheetName string, headerRow int) ([]string, []map[string]string, error) {
	if len(data) == 0 {
		return nil, nil, fmt.Errorf("importer.ParseRows: empty file")
	}
	if headerRow < 1 {
		headerRow = 1
	}
	if strings.EqualFold(fileType, "xlsx") {
		return parseXLSX(data, sheetName, headerRow)
	}
	return parseCSV(data, headerRow)
}

// parseCSV parses CSV bytes with a variable field count per record.
func parseCSV(data []byte, headerRow int) ([]string, []map[string]string, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	var records [][]string
	for { // bounded by the file's line count
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("importer.parseCSV: %w", err)
		}
		records = append(records, rec)
	}
	return rowsFromRecords(records, headerRow)
}

// parseXLSX parses XLSX bytes, choosing sheetName or the first sheet.
func parseXLSX(data []byte, sheetName string, headerRow int) ([]string, []map[string]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("importer.parseXLSX: open: %w", err)
	}
	defer func() { _ = f.Close() }()

	sheet := sheetName
	if sheet == "" {
		sheets := f.GetSheetList()
		if len(sheets) == 0 {
			return nil, nil, fmt.Errorf("importer.parseXLSX: workbook has no sheets")
		}
		sheet = sheets[0]
	}
	records, err := f.GetRows(sheet)
	if err != nil {
		return nil, nil, fmt.Errorf("importer.parseXLSX: get rows: %w", err)
	}
	return rowsFromRecords(records, headerRow)
}

// rowsFromRecords turns a 2-D record slice into headers plus header→cell maps.
// Data cells are padded/truncated to the header length.
func rowsFromRecords(records [][]string, headerRow int) ([]string, []map[string]string, error) {
	if len(records) < headerRow {
		return nil, nil, fmt.Errorf("importer: file has fewer than %d rows", headerRow)
	}
	headers := records[headerRow-1]
	if len(headers) == 0 {
		return nil, nil, fmt.Errorf("importer: header row is empty")
	}
	out := make([]map[string]string, 0, len(records)-headerRow)
	for i := headerRow; i < len(records); i++ { // bounded by len(records)
		rec := records[i]
		row := make(map[string]string, len(headers))
		for j := range headers { // bounded by len(headers)
			if j < len(rec) {
				row[headers[j]] = rec[j]
			} else {
				row[headers[j]] = ""
			}
		}
		out = append(out, row)
	}
	return headers, out, nil
}

// ApplyMapping applies a transient mapping to raw rows, producing mapped rows
// and per-row errors. A row missing a name is skipped and reported. Bad rate
// values coerce to 0 rather than failing the row.
func ApplyMapping(rows []map[string]string, m Mapping) ([]MappedRow, []RowError, error) {
	if m.Fields == nil {
		return nil, nil, fmt.Errorf("importer.ApplyMapping: nil fields")
	}
	mapped := make([]MappedRow, 0, len(rows))
	var errs []RowError
	for i := range rows { // bounded by len(rows)
		row := buildMappedRow(rows[i], m.Fields, m.TierCols)
		if row.Name == "" {
			errs = append(errs, RowError{Row: i + 1, Message: "name is required"})
			continue
		}
		mapped = append(mapped, row)
	}
	return mapped, errs, nil
}

// buildMappedRow applies the field + tier maps to a single raw row.
func buildMappedRow(raw map[string]string, fields, tierCols map[string]string) MappedRow {
	// Metadata is reserved; no column mapping populates it yet.
	out := MappedRow{Metadata: map[string]string{}, TierRates: map[string]float64{}}
	for header, field := range fields { // bounded by len(fields)
		val := strings.TrimSpace(raw[header])
		switch field {
		case "name":
			out.Name = val
		case "sku":
			out.Sku = val
		case "unit":
			out.Unit = val
		case "category":
			out.Category = val
		case "rate":
			out.Rate = parseFloat(val)
		}
	}
	for header, tierName := range tierCols { // bounded by len(tierCols)
		if val := strings.TrimSpace(raw[header]); val != "" {
			out.TierRates[tierName] = parseFloat(val)
		}
	}
	return out
}

// Diff compares mapped rows against the existing catalog, keyed by trimmed,
// lower-cased SKU. errCount is folded into the summary's Errors count.
func Diff(ctx context.Context, catalog *repository.CatalogRepo, mapped []MappedRow, errCount int) (DiffResult, error) {
	if catalog == nil {
		return DiffResult{}, fmt.Errorf("importer.Diff: nil catalog")
	}
	existing, err := catalog.List(ctx)
	if err != nil {
		return DiffResult{}, fmt.Errorf("importer.Diff: list: %w", err)
	}
	bySku := make(map[string]*repository.CatalogItem, len(existing))
	for _, it := range existing { // bounded by len(existing)
		key := strings.ToLower(strings.TrimSpace(it.Sku))
		if key != "" {
			bySku[key] = it
		}
	}

	res := DiffResult{New: []MappedRow{}, Updated: []UpdatedItem{}}
	for _, row := range mapped { // bounded by len(mapped)
		key := strings.ToLower(strings.TrimSpace(row.Sku))
		var match *repository.CatalogItem
		if key != "" {
			match = bySku[key]
		}
		if match == nil {
			res.New = append(res.New, row)
			continue
		}
		if differs(match, row) {
			res.Updated = append(res.Updated, UpdatedItem{Existing: match, Incoming: row})
		} else {
			res.UnchangedCount++
		}
	}
	res.Summary = Summary{
		Total:     len(mapped),
		New:       len(res.New),
		Updated:   len(res.Updated),
		Unchanged: res.UnchangedCount,
		Errors:    errCount,
	}
	return res, nil
}

// differs reports whether the incoming row changes any compared field.
func differs(existing *repository.CatalogItem, row MappedRow) bool {
	return existing.Name != row.Name ||
		existing.Rate != row.Rate ||
		existing.Unit != row.Unit ||
		existing.Category != row.Category
}

// Commit inserts new items and (when updateExisting) updates changed items,
// resolving each tier name to an existing rate tier or creating it. A fresh
// batch id is returned for tracing.
func Commit(ctx context.Context, catalog *repository.CatalogRepo, tiers *repository.RateTiersRepo, diff DiffResult, updateExisting bool) (CommitResult, error) {
	if catalog == nil || tiers == nil {
		return CommitResult{}, fmt.Errorf("importer.Commit: nil dependency")
	}
	resolver := newTierResolver(tiers)
	batchID := uuid.NewString()
	inserted := 0
	for _, row := range diff.New { // bounded by len(diff.New)
		item, err := catalog.Create(ctx, repository.CatalogItemInput{
			Name:     row.Name,
			Rate:     row.Rate,
			Unit:     row.Unit,
			Category: row.Category,
			Sku:      row.Sku,
			Metadata: metadataJSON(row.Metadata),
		})
		if err != nil {
			return CommitResult{}, fmt.Errorf("importer.Commit: create %q: %w", row.Name, err)
		}
		if err := applyTierRates(ctx, catalog, resolver, item.ID, row.TierRates); err != nil {
			return CommitResult{}, err
		}
		inserted++
	}

	updated := 0
	if updateExisting {
		for _, u := range diff.Updated { // bounded by len(diff.Updated)
			item, err := catalog.Update(ctx, u.Existing.ID, repository.CatalogItemInput{
				Name:     u.Incoming.Name,
				Rate:     u.Incoming.Rate,
				Unit:     u.Incoming.Unit,
				Category: u.Incoming.Category,
				Sku:      u.Incoming.Sku,
				Metadata: metadataJSON(u.Incoming.Metadata),
			})
			if err != nil {
				return CommitResult{}, fmt.Errorf("importer.Commit: update %d: %w", u.Existing.ID, err)
			}
			if item == nil {
				continue
			}
			if err := applyTierRates(ctx, catalog, resolver, item.ID, u.Incoming.TierRates); err != nil {
				return CommitResult{}, err
			}
			updated++
		}
	}
	return CommitResult{Inserted: inserted, Updated: updated, BatchID: batchID}, nil
}

// tierResolver caches tier name -> id, creating tiers on first use.
type tierResolver struct {
	tiers  *repository.RateTiersRepo
	byName map[string]int64
}

// newTierResolver seeds the cache lazily; ids are looked up on demand.
func newTierResolver(tiers *repository.RateTiersRepo) *tierResolver {
	return &tierResolver{tiers: tiers, byName: map[string]int64{}}
}

// resolve returns the id for a tier name, loading existing tiers once and
// creating the tier (audited) if absent. Lookup is case-insensitive.
func (tr *tierResolver) resolve(ctx context.Context, name string) (int64, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return 0, fmt.Errorf("importer: empty tier name")
	}
	if len(tr.byName) == 0 {
		existing, err := tr.tiers.List(ctx)
		if err != nil {
			return 0, fmt.Errorf("importer: list tiers: %w", err)
		}
		for _, t := range existing { // bounded by len(existing)
			tr.byName[strings.ToLower(strings.TrimSpace(t.Name))] = t.ID
		}
	}
	if id, ok := tr.byName[key]; ok {
		return id, nil
	}
	created, err := tr.tiers.Create(ctx, repository.RateTierInput{Name: strings.TrimSpace(name)})
	if err != nil {
		return 0, fmt.Errorf("importer: create tier %q: %w", name, err)
	}
	tr.byName[key] = created.ID
	return created.ID, nil
}

// applyTierRates resolves each tier name to an id and writes its override.
func applyTierRates(ctx context.Context, catalog *repository.CatalogRepo, resolver *tierResolver, itemID int64, rates map[string]float64) error {
	for name, rate := range rates { // bounded by len(rates)
		tierID, err := resolver.resolve(ctx, name)
		if err != nil {
			return err
		}
		if err := catalog.SetRate(ctx, itemID, tierID, rate); err != nil {
			return fmt.Errorf("importer: set rate item=%d tier=%q: %w", itemID, name, err)
		}
	}
	return nil
}

// metadataJSON marshals a metadata map to a JSON object string, defaulting to
// "{}" when empty or on error.
func metadataJSON(m map[string]string) string {
	if len(m) == 0 {
		return "{}"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// parseFloat strips currency/grouping noise and parses a float, returning 0 for
// blank or unparseable input (matching the legacy lenient behaviour).
func parseFloat(s string) float64 {
	cleaned := strings.NewReplacer("$", "", ",", "", " ", "").Replace(s)
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return 0
	}
	v, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0
	}
	return v
}
