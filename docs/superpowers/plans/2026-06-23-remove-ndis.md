# Remove NDIS — Pure Generic Invoicer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove NDIS entirely so Tallyo is a pure generic goods/services invoicer — no pricing zones, no price caps, no plan windows, no client types, no NDIS wording — keeping `payer` and `reference` as generic client fields.

**Architecture:** Two behaviour/schema phases, each kept green, then a docs+verification phase. Phase 1 strips NDIS client attributes; Phase 2 strips the pricing-zone / price-cap machinery and collapses the line validator; Phase 3 documents + verifies end-to-end. Clean-break migrations (edit in place; no data migration). The compiler + `sqlc generate` drive each rename/drop; the retained tax + custom-line tests guard against behaviour drift.

**Tech Stack:** Go 1.26 (chi, sqlc, goose, modernc SQLite), SvelteKit SPA (Svelte 5 runes), embedded SPA via `//go:embed`.

**Spec:** `docs/superpowers/specs/2026-06-23-remove-ndis-design.md`

---

## How to Work This Plan

This is a **removal** across an existing, well-tested codebase. Two task styles:
- **Removal tasks** — drop schema/columns/types/logic; the existing suite (minus the NDIS tests this plan deletes) is the safety net. After `sqlc generate`, `go build ./...` is the worklist. Delete NDIS-only tests rather than letting them fail.
- **Behaviour tasks** (the validator collapse, Phase 2) — keep the retained tax + custom-line + generic-pricing tests green; add a test if a path loses coverage.

**Each phase ends green and committed.** Run the gate at each task's end.

### The Green Gate
```bash
"$(go env GOPATH)/bin/sqlc" generate   # after any query/migration change
gofmt -w . && go vet ./... && go test ./... && CGO_ENABLED=0 go build ./cmd/tallyo
```
Frontend (after any `web/` change; do NOT run `npm install` — node_modules present):
```bash
cd web && npm run check && npm run build && cd ..
```

### Discipline (lessons from this session)
- One implementer at a time; nothing else touches the tree while it runs.
- Reviews are done inline by the controller (do NOT spawn reviewer subagents that may take implementation initiative).
- Verify the tree is clean + the gate is green after each subagent returns.
- Diagnostics/LSP lag is unreliable — trust `git status` + a real `go build`/`go test` at a single instant.
- Behavioural changes get a real run (smoke), not just compile-green.

---

## File Map (what changes)

| Area | Files |
|---|---|
| Migrations | `internal/db/migrations/tenant/00001_tenant.sql` (clients cols, business_profile.zone), `00003_catalogue.sql` (drop `item_prices` table) |
| Queries | `internal/db/queries/{clients,business_profile,item_prices}.sql`, any `line_items`/`estimate_line_items` refs unaffected |
| Gen | `internal/db/gen/` (regenerated) |
| Backend slices | `internal/billing/validation.go` (+ lineitem.go), `internal/pricelist/{repository,service,handler}.go`, `internal/client/{repository,service,handler}.go`, `internal/businessprofile/repository.go`, `internal/app/{auth_handlers.go,server.go,app.go}`, `internal/auth/tenants.go`, `internal/agent/*`, `internal/invoice/service.go`, `internal/estimate/service.go`, `internal/session/service.go` |
| Frontend | `web/src/lib/api/types.ts`, `web/src/lib/components/ClientEditor.svelte`, `web/src/routes/[tenant]/{clients,settings,price-list}/+page.svelte`, stores |
| Docs | `CLAUDE.md`, `docs/data-model.md`, `docs/superpowers/specs/2026-06-21-sqlite-db-per-tenant-design.md` |

---

## Phase 0: Baseline

### Task 0.1: Confirm green baseline
**Files:** none.
- [ ] **Step 1:** Confirm on branch `feat/generic-invoicing-de-ndis`, tree clean (`git status`), no other process running (`ps -Ao pid,command | grep cmd/tallyo`).
- [ ] **Step 2:** Run the full gate + frontend gate. Expected: all green. If not, STOP and report.

---

## Phase 1: Remove client NDIS attributes

Drop `clients.type`, `plan_start`, `plan_end`, `mgmt_type`; remove the validator's plan-window step; remove the client type-gating; update the frontend. Keep `reference` + `payer`.

### Task 1.1: Remove the plan-window step from the validator
**Files:**
- Modify: `internal/billing/validation.go` (remove `assertPlanWindow`, the `planWindow` read, the `clients` dep + the planStart/planEnd plumbing through `validate`/`validateSupportLine`).
- Test: `internal/billing/validation_test.go` (delete plan-window tests; keep the rest).

- [ ] **Step 1:** Read `validation.go`. Identify every use of plan dates: `planWindow`, `assertPlanWindow`, the `planStart, planEnd` params threaded through `validate` → `validateSupportLine`, and `v.clients`. Confirm `v.clients` is used ONLY for the plan window (it is — `planWindow` calls `clients.GetByID`).
- [ ] **Step 2:** Remove `assertPlanWindow`, the `planWindow` method, the `clients` field from `LineValidator`, and the `client` import. Remove the planStart/planEnd params from `validate`/`validateSupportLine` and their call sites. `NewLineValidator` no longer constructs `client.NewClients(...)`.
- [ ] **Step 3:** Delete plan-window tests in `validation_test.go` (e.g. out-of-plan cases, `seedClientPlan` usages tied to plan assertions). Keep tests that seed a client for other reasons (adjust to not assert plan window). Keep tax, cap (still present this phase), custom-line, generic-pricing tests.
- [ ] **Step 4:** `go test ./internal/billing/` green. Full Go gate green.
- [ ] **Step 5:** Commit: `refactor(billing): remove NDIS plan-window validation`

### Task 1.2: Drop client NDIS columns (schema + queries + gen + slice)
**Files:**
- Modify: `internal/db/migrations/tenant/00001_tenant.sql` (`clients`: drop `type`, `plan_start`, `plan_end`, `mgmt_type`; keep `reference`, `payer_id`, contact). Drop the `idx`/CHECK referencing `type`/`mgmt_type`.
- Modify: `internal/db/queries/clients.sql` (remove the 4 columns from create/update/select + the `ClientCols` allowlist keys that map to them).
- Modify: `internal/client/repository.go`, `service.go`, `handler.go` (drop the 4 fields from `Client`/`ClientInput`, the repo create/update params + scans, `validateClientInput`'s NDIS gating + `errInvalidType` + the `FieldError`/`ValidationError` types if now unused, and the handler's `writeClientInputError` mapping for `errInvalidType`). A client now requires only a name.
- Regenerate gen.

- [ ] **Step 1:** Rewrite the migration: remove the 4 columns + the `type` CHECK + `mgmt_type` CHECK + any related index. Keep `reference TEXT`, `payer_id INTEGER REFERENCES payers(id)`.
- [ ] **Step 2:** Update `clients.sql` (drop the columns everywhere; fix the listquery `ClientCols` allowlist — remove `type`/`mgmtType` keys).
- [ ] **Step 3:** `sqlc generate`.
- [ ] **Step 4:** Fix `internal/client/*` (compiler worklist): drop fields from types, repo, the `validateClientInput` NDIS branch (keep only "name required" if anything), `errInvalidType`. If `ValidationError`/`FieldError` in the client slice become unused, remove them. **Compiler-invisible spots to fix by hand (spec review):** the `ClientCols` listquery allowlist entries for `mgmtType`/`planStart`/`planEnd`/`type` (`repository.go:37-39`), `normType` (`repository.go:216`), and the `row.Scan(...)` column order (`repository.go:160` — currently scans `&f.planEnd, &f.mgmtType, …`). The migration also has `CHECK (type IN ('ndis','standard'))` (line 32) and `CHECK (mgmt_type IN ('plan','self'))` (line 36) — both removed with the columns.
- [ ] **Step 5:** Fix any other consumer the compiler flags (billing snapshot of client, invoice/estimate client snapshot JSON — these snapshot name/contact, should be unaffected; pdf client block).
- [ ] **Step 6:** Update/trim client tests (`internal/client/*_test.go`, `internal/app/clients_test.go`): delete `TestClientTypeFieldGating` and type/plan/mgmt assertions; keep create/list/update/delete + reference/payer.
- [ ] **Step 7:** Full Go gate green.
- [ ] **Step 8:** Commit: `refactor(client): drop NDIS fields (type, plan window, mgmt type)`

### Task 1.3: Frontend — generic client editor + list
**Files:**
- Modify: `web/src/lib/components/ClientEditor.svelte` (remove the type `<select>`, plan-start/plan-end inputs, management-type, and the `{#if clientType === 'ndis'}` gating + `clientType` state — show name, contact, optional reference, optional payer for every client).
- Modify: `web/src/routes/[tenant]/clients/+page.svelte` (drop the `type` column + filter).
- Modify: `web/src/lib/api/types.ts` (remove `ClientType`; drop `type`/`planStart`/`planEnd`/`mgmtType` from `Client`/`ClientInput`; keep `reference`, `payerId`).

- [ ] **Step 1:** Edit the three files; ensure no dangling references to removed fields.
- [ ] **Step 2:** Frontend gate green (`npm run check` 0/0, `npm run build`).
- [ ] **Step 3:** Commit: `refactor(web): generic client editor — drop NDIS client fields`

---

## Phase 2: Remove pricing zones + price caps; collapse the validator

### Task 2.1: Collapse the validator to catalogue-`unit_price` pricing
**Files:**
- Modify: `internal/billing/validation.go` (remove `tenantZone`, `applyZonePrice`, the zone-cap assertion, the fill-mode cap overwrite, the `profiles` dep + `businessprofile` import, the `control` param of `NewLineValidator`; keep `applyItemUnitPrice` as the single pricing step for catalogue lines; converge `Validate`/`ValidateFilling`).
- Modify callers of `NewLineValidator`: `internal/invoice/service.go`, `internal/estimate/service.go`, `internal/session/service.go` (drop the `control` arg) + ~30 test call sites (`NewLineValidator(conn, conn)` → `NewLineValidator(conn)`).
- Test: `internal/billing/validation_test.go`, `validation_filling_test.go`, `validation_pinning_test.go`, `internal/app/validation_e2e_test.go`, `internal/agent/draft_testhelpers_test.go`.

- [ ] **Step 1:** Read `validation.go`. `ValidateFilling`'s ONLY caller is `internal/invoice/service.go:164` (the agent reaches billing only through the invoice service — it does not call the validator directly). Drop `ValidateFilling` and point `invoice/service.go:164` at `Validate` (both now fill from `unit_price`).
- [ ] **Step 2:** Remove `tenantZone`, `applyZonePrice`, zone-cap assert, fill-mode cap, the `profiles` field + import, and change the signature to `NewLineValidator(tenant db.Executor)`. The catalogue-line path: resolve version → `GetItemByCode` → `snapshotSupportItem` → `applyItemUnitPrice` (fill when caller ≤0) → non-negativity. **Keep `snapshotSupportItem` (sets `taxable` from the item) and `computeLineTax` VERBATIM** — only the zone/cap/plan-window calls are removed; tax + custom-line behaviour must not change.
- [ ] **Step 3:** Update the three service callers to `NewLineValidator(tdb)` (drop control). Update `internal/agent` create/divide to call the single validate path.
- [ ] **Step 4:** Update tests: delete cap/zone/fill-cap tests (`TestValidateOverCapRejected`, `…AtCap…`, `…Quotable…`, `…Zone…`, `TestValidateFilling*` cap behaviour); fix `NewLineValidator(conn, conn)` → `NewLineValidator(conn)` everywhere; drop `setTenantZone`/`setNationalZone`/`seedZonedCatalog` cap helpers (or simplify to seed a `unit_price` item). Keep `TestGenericCodedLinePricesFromItemUnitPrice`, `TestLineValidatorReadsCatalogueFromTenant` (update its signature), tax + custom-line tests.
- [ ] **Step 5:** Full Go gate green.
- [ ] **Step 6:** Commit: `refactor(billing): collapse validator — catalogue unit_price pricing, drop zone caps + plan window`

### Task 2.2: Drop `item_prices` (table, repo, search-by-zone, endpoint, agent interface, frontend)
**Files:**
- Modify: `internal/db/migrations/tenant/00003_catalogue.sql` (drop `item_prices` CREATE + index + Down DROP).
- Modify/delete: `internal/db/queries/item_prices.sql`; remove prices queries in `items.sql`/`price_list_versions.sql` if any.
- Modify: `internal/pricelist/repository.go` (drop `ItemPrice`, prices repo methods, `ResolveZonePrice`; **remove the `UpsertItemPrice` call in the import write path at ~`repository.go:354`** — generic import writes `unit_price` only, no prices rows), `service.go` (the `Match` struct: drop `Zone`/`PriceCap`/`Quotable`; `SearchForDate`: drop the `zone` param + the per-result `ResolveZonePrice` call), `handler.go` (drop the `…/items/{itemUUID}/prices` route + handler).
- Modify: `internal/agent/deps.go` (`CatalogueSearcher` interface — drop the `zone` param from `SearchForDate` signature, ~line 51), `smart_draft_propose.go` (`catalogueMatchView` — drop `PriceCap`, ~line 144; the `search_catalogue` call passes `""` zone at ~line 134 — remove that arg; tool schema/result wording drops `priceCap`).
- Regenerate gen (drop `ItemPrice`, `CreateItemPrice`/`UpsertItemPrice`, `ListItemPrices*`, `ResolveZonePrice…`).
- Frontend: `web/src/lib/components/LineItemsEditor.svelte` (remove the `ItemPrice`/`Zone` imports, `loadPrices(item.id)`, the `match.priceCap` per-line cap cache + the "Cap ({zone}): …" display ~lines 5,39,58-66,345), `web/src/routes/[tenant]/price-list/+page.svelte` (remove `pricesItemId`/`prices`/`togglePrices`/`zoneLabel`/the prices column + "price caps by zone" copy ~lines 6,40-43,102-114,207,382), the `priceList.loadPrices` method in the API client, and `ItemPrice` type in `types.ts`.

- [ ] **Step 1:** Remove the `item_prices` table from `00003_catalogue.sql` (+ Down); delete `internal/db/queries/item_prices.sql`.
- [ ] **Step 2:** `sqlc generate`.
- [ ] **Step 3:** Fix `internal/pricelist/*` (compiler worklist): drop `ItemPrice`, `ResolveZonePrice`, prices repo methods, the prices handler+route, the import `UpsertItemPrice` call, and strip `Zone`/`PriceCap`/`Quotable` from `Match` + the `zone` param from `SearchForDate`.
- [ ] **Step 4:** Fix `internal/agent`: update the `CatalogueSearcher` interface signature, `SearchForDate` call site (drop the `""` zone arg), `catalogueMatchView` (drop `PriceCap`), and the `search_catalogue` tool result/schema wording. `go test ./internal/agent/`.
- [ ] **Step 5:** Frontend: remove `loadPrices` (API client), the LineItemsEditor cap cache + display, the price-list prices column + `zoneLabel`, and `ItemPrice` in `types.ts`.
- [ ] **Step 6:** Full Go gate + frontend gate green.
- [ ] **Step 7:** Commit: `refactor(pricelist): drop item_prices, zone search, and price-cap UI`

### Task 2.3: Drop `business_profile.zone` + signup zone
**Files:**
- Modify: `internal/db/migrations/tenant/00001_tenant.sql` (drop `zone` from `business_profile`).
- Modify: `internal/db/queries/business_profile.sql` (drop `zone`).
- Modify: `internal/businessprofile/repository.go` (drop `Zone` field, `validZone`, zone in create/update/scan).
- Modify: `internal/app/auth_handlers.go` (`signupRequest`: drop `Zone`; `validateSignup`: drop zone logic; remove `allowedZones`), `internal/auth/tenants.go` (`SignupInput`: drop `Zone`; `ProvisionBusinessProfile`: drop zone), the audit `Changes` map.
- Modify frontend: `web/src/routes/[tenant]/settings/+page.svelte` (remove the zone field + its `bind`), `web/src/lib/stores/businessProfile.svelte.ts` (drop the `zone` field, its `'national'` default, and the `raw.zone ?? 'national'` mapping ~lines 3,10,23,36 — else save still ships `zone`), `web/src/lib/api/types.ts` (remove `Zone`, drop `zone` from the business-profile type). Signup field already removed.
- Regenerate gen.

- [ ] **Step 1:** Drop `zone` from the migration + `business_profile.sql`; `sqlc generate`.
- [ ] **Step 2:** Fix `internal/businessprofile/*`, `internal/app/auth_handlers.go`, `internal/auth/tenants.go` (compiler worklist).
- [ ] **Step 3:** Update tests: `internal/app/signup_test.go` (drop zone cases), `internal/businessprofile/*_test.go` (drop zone tests), `internal/auth/tenants_test.go`.
- [ ] **Step 4:** Frontend: remove the settings zone field + `Zone` type.
- [ ] **Step 5:** Full Go gate + frontend gate green.
- [ ] **Step 6:** Commit: `refactor: drop business_profile.zone + signup zone (no NDIS pricing zones)`

### Task 2.4: Agent prompts — drop zone/cap wording
**Files:**
- Modify: `internal/agent/smart_divide_session.go`, `smart_draft_propose.go` (tool descriptions: no "zone"/"price cap"; "the platform applies the catalogue price").
- Test: `internal/agent/*_test.go` (drop zone-cap fixtures; ensure the divide/create path tests pass with `unit_price` pricing).

- [ ] **Step 1:** Update prompts/tool text + any test fixtures seeding zone caps → seed `unit_price` items instead.
- [ ] **Step 2:** `go test ./internal/agent/` green; full gate.
- [ ] **Step 3:** Commit: `refactor(agent): catalogue unit_price pricing in smarts; drop zone/cap wording`

---

## Phase 3: Docs + Final Verification

### Task 3.1: Update docs
**Files:** `CLAUDE.md`, `docs/data-model.md`, `docs/superpowers/specs/2026-06-21-sqlite-db-per-tenant-design.md`.
- [ ] **Step 1:** CLAUDE.md: remove NDIS-as-optional language; describe a generic price list (items + versions + import), generic clients (name/contact/reference/payer), no zones/caps/plan-windows/client-types. Update the "Price list" section + conventions.
- [ ] **Step 2:** `docs/data-model.md` ERD: drop `item_prices`; `clients` loses type/plan/mgmt; `business_profile` loses zone.
- [ ] **Step 3:** DB-per-tenant spec ERD: same table updates.
- [ ] **Step 4:** Commit: `docs: remove NDIS from CLAUDE.md + ERDs`

### Task 3.2: NDIS-surface sweep + final gate
**Files:** none (verification).
- [ ] **Step 1:** Grep acceptance check — `grep -rniE 'ndis|\bzone\b|price.?cap|priceCap|plan.?(start|end|window)|mgmt.?type|ClientType|item_prices|Quotable' --include='*.go' --include='*.ts' --include='*.svelte' --include='*.sql' internal web/src | grep -v node_modules`. Expected: ZERO hits except the migration's `-- +goose Down` artifacts / historical comments. Review every remaining hit; anything live is a missed surface (the spec review specifically flagged `Match.Zone/PriceCap/Quotable`, `LineItemsEditor` cap display, the `businessProfile` store `zone`, `loadPrices`, `zoneLabel`, `CatalogueSearcher` zone param — confirm all gone).
- [ ] **Step 2:** Full gate with race: `gofmt -l .` empty, `go vet ./...`, `go test ./... -race` (0 failures), `CGO_ENABLED=0 go build ./cmd/tallyo`; `cd web && npm run check && npm run build`.
- [ ] **Step 3:** End-to-end smoke on a fresh data dir (real server): `signup` (confirm NO zone field in payload, generic tenant) → create a client (name only) → `POST …/price-list/import/commit` a CSV → invoice a catalogue item by code (prices from `unit_price`) + a free-form line → expect 201, correct subtotal/total. Confirm server log has no errors.
- [ ] **Step 4:** Commit any doc/tracker updates.

---

## Done When
- Full `-race` gate green; frontend `check`/`build` clean.
- Signup has no NDIS prompt; a generic tenant + name-only clients + catalogue/custom/free-form invoicing all work end-to-end (smoke-verified on a real server).
- Grep sweep: no NDIS/zone/cap/plan-window/client-type/item_prices references remain except intentional generic ones.
- Docs (CLAUDE.md + both ERDs) reflect the generic-only model.
