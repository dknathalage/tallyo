# Domain Port — Batch 8: import/export Implementation Plan (FINAL)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Port the last subsystem — CSV/Excel **export** (catalog, invoices, estimates) and catalog **import** (CSV+Excel via saved `column_mappings`, with a diff→commit flow).

**Architecture:** Export uses Go `encoding/csv` and `github.com/xuri/excelize/v2` (pure-Go, cgo-free) to stream files. Import: upload a file → parse rows → apply a column mapping (header→field) → **diff** against existing catalog (match by SKU: new / updated / unchanged) → **commit** (insert new + update existing). `column_mappings` is a CRUD table storing reusable mappings (`mapping`, `tier_mapping`, `metadata_mapping`, `file_type`, `sheet_name`, `header_row`).

**Spec:** `docs/superpowers/specs/2026-06-05-domain-port-decomposition-design.md` (Import/Export section).

**Reference (old code, port faithfully):** `src/lib/csv/{export-catalog,export-invoices,export-estimates,parse,columns}.ts`, `src/lib/import/{diff-catalog,commit-catalog,map-columns,parse-file}.ts`, `src/routes/api/export/*`, `src/routes/api/import/catalog`, `src/lib/db/queries/column-mappings.ts`. Reuse `internal/repository/catalog.go` (catalog items for diff/commit) + Batch-1/2 patterns.

**Schema (column_mappings, verbatim, clean-break):**
```sql
column_mappings: id PK AUTOINCREMENT, uuid TEXT NOT NULL UNIQUE, name TEXT NOT NULL,
  entity_type TEXT NOT NULL DEFAULT 'catalog', mapping TEXT NOT NULL DEFAULT '{}',
  tier_mapping TEXT DEFAULT '{}', metadata_mapping TEXT DEFAULT '[]',
  file_type TEXT DEFAULT 'csv', sheet_name TEXT DEFAULT '', header_row INTEGER DEFAULT 1,
  created_at TEXT NOT NULL, updated_at TEXT NOT NULL
```

---

## Task 1: column_mappings migration + CRUD (backend)

**Files:** `internal/db/migrations/00009_column_mappings.sql`; `internal/db/queries/column_mappings.sql`; `internal/repository/column_mapping.go`; `internal/service/column_mapping.go`; `internal/http/column_mappings.go`; wire `server.go` + `cmd/tallyo`; regen; tests.

- [ ] **Step 1: Migration** — column_mappings table. Migration test → version 9.
- [ ] **Step 2: sqlc** — `ListColumnMappings` (optional WHERE entity_type=?; just `ListColumnMappings` ORDER BY name + `ListColumnMappingsByEntity`), `GetColumnMapping`, `CreateColumnMapping` (RETURNING *), `UpdateColumnMapping` (RETURNING *), `DeleteColumnMapping`. Report gen types.
- [ ] **Step 3: Repository** `column_mapping.go` — domain `ColumnMapping{ID, UUID, Name, EntityType string; Mapping, TierMapping, MetadataMapping string (JSON strings); FileType, SheetName string; HeaderRow int64; CreatedAt, UpdatedAt}` (camelCase: entityType, tierMapping, metadataMapping, fileType, sheetName, headerRow). CRUD via audit.WithTx (entity "column_mapping"). `List(ctx, entityType string)` (empty → all). Validate name non-empty. Defaults: mapping "{}", tier_mapping "{}", metadata_mapping "[]", file_type "csv", header_row 1.
- [ ] **Step 4: Service** (broadcast `column_mapping`) + **Handlers** REST `/api/column-mappings` (GET ?entityType=, POST, GET/PUT/DELETE {id}) behind RequireAuth. Wire Deps + cmd. Tests.
- [ ] **Step 5: Run** all gates. **Commit** `feat(import): column_mappings CRUD`.

---

## Task 2: Export (CSV + Excel)

**Files:** Create `internal/export/export.go` (+ `_test.go`); `internal/http/export.go` (+ test); wire `server.go` (reuse the catalog/invoice/estimate services to load data). Add `go get github.com/xuri/excelize/v2`.

- [ ] **Step 1: Export package** — pure functions returning bytes:
  - `CatalogCSV(items []*repository.CatalogItem) ([]byte, error)` — header `name,sku,rate,unit,category` (+ metadata if simple) then a row per item (encoding/csv).
  - `InvoicesCSV(invoices []*repository.Invoice) ([]byte, error)` — header `invoiceNumber,clientName,date,dueDate,status,subtotal,taxAmount,total,currency`.
  - `EstimatesCSV(estimates []*repository.Estimate) ([]byte, error)` — header `estimateNumber,clientName,date,validUntil,status,subtotal,taxAmount,total,currency`.
  - `CatalogXLSX(items) ([]byte, error)` — same columns via excelize (one sheet "Catalog"); `f.WriteToBuffer()`. (Confirm excelize is cgo-free with `CGO_ENABLED=0 go build`.)
  - Tests: each CSV has the header + a row; XLSX bytes start with `PK` (xlsx is a zip).
- [ ] **Step 2: Handlers** (behind RequireAuth): `GET /api/export/catalog?format=csv|xlsx`, `GET /api/export/invoices` (csv), `GET /api/export/estimates` (csv). Each loads via the service (List), renders, sets `Content-Type` (text/csv or application/vnd.openxmlformats-officedocument.spreadsheetml.sheet) + `Content-Disposition: attachment; filename="catalog.csv"`. Tests: 200 + content-type + non-empty body.
- [ ] **Step 3: Wire** server.go (a new `ExportHandler` holding the catalog/invoice/estimate services, or add methods to existing handlers — cleanest: a dedicated `ExportHandler` in Deps). Nil-safe.
- [ ] **Step 4: Run** gates + `CGO_ENABLED=0 go build ./cmd/tallyo` (excelize must stay cgo-free). Boot smoke (export catalog csv + xlsx). **Commit** `feat(export): CSV + Excel export for catalog, invoices, estimates`.

---

## Task 3: Catalog import (parse → map → diff → commit)

**Files:** Create `internal/importer/importer.go` (+ `_test.go`); `internal/http/import.go` (+ test); wire `server.go`.

- [ ] **Step 1: Importer package** (pure logic + DB for diff/commit):
  - `ParseRows(data []byte, fileType, sheetName string, headerRow int) (headers []string, rows []map[string]string, err error)` — CSV via encoding/csv; XLSX via excelize (read the sheet, headerRow-based). Each row maps header→cell.
  - `MappedRow{Name, Sku, Unit, Category string; Rate float64; Metadata map[string]string; TierRates map[int64]float64}`; `ApplyMapping(rows []map[string]string, mapping ColumnMapping) ([]MappedRow, []RowError)` — uses the mapping JSON (header→field), tier_mapping (header→tierId), metadata_mapping (list of headers→metadata keys) to build MappedRow; rows with a missing required field (name) → RowError.
  - `Diff(ctx, db, mapped []MappedRow) (DiffResult, error)` — load existing catalog; match by SKU (lowercased, trimmed); classify new (no sku match) / updated (sku match, fields differ) / unchanged. `DiffResult{New []MappedRow; Updated []UpdatedItem{Existing CatalogItem; Incoming MappedRow}; UnchangedCount int; Summary{Total,New,Updated,Unchanged,Errors int}}`.
  - `Commit(ctx, catalogRepo, diff DiffResult, updateExisting bool) (CommitResult{Inserted, Updated int; BatchID string}, error)` — for each New: catalogRepo.Create; for each Updated (if updateExisting): catalogRepo.Update; apply tier rates via catalogRepo.SetRate. Use a batch id (uuid) for audit correlation.
  - Tests: parse a small CSV; ApplyMapping; Diff (seed 1 existing item by SKU → an incoming row with same SKU+diff = updated, a new SKU = new, identical = unchanged); Commit (inserts new, updates existing).
- [ ] **Step 2: Handlers** (behind RequireAuth):
  - `POST /api/import/catalog/preview` — multipart form: a `file` + a `mappingId` (or inline mapping JSON) + optional fileType/sheetName/headerRow. Parse → ApplyMapping → Diff → return the DiffResult (200).
  - `POST /api/import/catalog/commit` — JSON body: the mapped rows (or a re-parse) + `updateExisting bool`. SIMPLEST + robust: the commit re-accepts the file + mapping + updateExisting, re-parses+diffs server-side, then commits (stateless — avoids trusting client-sent diff). So commit is also multipart (file + mappingId + updateExisting). Return CommitResult (200).
  - Validate file present → 400; mapping resolvable → 400. Tests (seed catalog + a mapping): preview returns a diff; commit inserts/updates.
- [ ] **Step 3: Wire** server.go (`ImportHandler` in Deps holding the catalog service/repo + column-mapping repo). Nil-safe.
- [ ] **Step 4: Run** gates + cgo-free build. Boot smoke (create a mapping; POST a small CSV to preview → diff; commit → catalog items created). **Commit** `feat(import): catalog import with diff and commit`.

---

## Task 4: Frontend — export buttons, import wizard, mappings UI

**Files:** modify catalog/invoices/estimates pages (export buttons); new `web/src/routes/import/+page.svelte` (catalog import wizard); optional `web/src/routes/column-mappings/+page.svelte`; types; nav.

- [ ] **Step 1: Export buttons** — on catalog page add "Export CSV" + "Export Excel" links (`<a href="/api/export/catalog?format=csv" ...>`); on invoices page "Export CSV" (`/api/export/invoices`); estimates page "Export CSV".
- [ ] **Step 2: Column mappings UI** — a `createCollectionStore('column-mappings','column_mapping')`; a simple page to list/create/edit mappings (name, entityType, and the mapping JSON fields — a textarea for the JSON is acceptable for the skeleton; a full visual mapper is out of scope). Nav link "Mappings".
- [ ] **Step 3: Import wizard** `web/src/routes/import/+page.svelte` — steps: (1) pick a saved mapping + choose a file (`<input type=file>`); (2) submit a multipart POST to `/api/import/catalog/preview` (use `FormData` + `fetch(..., {credentials:'include'})`) → show the diff summary (new/updated/unchanged counts + a sample table); (3) an "Update existing" checkbox + a "Commit import" button → POST to `/api/import/catalog/commit` (same FormData + updateExisting) → show the CommitResult (inserted/updated). Nav link "Import".
- [ ] **Step 4: Verify** `npm run check` (0/0), build (200.html), `touch build/.gitkeep`. **Commit** `feat(web): export buttons, column mappings, and catalog import wizard`.

---

## Task 5: Batch 8 acceptance + rewrite-complete note

- [ ] **Step 1: Gates** — `go test ./... -race`, vet, gofmt, `npm run check` + build, `CGO_ENABLED=0 go build ./cmd/tallyo`.
- [ ] **Step 2: Live smoke** — boot; setup+login; seed a couple catalog items; `curl /api/export/catalog?format=csv` → CSV with rows; `?format=xlsx` → `PK` bytes; `curl /api/export/invoices` (after creating an invoice) → CSV; create a column mapping; POST a small CSV (name,sku,rate) to `/api/import/catalog/preview` (multipart) → diff (e.g. 1 new); commit → the catalog item appears in `GET /api/catalog`. Capture output.
- [ ] **Step 3: Commit** `chore: batch 8 acceptance — import/export full-stack`.
- [ ] **Step 4: Update CLAUDE.md** — now that all domains are ported, refresh the body (Tech Stack/Project Layout/Commands/Run) from the old Electron description to the Go web-service reality (Go + chi + SQLite/modernc + sqlc + maroto + excelize + SvelteKit SPA; `tallyo serve`; `go test ./...` / `npm run check`). Note the old `src/`/`electron/`/`drizzle/` tree is now superseded (a follow-up can delete it). **Commit** `docs: update CLAUDE.md for the Go web-service architecture`.

---

## Done When

- column_mappings CRUD; CSV export for catalog/invoices/estimates + XLSX for catalog (cgo-free); catalog import parses CSV/XLSX, applies a saved mapping, diffs by SKU, and commits (insert new + update existing + tier rates); mutations audited + broadcast.
- Frontend: export buttons, mappings management, and a catalog import wizard (preview diff → commit).
- All gates green; cgo-free binary builds; live smoke confirms export bytes + an import round-trip; CLAUDE.md updated.

**This completes the domain port** (Batches 0–8). Out of scope remained: Dashboard, Reports, AI chat. A final follow-up can remove the legacy `src/`/`electron/`/`drizzle/` tree.
