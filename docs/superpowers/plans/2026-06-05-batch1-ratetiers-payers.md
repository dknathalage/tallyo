# Domain Port — Batch 1: rate_tiers + payers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Port two independent base domains — rate_tiers and payers — full-stack (Go API + Svelte SPA) onto the foundation, establishing the reusable per-domain template (migration → sqlc → repository[WithAudit] → service[broadcast] → handlers → createCollectionStore + routes).

**Architecture:** Each domain follows the BusinessProfile/users pattern. Clean-break goose migration ports the table verbatim from `src/lib/db/drizzle-schema.ts`. sqlc typed queries. Repository wraps gen + `audit.WithTx`. Service broadcasts an SSE event after commit. chi REST handlers under `/api/<domain>` behind RequireAuth, ctx threaded. Frontend instantiates `createCollectionStore` + list/new/edit routes.

**Tech Stack:** Go 1.26 (modernc sqlite, sqlc, goose, chi, scs), Svelte 5 runes + TS.

**Spec:** `docs/superpowers/specs/2026-06-05-domain-port-decomposition-design.md`

**Reference templates (copy these patterns):**
- Migration: `internal/db/migrations/00002_auth.sql`
- sqlc queries: `internal/db/queries/users.sql`
- Repository (WithAudit, mid-tx-id manual log, toX mapper, nz helper): `internal/auth/users.go`, `internal/repository/business_profile.go`
- Service (broadcast): `internal/service/business_profile.go`
- Handlers (REST behind RequireAuth, WriteJSON/WriteError/DecodeJSON): `internal/http/business_profile.go`, `internal/http/invites.go` (chi.URLParam)
- Deps wiring: `internal/http/server.go`, `cmd/tallyo/main.go`
- Frontend store: `web/src/lib/stores/collection.svelte.ts` (createCollectionStore), `web/src/lib/stores/businessProfile.svelte.ts`
- Frontend routes: `web/src/routes/settings/+page.svelte`

**Schema to port (verbatim, clean-break):**
```sql
rate_tiers: id INTEGER PK AUTOINCREMENT, uuid TEXT NOT NULL UNIQUE, name TEXT NOT NULL UNIQUE,
            description TEXT DEFAULT '', sort_order INTEGER DEFAULT 0,
            created_at TEXT NOT NULL, updated_at TEXT NOT NULL
payers:     id INTEGER PK AUTOINCREMENT, uuid TEXT NOT NULL UNIQUE, name TEXT NOT NULL,
            email TEXT DEFAULT '', phone TEXT DEFAULT '', address TEXT DEFAULT '',
            metadata TEXT DEFAULT '{}', created_at TEXT NOT NULL, updated_at TEXT NOT NULL
```

**Deferred (note, do NOT build here):** payer `getPayerClients` (needs clients → Batch 2), `buildPayerSnapshot` (needs invoice snapshots → Batch 3). rate_tier `getDefaultTier` (lowest sort_order) — include it, it's pure rate_tiers.

---

## Task 1: Migration 00003 + sqlc queries (rate_tiers, payers)

**Files:**
- Create: `internal/db/migrations/00003_rate_tiers_payers.sql`
- Create: `internal/db/queries/rate_tiers.sql`, `internal/db/queries/payers.sql`
- Regenerate: `internal/db/gen/**`
- Test: append to `internal/db/migrate_test.go`

- [ ] **Step 1: Write the migration** `00003_rate_tiers_payers.sql` (goose Up creating both tables per the schema above with `created_at`/`updated_at` as `TEXT NOT NULL`; goose Down drops payers then rate_tiers). Match the column types/constraints exactly.

- [ ] **Step 2: Write sqlc queries.**
  `rate_tiers.sql`: `ListRateTiers` (`:many` ORDER BY sort_order, id), `GetRateTier` (`:one` by id), `GetDefaultTier` (`:one` ORDER BY sort_order, id LIMIT 1), `CreateRateTier` (`:one` RETURNING *), `UpdateRateTier` (`:one` RETURNING * — set name/description/sort_order/updated_at WHERE id), `DeleteRateTier` (`:exec`).
  `payers.sql`: `ListPayers` (`:many` ORDER BY name), `SearchPayers` (`:many` WHERE name LIKE ? OR email LIKE ? ORDER BY name), `GetPayer` (`:one`), `CreatePayer` (`:one` RETURNING *), `UpdatePayer` (`:one` RETURNING *), `DeletePayer` (`:exec`).

- [ ] **Step 3: Generate + build.** `"$(go env GOPATH)/bin/sqlc" generate && go build ./internal/db/gen/`. INSPECT `internal/db/gen/models.go` and report the exact `RateTier` and `Payer` struct fields + types (nullable → sql.NullString) and the `Create*Params`/`Update*Params` shapes — downstream tasks need them.

- [ ] **Step 4: Migration test** — append `TestMigrateCreatesRateTiersPayers` asserting both tables exist after Migrate (mirror `TestMigrateCreatesAuthTables`).

- [ ] **Step 5: Run** `go test ./internal/db/... -race`, `go vet`, `gofmt`. **Commit** `feat(db): rate_tiers + payers migration and sqlc queries`.

---

## Task 2: RateTier repository

**Files:** Create `internal/repository/rate_tier.go` (+ `_test.go`).

Follow `internal/auth/users.go` exactly (it's the cleanest CRUD-with-WithAudit example).

- [ ] **Step 1: Failing test** `rate_tier_test.go` — temp migrated DB; `NewRateTiers(conn)`; cover: Create (returns row, audited), List (ordered), Get (nil on missing), Update (changes name/desc/sort), Delete, GetDefault (lowest sort_order), Create rejects empty name, audit rows for create+update+delete = 3. Assert via `errors.Is(sql.ErrNoRows)` → nil patterns.

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement** `internal/repository/rate_tier.go`:
  - Domain `RateTier{ID int64; UUID, Name, Description string; SortOrder int64; CreatedAt, UpdatedAt string}` (json tags camelCase: `sortOrder`, `createdAt`, `updatedAt`).
  - `RateTierInput{Name, Description string; SortOrder int64}` (json camelCase).
  - `RateTiersRepo{db *sql.DB}`, `NewRateTiers(db)` panic-if-nil.
  - `List`, `Get`(nil on missing), `GetDefault`(nil on empty), `Create`(validate name; uuid; RFC3339 ts; `audit.WithTx` Action:"" + manual Log with real id, entity "rate_tier"/"create"), `Update`(audit "update", EntityID known), `Delete`(audit "delete").
  - `toRateTier(gen.RateTier) *RateTier`, reuse `nz`.

- [ ] **Step 4: Run** `go test ./internal/repository/ -race`, vet, gofmt. **Commit** `feat(repository): rate tier repository`.

---

## Task 3: Payer repository

**Files:** Create `internal/repository/payer.go` (+ `_test.go`). Same pattern.

- [ ] **Step 1: Failing test** — Create/List/Get/Update/Delete; List search (name/email substring); Create rejects empty name; audit rows. Defaults: email/phone/address default "", metadata default "{}".

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement** `internal/repository/payer.go`:
  - Domain `Payer{ID int64; UUID, Name, Email, Phone, Address, Metadata string; CreatedAt, UpdatedAt string}` (camelCase json).
  - `PayerInput{Name, Email, Phone, Address, Metadata string}`.
  - `PayersRepo{db}`, `NewPayers` panic-if-nil.
  - `List(ctx, search string)` — if search != "" use SearchPayers with `%search%` for both name+email else ListPayers. `Get`, `Create`(validate name; metadata default "{}"; audit "payer"/"create" real id), `Update`, `Delete`.
  - `toPayer`, `nz`.

- [ ] **Step 4: Run** tests, vet, gofmt. **Commit** `feat(repository): payer repository`.

---

## Task 4: RateTier + Payer services (broadcast)

**Files:** Create `internal/service/rate_tier.go`, `internal/service/payer.go` (+ tests).

Follow `internal/service/business_profile.go`: hold repo + `*realtime.Hub`, panic-if-nil-hub, methods take ctx, broadcast AFTER a successful mutation.

- [ ] **Step 1: Failing tests** — for each service: a mutation (Create) persists AND emits an event (`{entity:"rate_tier"|"payer", id:<new id>, action:"create"}`) on a subscribed hub; a failed mutation (empty name) emits NO event. Use the hub-subscribe pattern from `business_profile_test.go`.

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement** each service:
  - `RateTierService{repo, hub}`, `NewRateTierService(db, hub)`. Methods: `List`, `Get`, `GetDefault`, `Create`(broadcast "rate_tier"/created-id/"create"), `Update`(broadcast "update"), `Delete`(broadcast "delete"). Each broadcast happens only after the repo call returns nil; the event ID is the affected row id (Create returns the new RateTier so use its ID; Update/Delete use the id arg).
  - `PayerService` analogous (entity "payer").

- [ ] **Step 4: Run** tests, vet, gofmt. **Commit** `feat(service): rate tier + payer services with SSE broadcast`.

---

## Task 5: RateTier + Payer HTTP handlers

**Files:** Create `internal/http/rate_tiers.go`, `internal/http/payers.go` (+ tests). Modify `internal/http/server.go` (Deps + routes).

Follow `internal/http/business_profile.go` + `invites.go` (chi.URLParam for `{id}`). All routes behind the RequireAuth group.

- [ ] **Step 1: Failing tests** (local chi router + cookiejar + logged-in owner, mirror business_profile_test.go) for each domain:
  - `GET /api/rate-tiers` → 200 list (empty → `[]`, NOT null — return empty slice).
  - `POST /api/rate-tiers {name}` → 201 + created.
  - `GET /api/rate-tiers/{id}` → 200; missing id → 404.
  - `PUT /api/rate-tiers/{id} {name,...}` → 200; empty name → 400.
  - `DELETE /api/rate-tiers/{id}` → 204 (or 200).
  - unauth → 401.
  Same set for `/api/payers` (POST also accepts email/phone/address; GET list supports `?search=`).

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement** handlers:
  - `RateTierHandler{svc}`, `NewRateTierHandler(svc)` panic-if-nil. Methods `List`(WriteJSON the slice; ensure non-nil empty slice → `[]`), `Get`(parse id via `strconv.ParseInt(chi.URLParam(r,"id"),10,64)`, 400 on bad id, 404 on nil), `Create`(DecodeJSON RateTierInput, 400 empty name, 201), `Update`(id + input, 400 empty name, 404 missing, 200), `Delete`(204).
  - `PayerHandler{svc}` analogous; `List` reads `r.URL.Query().Get("search")`.
  - Parse-id helper: extract a small `parseID(r) (int64, error)` shared in a handlers util if convenient (or inline).
- [ ] **Step 4: Wire `server.go`** — add `RateTiers *RateTierHandler` and `Payers *PayerHandler` to `Deps`; in the RequireAuth group register the REST routes (`api.Route("/rate-tiers", ...)` with Get/Post/Get{id}/Put{id}/Delete{id}; same for `/payers`). Add the two to the group-formation guard. Nil-safe.

- [ ] **Step 5: Run** `go test ./internal/http/ -race`, vet, gofmt. **Commit** `feat(http): rate tier + payer REST endpoints`.

---

## Task 6: Wire services into cmd/tallyo

**Files:** Modify `cmd/tallyo/main.go`.

- [ ] **Step 1:** Construct `service.NewRateTierService(conn, hub)` + `service.NewPayerService(conn, hub)`, build their handlers, add to the `httpapi.Deps` literal (RateTiers, Payers). Build.

- [ ] **Step 2: Boot smoke** — start binary, setup+login, then `curl` create+list a rate tier and a payer (authed); confirm 201 + list shows them. Capture output.

- [ ] **Step 3:** vet, gofmt, `go test ./... -race`. **Commit** `feat(cmd): wire rate tier + payer services`.

---

## Task 7: Frontend — rate-tiers + payers UI

**Files:** Create `web/src/lib/stores/rateTiers.svelte.ts`, `web/src/lib/stores/payers.svelte.ts`; routes `web/src/routes/rate-tiers/+page.svelte`, `web/src/routes/payers/+page.svelte` (list + inline create/edit/delete is fine for the skeleton — a full new/edit route per item is optional); modify `web/src/routes/+layout.svelte` (nav links). Extract reusable list/form components if it reduces duplication.

- [ ] **Step 1: Stores** — `rateTiers.svelte.ts`: `export const rateTiers = createCollectionStore<RateTier, RateTierInput>('rate-tiers', 'rate_tier')` with TS types matching the API (camelCase). Same for payers (`'payers', 'payer'`). Define the TS interfaces in `web/src/lib/api/types.ts` (or a per-domain types file).

- [ ] **Step 2: Routes** — each page: `onMount(() => { store.ensureSubscribed(); store.load(); })`; render the list (table) with create form (name + fields) calling `store.crud.create(...)` then `store.load()`; edit (inline or a small form) via `store.crud.update(id, ...)`; delete via `store.crud.remove(id)` then `store.load()`. Show `store.loading`/`store.error`. Use Svelte 5 runes + Tailwind. Keep components small.

- [ ] **Step 3: Nav** — add "Rate Tiers" and "Payers" links to `+layout.svelte` nav (next to Settings).

- [ ] **Step 4: Verify** `cd web && npm run check` (0/0), `npm run build` (emits 200.html), `touch build/.gitkeep`. **Commit** `feat(web): rate tiers + payers UI with live SSE collection stores`.

---

## Task 8: Batch 1 acceptance

- [ ] **Step 1: Gates** — `go test ./... -race`, `go vet ./...`, `gofmt -l .` (non-web) clean; `cd web && npm run check && npm run build`.

- [ ] **Step 2: Live smoke + SSE** — boot the built binary; setup+login; via curl: create 2 rate tiers + 2 payers, list both (verify ordering + search for payers), update one, delete one. Open an SSE stream and confirm a `rate_tier`/`create` event fires on create (the realtime path for a real domain). Capture output.

- [ ] **Step 3: Commit** `chore: batch 1 acceptance — rate_tiers + payers full-stack`.

---

## Done When

- `rate_tiers` + `payers` tables migrated; full CRUD over `/api/rate-tiers` + `/api/payers` behind auth; mutations audited + broadcast SSE events.
- Frontend pages list/create/edit/delete both, live-updating via `createCollectionStore` + SSE.
- `go test ./... -race`, vet, gofmt, `npm run check` all clean; live smoke + SSE event confirmed.

This establishes the domain template. Batch 2 (tax_rates, clients, catalog) replicates it; clients can now reference rate_tiers (FK) and payers.
