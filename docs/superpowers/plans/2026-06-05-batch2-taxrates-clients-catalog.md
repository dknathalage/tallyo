# Domain Port — Batch 2: tax_rates + clients + catalog Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Port tax_rates, clients (FK → rate_tiers + payers), and catalog (catalog_items + catalog_item_rates tier pricing) full-stack onto the foundation, reaching parity with the old query modules.

**Architecture:** Same per-domain pattern as Batch 1 (migration → sqlc → repository[audit.WithTx] → service[broadcast] → handlers → createCollectionStore + routes). Three domain wrinkles: (1) tax_rate `is_default` is **exclusive** — setting a default unsets all others in the same tx; (2) client list LEFT JOINs rate_tiers + payers to surface `pricingTierName`/`payerName`; (3) catalog items have per-tier rates in `catalog_item_rates` (upsert + effective-rate fallback to the item's base rate).

**Spec:** `docs/superpowers/specs/2026-06-05-domain-port-decomposition-design.md`

**Reference templates:** `internal/repository/rate_tier.go` + `payer.go` (Batch 1, the closest CRUD examples), `internal/service/rate_tier.go`, `internal/http/rate_tiers.go` + `payers.go` (+ parseID), `internal/http/server.go` Deps wiring, `web/src/routes/rate-tiers/+page.svelte` + stores.

**Schema to port (verbatim, clean-break; clients/* FKs to rate_tiers/payers which now exist):**
```sql
tax_rates:   id PK AUTOINCREMENT, uuid TEXT NOT NULL UNIQUE, name TEXT NOT NULL,
             rate REAL NOT NULL DEFAULT 0, is_default INTEGER NOT NULL DEFAULT 0,
             created_at TEXT NOT NULL, updated_at TEXT NOT NULL
clients:     id PK AUTOINCREMENT, uuid TEXT NOT NULL UNIQUE, name TEXT NOT NULL,
             email TEXT DEFAULT '', phone TEXT DEFAULT '', address TEXT DEFAULT '',
             pricing_tier_id INTEGER REFERENCES rate_tiers(id) ON DELETE SET NULL,
             metadata TEXT DEFAULT '{}',
             payer_id INTEGER REFERENCES payers(id) ON DELETE SET NULL,
             created_at TEXT NOT NULL, updated_at TEXT NOT NULL
             -- indexes: idx_clients_payer ON (payer_id)
catalog_items: id PK AUTOINCREMENT, uuid TEXT NOT NULL UNIQUE, name TEXT NOT NULL,
             rate REAL NOT NULL DEFAULT 0, unit TEXT DEFAULT '', category TEXT DEFAULT '',
             sku TEXT DEFAULT '', metadata TEXT DEFAULT '{}',
             created_at TEXT NOT NULL, updated_at TEXT NOT NULL
catalog_item_rates: id PK AUTOINCREMENT,
             catalog_item_id INTEGER NOT NULL REFERENCES catalog_items(id) ON DELETE CASCADE,
             rate_tier_id INTEGER NOT NULL REFERENCES rate_tiers(id) ON DELETE CASCADE,
             rate REAL NOT NULL DEFAULT 0,
             UNIQUE(catalog_item_id, rate_tier_id)
```

**Deferred (note, do NOT build here):** `buildClientSnapshot` (→ Batch 3), `getClientRevenueSummary`/client stats (→ Batch 3 rider, needs invoices). Everything else is in scope.

---

## Task 1: Migration 00004 + sqlc queries (all four tables)

**Files:** Create `internal/db/migrations/00004_taxrates_clients_catalog.sql`; `internal/db/queries/{tax_rates,clients,catalog_items,catalog_item_rates}.sql`; regenerate gen; append migration test.

- [ ] **Step 1: Migration** — port the four tables exactly (goose Up; Down drops in FK-safe reverse: catalog_item_rates, catalog_items, clients, tax_rates). FKs on clients reference the existing rate_tiers/payers; catalog_item_rates references catalog_items + rate_tiers.

- [ ] **Step 2: sqlc queries.**
  - `tax_rates.sql`: `ListTaxRates` (ORDER BY is_default DESC, name), `GetTaxRate`, `GetDefaultTaxRate` (WHERE is_default=1 LIMIT 1), `CreateTaxRate` (RETURNING *), `UpdateTaxRate` (RETURNING *), `DeleteTaxRate`, **`ClearDefaultTaxRates`** (`UPDATE tax_rates SET is_default=0` — used to enforce exclusivity).
  - `clients.sql`: `ListClients` + `SearchClients` — both LEFT JOIN rate_tiers + payers, selecting client cols PLUS `rate_tiers.name AS pricing_tier_name`, `payers.name AS payer_name`, ORDER BY clients.name (search: WHERE clients.name LIKE ? OR clients.email LIKE ?). `GetClient` (single, with the same joins). `CreateClient` (RETURNING *), `UpdateClient` (RETURNING *), `DeleteClient`. (pricing_tier_id/payer_id are nullable.)
  - `catalog_items.sql`: `ListCatalogItems` (ORDER BY name), `SearchCatalogItems` (name/sku/category LIKE), `GetCatalogItem`, `ListCategories` (`SELECT DISTINCT category FROM catalog_items WHERE category <> '' ORDER BY category`), `CreateCatalogItem` (RETURNING *), `UpdateCatalogItem` (RETURNING *), `DeleteCatalogItem`.
  - `catalog_item_rates.sql`: `UpsertCatalogItemRate` (`INSERT ... ON CONFLICT(catalog_item_id, rate_tier_id) DO UPDATE SET rate=excluded.rate`), `GetCatalogItemRate` (WHERE catalog_item_id=? AND rate_tier_id=?), `ListRatesForItem` (WHERE catalog_item_id=?), `DeleteCatalogItemRate`.
  NOTE the JOIN queries: sqlc will generate a row struct with the joined columns (e.g. `ListClientsRow{...client cols..., PricingTierName sql.NullString, PayerName sql.NullString}`). Report those generated row types.

- [ ] **Step 3: Generate + build** (`sqlc generate && go build ./internal/db/gen/`). REPORT the exact generated structs: `TaxRate`, `Client`, `CatalogItem`, `CatalogItemRate`, the JOIN row structs (`ListClientsRow`/`GetClientRow`/`SearchClientsRow`), and all Create/Update Params. Critical for downstream tasks. (is_default → likely `int64`/`sql.NullInt64`; rate → `float64`; nullable FKs → `sql.NullInt64`.)

- [ ] **Step 4: Migration test** — `TestMigrateCreatesBatch2Tables` asserting tax_rates, clients, catalog_items, catalog_item_rates exist.

- [ ] **Step 5: Run** `go test ./internal/db/... -race`, vet, gofmt → version 4. **Commit** `feat(db): tax_rates + clients + catalog migration and sqlc queries`.

---

## Task 2: TaxRate repository (exclusive default)

**Files:** Create `internal/repository/tax_rate.go` (+ `_test.go`). Pattern: `rate_tier.go`.

- [ ] **Step 1: Failing test** — Create (audited); Create with isDefault=true sets is_default and (with a pre-existing default) UNSETS the old default — assert only one row has is_default after; GetDefault returns it; List ordered is_default-first then name; Update toggling default; Update with isDefault=true unsets others; Delete; Create rejects empty name.

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement** `internal/repository/tax_rate.go`:
  - Domain `TaxRate{ID int64; UUID, Name string; Rate float64; IsDefault bool; CreatedAt, UpdatedAt string}` (json camelCase: rate, isDefault, createdAt, updatedAt).
  - `TaxRateInput{Name string; Rate float64; IsDefault bool}`.
  - `NewTaxRates(db)` panic-if-nil. List, Get(nil-missing), GetDefault(nil-missing).
  - `Create(ctx, in)`: validate name; in ONE `audit.WithTx` (Action:"" + manual Log real id): if in.IsDefault → `ClearDefaultTaxRates(tx)` FIRST, then `CreateTaxRate` with is_default = bool→int (1/0). Map result.
  - `Update(ctx, id, in)`: same exclusivity — if in.IsDefault → ClearDefaultTaxRates first (in the tx), then UpdateTaxRate; auto-log or manual (id known) "tax_rate"/"update"; nil on ErrNoRows.
  - `Delete(ctx, id)`: audit "tax_rate"/"delete".
  - Helpers: `toTaxRate` (is_default int→bool, rate float64); a bool→int helper for is_default. Keep funcs <60 lines.

- [ ] **Step 4: Run** tests, vet, gofmt. **Commit** `feat(repository): tax rate repository with exclusive default`.

---

## Task 3: Client repository (joins + bulkDelete)

**Files:** Create `internal/repository/client.go` (+ `_test.go`).

- [ ] **Step 1: Failing test** — Create (with pricingTierId + payerId set to a real tier/payer; and with them null); List returns clients with `PricingTierName`/`PayerName` populated when FKs set (create a tier+payer first); search filters name/email; Get; Update; Delete; BulkDelete; Create rejects empty name; FK SET NULL behavior is not tested here (deletion of tiers is Batch 1's concern). Audit rows.

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement** `internal/repository/client.go`:
  - Domain `Client{ID int64; UUID, Name, Email, Phone, Address string; PricingTierID *int64; PricingTierName string; Metadata string; PayerID *int64; PayerName string; CreatedAt, UpdatedAt string}` (json camelCase: pricingTierId, pricingTierName, payerId, payerName, etc.). Use `*int64` for nullable FKs so the JSON is `null` when unset (frontend needs to distinguish).
  - `ClientInput{Name, Email, Phone, Address string; PricingTierID *int64; Metadata string; PayerID *int64}`.
  - `NewClients(db)` panic-if-nil.
  - `List(ctx, search string) ([]*Client, error)` — ListClients or SearchClients (join rows); map joined row → Client (unwrap PricingTierName/PayerName NullString → string; pricing_tier_id/payer_id NullInt64 → *int64).
  - `Get(ctx, id)` (join row, nil-missing). `Create` (validate name; metadata default "{}"; wrap nullable FKs: `sql.NullInt64{Int64:*id, Valid:id!=nil}`; audit "client"/"create" real id). `Update` (audit "update", nil-missing). `Delete` (audit "delete"). `BulkDelete(ctx, ids)` (one tx, bounded loop, single "client"/"bulk_delete" audit).
  - Helpers: `nullID(*int64) sql.NullInt64`, `ptrID(sql.NullInt64) *int64`, map helpers. Keep funcs <60 lines (extract a `toClientFromListRow` / `toClientFromGetRow` if the generated row types differ between queries; if they share a shape you can unify).

- [ ] **Step 4: Run** tests, vet, gofmt. **Commit** `feat(repository): client repository with tier/payer joins and bulk delete`.

---

## Task 4: Catalog repository (+ catalog_item_rates)

**Files:** Create `internal/repository/catalog.go` (+ `_test.go`).

- [ ] **Step 1: Failing test** — CatalogItem Create/List/Get/Update/Delete/BulkDelete/Search/Categories. Tier rates: `SetRate(itemId, tierId, rate)` upserts (insert then update-on-conflict — call twice, assert latest rate); `GetRates(itemId)` lists tier rates; `EffectiveRate(itemId, tierId)` returns the tier-specific rate when set, else the item's base rate; `EffectiveRate(itemId, nil)` returns base rate. Create rejects empty name. Audit rows for item mutations.

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement** `internal/repository/catalog.go`:
  - Domain `CatalogItem{ID int64; UUID, Name string; Rate float64; Unit, Category, Sku, Metadata string; CreatedAt, UpdatedAt string}` (json camelCase). `CatalogItemInput{Name string; Rate float64; Unit, Category, Sku, Metadata string}`. `CatalogItemRate{RateTierID int64; Rate float64}` (json: rateTierId, rate).
  - `NewCatalog(db)` panic-if-nil. List, Search(ctx, search), Get(nil-missing), Categories(ctx)([]string — non-nil), Create (validate name; metadata "{}"; audit "catalog_item"/"create" real id), Update (nil-missing), Delete, BulkDelete.
  - `SetRate(ctx, itemId, tierId int64, rate float64) error` — UpsertCatalogItemRate; audit "catalog_item"/"set_rate" (EntityID itemId, Changes {tierId, rate}). (Use audit.WithTx auto-log; id known.)
  - `GetRates(ctx, itemId) ([]*CatalogItemRate, error)` — non-nil slice.
  - `EffectiveRate(ctx, itemId int64, tierId *int64) (float64, error)` — if tierId != nil: GetCatalogItemRate(itemId,*tierId); if found return its rate; else fall back to Get(itemId).Rate (0 if item missing). If tierId nil: return the item's base rate.
  - Keep funcs <60 lines.

- [ ] **Step 4: Run** tests, vet, gofmt. **Commit** `feat(repository): catalog repository with per-tier rates`.

---

## Task 5: Services (tax_rate, client, catalog) with broadcast

**Files:** Create `internal/service/{tax_rate,client,catalog}.go` (+ tests). Pattern: `internal/service/rate_tier.go`.

- [ ] **Step 1: Failing tests** — for each: Create persists + broadcasts (`{entity, id, action:"create"}`, entities `tax_rate`/`client`/`catalog_item`); empty-name → no event. Catalog `SetRate` broadcasts `catalog_item`/`set_rate` (id = itemId). Client BulkDelete broadcasts `client`/`bulk_delete`.

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement** the three services: hold repo + hub, panic-if-nil-hub, ctx threaded, broadcast after success. Methods mirror each repo's public surface (List/Get/GetDefault/Create/Update/Delete/BulkDelete/Search/Categories/SetRate/GetRates/EffectiveRate as applicable). Broadcast entity strings: `tax_rate`, `client`, `catalog_item`.

- [ ] **Step 4: Run** tests, vet, gofmt. **Commit** `feat(service): tax rate + client + catalog services with broadcast`.

---

## Task 6: HTTP handlers + Deps wiring

**Files:** Create `internal/http/{tax_rates,clients,catalog}.go` (+ tests). Modify `internal/http/server.go`.

- [ ] **Step 1: Failing tests** (local-router + cookiejar + owner) — full CRUD for each:
  - `/api/tax-rates` CRUD (POST accepts name/rate/isDefault; setting isDefault then GET list shows exclusive default).
  - `/api/clients` CRUD (POST accepts pricingTierId/payerId nullable; `?search=`; `POST /api/clients/bulk-delete`; list returns pricingTierName/payerName).
  - `/api/catalog` CRUD (`?search=`, `GET /api/catalog/categories`, `POST /api/catalog/bulk-delete`; tier rates: `PUT /api/catalog/{id}/rates/{tierId} {rate}` → SetRate 200; `GET /api/catalog/{id}/rates` → list).
  - 404/400/401 + non-nil `[]` as established.

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement** handlers (follow `rate_tiers.go`/`payers.go`; reuse `parseID`; for the catalog rates routes parse `{id}` and `{tierId}` via `chi.URLParam`). Map service errors to status codes (500 default; 400 validation; 404 nil; 204 delete). Catalog `SetRate` handler: parse item id + tier id, DecodeJSON `{rate float64}`, call svc, 200.

- [ ] **Step 4: Wire server.go** — add `TaxRates`, `Clients`, `Catalog` handlers to `Deps` + the group guard; register routes in the RequireAuth group (including the catalog sub-routes `/catalog/{id}/rates` and `/catalog/{id}/rates/{tierId}`, and `/categories`, `/bulk-delete`). Nil-safe.

- [ ] **Step 5: Run** `go test ./internal/http/ -race`, vet, gofmt. **Commit** `feat(http): tax rate + client + catalog REST endpoints`.

---

## Task 7: Wire cmd/tallyo

**Files:** Modify `cmd/tallyo/main.go`.

- [ ] **Step 1:** Construct the three services + handlers, add to `httpapi.Deps`. Build.
- [ ] **Step 2: Boot smoke** — setup+login, then curl create+list a tax rate (toggle default), a client (with a tier + payer FK, verify pricingTierName/payerName in list), a catalog item + set a tier rate + get effective rate. Capture output.
- [ ] **Step 3:** vet, gofmt, `go test ./... -race`. **Commit** `feat(cmd): wire tax rate + client + catalog services`.

---

## Task 8: Frontend — tax-rates + clients + catalog UI

**Files:** stores `web/src/lib/stores/{taxRates,clients,catalog}.svelte.ts`; routes `web/src/routes/{tax-rates,clients,catalog}/+page.svelte`; types in `web/src/lib/api/types.ts`; nav in `+layout.svelte`.

- [ ] **Step 1: Types + stores** — TS interfaces (camelCase matching the API incl. client `pricingTierId/pricingTierName/payerId/payerName`, tax `isDefault`, catalog `rate/unit/category/sku`). `createCollectionStore` for each (`'tax-rates','tax_rate'`, `'clients','client'`, `'catalog','catalog_item'`).
- [ ] **Step 2: Routes** — list/create/edit/delete pages (runes + Tailwind, match Batch 1 pages). tax-rates: an "is default" checkbox/toggle. clients: pricing-tier + payer selects populated from the rateTiers/payers stores (load them too), showing names in the list. catalog: base rate + category; a small per-item tier-rate editor (optional inline — at minimum list/create/edit the item; tier rates can be a follow-up if it bloats, but include `SetRate` call wiring). Client-side search filter via `$derived` (as Batch 1).
- [ ] **Step 3: Nav** — add "Tax Rates", "Clients", "Catalog" links.
- [ ] **Step 4: Verify** `npm run check` (0/0), `npm run build` (200.html), `touch build/.gitkeep`. **Commit** `feat(web): tax rates + clients + catalog UI`.

---

## Task 9: Batch 2 acceptance

- [ ] **Step 1: Gates** — `go test ./... -race`, vet, gofmt, `npm run check` + build.
- [ ] **Step 2: Live smoke** — boot binary (wait until `/api/setup/status` responds before firing requests); setup+login; exercise: tax-rate exclusive default (create 2, second default unsets first); client with tier+payer FKs (list shows names); catalog item + tier rate + effective rate; an SSE event for one domain. Capture output.
- [ ] **Step 3: Commit** `chore: batch 2 acceptance — tax_rates + clients + catalog full-stack`.

---

## Done When

- tax_rates (exclusive default), clients (tier/payer joins, bulk delete), catalog (items + per-tier rates, categories, search) all full-stack over `/api/*` behind auth; mutations audited + broadcast.
- Frontend pages for all three, live via SSE stores; clients show tier/payer names + selects.
- All gates green; live smoke confirms exclusive default + client joins + catalog effective-rate + an SSE event.

Batch 3 (invoices) can now reference clients + tax_rates + catalog + rate_tiers, and use `internal/numbering`.
