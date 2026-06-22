# Generic Invoicing Core (De-NDIS) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn Tallyo from an NDIS-only invoicer into a generic goods-and-services invoicer where NDIS behaviour is an optional, data-driven capability — without duplicating any code path.

**Architecture:** A sequenced full rename (participant→client, plan_manager→payer, catalog→items, shift→session, gst_free→taxable) over the modular-monolith slices, followed by two behaviour changes: (a) the `LineValidator` gates each NDIS step on data presence instead of running unconditionally, and (b) the fixed NDIS XLSX ingest is replaced by a generic two-step upload-and-map importer reusing `importer.ParseRows`. Clean-break schema — no data migration.

**Tech Stack:** Go 1.26 (chi, sqlc, goose, modernc SQLite), SvelteKit SPA (Svelte 5 runes, TS), `importer.ParseRows` (CSV/XLSX).

**Spec:** `docs/superpowers/specs/2026-06-23-generic-invoicing-de-ndis-design.md`

---

## How to Work This Plan

This is a **rename-heavy refactor of an existing, well-tested codebase**, not greenfield feature work. Two task styles appear below:

- **Mechanical-rename tasks** — the existing test suite is the safety net. The task gives the exact rename mapping, the files, the regen command, and the green gate. You do NOT write new tests first; you keep the existing ones green (updating their identifiers as part of the rename). Verify with the gate, then commit.
- **New-behaviour tasks** (Phases 6–7, plus the schema additions) — full TDD: failing test → run → implement → pass → commit, with complete code in the step.

**Each phase ends green and is committed independently.** Do not start a phase until the previous phase's gate passes.

### The Green Gate (run at the end of every task unless noted)

```bash
# Regenerate gen if any query/migration changed:
"$(go env GOPATH)/bin/sqlc" generate
# Go gate:
gofmt -w . && go vet ./... && go test ./... && CGO_ENABLED=0 go build ./cmd/tallyo
```

Frontend gate (after any `web/` change):

```bash
cd web && npm run check && npm run build && cd ..
```

### Rename discipline

- Rename in this order **per entity**: migration SQL → `queries/*.sql` → `sqlc generate` → Go slice (repository → service → handler → tests) → cross-slice references (gen consumers, `internal/app`, `internal/billing`, `internal/agent`, `internal/pdf`) → frontend (`web/src/lib/api/types.ts`, stores, routes).
- After `sqlc generate`, the compiler is your worklist: `go build ./...` errors point at every gen consumer to fix.
- **UUID addressing stays** (CLAUDE.md): renamed routes remain uuid-addressed (`/clients/{clientUUID}`, etc.).
- Keep internal billing **method** names (`ValidateFilling`, `ResolveZonePrice`, `applySupportItem…`) as-is — renaming them is churn with no payoff (per spec review advisory). Only rename types/fields the API or DB exposes.
- **Columns vs FKs:** the uuid columns (`line_items.support_item_id`, `catalog_version_id`, the uuid `shift_id`) are **TEXT-stored pinned UUIDs with no `REFERENCES` clause** — rename the plain column **and its index** (`idx_line_items_support_item`, `idx_line_items_shift`, `idx_shifts_participant_date`, etc.), not a foreign key. The int `shift_id` IS a real FK. Also fix the **stale "control-DB" comments** in `00003_catalogue.sql` during Phase 5 — the catalogue is tenant-owned now.

---

## File Structure (what changes)

| Area | Files |
|---|---|
| Migrations | `internal/db/migrations/tenant/00001_tenant.sql`, `00003_catalogue.sql` (rewritten in place — clean-break) |
| Queries | `internal/db/queries/{participants,plan_managers,shifts,support_items,support_item_prices,catalog_versions,line_items,estimate_line_items,custom_items,recurring_templates,business_profile}.sql` → renamed files/queries |
| Gen | `internal/db/gen/` (regenerated; never hand-edit) + `sqlc.yaml` schema path |
| Slices renamed | `internal/participant/`→`internal/client/`, `internal/planmanager/`→`internal/payer/`, `internal/shift/`→`internal/session/`, `internal/catalog/`→`internal/pricelist/` |
| Slices edited | `internal/billing/`, `internal/invoice/`, `internal/estimate/`, `internal/recurring/`, `internal/customitem/`, `internal/businessprofile/`, `internal/agent/`, `internal/app/`, `internal/pdf/` |
| New (import) | `internal/importer/mapping.go` (+test), `internal/pricelist/import_handler.go` (+test) |
| Frontend | `web/src/lib/api/types.ts`, `web/src/lib/stores/*`, `web/src/routes/[tenant]/{participants→clients,plan-managers→payers,shifts→sessions,support-catalog→price-list}/*` |
| Docs | `docs/data-model.md`, the DB-per-tenant spec ERD, `CLAUDE.md` |

---

## Phase 0: Baseline & Branch

### Task 0.1: Confirm green baseline

**Files:** none.

- [ ] **Step 1: Branch off**

```bash
git checkout -b feat/generic-invoicing-de-ndis
```

- [ ] **Step 2: Run the full gate, confirm green BEFORE any change**

```bash
gofmt -l . && go vet ./... && go test ./... && CGO_ENABLED=0 go build ./cmd/tallyo
cd web && npm install && npm run check && npm run build && cd ..
```

Expected: all pass, `gofmt -l .` prints nothing. If anything fails, STOP — fix or report before starting; you need a clean baseline to trust the rename gates.

---

## Phase 1: `gst_free` → `taxable` (rename + invert)

This is a semantic inversion (`taxable = !gst_free`), so it is isolated and gets explicit test coverage. Tax is added for **taxable** lines (previously: skipped for **gst_free** lines).

### Task 1.1: Lock current tax behaviour with a characterization test

**Files:**
- Test: `internal/billing/validation_test.go` (add a test; confirm it passes against current `gst_free`).

- [ ] **Step 1: Add a test pinning the tax math on a mixed invoice**

Add a test that builds two lines — one currently `gst_free=true` (no tax) and one `gst_free=false` (taxed at the default rate) — and asserts the computed tax equals only the taxed line's `Round2(lineTotal*rate)`. Use the existing test helpers in `internal/billing`.

- [ ] **Step 2: Run it green against current code**

```bash
go test ./internal/billing/ -run TestComputeLineTax -v
```
Expected: PASS (this characterizes today's behaviour before the inversion).

- [ ] **Step 3: Commit**

```bash
git add internal/billing/validation_test.go && git commit -m "test(billing): characterize tax math before taxable inversion"
```

### Task 1.2: Invert and rename across schema + Go + frontend

**Files:**
- Modify: `internal/db/migrations/tenant/00001_tenant.sql` (`gst_free` columns on `line_items`, `estimate_line_items`, `custom_items`; `recurring_templates` JSON shape doc), `00003_catalogue.sql` (`support_items.gst_free`).
- Modify queries: `line_items.sql`, `estimate_line_items.sql`, `custom_items.sql`, `support_items.sql`, `recurring_templates.sql`.
- Modify Go: `internal/billing/lineitem.go` (`GstFree`→`Taxable`, invert reads), `validation.go` (`computeLineTax` skip-condition flips to `if !items[i].Taxable { continue }`; `snapshotSupportItem` sets `line.Taxable = !item.Taxable`… see note), `internal/customitem/`, `internal/recurring/`, `internal/invoice/`, `internal/estimate/`.
- Modify frontend: `web/src/lib/api/types.ts` (`gstFree`→`taxable`), all forms/tables referencing it (invert checkbox semantics + labels).

> **Inversion detail.** In the DB, replace `gst_free INTEGER` with `taxable INTEGER NOT NULL DEFAULT 1`. Everywhere a value was read, `taxable = !gst_free`. On `support_items` the column also inverts; `snapshotSupportItem` (validation.go:365) currently does `line.GstFree = item.GstFree` — it becomes `line.Taxable = item.Taxable` (still authoritative for coded lines).

- [ ] **Step 1: Rewrite the migration columns** (`gst_free`→`taxable`, default 1).
- [ ] **Step 2: Update queries** to select/insert `taxable`.
- [ ] **Step 3: Regenerate gen**

```bash
"$(go env GOPATH)/bin/sqlc" generate
```

- [ ] **Step 4: Update Go** — `go build ./...` lists every reference; flip each (`GstFree`→`Taxable`, invert the boolean at each read/skip site).
- [ ] **Step 5: Update the Task 1.1 test** to use `taxable` (same assertion, inverted inputs), and run it.

```bash
go test ./internal/billing/ -run TestComputeLineTax -v
```
Expected: PASS.

- [ ] **Step 6: Go gate**

```bash
gofmt -w . && go vet ./... && go test ./... && CGO_ENABLED=0 go build ./cmd/tallyo
```
Expected: all pass.

- [ ] **Step 7: Frontend** — rename `gstFree`→`taxable` in types + components; invert checkbox label ("GST free" → "Taxable") and its bound value. Run the frontend gate.

- [ ] **Step 8: Commit**

```bash
git add -A && git commit -m "refactor: rename gst_free to taxable (inverted) across schema, backend, SPA"
```

---

## Phase 2: `participant` → `client` (+ `ndis_number` → `reference`, add `type`)

### Task 2.1: Rename the slice, table, and references

**Files:**
- Rename dir: `internal/participant/` → `internal/client/` (package `participant`→`client`; rename types `Participant`→`Client`, `ParticipantsRepo`→`ClientsRepo`, `ParticipantInput`→`ClientInput`, etc.).
- Modify migration `00001_tenant.sql`: table `participants`→`clients`; column `ndis_number`→`reference`; ADD `type TEXT NOT NULL DEFAULT 'standard' CHECK(type IN ('ndis','standard'))`; make `plan_start`, `plan_end`, `mgmt_type` nullable (no NOT NULL).
- Rename query file `participants.sql`→`clients.sql`; update query names (`GetParticipantByUUID`→`GetClientByUUID`, etc.) and the `participant_id` FK column references in `invoices.sql`, `estimates.sql`, `shifts.sql`, `recurring_templates.sql`.
- Modify migration FK columns: `invoices.participant_id`→`client_id`, `estimates.participant_id`→`client_id`, `shifts.participant_id`→`client_id`, `recurring_templates.participant_id`→`client_id`.
- Modify Go consumers: `internal/billing/validation.go` (`participants` repo field + `assertPlanWindow` call site), `internal/invoice/`, `internal/estimate/`, `internal/recurring/`, `internal/shift/`, `internal/agent/`, `internal/app/app.go` (wiring), `internal/pdf/` (snapshot labels).
- Frontend: route `web/src/routes/[tenant]/participants/`→`clients/`; `types.ts` `Participant`→`Client`, `ndisNumber`→`reference`, add `type`; stores; nav labels.

- [ ] **Step 1: Move the slice dir + rename package/types**

```bash
git mv internal/participant internal/client
```
Then rename the package declaration and exported identifiers within (`participant`→`client`, `Participant`→`Client`, …). Update `_test.go` files too.

- [ ] **Step 2: Rewrite migration** — table + columns + new `type` + nullable plan fields + FK column renames in the four referencing tables.
- [ ] **Step 3: Rename query file + queries**, fix FK column names in referencing query files.
- [ ] **Step 4: Regenerate gen** (`sqlc generate`).
- [ ] **Step 5: Fix all consumers** — `go build ./...` is the worklist. Update `internal/app/app.go` wiring (`participant.NewService`→`client.NewService`, struct field names on the server/handler registry).
- [ ] **Step 6: Go gate** — all pass.
- [ ] **Step 7: Frontend** — `git mv` the route dir, rename types/fields/labels, add the `type` field to the client editor (hidden NDIS fields handled in Phase 6). Frontend gate.
- [ ] **Step 8: Commit**

```bash
git add -A && git commit -m "refactor: rename participant to client; ndis_number to reference; add client.type"
```

---

## Phase 3: `plan_manager` → `payer`

### Task 3.1: Rename slice, table, and the client FK

**Files:**
- Rename dir: `internal/planmanager/`→`internal/payer/` (package + types `PlanManager`→`Payer`, `PlanManagersRepo`→`PayersRepo`, etc.).
- Migration `00001_tenant.sql`: table `plan_managers`→`payers`; `clients.plan_manager_id`→`clients.payer_id`; snapshot column `payer_snapshot` stays (already generic).
- Query file `plan_managers.sql`→`payers.sql`; rename queries; fix `payer_id` reference in `clients.sql`.
- Go consumers: `internal/invoice/`, `internal/estimate/` (payer snapshot), `internal/client/` (FK field `PlanManagerID`→`PayerID`), `internal/app/app.go`, `internal/pdf/` (payer label), `internal/agent/`.
- Frontend: route `plan-managers/`→`payers/`; `types.ts` `PlanManager`→`Payer`, `planManagerId`→`payerId`; labels.

- [ ] **Step 1:** `git mv internal/planmanager internal/payer`; rename package/types.
- [ ] **Step 2:** Rewrite migration table + FK column.
- [ ] **Step 3:** Rename query file/queries; fix `clients.sql` FK.
- [ ] **Step 4:** `sqlc generate`.
- [ ] **Step 5:** Fix consumers (compiler worklist) + `internal/app` wiring.
- [ ] **Step 6:** Go gate — all pass.
- [ ] **Step 7:** Frontend `git mv` route + rename types/labels. Frontend gate.
- [ ] **Step 8: Commit**

```bash
git add -A && git commit -m "refactor: rename plan_manager to payer"
```

---

## Phase 4: `shift` → `session` (entity rename only; status values unchanged)

### Task 4.1: Rename slice, table, and references — keep the lifecycle

**Files:**
- Rename dir: `internal/shift/`→`internal/session/` (package + types `Shift`→`Session`, `ShiftsRepo`→`SessionsRepo`, `ShiftInput`→`SessionInput`; **keep** status strings `scheduled/recorded/drafted/sent/paid` and the `ClearForInvoice` revert).
- Migration `00001_tenant.sql`: table `shifts`→`sessions`; `shifts.line_items` FK `shift_id`→`session_id` on `line_items`.
- Query file `shifts.sql`→`sessions.sql`; rename queries; fix `shift_id` in `line_items.sql`.
- Cross-slice interface: `invoice.ShiftLinker`→`invoice.SessionLinker`, `shift.InvoiceChecker`→`session.InvoiceChecker` (these break the invoice↔session cycle — keep the pattern, rename the types). Update `internal/app/app.go` wiring.
- Go consumers: `internal/invoice/` (linker), `internal/billing` (line item `ShiftID`→`SessionID` if present), `internal/agent/`.
- Frontend: route `shifts/`→`sessions/`; `types.ts` `Shift`→`Session`, `shiftId`→`sessionId`; the root `[tenant]/+page.svelte` (shifts hub) labels.

- [ ] **Step 1:** `git mv internal/shift internal/session`; rename package/types; **leave status strings + transitions untouched**.
- [ ] **Step 2:** Migration table + `line_items.shift_id`→`session_id`.
- [ ] **Step 3:** Rename query file/queries; fix `line_items.sql`.
- [ ] **Step 4:** `sqlc generate`.
- [ ] **Step 5:** Fix consumers + rename the `ShiftLinker`/`InvoiceChecker` interface types + `internal/app` wiring.
- [ ] **Step 6:** Go gate — all pass. Confirm a session lifecycle test still exercises `scheduled→recorded→drafted` and the revert.
- [ ] **Step 7:** Frontend `git mv` + rename. Frontend gate.
- [ ] **Step 8: Commit**

```bash
git add -A && git commit -m "refactor: rename shift to session (lifecycle unchanged)"
```

---

## Phase 5: `catalog`/`support_items` → `pricelist`/`items` (+ `items.unit_price`)

### Task 5.1: Rename catalogue tables/columns and add the generic base price

**Files:**
- Rename dir: `internal/catalog/`→`internal/pricelist/` (package + types: `CatalogRepo`→`ItemsRepo`, `SupportItem`→`Item`, `CatalogVersion`→`PriceListVersion`, `SupportItemPrice`→`ItemPrice`, `IngestItem`→`ImportItem`; keep `ResolveZonePrice` name).
- Migration `00003_catalogue.sql`: `catalog_versions`→`price_list_versions`, `support_items`→`items`, `support_item_prices`→`item_prices`; collapse `support_category`+`registration_group`+`claim_type` → single nullable `category TEXT`; ADD `items.unit_price REAL` (nullable, generic base price); `item_prices.zone`/`price_cap` stay nullable.
- Migration `00001_tenant.sql`: `line_items.support_item_id`→`item_id`, `line_items.catalog_version_id`→`price_list_version_id`; same on `estimate_line_items`.
- Rename query files: `support_items.sql`→`items.sql`, `support_item_prices.sql`→`item_prices.sql`, `catalog_versions.sql`→`price_list_versions.sql`; rename queries; fix `item_id`/`price_list_version_id` in `line_items.sql`, `estimate_line_items.sql`.
- `sqlc.yaml`: update schema path `internal/db/migrations/tenant/00003_catalogue.sql` (filename if you rename the migration file — prefer keeping the goose filename `00003_catalogue.sql` to avoid version churn; only the table names inside change).
- Go consumers: `internal/billing/` (`cat` field type, `snapshotSupportItem`, `applyZonePrice`, `LineItemInput.SupportItemID`→`ItemID`, `CatalogVersionID`→`PriceListVersionID`, `Code` stays), `internal/invoice/`, `internal/estimate/`, `internal/recurring/`, `internal/agent/`, `internal/app/app.go`.
- Frontend: route `support-catalog/`→`price-list/`; `types.ts` `SupportItem`→`Item`, `CatalogVersion`→`PriceListVersion`, `supportItemId`→`itemId`, `catalogVersionId`→`priceListVersionId`, add `unitPrice`, `category`; labels.

- [ ] **Step 1:** `git mv internal/catalog internal/pricelist`; rename package/types (keep method names).
- [ ] **Step 2:** Rewrite `00003_catalogue.sql` (table renames, `category` collapse, add `items.unit_price`) and `00001_tenant.sql` line-item FK renames.
- [ ] **Step 3:** Rename query files/queries; fix FK refs.
- [ ] **Step 4:** `sqlc generate`.
- [ ] **Step 5:** Fix consumers (compiler worklist) + wiring.
- [ ] **Step 6:** Go gate — all pass.
- [ ] **Step 7:** Frontend `git mv` route + rename types/labels (browse UI; new import UI lands in Phase 7). Frontend gate.
- [ ] **Step 8: Commit**

```bash
git add -A && git commit -m "refactor: rename catalog/support_items to pricelist/items; add items.unit_price; collapse category"
```

---

## Phase 6: Data-presence validation + client.type gating

### Task 6.1: Gate the zone-cap block on a configured zone

**Files:**
- Test: `internal/billing/validation_test.go`.
- Modify: `internal/billing/validation.go` (the `validate`/`validateSupportLine` path that calls `tenantZone` + `applyZonePrice`).

- [ ] **Step 1: Failing test** — a coded line on a tenant with **no** `business_profile.zone` set: assert the line passes with its caller-supplied `unit_price` untouched (no "no price published" / "exceeds cap" error). Today this errors because `tenantZone` defaults to `"national"` and `ResolveZonePrice` finds no row.

```bash
go test ./internal/billing/ -run TestGenericTenantSkipsZoneCap -v
```
Expected: FAIL.

- [ ] **Step 2: Implement** — read `business_profile.zone`; when it is empty/null, skip the `applyZonePrice` call entirely (no resolve, no cap assert, no fill). Keep the snapshot (code/name) and `taxable` defaulting. When zone IS set, behaviour is exactly as today (cap assert + fill mode preserved).

> Adjust `tenantZone` (validation.go:397) to return `("", nil)` when no profile/zone instead of defaulting to `"national"`, and guard the caller: `if zone != "" { v.applyZonePrice(...) }`.

- [ ] **Step 3: Run** the new test (PASS) and the full billing suite (still green — NDIS path with a zone set is unchanged).

```bash
go test ./internal/billing/ -v
```

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "feat(billing): gate NDIS zone-cap validation on a configured zone"
```

### Task 6.2: Require NDIS fields only for `type='ndis'` clients

**Files:**
- Test: `internal/client/service_test.go`.
- Modify: `internal/client/service.go` (create/update validation).

- [ ] **Step 1: Failing tests** — (a) a `standard` client with only `name` is accepted (no plan dates required); (b) an `ndis` client missing `plan_start`/`plan_end` is rejected with a field error.

```bash
go test ./internal/client/ -run TestClientTypeFieldGating -v
```
Expected: FAIL.

- [ ] **Step 2: Implement** the gate in the service: `if in.Type == "ndis"` require `plan_start`, `plan_end`, `mgmt_type`; otherwise leave them optional. Default `type` to `standard` when blank. (≥2 assertions per Power-of-10 rule 5.)
- [ ] **Step 3: Run** (PASS) + `go test ./internal/client/`.
- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "feat(client): require NDIS fields only for ndis-type clients"
```

### Task 6.3: Plan-window already permissive — add a generic regression test

**Files:**
- Test: `internal/billing/validation_test.go`.

- [ ] **Step 1:** `assertPlanWindow` is already open on empty bounds (validation.go:330). Add a regression test: a client with no plan dates + a coded line on a zone-less tenant passes. Run it (should PASS immediately — this locks the generic happy path end-to-end).
- [ ] **Step 2: Commit**

```bash
git add -A && git commit -m "test(billing): lock generic line happy path (no zone, no plan window)"
```

### Task 6.4: Frontend — show NDIS fields only for NDIS clients

**Files:**
- Modify: `web/src/routes/[tenant]/clients/` editor; settings page (zone field).

- [ ] **Step 1:** In the client editor, bind a `type` select; `{#if type === 'ndis'}` wrap the `reference`/plan-date/`mgmt_type`/payer fields. In settings, label `zone` as NDIS-only / optional.
- [ ] **Step 2:** Frontend gate (`npm run check && npm run build`).
- [ ] **Step 3: Commit**

```bash
git add -A && git commit -m "feat(web): surface NDIS client fields only for ndis-type clients"
```

---

## Phase 7: Generic catalogue import (replaces fixed NDIS XLSX ingest)

### Task 7.1: Generic mapping applier in the importer

**Files:**
- Create: `internal/importer/mapping.go`
- Test: `internal/importer/mapping_test.go`

- [ ] **Step 1: Failing test** — given headers `["Product","SKU","Price"]`, rows, and a mapping `{"Product":"name","SKU":"code","Price":"unitPrice"}`, `ApplyMapping` returns `[]ImportRow{ {Name:"Widget", Code:"W1", UnitPrice:9.99} }`; a mapping missing the required `name` target returns an error.

```bash
go test ./internal/importer/ -run TestApplyMapping -v
```
Expected: FAIL (undefined).

- [ ] **Step 2: Implement** `ApplyMapping(headers []string, rows []map[string]string, mapping map[string]string) ([]ImportRow, error)`:

```go
package importer

import (
	"fmt"
	"strings"
)

// ImportRow is one generic catalogue item parsed from an uploaded file.
type ImportRow struct {
	Name      string
	Code      string
	Unit      string
	Category  string
	UnitPrice float64
	Taxable   bool
}

// validTargets are the generic item fields a source column may map onto.
var validTargets = map[string]bool{
	"name": true, "code": true, "unit": true,
	"category": true, "unitPrice": true, "taxable": true,
}

// ApplyMapping turns parsed rows into ImportRows using a sourceHeader→targetField
// map. "name" is required; unmapped/empty cells are zero values. taxable defaults
// to true (generic items are taxable unless the source says otherwise).
// ponytail: single price column; multi-zone NDIS cap import is a later extension.
func ApplyMapping(headers []string, rows []map[string]string, mapping map[string]string) ([]ImportRow, error) {
	if len(mapping) == 0 {
		return nil, fmt.Errorf("import mapping is empty")
	}
	hasName := false
	for _, target := range mapping { // bounded by len(mapping)
		if !validTargets[target] {
			return nil, fmt.Errorf("unknown target field %q", target)
		}
		if target == "name" {
			hasName = true
		}
	}
	if !hasName {
		return nil, fmt.Errorf("a source column must map to the required field \"name\"")
	}
	out := make([]ImportRow, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		r := ImportRow{Taxable: true}
		for header, target := range mapping {
			cell := strings.TrimSpace(rows[i][header])
			switch target {
			case "name":
				r.Name = cell
			case "code":
				r.Code = cell
			case "unit":
				r.Unit = cell
			case "category":
				r.Category = cell
			case "unitPrice":
				r.UnitPrice = ParseFloat(cell)
			case "taxable":
				r.Taxable = !(cell == "" || cell == "0" || strings.EqualFold(cell, "false") || strings.EqualFold(cell, "no"))
			}
		}
		if r.Name == "" {
			continue // skip spacer/blank rows (no name)
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no data rows with a name were found")
	}
	return out, nil
}
```

- [ ] **Step 3: Run** (PASS) + `go test ./internal/importer/`.
- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "feat(importer): generic column→field mapping applier"
```

### Task 7.2: Pricelist import service — inspect + commit

**Files:**
- Modify: `internal/pricelist/service.go` (replace `IngestService.IngestXLSX`/`ParseXLSX` with `Inspect` + `ImportMapped`).
- Delete: the NDIS-specific `ParseXLSX` + column constants + `buildIngestItems`/`zonePrices`/`parseCap` in `internal/pricelist/service.go`.
- Test: `internal/pricelist/import_test.go`.

- [ ] **Step 1: Failing test** — `Inspect(data, "csv", "", 1)` returns `{headers, sampleRows}` without writing; `ImportMapped(ctx, data, fileType, headerRow, mapping, label)` creates one `price_list_version` + its `items` (with `unit_price`), atomically; a duplicate/blank-name file behaves per `ApplyMapping`. Use the tenant test DB helper already in the slice.

```bash
go test ./internal/pricelist/ -run TestImport -v
```
Expected: FAIL.

- [ ] **Step 2: Implement** `Inspect` (thin wrapper over `importer.ParseRows`, capping sample to 10 rows) and `ImportMapped` (calls `importer.ParseRows` → `importer.ApplyMapping` → repo insert of a new version + items in one tx; broadcasts SSE after commit, matching the existing ingest). Keep the whole-file-or-nothing reject behaviour. Delete the dead NDIS parser.
- [ ] **Step 3: Run** (PASS) + `go test ./internal/pricelist/`.
- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "feat(pricelist): generic inspect + mapped import; remove NDIS XLSX parser"
```

### Task 7.3: HTTP endpoints

**Files:**
- Modify: `internal/pricelist/handler.go` — replace `POST …/support-catalog/versions` (Ingest) with:
  - `POST …/items/import/inspect` → `Inspect`
  - `POST …/items/import/commit` → `ImportMapped`
  - rename the read routes (`/support-catalog/versions` → `/price-list/versions`, `/versions/{versionUUID}/items`, `/items/{itemUUID}/prices`). Keep the `RequireRole("owner","admin")` gate on the write endpoints.
- Test: `internal/pricelist/handler_test.go`.

- [ ] **Step 1: Failing test** — `inspect` returns headers JSON for a multipart CSV upload and persists nothing; `commit` with a valid mapping returns the new version and the items are queryable; non-owner/admin gets 403.
- [ ] **Step 2: Implement** the handlers (multipart parse: `file`, `mapping` JSON, `label`, optional `headerRow`/`sheetName`). Reuse `httpx` helpers.
- [ ] **Step 3: Run** (PASS) + Go gate.
- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "feat(pricelist): inspect + commit import endpoints"
```

### Task 7.4: Frontend import wizard

**Files:**
- Modify: `web/src/routes/[tenant]/price-list/+page.svelte` (replace the XLSX-only form).
- Modify: `web/src/lib/api/client.ts` if a second multipart call is needed.

- [ ] **Step 1:** Build the flow: file picker (accept `.csv,.xlsx`) + header-row input → POST `inspect` → render one `<select>` per returned header (options: the generic target fields + "ignore") → preview the mapped sample → "Import" POSTs `commit` with the mapping + label. Owner/admin gated (existing `session` role check).
- [ ] **Step 2:** Frontend gate.
- [ ] **Step 3: Commit**

```bash
git add -A && git commit -m "feat(web): upload-and-map price-list import wizard"
```

---

## Phase 8: Docs & Final Gate

### Task 8.1: Update the data-model ERDs and CLAUDE.md

**Files:**
- Modify: `docs/data-model.md` (renamed tables/columns + `items.unit_price` + `clients.type`).
- Modify: `docs/superpowers/specs/2026-06-21-sqlite-db-per-tenant-design.md` (ERD — table renames; catalogue tables stay tenant-owned).
- Modify: `CLAUDE.md` — update slice list (`participant`→`client`, `planmanager`→`payer`, `shift`→`session`, `catalog`→`pricelist`), the NDIS-catalogue section (now generic price list + upload-and-map import), and the cross-slice interface names (`SessionLinker`/`InvoiceChecker`).

- [ ] **Step 1:** Edit all three docs to match the new schema/slice names.
- [ ] **Step 2: Commit**

```bash
git add -A && git commit -m "docs: update ERDs + CLAUDE.md for generic invoicing rename"
```

### Task 8.2: Full gate, green

**Files:** none.

- [ ] **Step 1: Run the complete gate with race**

```bash
gofmt -l . && go vet ./... && go test ./... -race && CGO_ENABLED=0 go build ./cmd/tallyo
cd web && npm run check && npm run build && cd ..
```
Expected: all pass; `gofmt -l .` prints nothing.

- [ ] **Step 2:** Grep for stragglers — no `participant`, `plan_manager`, `support_item`, `catalog_version`, `gst_free`, `shift` identifiers remain except where intentional (the `ResolveZonePrice`/billing method names, NDIS-specific labels behind the `type==='ndis'` UI gate).

```bash
grep -rn -iE "participant|plan.?manager|support.?item|gst.?free" --include=*.go --include=*.ts --include=*.svelte internal web | grep -vi "node_modules"
```
Review each remaining hit is intentional.

- [ ] **Step 3:** Manual smoke (optional, via `/run` or `go run ./cmd/tallyo`): create a `standard` client, add an item with a `unit_price`, invoice it (no zone/plan errors); import a small CSV via the wizard.

---

## Done When

- Full gate green (`-race`), frontend `check`/`build` clean.
- A generic (standard) client can be created with just a name and invoiced from items/custom lines with no NDIS constraints.
- An NDIS client (type=ndis, zone configured) still gets cap + plan-window enforcement and fill mode (agent path) exactly as before.
- A CSV/XLSX can be uploaded, columns mapped, and committed as a new price-list version.
- Docs (both ERDs + CLAUDE.md) reflect the rename.
