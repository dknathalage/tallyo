# Catalog Import Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the saved-mapping catalog import with an inline, auto-detected mapping done at import time, folded into the Catalog section, with per-price-column tier selection (existing / create-new / ignore).

**Architecture:** A new pure-Go `DetectMapping` infers field + price-column mapping from headers and sampled values. The mapping becomes a transient request value (no DB persistence); tier rates are keyed by tier **name** and resolved (existing-or-create) at commit. The `column_mappings` table and its whole stack are removed. Import routes move under `/api/catalog/import/*`; the SPA gets a `/catalog/import` wizard.

**Tech Stack:** Go 1.26 (chi, sqlc, goose, modernc sqlite, excelize), SvelteKit SPA (Svelte 5 runes, Tailwind 4).

**Spec:** `docs/superpowers/specs/2026-06-05-catalog-import-redesign-design.md`

---

## File Structure

**Created**
- `internal/importer/detection.go` — `DetectMapping`, `Suggestion`, `PriceColumn`, synonym table, currency sniffing.
- `internal/importer/detection_test.go` — detection table tests.
- `internal/db/migrations/00010_drop_column_mappings.sql` — drop the table (down re-creates it).
- `web/src/routes/catalog/import/+page.svelte` — the 3-step import wizard.

**Modified**
- `internal/importer/importer.go` — transient `Mapping`; `ApplyMapping(rows, Mapping)`; `MappedRow.TierRates map[string]float64`; `Commit(... rateTiers, ...)` resolving tier names.
- `internal/importer/importer_test.go` — update `ApplyMapping`/`Commit` call sites.
- `internal/http/import.go` — inline mapping from request JSON; add `Parse`; drop `mappings` dep; pass `RateTiersRepo` to `Commit`.
- `internal/http/server.go` — move import routes under `/catalog/import`, add `/parse`, delete `column-mappings` routes, drop `ColumnMappings` from `Deps`.
- `main.go` — drop column-mapping construction; update `NewImportHandler` args.
- `web/src/routes/+layout.svelte` — nav: drop "Mappings"; point "Import" at `/catalog/import` (or remove and link from catalog).
- `web/src/routes/catalog/+page.svelte` — add an "Import" button linking to `/catalog/import`.
- `web/src/lib/api/types.ts` — remove `ColumnMapping`/`ColumnMappingInput`; add import DTOs.

**Deleted**
- `internal/http/column_mappings.go`, `internal/http/column_mappings_test.go`, `internal/service/column_mapping.go`, `internal/repository/column_mapping.go`, `internal/repository/column_mapping_test.go`, `internal/db/queries/column_mappings.sql`.
- `web/src/routes/column-mappings/` (whole dir), `web/src/lib/stores/columnMappings.svelte.ts`, `web/src/routes/import/` (old wizard, replaced).

**Existing tests that MUST be updated (they reference the old API / dropped table — found by review):**
- `internal/importer/importer_test.go` — old `Commit` arity + `map[int64]` tier keys (Task 2).
- `internal/http/import_test.go` — built entirely on the `mappingId` API (Task 3, full rewrite).
- `internal/db/migrate_test.go` — `TestMigrateCreatesColumnMappings` asserts the table exists; must be removed/inverted (Task 4).

---

## Task 1: Auto-detection (`DetectMapping`)

**Files:**
- Create: `internal/importer/detection.go`
- Test: `internal/importer/detection_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/importer/detection_test.go`:

```go
package importer

import "testing"

func sampleRows(headers []string, rows ...[]string) []map[string]string {
	out := make([]map[string]string, 0, len(rows))
	for _, r := range rows {
		m := map[string]string{}
		for i, h := range headers {
			if i < len(r) {
				m[h] = r[i]
			}
		}
		out = append(out, m)
	}
	return out
}

func TestDetectMappingNDISStyle(t *testing.T) {
	headers := []string{"Support Item Number", "Support Item Name", "Support Category Name", "Unit", "ACT", "Remote", "Very Remote"}
	sample := sampleRows(headers,
		[]string{"01_011_0107_1_1", "Assistance With Self-Care", "Daily Activities", "H", "$67.56", "$94.58", "$101.34"},
	)
	s := DetectMapping(headers, sample)
	if s.Fields["Support Item Number"] != "sku" {
		t.Errorf("sku: got %q", s.Fields["Support Item Number"])
	}
	if s.Fields["Support Item Name"] != "name" {
		t.Errorf("name: got %q", s.Fields["Support Item Name"])
	}
	if s.Fields["Support Category Name"] != "category" {
		t.Errorf("category: got %q", s.Fields["Support Category Name"])
	}
	if s.Fields["Unit"] != "unit" {
		t.Errorf("unit: got %q", s.Fields["Unit"])
	}
	if s.BaseHeader != "ACT" {
		t.Errorf("base: got %q want ACT (leftmost price col)", s.BaseHeader)
	}
	gotTiers := map[string]bool{}
	for _, p := range s.PriceCols {
		gotTiers[p.Header] = true
		if p.SuggestName != p.Header {
			t.Errorf("suggest name: got %q want %q", p.SuggestName, p.Header)
		}
	}
	if !gotTiers["Remote"] || !gotTiers["Very Remote"] || gotTiers["ACT"] {
		t.Errorf("tiers: got %v", gotTiers)
	}
}

func TestDetectMappingGenericPriceAndTier(t *testing.T) {
	headers := []string{"name", "sku", "unit", "price", "premium"}
	sample := sampleRows(headers, []string{"Widget", "W1", "ea", "10.00", "12.50"})
	s := DetectMapping(headers, sample)
	if s.BaseHeader != "price" {
		t.Errorf("base: got %q want price", s.BaseHeader)
	}
	if len(s.PriceCols) != 1 || s.PriceCols[0].Header != "premium" {
		t.Errorf("tiers: got %+v want [premium]", s.PriceCols)
	}
}

func TestDetectMappingCodeColumnIsNotTier(t *testing.T) {
	headers := []string{"name", "category number", "price"}
	sample := sampleRows(headers,
		[]string{"A", "101", "5.00"},
		[]string{"B", "102", "6.00"},
	)
	s := DetectMapping(headers, sample)
	if len(s.PriceCols) != 0 {
		t.Errorf("integer code column must not be a tier: got %+v", s.PriceCols)
	}
	if s.BaseHeader != "price" {
		t.Errorf("base: got %q", s.BaseHeader)
	}
}

func TestDetectMappingMissingName(t *testing.T) {
	headers := []string{"sku", "price"}
	s := DetectMapping(headers, sampleRows(headers, []string{"X1", "9.99"}))
	for _, f := range s.Fields {
		if f == "name" {
			t.Fatalf("no name column should be detected")
		}
	}
	if s.BaseHeader != "price" {
		t.Errorf("base: got %q", s.BaseHeader)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/importer/ -run TestDetectMapping -v`
Expected: FAIL — `undefined: DetectMapping`.

- [ ] **Step 3: Implement `detection.go`**

`internal/importer/detection.go`:

```go
package importer

import (
	"regexp"
	"strings"
)

// Suggestion is the auto-detected mapping proposal from DetectMapping. Fields
// maps a source header to one of name|sku|unit|category|rate. BaseHeader is the
// header chosen as the base catalog rate ("" if none). PriceCols are the other
// detected price columns, in original header order, each proposable as a tier.
type Suggestion struct {
	Fields     map[string]string `json:"fields"`
	BaseHeader string            `json:"baseHeader"`
	PriceCols  []PriceColumn     `json:"priceCols"`
}

// PriceColumn is a non-base price column the user can map to a tier.
type PriceColumn struct {
	Header      string `json:"header"`
	SuggestName string `json:"suggestName"`
}

// fieldSynonyms lists, per target field, the token-sequences that identify a
// header. Order matters: fields are claimed top-down, each at most once, and a
// header is consumed by the first field that claims it. Needles are matched as
// contiguous token sub-sequences (so "id" never matches "video").
var fieldSynonyms = []struct {
	field   string
	needles []string
}{
	{"sku", []string{"support item number", "item number", "item code", "sku", "code", "ref"}},
	{"name", []string{"support item name", "item name", "name", "description", "product", "service"}},
	{"category", []string{"support category", "category", "group", "class", "type"}},
	{"unit", []string{"unit of measure", "unit", "uom"}},
	{"rate", []string{"unit price", "base price", "price limit", "price", "rate", "cost", "amount"}},
}

// priceHeaderNeedles flag a header as price-like by name alone.
var priceHeaderNeedles = []string{"price", "rate", "cost", "amount", "cap", "limit", "fee"}

var currencyRe = regexp.MustCompile(`^\$?\s*\d{1,3}(,\d{3})*(\.\d{1,2})?$|^\$?\s*\d+\.\d{1,2}$`)

// tokens normalizes a header to lower-case alphanumeric words.
func tokens(h string) []string {
	var b strings.Builder
	for _, r := range strings.ToLower(h) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Fields(b.String())
}

// containsSeq reports whether needle's tokens appear contiguously in hay.
func containsSeq(hay, needle []string) bool {
	if len(needle) == 0 || len(needle) > len(hay) {
		return false
	}
	for i := 0; i+len(needle) <= len(hay); i++ { // bounded by len(hay)
		match := true
		for j := range needle { // bounded by len(needle)
			if hay[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// matchField returns the field a header maps to, given already-claimed fields.
func matchField(header string, claimed map[string]bool) string {
	hay := tokens(header)
	for _, fs := range fieldSynonyms { // bounded by len(fieldSynonyms)
		if claimed[fs.field] {
			continue
		}
		for _, n := range fs.needles { // bounded by len(needles)
			if containsSeq(hay, tokens(n)) {
				return fs.field
			}
		}
	}
	return ""
}

// priceLikeByName reports whether the header name alone signals a price column.
func priceLikeByName(header string) bool {
	hay := tokens(header)
	for _, n := range priceHeaderNeedles { // bounded by len(priceHeaderNeedles)
		if containsSeq(hay, tokens(n)) {
			return true
		}
	}
	return false
}

// priceLikeByValue reports whether the majority of sampled non-empty cells for a
// header parse as currency with a decimal/$, excluding integer-only code columns.
func priceLikeByValue(header string, sample []map[string]string) bool {
	nonEmpty, currency := 0, 0
	for _, row := range sample { // bounded by len(sample)
		v := strings.TrimSpace(row[header])
		if v == "" {
			continue
		}
		nonEmpty++
		if (strings.Contains(v, ".") || strings.Contains(v, "$")) && currencyRe.MatchString(v) {
			currency++
		}
	}
	return nonEmpty > 0 && currency*2 > nonEmpty
}

// DetectMapping infers a Suggestion from headers plus up to a handful of sample
// rows. It claims one header per field, then treats remaining price-like columns
// as the base rate (a rate-named column, else the left-most) plus tier columns.
func DetectMapping(headers []string, sample []map[string]string) Suggestion {
	s := Suggestion{Fields: map[string]string{}, PriceCols: []PriceColumn{}}
	claimed := map[string]bool{}
	rateHeader := ""
	for _, h := range headers { // bounded by len(headers)
		f := matchField(h, claimed)
		if f == "" {
			continue
		}
		claimed[f] = true
		if f == "rate" {
			rateHeader = h
			continue // base rate is tracked separately, not in Fields
		}
		s.Fields[h] = f
	}

	var priceCols []string
	for _, h := range headers { // bounded by len(headers)
		if _, taken := s.Fields[h]; taken || h == rateHeader {
			continue
		}
		if priceLikeByName(h) || priceLikeByValue(h, sample) {
			priceCols = append(priceCols, h)
		}
	}

	s.BaseHeader = rateHeader
	if s.BaseHeader == "" && len(priceCols) > 0 {
		s.BaseHeader = priceCols[0]
		priceCols = priceCols[1:]
	}
	if rateHeader != "" {
		s.Fields[rateHeader] = "rate"
	} else if s.BaseHeader != "" {
		s.Fields[s.BaseHeader] = "rate"
	}
	for _, h := range priceCols { // bounded by len(priceCols)
		s.PriceCols = append(s.PriceCols, PriceColumn{Header: h, SuggestName: h})
	}
	return s
}
```

> Note: `Fields` carries the base-rate header as `"rate"` so the client renders it pre-selected; `BaseHeader` names it explicitly for convenience.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/importer/ -run TestDetectMapping -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/importer/detection.go internal/importer/detection_test.go
git commit -m "feat(importer): auto-detect catalog column mapping from headers + values"
```

---

## Task 2: Transient mapping, name-keyed tiers, create-if-missing commit

**Files:**
- Modify: `internal/importer/importer.go`
- Modify: `internal/importer/importer_test.go`

- [ ] **Step 1: Update the importer tests first (red)**

In `internal/importer/importer_test.go` — this package is `importer` (same package, no `importer.` qualifier on calls). **Every one of these existing sites must change or the test package won't compile** (grep `ColumnMapping`, `ApplyMapping(`, `Commit(`, `TierRates` to find them all):

- `TestApplyMapping`, `TestApplyMappingMissingName`, `TestApplyMappingTiersAndMetadata` (~lines 70-139) construct `m := &repository.ColumnMapping{Mapping: ..., TierMapping: ..., MetadataMapping: ...}` and call `ApplyMapping(rows, m)`. The tier test asserts `mapped[0].TierRates[1]` (int64 key) and `mapped[0].Metadata["color"]`.
- `TestCommit`, `TestCommitSkipUpdatesWhenDisabled` (~lines 215, 245) call `Commit(t.Context(), cat, diff, true/false)` (old 3-arg) and build `TierRates: map[int64]float64{1: ...}` (~line 209).

Apply these changes:

1. Replace every `&repository.ColumnMapping{...}` + `ApplyMapping(rows, m)` with the transient `Mapping` (note tier mapping is header→**name**, and metadata mapping is **gone** — drop the metadata assertions, since the new wizard has no metadata mapping):

```go
m := Mapping{
	Fields:   map[string]string{"Name": "name", "SKU": "sku", "Price": "rate"},
	TierCols: map[string]string{"Gold Price": "Gold"},
}
mapped, errs, err := ApplyMapping(rows, m)
```

2. Change the tier assertion to a **name** key (and delete the `Metadata["color"]` assertion):

```go
if mapped[0].TierRates["Gold"] != 20 {
	t.Fatalf("tier rate by name: %v", mapped[0].TierRates)
}
```

3. Change both existing `Commit(...)` calls to the new 4-arg form and the `TierRates` literals to `map[string]float64`:

```go
res, err := Commit(t.Context(), cat, tiers, diff, true)   // and false in the other test
// ...
TierRates: map[string]float64{"Gold": 20},
```

4. Update the test DB helper to also return a `*repository.RateTiersRepo` (the commit now needs it). Replace `newCatalog`'s callers with `newCatalogAndTiers`, or add this alongside it:

```go
func newCatalogAndTiers(t *testing.T) (*repository.CatalogRepo, *repository.RateTiersRepo) {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "importer.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return repository.NewCatalog(conn), repository.NewRateTiers(conn)
}
```

   Then update `TestCommit` and `TestCommitSkipUpdatesWhenDisabled` to obtain both repos: `cat, tiers := newCatalogAndTiers(t)` and pass `tiers` into `Commit`.

5. Add a commit-creates-tier test:

```go
func TestCommitCreatesTierByName(t *testing.T) {
	cat, tiers := newCatalogAndTiers(t)
	diff := importer.DiffResult{New: []importer.MappedRow{{
		Name: "Item", Sku: "S1", Rate: 10,
		TierRates: map[string]float64{"Remote": 15},
	}}}
	res, err := importer.Commit(context.Background(), cat, tiers, diff, false)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.Inserted != 1 {
		t.Fatalf("inserted: %d", res.Inserted)
	}
	all, _ := tiers.List(context.Background())
	found := false
	for _, tr := range all {
		if tr.Name == "Remote" {
			found = true
		}
	}
	if !found {
		t.Errorf("tier 'Remote' should have been created")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/importer/ -v`
Expected: FAIL — compile errors (`Mapping` undefined, `Commit` arity, `TierRates` type).

- [ ] **Step 3: Change `importer.go`**

Make these edits in `internal/importer/importer.go`:

a) `MappedRow.TierRates` type → name-keyed:

```go
	TierRates map[string]float64 `json:"tierRates"`
```

b) Add the transient `Mapping` type (replaces `*repository.ColumnMapping` arg). Put it above `ApplyMapping`:

```go
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
```

c) Rewrite `ApplyMapping` to use `Mapping` and name-keyed tiers:

```go
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
```

d) Rewrite `buildMappedRow` (drop metadata map; tiers keyed by name):

```go
// buildMappedRow applies the field + tier maps to a single raw row.
func buildMappedRow(raw map[string]string, fields, tierCols map[string]string) MappedRow {
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
```

e) Delete the now-unused parsers: `parseFieldMap`, `parseTierMap`, `parseMetadataMap`, `metadataMapEntry`. (Keep `metadataJSON` and `parseFloat`.)

f) Change `Commit` + `applyTierRates` to take a `*repository.RateTiersRepo` and resolve names:

```go
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
			Name: row.Name, Rate: row.Rate, Unit: row.Unit,
			Category: row.Category, Sku: row.Sku, Metadata: metadataJSON(row.Metadata),
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
				Name: u.Incoming.Name, Rate: u.Incoming.Rate, Unit: u.Incoming.Unit,
				Category: u.Incoming.Category, Sku: u.Incoming.Sku, Metadata: metadataJSON(u.Incoming.Metadata),
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
```

> The `len(tr.byName) == 0` guard re-lists only while the cache is empty; an all-new-tier import lists once then caches. Acceptable for import-sized data.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/importer/ -v`
Expected: PASS (detection + apply + diff + commit-creates-tier).

- [ ] **Step 5: Commit**

```bash
git add internal/importer/importer.go internal/importer/importer_test.go
git commit -m "feat(importer): transient mapping with name-keyed tiers, create-if-missing commit"
```

---

## Task 3: Rework the HTTP import handler (inline mapping + Parse)

**Files:**
- Modify: `internal/http/import.go`
- Rewrite: `internal/http/import_test.go` (already exists, ~200 lines, built on the OLD `mappingId` API — it calls `repository.NewColumnMappings`, `repository.ColumnMappingInput`, the old 2-arg `NewImportHandler(catalogRepo, mappings)`, posts a `mappingId` field, and hits `/import/catalog/preview`. All of that is gone — the file must be rewritten, not appended to.)

- [ ] **Step 1: Rewrite the handler test (red)**

Delete the entire body of `internal/http/import_test.go` (every test references the removed mappings API, including `TestImportPreviewBadMapping` which tests a now-nonexistent failure mode) and replace with tests against the new API. Use the existing http test harness pattern (see `internal/http/catalog_test.go` for how the test server + auth/session are built; reuse the same `newTestServer`-style helper and register `Import: httpapi.NewImportHandler(catalogRepo, rateTiersRepo)`). Cover:

```go
// TestImportParseSuggests: POST multipart {file: "name,sku,price\nWidget,W1,10.00"}
//   to /api/catalog/import/parse → 200, body.suggestion.fields["price"] == "rate".
// TestImportPreviewMissingName: POST /preview with mapping omitting a name field
//   (and a file lacking one) → diff with all-rows-errored, OR 400 — assert the
//   behaviour your ApplyMapping produces (name-less rows are RowErrors, so /preview
//   returns 200 with summary.errors > 0; assert that).
// TestImportCommitCreatesItemAndTier: POST /preview then /commit with mapping
//   {fields:{name,sku,price→rate}, tierCols:{Remote:"Remote"}} → 200, then GET
//   /api/catalog shows the item and /api/rate-tiers shows "Remote".
```

Build the multipart body with `mime/multipart`; set `mapping` as a JSON string field and `file` as a form file. Mirror the auth/session setup other `internal/http` tests use.

- [ ] **Step 2: Rewrite `import.go`**

`internal/http/import.go`:

```go
package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/dknathalage/tallyo/internal/importer"
	"github.com/dknathalage/tallyo/internal/repository"
)

const maxImportUpload = 10 << 20 // 10 MiB

// ImportHandler serves the catalog import parse/preview/commit routes. It holds
// the catalog repo (diff/commit) and rate-tiers repo (tier create-if-missing).
// Routes are auth-gated by the server's RequireAuth group.
type ImportHandler struct {
	catalog *repository.CatalogRepo
	tiers   *repository.RateTiersRepo
}

// NewImportHandler constructs the handler. A nil dependency is a programmer error.
func NewImportHandler(catalog *repository.CatalogRepo, tiers *repository.RateTiersRepo) *ImportHandler {
	if catalog == nil || tiers == nil {
		panic("NewImportHandler: nil dependency")
	}
	return &ImportHandler{catalog: catalog, tiers: tiers}
}

// readUpload reads the multipart "file" field, bounded by maxImportUpload.
func (h *ImportHandler) readUpload(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	if err := r.ParseMultipartForm(maxImportUpload); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid multipart form")
		return nil, false
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "file required")
		return nil, false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxImportUpload))
	if err != nil || len(data) == 0 {
		WriteError(w, http.StatusBadRequest, "file required")
		return nil, false
	}
	return data, true
}

// parseMapping reads the "mapping" multipart field (JSON) into a transient Mapping.
func parseMapping(r *http.Request) (importer.Mapping, error) {
	var m importer.Mapping
	raw := r.FormValue("mapping")
	if raw == "" {
		return m, fmt.Errorf("mapping required")
	}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return m, fmt.Errorf("invalid mapping: %w", err)
	}
	if m.HeaderRow < 1 {
		m.HeaderRow = 1
	}
	return m, nil
}

// Parse parses an uploaded file, samples rows, runs detection, and returns
// headers + sample + a suggested mapping. Writes nothing.
func (h *ImportHandler) Parse(w http.ResponseWriter, r *http.Request) {
	data, ok := h.readUpload(w, r)
	if !ok {
		return
	}
	fileType := r.FormValue("fileType")
	sheet := r.FormValue("sheetName")
	headerRow := 1
	if v := r.FormValue("headerRow"); v != "" {
		fmt.Sscanf(v, "%d", &headerRow)
	}
	headers, rows, err := importer.ParseRows(data, fileType, sheet, headerRow)
	if err != nil {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("parse file: %v", err))
		return
	}
	sample := rows
	if len(sample) > 50 {
		sample = sample[:50]
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"headers":    headers,
		"sample":     sample,
		"suggestion": importer.DetectMapping(headers, sample),
	})
}

// diffFromRequest runs parse → map → diff using the inline mapping.
func (h *ImportHandler) diffFromRequest(w http.ResponseWriter, r *http.Request) (importer.DiffResult, bool) {
	data, ok := h.readUpload(w, r)
	if !ok {
		return importer.DiffResult{}, false
	}
	m, err := parseMapping(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return importer.DiffResult{}, false
	}
	_, rows, err := importer.ParseRows(data, m.FileType, m.SheetName, m.HeaderRow)
	if err != nil {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("parse file: %v", err))
		return importer.DiffResult{}, false
	}
	mapped, rowErrs, err := importer.ApplyMapping(rows, m)
	if err != nil {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("apply mapping: %v", err))
		return importer.DiffResult{}, false
	}
	diff, err := importer.Diff(r.Context(), h.catalog, mapped, len(rowErrs))
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return importer.DiffResult{}, false
	}
	return diff, true
}

// Preview returns the diff without writing anything.
func (h *ImportHandler) Preview(w http.ResponseWriter, r *http.Request) {
	diff, ok := h.diffFromRequest(w, r)
	if !ok {
		return
	}
	WriteJSON(w, http.StatusOK, diff)
}

// Commit re-parses, diffs, and writes new items (+ updated when updateExisting),
// creating any referenced tiers.
func (h *ImportHandler) Commit(w http.ResponseWriter, r *http.Request) {
	diff, ok := h.diffFromRequest(w, r)
	if !ok {
		return
	}
	updateExisting := r.FormValue("updateExisting") == "true"
	res, err := importer.Commit(r.Context(), h.catalog, h.tiers, diff, updateExisting)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, res)
}
```

- [ ] **Step 3: Build the package**

Run: `go build ./internal/http/`
Expected: FAIL only at `server.go`/`main.go` call sites (fixed in Task 4). The handler file itself compiles; if `import_test.go` needs the server helper, finish wiring in Task 4 then return.

- [ ] **Step 4: Commit**

```bash
git add internal/http/import.go internal/http/import_test.go
git commit -m "feat(http): inline import mapping + /parse auto-detect endpoint"
```

---

## Task 4: Remove `column_mappings` + rewire routes

**Files:**
- Create: `internal/db/migrations/00010_drop_column_mappings.sql`
- Delete: `internal/http/column_mappings.go`, `internal/service/column_mapping.go`, `internal/repository/column_mapping.go`, `internal/repository/column_mapping_test.go`, `internal/db/queries/column_mappings.sql`
- Modify: `internal/http/server.go`, `main.go`

- [ ] **Step 1: Add the drop migration**

`internal/db/migrations/00010_drop_column_mappings.sql` — copy the `CREATE TABLE column_mappings (...)` body from `00009_column_mappings.sql` into the Down block verbatim:

```sql
-- +goose Up
DROP TABLE column_mappings;

-- +goose Down
-- (paste the exact CREATE TABLE column_mappings (...) from 00009 here)
```

- [ ] **Step 2: Delete the column-mapping Go files (incl. the http test)**

```bash
git rm internal/http/column_mappings.go internal/http/column_mappings_test.go \
  internal/service/column_mapping.go \
  internal/repository/column_mapping.go internal/repository/column_mapping_test.go \
  internal/db/queries/column_mappings.sql
```

- [ ] **Step 2b: Fix `migrate_test.go`**

`internal/db/migrate_test.go` has `TestMigrateCreatesColumnMappings` (~lines 117-130) asserting the table **exists** after migration. With `00010` it no longer does. Either delete that test, or invert it to assert the table is **absent** after migration (query `sqlite_master` for `column_mappings` and expect zero rows).

- [ ] **Step 3: Regenerate sqlc**

Run: `"$(go env GOPATH)/bin/sqlc" generate`
Expected: `internal/db/gen` (`models.go`, `querier.go`, the `column_mappings.sql.go` file) no longer references column_mappings. Mechanism: `sqlc.yaml` sets `schema: internal/db/migrations`, so sqlc reads the **migrations** dir — the `00010` drop is seen as schema, and removing `queries/column_mappings.sql` removes the generated methods. Confirm `git status` shows the gen file deleted/trimmed.

- [ ] **Step 4: Rewire `server.go`**

In `internal/http/server.go`:
- Delete the `ColumnMappings *ColumnMappingHandler` field (~line 79-81) and remove `deps.ColumnMappings != nil` from the guard (~line 138).
- Delete the `if deps.ColumnMappings != nil { ... }` route block (~lines 234-240).
- Replace the import route block (~lines 247-248) with the catalog-nested routes:

```go
				if deps.Import != nil {
					pr.Post("/catalog/import/parse", deps.Import.Parse)
					pr.Post("/catalog/import/preview", deps.Import.Preview)
					pr.Post("/catalog/import/commit", deps.Import.Commit)
				}
```

- [ ] **Step 5: Rewire `main.go`**

In `main.go`:
- Delete `columnMappingSvc := service.NewColumnMappingService(conn, hub)` (line 112) and `columnMappingsRepo := repository.NewColumnMappings(conn)` (line 117).
- Delete `ColumnMappings: httpapi.NewColumnMappingHandler(columnMappingSvc),` (line 151).
- **Add** a rate-tiers repo — `main.go` currently constructs only the rate-tier *service*, not a repo, so this variable does **not** exist yet. Add near the other repo constructions: `rateTiersRepo := repository.NewRateTiers(conn)`.
- Change line 153 to: `Import: httpapi.NewImportHandler(catalogRepo, rateTiersRepo),`.

- [ ] **Step 6: Gate the backend**

Run: `go build ./... && go test ./... && go vet ./... && gofmt -l .`
Expected: build OK, tests PASS, vet clean, gofmt prints nothing.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: remove persisted column_mappings; nest import under /api/catalog/import"
```

---

## Task 5: SPA — wizard under catalog, remove mappings UI

**Files:**
- Create: `web/src/routes/catalog/import/+page.svelte`
- Modify: `web/src/routes/catalog/+page.svelte`, `web/src/routes/+layout.svelte`, `web/src/lib/api/types.ts`
- Delete: `web/src/routes/column-mappings/`, `web/src/lib/stores/columnMappings.svelte.ts`, `web/src/routes/import/`

- [ ] **Step 1: Remove the old mappings + import UI**

```bash
git rm -r web/src/routes/column-mappings web/src/routes/import \
  web/src/lib/stores/columnMappings.svelte.ts
```

In `web/src/lib/api/types.ts`, delete `ColumnMapping` and `ColumnMappingInput` (~lines 252-275) and add import DTOs:

```ts
export interface ImportSuggestion {
	fields: Record<string, string>;
	baseHeader: string;
	priceCols: { header: string; suggestName: string }[];
}
export interface ImportParseResult {
	headers: string[];
	sample: Record<string, string>[];
	suggestion: ImportSuggestion;
}
export interface ImportDiffSummary {
	total: number; new: number; updated: number; unchanged: number; errors: number;
}
```

- [ ] **Step 2: Fix the nav (`+layout.svelte`)**

Remove the `/column-mappings` link (line 81). Remove or repurpose the `/import` link (line 80) — delete it; import is reached from the catalog page.

- [ ] **Step 3: Repoint the existing Import link on catalog**

`web/src/routes/catalog/+page.svelte` **already has** an Import link (~line 128) pointing at the old `/import` route. Repoint it (do not add a duplicate):

```svelte
<a href="/catalog/import" ...>Import</a>   <!-- was href="/import" -->
```

- [ ] **Step 4: Build the wizard**

`web/src/routes/catalog/import/+page.svelte` — three steps driven by `$state`. Use the existing `apiFetch`/client in `web/src/lib/api/client.ts` (match how other routes call the API; the requests are `multipart/form-data`, so build a `FormData`). Tier options come from the rate-tiers store/endpoint.

```svelte
<script lang="ts">
	import { goto } from '$app/navigation';
	import type { ImportParseResult } from '$lib/api/types';

	// step: upload -> review -> diff
	let step = $state<'upload' | 'review' | 'diff'>('upload');
	let file = $state<File | null>(null);
	let parsed = $state<ImportParseResult | null>(null);
	let fields = $state<Record<string, string>>({});       // header -> field|ignore
	let tierChoice = $state<Record<string, string>>({});    // header -> tierId|'new'|'ignore'
	let tierNewName = $state<Record<string, string>>({});   // header -> name (when 'new')
	let tiers = $state<{ id: number; name: string }[]>([]);
	let diff = $state<{ summary: ImportParseResult['sample'] extends never ? never : any } | null>(null);
	let updateExisting = $state(false);
	let error = $state('');

	const FIELD_OPTS = ['name', 'sku', 'unit', 'category', 'rate', 'ignore'];

	async function loadTiers() {
		const r = await fetch('/api/rate-tiers', { credentials: 'include' });
		tiers = r.ok ? await r.json() : [];
	}

	async function doParse() {
		error = '';
		if (!file) return;
		const fd = new FormData();
		fd.set('file', file);
		const r = await fetch('/api/catalog/import/parse', { method: 'POST', body: fd, credentials: 'include' });
		if (!r.ok) { error = await r.text(); return; }
		parsed = await r.json();
		fields = {};
		for (const h of parsed!.headers) fields[h] = parsed!.suggestion.fields[h] ?? 'ignore';
		tierChoice = {}; tierNewName = {};
		for (const p of parsed!.suggestion.priceCols) {
			const existing = tiers.find((t) => t.name.toLowerCase() === p.suggestName.toLowerCase());
			tierChoice[p.header] = existing ? String(existing.id) : 'new';
			tierNewName[p.header] = p.suggestName;
		}
		step = 'review';
	}

	function buildMapping() {
		const f: Record<string, string> = {};
		for (const [h, v] of Object.entries(fields)) if (v !== 'ignore') f[h] = v;
		const tierCols: Record<string, string> = {};
		for (const [h, choice] of Object.entries(tierChoice)) {
			if (choice === 'ignore') continue;
			if (choice === 'new') tierCols[h] = (tierNewName[h] || h).trim();
			else tierCols[h] = tiers.find((t) => String(t.id) === choice)?.name ?? '';
		}
		return JSON.stringify({ fields: f, tierCols, fileType: '', sheetName: '', headerRow: 1 });
	}

	function nameMapped() {
		return Object.values(fields).includes('name');
	}

	async function doPreview() {
		error = '';
		if (!nameMapped()) { error = 'Map a column to "name" first.'; return; }
		const fd = new FormData();
		fd.set('file', file!);
		fd.set('mapping', buildMapping());
		const r = await fetch('/api/catalog/import/preview', { method: 'POST', body: fd, credentials: 'include' });
		if (!r.ok) { error = await r.text(); return; }
		diff = await r.json();
		step = 'diff';
	}

	async function doCommit() {
		error = '';
		const fd = new FormData();
		fd.set('file', file!);
		fd.set('mapping', buildMapping());
		fd.set('updateExisting', String(updateExisting));
		const r = await fetch('/api/catalog/import/commit', { method: 'POST', body: fd, credentials: 'include' });
		if (!r.ok) { error = await r.text(); return; }
		await goto('/catalog');
	}

	$effect(() => { loadTiers(); });
</script>

<h1 class="mb-4 text-xl font-semibold">Import catalog</h1>
{#if error}<p class="mb-3 rounded bg-red-50 px-3 py-2 text-sm text-red-700">{error}</p>{/if}

{#if step === 'upload'}
	<input type="file" accept=".csv,.xlsx" onchange={(e) => (file = (e.currentTarget as HTMLInputElement).files?.[0] ?? null)} />
	<button class="ml-2 rounded bg-blue-600 px-3 py-1.5 text-sm text-white disabled:opacity-50" disabled={!file} onclick={doParse}>Next</button>
{:else if step === 'review' && parsed}
	<table class="w-full text-sm">
		<thead><tr><th class="text-left">Column</th><th class="text-left">Maps to</th></tr></thead>
		<tbody>
			{#each parsed.headers as h}
				<tr>
					<td class="py-1 pr-4 font-mono">{h}</td>
					<td>
						{#if parsed.suggestion.priceCols.some((p) => p.header === h)}
							<select bind:value={tierChoice[h]} class="rounded border px-2 py-1">
								{#each tiers as t}<option value={String(t.id)}>{t.name}</option>{/each}
								<option value="new">Create new tier…</option>
								<option value="ignore">Ignore</option>
							</select>
							{#if tierChoice[h] === 'new'}
								<input bind:value={tierNewName[h]} class="ml-2 rounded border px-2 py-1" placeholder="Tier name" />
							{/if}
						{:else}
							<select bind:value={fields[h]} class="rounded border px-2 py-1">
								{#each FIELD_OPTS as opt}<option value={opt}>{opt}</option>{/each}
							</select>
						{/if}
					</td>
				</tr>
			{/each}
		</tbody>
	</table>
	{#if !nameMapped()}<p class="mt-2 text-sm text-amber-700">Pick a column for “name”.</p>{/if}
	<button class="mt-3 rounded bg-blue-600 px-3 py-1.5 text-sm text-white" onclick={doPreview}>Preview diff</button>
{:else if step === 'diff' && diff}
	<p class="text-sm">New {diff.summary.new} · Updated {diff.summary.updated} · Unchanged {diff.summary.unchanged} · Errors {diff.summary.errors}</p>
	<label class="mt-2 flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={updateExisting} /> Update existing items</label>
	<button class="mt-3 rounded bg-green-600 px-3 py-1.5 text-sm text-white" onclick={doCommit}>Commit import</button>
{/if}
```

> Match existing routes' API-call style: if the app wraps fetch in `web/src/lib/api/client.ts` with base URL/CSRF, use that wrapper instead of raw `fetch`. Keep Tailwind classes consistent with sibling pages. Replace the `diff` `$state` type with the real `DiffResult` shape from `types.ts` (drop the placeholder generic).

- [ ] **Step 5: svelte-check + build**

Run: `cd web && npm run check && npm run build`
Expected: 0 errors / 0 warnings; `web/build` emitted.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(web): inline catalog import wizard; remove column-mappings UI"
```

---

## Task 6: Full-stack gate + smoke

**Files:** none (verification only).

- [ ] **Step 1: Backend gate**

Run: `go test ./... -race && go vet ./... && gofmt -l . && CGO_ENABLED=0 go build .`
Expected: all PASS; gofmt prints nothing; binary builds.

- [ ] **Step 2: Frontend gate**

Run: `cd web && npm run check && npm run build && cd ..`
Expected: 0/0; `web/build` present (the Go binary embeds it).

- [ ] **Step 3: Boot smoke**

Run: `DATA_DIR=$(mktemp -d) go run . --port 8099 &` then exercise:
- Sign in (first-run setup), create a rate tier or two.
- `/catalog/import`: upload a small CSV `name,sku,unit,price,Remote\nAssist,A1,H,10.00,15.00`.
- Verify the review step pre-maps name/sku/unit/rate and offers `Remote` as a tier (existing or create-new).
- Commit; confirm the catalog list shows the item and `Remote` exists as a tier with the per-tier rate.

Stop the server (`kill %1`).

- [ ] **Step 4: Final commit (if any smoke fixes)**

```bash
git add -A
git commit -m "test: catalog import redesign smoke fixes"
```

---

## Notes for the implementer

- **TDD order matters**: Tasks 1–2 are pure-Go and fully test-first. Task 3's handler compiles but its package only links after Task 4 rewires `server.go`/`main.go` — do 3 then 4 back-to-back before running the http package tests.
- **Audited mutations**: tier creation goes through `repository.RateTiersRepo.Create`, which already audits + can broadcast; do not write `rate_tiers` directly.
- **No schema edits to 00009**: the clean-break policy requires a forward `00010` drop migration.
- **YAGNI**: no metadata-column mapping in the new wizard (the old `MetadataMapping` is dropped); `metadataJSON`/`parseFloat` stay only because `Commit`/`buildMappedRow` still use them.
- Reference skills: @superpowers:test-driven-development, @superpowers:verification-before-completion.
```
