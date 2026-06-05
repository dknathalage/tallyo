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
	Name      string            `json:"name"`
	Sku       string            `json:"sku"`
	Unit      string            `json:"unit"`
	Category  string            `json:"category"`
	Rate      float64           `json:"rate"`
	Metadata  map[string]string `json:"metadata"`
	TierRates map[int64]float64 `json:"tierRates"`
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

// metadataMapEntry is one {header,key} pair in a column mapping's
// MetadataMapping JSON array.
type metadataMapEntry struct {
	Header string `json:"header"`
	Key    string `json:"key"`
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

// ApplyMapping applies a column mapping to raw rows, producing mapped rows and a
// list of per-row errors. A row missing a name is skipped and reported as an
// error. Bad rate values coerce to 0 rather than failing the row.
func ApplyMapping(rows []map[string]string, m *repository.ColumnMapping) ([]MappedRow, []RowError, error) {
	if m == nil {
		return nil, nil, fmt.Errorf("importer.ApplyMapping: nil mapping")
	}
	fieldMap, err := parseFieldMap(m.Mapping)
	if err != nil {
		return nil, nil, err
	}
	tierMap, err := parseTierMap(m.TierMapping)
	if err != nil {
		return nil, nil, err
	}
	metaMap, err := parseMetadataMap(m.MetadataMapping)
	if err != nil {
		return nil, nil, err
	}

	mapped := make([]MappedRow, 0, len(rows))
	var errs []RowError
	for i := range rows { // bounded by len(rows)
		row := buildMappedRow(rows[i], fieldMap, tierMap, metaMap)
		if row.Name == "" {
			errs = append(errs, RowError{Row: i + 1, Message: "name is required"})
			continue
		}
		mapped = append(mapped, row)
	}
	return mapped, errs, nil
}

// buildMappedRow applies the parsed maps to a single raw row.
func buildMappedRow(raw map[string]string, fieldMap map[string]string, tierMap map[string]int64, metaMap []metadataMapEntry) MappedRow {
	out := MappedRow{Metadata: map[string]string{}, TierRates: map[int64]float64{}}
	for header, field := range fieldMap {
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
	for header, tierID := range tierMap {
		if val := strings.TrimSpace(raw[header]); val != "" {
			out.TierRates[tierID] = parseFloat(val)
		}
	}
	for _, e := range metaMap {
		if val := strings.TrimSpace(raw[e.Header]); val != "" {
			out.Metadata[e.Key] = val
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
// applying per-tier rates for both. A fresh batch id is returned for tracing.
func Commit(ctx context.Context, catalog *repository.CatalogRepo, diff DiffResult, updateExisting bool) (CommitResult, error) {
	if catalog == nil {
		return CommitResult{}, fmt.Errorf("importer.Commit: nil catalog")
	}
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
		if err := applyTierRates(ctx, catalog, item.ID, row.TierRates); err != nil {
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
			if err := applyTierRates(ctx, catalog, item.ID, u.Incoming.TierRates); err != nil {
				return CommitResult{}, err
			}
			updated++
		}
	}
	return CommitResult{Inserted: inserted, Updated: updated, BatchID: batchID}, nil
}

// applyTierRates writes each per-tier override for an item.
func applyTierRates(ctx context.Context, catalog *repository.CatalogRepo, itemID int64, rates map[int64]float64) error {
	for tierID, rate := range rates { // bounded by len(rates)
		if err := catalog.SetRate(ctx, itemID, tierID, rate); err != nil {
			return fmt.Errorf("importer: set rate item=%d tier=%d: %w", itemID, tierID, err)
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

// parseFieldMap parses the header→field JSON object. An empty string is "{}".
func parseFieldMap(s string) (map[string]string, error) {
	if strings.TrimSpace(s) == "" {
		return map[string]string{}, nil
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, fmt.Errorf("importer: parse mapping: %w", err)
	}
	return out, nil
}

// parseTierMap parses the header→tierId(string) JSON object into int64 ids.
func parseTierMap(s string) (map[string]int64, error) {
	if strings.TrimSpace(s) == "" {
		return map[string]int64{}, nil
	}
	var raw map[string]string
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil, fmt.Errorf("importer: parse tier mapping: %w", err)
	}
	out := make(map[string]int64, len(raw))
	for header, idStr := range raw {
		id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("importer: tier id %q: %w", idStr, err)
		}
		out[header] = id
	}
	return out, nil
}

// parseMetadataMap parses the JSON array of {header,key} entries.
func parseMetadataMap(s string) ([]metadataMapEntry, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []metadataMapEntry
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, fmt.Errorf("importer: parse metadata mapping: %w", err)
	}
	return out, nil
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
