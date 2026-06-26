# Tallyo Code Migration Implementation Plan (Plan 1 of 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate Tallyo's persistence from embedded SQLite to Postgres, and simplify the app to a single stateless web binary by removing the SSE realtime subsystem (SPA polls), all background sweeps (overdue → UI-derived), and the recurring-invoice feature.

**Architecture:** A modular-monolith Go backend (chi + sqlc + goose) serving an embedded SvelteKit SPA. After this plan: one `cmd/tallyo` binary, Postgres-only (pgx/v5 via `database/sql`), no in-process connection state, no background work. Tenancy stays logical (`tenant_id` guards). This is Plan 1 of 3 — **Plan 2 (Docker/compose)** and **Plan 3 (OpenTofu/Terragrunt)** follow and depend on this one.

**Tech Stack:** Go 1.26, chi v5, `jackc/pgx/v5` (+ `pgx/v5/stdlib`, `pgconn`), sqlc (postgresql engine), goose (postgres dialect), `alexedwards/scs/postgresstore`, SvelteKit + Svelte 5 runes + Vitest.

**Spec:** `docs/superpowers/specs/2026-06-26-postgres-gcp-migration-design.md`

---

## Sequencing & parallelization

**The four phases are SEQUENTIAL — do not run them as parallel worktrees.** They share hot files (`internal/app/app.go`, `internal/app/server.go`, `internal/app/sweep.go`, `internal/invoice/service.go`, and a couple of SPA files). Run in this order:

1. **Phase 1 — Remove recurring feature** (runs on SQLite baseline; stays green).
2. **Phase 2 — Remove overdue sweep → UI-derived** (SQLite baseline; recurring already gone, so `sweep.go` is deleted *entirely* here — use **Variant A**, skip Variant B).
3. **Phase 3 — Remove SSE realtime → polling** (SQLite baseline; the `overdue_sweep` broadcast is already gone; recurring broadcasts already gone).
4. **Phase 4 — Migrate SQLite → Postgres** (requires 1–3 done: the recurring/overdue queries are already deleted, so nothing to port).

**Where parallelism actually exists** (not within this plan):
- **Plan 2 (Docker)** and **Plan 3 (IaC)** are independent of each other; Plan 3 is independent of this plan's code (depends only on an image existing). They can be drafted/executed in parallel once this plan lands.
- Plan-drafting was already parallelized (4 agents).

**Rule for every phase:** after each task's commit, `go build ./... && go test ./...` (Phases 1–3 against SQLite) and `cd web && npm run check` stay green. Phase 4 runs tests against Postgres via `TEST_DATABASE_URL`.

---

## File-structure map (what changes)

**Deleted:** `internal/recurring/` (slice), `internal/realtime/` (hub + SSE handler), `internal/events/` (Notifier), `internal/app/sweep.go`, `internal/app/recurring_test.go`, `internal/app/events_test.go`, `internal/db/sqlite_test.go`, `internal/db/queries/recurring_templates.sql`, `web/src/lib/realtime/events.ts`, `web/src/lib/stores/recurring.svelte.ts`, `web/src/routes/[tenant]/recurring/`.

**Renamed:** `internal/db/sqlite.go` → `internal/db/postgres.go`.

**Created:** `internal/db/testdb.go` (test helper), `web/src/lib/invoiceStatus.ts` (+ test), `web/src/lib/realtime/poll.ts` (+ test).

**Heavily modified:** `internal/app/app.go`, `internal/app/server.go`, every `internal/*/service.go` (drop hub/notifier), `internal/numbering/numbering.go`, `internal/db/migrate.go`, `internal/auth/session.go`, `sqlc.yaml`, all `internal/db/queries/*.sql`, all `internal/db/migrations/**/*.sql`, `internal/db/migrate_test.go`, `cmd/tallyo/main.go`, `.env`, three SPA stores + `+layout.svelte` + a few invoice routes.

---

<!-- ============================ PHASE 1 ============================ -->

## Phase 1 — Remove recurring feature

Runs on the current SQLite baseline. Order: (1) unwire backend + delete slice + drop sweep generation, (2) delete queries + regenerate gen, (3) edit baseline migration + fix migrate_test, (4) entrypoint/.env, (5) SPA.

> **MERGE-HOTSPOT:** Task 1 edits `internal/app/app.go`, `internal/app/server.go`, and `internal/app/sweep.go` (the recurring half only — overdue Phase 2 deletes the whole file). These are sequenced, not parallel.

### Task 1 — Unwire recurring from `internal/app` and delete the slice

**Files:**
- Modify: `internal/app/app.go` (import; `Config.FeatureRecurring` field ~:49; `recurringSvc := recurring.NewService(...)` ~:163; `Recurring:` Deps entry ~:204; `"recurring"` features entry ~:211; `recForSweep` block ~:231-236; the recurring args to `runSweepOnce`/`runSweeper` ~:237,:239)
- Modify: `internal/app/server.go` (import ~:17; `Recurring *recurring.Handler` field ~:47; route block ~:156-158)
- Modify: `internal/app/sweep.go` (import ~:9; `rec *recurring.Service` param ~:25,:52; recurring block ~:39-46; internal call ~:60)
- Delete: `internal/app/recurring_test.go`
- Delete: `internal/recurring/` (all 12 files)

- [ ] **Step 1:** `rm internal/app/recurring_test.go` (imports the slice; must go first).
- [ ] **Step 2:** `rm -rf internal/recurring`.
- [ ] **Step 3:** `server.go` — remove the `internal/recurring` import, the `Recurring *recurring.Handler` Deps field, and the route block:
  ```go
  			if deps.Recurring != nil && deps.Features["recurring"] {
  				deps.Recurring.Routes(pr)
  			}
  ```
- [ ] **Step 4:** `app.go` — remove the `internal/recurring` import, the `FeatureRecurring bool` Config field, `recurringSvc := recurring.NewService(database, hub)`, the `Recurring: recurring.NewHandler(recurringSvc),` Deps entry, and the `"recurring": cfg.FeatureRecurring,` features entry.
- [ ] **Step 5:** `app.go` sweep wiring — delete the `recForSweep` block (~:231-236) and drop the recurring arg from the two sweep calls: `runSweepOnce(tenants.ActiveTenantIDs, invoiceSvc, logger)` and `go runSweeper(tenants.ActiveTenantIDs, invoiceSvc, logger, overdueDone)`.
- [ ] **Step 6:** `sweep.go` — remove the `internal/recurring` import; drop the `rec *recurring.Service` param from `runSweepOnce` (:25) and `runSweeper` (:52); delete the recurring block (~:39-46); fix the internal `runSweepOnce(activeTenants, inv, logger)` call (:60); update the doc comment to "overdue sweep" only.
- [ ] **Step 7:** Verify: `go build ./... && go vet ./internal/app/... && go test ./internal/app/ ./internal/invoice/...` → build/vet clean, tests PASS. `gofmt -l internal/app/` → empty.
- [ ] **Step 8:** Commit: `feat(recurring)!: remove recurring slice and app wiring`

### Task 2 — Delete recurring queries and regenerate gen

**Files:** Delete `internal/db/queries/recurring_templates.sql`; regenerate `internal/db/gen/` (do not hand-edit).

- [ ] **Step 1:** `rm internal/db/queries/recurring_templates.sql`.
- [ ] **Step 2:** `"$(go env GOPATH)/bin/sqlc" generate` → success (still sqlite engine — correct, Postgres is Phase 4).
- [ ] **Step 3:** `grep -rn "RecurringTemplate" internal/db/gen/` → no output.
- [ ] **Step 4:** `go build ./... && go test ./...` → all PASS.
- [ ] **Step 5:** Commit: `feat(recurring)!: drop recurring_templates queries, regenerate gen`

### Task 3 — Drop the `recurring_templates` table from the baseline migration

**Files:** `internal/db/migrations/tenant/00001_tenant.sql` (CREATE TABLE + 3 indexes ~:216-233; DROP in Down ~:236); `internal/db/migrate_test.go` (remove `"recurring_templates"` from expected-tables list ~:39).

- [ ] **Step 1:** Remove the full `CREATE TABLE recurring_templates (...)` block and the three `idx_recurring_*` index lines from `-- +goose Up`.
- [ ] **Step 2:** Remove `DROP TABLE recurring_templates;` from `-- +goose Down`.
- [ ] **Step 3:** In `migrate_test.go:39`, remove `"recurring_templates", ` from the table list.
- [ ] **Step 4:** `grep -rni "recurring" internal/db/` → no output.
- [ ] **Step 5:** `go test ./internal/db/... ./...` → all PASS.
- [ ] **Step 6:** Commit: `feat(recurring)!: drop recurring_templates table from baseline migration`

### Task 4 — Retire the `FeatureRecurring` env gate

**Files:** `cmd/tallyo/main.go` (~:38); `.env` (~:7).

- [ ] **Step 1:** Delete `cmd/tallyo/main.go:38` `FeatureRecurring: app.EnvBool("TALLYO_FEATURE_RECURRING", true),` (Config field already gone in Task 1).
- [ ] **Step 2:** Delete the `TALLYO_FEATURE_RECURRING=...` line in `.env`.
- [ ] **Step 3:** `grep -rn "TALLYO_FEATURE_RECURRING\|FeatureRecurring" . --include=*.go --include=.env` → no output.
- [ ] **Step 4:** `CGO_ENABLED=0 go build ./cmd/tallyo` → success.
- [ ] **Step 5:** Commit: `feat(recurring)!: remove TALLYO_FEATURE_RECURRING gate`

### Task 5 — Remove recurring from the SPA

**Files:** Delete `web/src/lib/stores/recurring.svelte.ts`, `web/src/routes/[tenant]/recurring/`; modify `web/src/lib/stores/features.svelte.ts` (recurring getter), `web/src/routes/[tenant]/+layout.svelte` (nav link), `web/src/lib/api/types.ts` (recurring types).

- [ ] **Step 1:** `rm web/src/lib/stores/recurring.svelte.ts` and `rm -rf "web/src/routes/[tenant]/recurring"`.
- [ ] **Step 2:** `features.svelte.ts` — remove the `recurring` getter (it is the last getter; no trailing-comma fix needed).
- [ ] **Step 3:** `+layout.svelte` — remove the recurring nav-link entry (`...(features.recurring ? [{ href: '/recurring', label: 'Recurring' }] : [])`) and its array seam.
- [ ] **Step 4:** `api/types.ts` — delete `RecurringFrequency`, `RecurringLine`, `RecurringTemplate`, `RecurringInput`.
- [ ] **Step 5:** `grep -rni "recurring" web/src` → no output.
- [ ] **Step 6:** `cd web && npm run check` → 0 errors / 0 warnings; `cd web && npm run build` → success.
- [ ] **Step 7:** Commit: `feat(recurring)!: remove recurring UI, store, and types from SPA`

**Phase 1 exit:** `grep -rni "recurring" internal cmd web/src --include=*.go --include=*.svelte --include=*.ts --include=*.sql` → empty (incidental prose in `internal/billing/*` comments / CLAUDE.md / docs is out of scope). `go test ./...` + `cd web && npm run check` green.

---

<!-- ============================ PHASE 2 ============================ -->

## Phase 2 — Remove overdue sweep (overdue → UI-derived)

Runs on SQLite baseline. **Recurring is already removed (Phase 1), so use Variant A in Task 5: delete `internal/app/sweep.go` entirely and remove its goroutine launch in `app.go`.** Ignore the conditional Variant B.

> **MERGE-HOTSPOT:** `internal/app/sweep.go`, `internal/app/app.go`, `internal/invoice/service.go`.

### Task 1 — Frontend overdue helper + test

**Files:** new `web/src/lib/invoiceStatus.ts`, `web/src/lib/invoiceStatus.test.ts`.

- [ ] **Step 1:** Create `web/src/lib/invoiceStatus.ts`:
  ```ts
  // Overdue is a UI-derived display state, not a stored status. The server stores
  // only 'draft' | 'sent' | 'paid'; an invoice reads as overdue when it is 'sent'
  // and its due date is strictly before today. (spec §2.2 — overdue sweep removed.)

  export type StoredInvoiceStatus = 'draft' | 'sent' | 'paid' | string;
  export type EffectiveInvoiceStatus = StoredInvoiceStatus | 'overdue';

  function todayYMD(): string {
  	const d = new Date();
  	const y = d.getFullYear();
  	const m = String(d.getMonth() + 1).padStart(2, '0');
  	const day = String(d.getDate()).padStart(2, '0');
  	return `${y}-${m}-${day}`;
  }

  /** A 'sent' invoice past its due date (compared by YYYY-MM-DD prefix). Blank dueDate is not overdue. */
  export function isOverdue(status: string, dueDate: string | null | undefined): boolean {
  	if (status !== 'sent') return false;
  	if (!dueDate) return false;
  	return dueDate.slice(0, 10) < todayYMD();
  }

  /** Status to DISPLAY: stored status, except a past-due 'sent' surfaces as 'overdue'. Never persisted. */
  export function effectiveStatus(status: string, dueDate: string | null | undefined): EffectiveInvoiceStatus {
  	return isOverdue(status, dueDate) ? 'overdue' : status;
  }
  ```
- [ ] **Step 2:** Create `web/src/lib/invoiceStatus.test.ts`:
  ```ts
  import { describe, it, expect } from 'vitest';
  import { isOverdue, effectiveStatus } from './invoiceStatus';

  const PAST = '2000-01-01';
  const FUTURE = '2999-12-31';

  describe('isOverdue', () => {
  	it('true only for past-due sent', () => expect(isOverdue('sent', PAST)).toBe(true));
  	it('false for future sent', () => expect(isOverdue('sent', FUTURE)).toBe(false));
  	it('false for non-sent', () => {
  		expect(isOverdue('draft', PAST)).toBe(false);
  		expect(isOverdue('paid', PAST)).toBe(false);
  	});
  	it('false for blank dueDate', () => {
  		expect(isOverdue('sent', null)).toBe(false);
  		expect(isOverdue('sent', undefined)).toBe(false);
  		expect(isOverdue('sent', '')).toBe(false);
  	});
  	it('ignores time-of-day noise', () => expect(isOverdue('sent', `${PAST}T23:59:59Z`)).toBe(true));
  });

  describe('effectiveStatus', () => {
  	it('promotes past-due sent', () => expect(effectiveStatus('sent', PAST)).toBe('overdue'));
  	it('passes through otherwise', () => {
  		expect(effectiveStatus('sent', FUTURE)).toBe('sent');
  		expect(effectiveStatus('draft', PAST)).toBe('draft');
  		expect(effectiveStatus('paid', PAST)).toBe('paid');
  	});
  });
  ```
- [ ] **Step 3:** Verify: `cd web && npm run check` → 0/0; `cd web && npx vitest run src/lib/invoiceStatus.test.ts` → all pass.
- [ ] **Step 4:** Commit: `feat(web): add UI-derived invoice overdue helper + test`

### Task 2 — Wire helper into the three frontend read sites

**Files:** `web/src/routes/[tenant]/invoices/[uuid]/+page.svelte` (detail badge/`statusTone` ~:245, badge ~:441/:567, `nextAction` ~:261, Smart gate ~:513); `web/src/lib/components/ClientEditor.svelte` (~:146/:152); `web/src/routes/[tenant]/invoices/+page.svelte` (`STATUSES` ~:9, status column ~:52-57).

- [ ] **Step 1:** Detail page — `import { effectiveStatus } from '$lib/invoiceStatus';`; pass `effectiveStatus((detail ?? row).status, (detail ?? row).dueDate)` to the badge/tone at the two sites. Simplify `nextAction` to gate on `status === 'sent'` (overdue invoices are stored `sent`). In the follow-up Smart gate drop the dead `=== 'overdue'` disjunct, leaving `=== 'sent'`.
- [ ] **Step 2:** `ClientEditor.svelte` — if a `dueDate` is in scope at the `invStatusTone` call, switch to `invStatusTone(effectiveStatus(inv.status, inv.dueDate))` (+ import); if not, leave `invStatusTone` unchanged (the `'overdue'` case becomes harmless dead code) and note the choice in the commit body.
- [ ] **Step 3:** List page — remove `'overdue'` from `STATUSES` (→ `['draft', 'sent', 'paid']`); add a `cell: (inv) => effectiveStatus(inv.status, inv.dueDate)` to the status column so a past-due `sent` row displays "overdue" (it still filters under `sent`); `import { effectiveStatus }`. **Design note (record in commit):** the server status filter is server-side + paginated, so a dedicated client-side "overdue" filter chip is dropped — per spec §2.2 the requirement is only that overdue is *shown*.
- [ ] **Step 4:** Verify: `cd web && npm run check` → 0/0; `cd web && npx vitest run` → pass; `grep -rn "status === 'overdue'" web/src` → only tone-case matches, no live status reads.
- [ ] **Step 5:** Commit: `feat(web): derive invoice overdue from due_date client-side`

### Task 3 — Remove the read-time sweep in `Handler.List`

**Files:** `internal/invoice/handler.go` (~:124-129).

- [ ] **Step 1:** Delete the `MarkOverdueForTenant` call + error-log block at the top of `List`; update the `List` doc comment. Prune now-unused imports (`slog`, `reqctx`) — grep before removing each.
- [ ] **Step 2:** Verify: `go build ./... && go vet ./...` clean; `gofmt -l internal/invoice/` empty; `go test ./internal/invoice/...` PASS.
- [ ] **Step 3:** Commit: `refactor(invoice): drop read-time overdue sweep from list handler`

### Task 4 — Remove the overdue flip machinery + query

**Files:** `internal/invoice/service.go` (`MarkOverdueForTenant` ~:357-371); `internal/invoice/status.go` (repo `MarkOverdueForTenant` ~:80-111, `flipOverdue` ~:123-134); `internal/invoice/repository.go` (`OverdueInvoice` ~:77-82); `internal/db/queries/invoices.sql` (`SelectOverdueInvoicesForTenant` ~:77-79); regenerate `internal/db/gen/`.

> **Note (corrected from review):** `ActiveTenantIDs` is NOT kept "because the sweep needs it." The sweep driver uses **`auth`'s** `tenants.ActiveTenantIDs` (`app.go:237/239`), not the invoice copy. The invoice `Service.ActiveTenantIDs` + `InvoicesRepo.ActiveTenantIDs` are referenced only by their own tests. Since the sweep (the sole production consumer of *either* copy) is removed in Task 5, **all** `ActiveTenantIDs` + the shared `ListActiveTenantIDs` query are removed in Task 5. This task removes the overdue-flip machinery + the overdue tests; Task 5 removes the sweep + `ActiveTenantIDs`.

- [ ] **Step 1:** Delete `MarkOverdueForTenant` from `internal/invoice/service.go`.
- [ ] **Step 2:** Delete `MarkOverdueForTenant` + `flipOverdue` (incl. the sent→overdue audit write) from `status.go`; remove now-unused `errors` import if applicable; update the file-top comment. (Leave `ActiveTenantIDs` in both `service.go` and `status.go` for Task 5.)
- [ ] **Step 3:** Delete the `OverdueInvoice` struct from `repository.go`.
- [ ] **Step 4:** Delete the `SelectOverdueInvoicesForTenant` block from `invoices.sql` (this removes the only `date('now')` use). **Do NOT delete `ListActiveTenantIDs` yet** — it lives in `internal/db/queries/tenants.sql:15` and is still referenced by `auth` + invoice `ActiveTenantIDs` (removed in Task 5).
- [ ] **Step 5:** Delete the overdue tests that reference the removed symbols: `TestSweepSkipsSuspendedAndScopesBroadcast` (`internal/invoice/service_test.go:69`, calls `MarkOverdueForTenant` + asserts the `overdue_sweep` broadcast), `TestInvoiceMarkOverdueForTenant` (`internal/invoice/repository_extra_test.go:102`), and `TestInvoiceMarkOverdueRequiresTenant` (`internal/invoice/repository_extra_test.go:145`). Keep `TestInvoiceActiveTenantIDs` (:153) / the `ActiveTenantIDs` assertion for now — Task 5 removes them with the method.
- [ ] **Step 6:** `"$(go env GOPATH)/bin/sqlc" generate` → success; `git diff internal/db/gen/` shows only the removed `SelectOverdueInvoicesForTenant` method.
- [ ] **Step 7:** Verify: `go build ./... && go vet ./...` clean; `gofmt -l internal/` empty; `grep -rn "MarkOverdueForTenant\|flipOverdue\|SelectOverdueInvoicesForTenant\|OverdueInvoice" internal/` → only `internal/app/sweep.go` + `app.go` (removed next task); `go test ./...` PASS.
- [ ] **Step 8:** Commit: `refactor(invoice): remove overdue flip machinery (service/repo/query/type)`

### Task 5 — Delete the sweep driver + all `ActiveTenantIDs` (recurring already gone)

**Files:** delete `internal/app/sweep.go`; `internal/app/app.go` (sweep launch block ~:237-240 + preceding comment — **but keep `tenants := auth.NewTenants(database)` at :152, it is still used by `Deps.Tenants`/`Signup`/`Auth` at :189/:191/:192**); `internal/auth/tenants.go` (`ActiveTenantIDs` ~:56-62); `internal/invoice/service.go` (`ActiveTenantIDs` ~:351-355) + `internal/invoice/status.go` (`ActiveTenantIDs` ~:113-121); `internal/db/queries/tenants.sql:15` (`ListActiveTenantIDs`); `internal/invoice/{service_test.go,repository_extra_test.go}` (the `ActiveTenantIDs` test + assertion).

- [ ] **Step 1:** `rm internal/app/sweep.go`.
- [ ] **Step 2:** In `app.go`, delete the launch block (and preceding sweep comment):
  ```go
  	runSweepOnce(tenants.ActiveTenantIDs, invoiceSvc, logger)
  	overdueDone := make(chan struct{})
  	go runSweeper(tenants.ActiveTenantIDs, invoiceSvc, logger, overdueDone)
  	defer close(overdueDone)
  ```
  **Do NOT remove `tenants := auth.NewTenants(database)`** — it stays (used by the Deps wiring). Prune any now-unused `time`/`reqctx` imports in `app.go` (grep first).
- [ ] **Step 3:** The sweep was the only production caller of `ActiveTenantIDs` (both copies). Remove them all:
  - `internal/auth/tenants.go`: delete `TenantsRepo.ActiveTenantIDs` (~:56-62).
  - `internal/invoice/service.go`: delete `Service.ActiveTenantIDs` (~:351-355).
  - `internal/invoice/status.go`: delete `InvoicesRepo.ActiveTenantIDs` (~:113-121).
  - `internal/db/queries/tenants.sql`: delete the `ListActiveTenantIDs` query (~:15, now unreferenced by both), then `"$(go env GOPATH)/bin/sqlc" generate`.
  - Delete `TestInvoiceActiveTenantIDs` (`repository_extra_test.go:153`) and the `ActiveTenantIDs` assertion block in `service_test.go` (~:83-88, if that test wasn't already removed in Task 4).
- [ ] **Step 4:** Verify: `go build ./... && go vet ./...` clean (confirms no dangling `ActiveTenantIDs`/`ListActiveTenantIDs` refs in `auth`, `invoice`, `gen`); `gofmt -l internal/` empty; `CGO_ENABLED=0 go build ./cmd/tallyo` builds; `grep -rn "MarkOverdueForTenant\|flipOverdue\|SelectOverdueInvoicesForTenant\|OverdueInvoice\|overdueSweepInterval\|runSweeper\|ActiveTenantIDs\|ListActiveTenantIDs" internal/ cmd/` → no matches; `grep -rni "overdue" internal/invoice/` → none; `go test ./...` PASS.
- [ ] **Step 5:** Commit: `refactor(app): remove overdue sweep + ActiveTenantIDs; overdue is now UI-derived`

**Phase 2 exit:** no `MarkOverdueForTenant`/`flipOverdue`/`SelectOverdueInvoicesForTenant`/`OverdueInvoice` in Go; invoice API returns raw status; SPA shows computed overdue at list cell + detail badge + ClientEditor; dashboard `+page.svelte:29` untouched. Full gate green.

---

<!-- ============================ PHASE 3 ============================ -->

## Phase 3 — Remove SSE realtime → client polling

Runs on SQLite baseline. Strip broadcasts from each slice *while* `internal/realtime/` still exists (so it still compiles), then delete `realtime/`+`events/`+app wiring, then swap the frontend.

> **Finding carried from drafting:** broadcasts are NOT only the notifier CRUD calls. Many slices also call `s.hub.Broadcast(realtime.Event{...})` **directly** for non-CRUD actions, AND per-slice tests `hub.Subscribe(tenantID)` and assert events fire — those assertions must be deleted too. Recurring broadcasts and the invoice `overdue_sweep` broadcast are already gone (Phases 1–2).
>
> **MERGE-HOTSPOT:** `internal/app/app.go`, `internal/app/server.go`.

### Task 1 — Strip broadcasts from notifier-style CRUD slices

**Files:** `internal/{client,payer,taxrate,catalogue,businessprofile}/service.go` + their `*_test.go`. (taxrate has only `events`, no direct broadcast; catalogue has direct `bulk_delete`+`import` broadcasts; businessprofile uses the hub directly with a nil-hub panic.)

- [ ] **Step 1:** For each slice's `service.go`: remove `events`/`realtime` imports, the `hub *realtime.Hub` and/or `events events.Notifier` fields, the nil-hub panic, the `hub` ctor param (→ e.g. `NewService(db db.Executor) *Service`), and every `s.events.*` and `s.hub.Broadcast(...)` call. Drop the stale "nil hub is a programmer error" comment.
- [ ] **Step 2:** For each `*_test.go`: change helpers from `hub := realtime.NewHub(); NewService(conn, hub)` → `NewService(conn)` (drop returned hub); delete every `hub.Subscribe(...)` block + event assertions; remove the `realtime` import; fix helper call-site arity.
- [ ] **Step 3:** Verify (per slice or batched): `go test ./internal/client/... ./internal/payer/... ./internal/taxrate/... ./internal/catalogue/... ./internal/businessprofile/...` → all `ok`; `gofmt -l` on those dirs empty. (`realtime/` still exists → other packages compile.)
- [ ] **Step 4:** Commit: `refactor(slices): drop SSE broadcasts from client/payer/taxrate/catalogue/businessprofile`

### Task 2 — Strip broadcasts from billing slices + payment service

**Files:** `internal/invoice/service.go` (fields/ctor/panic + notifier + remaining direct broadcasts — `overdue_sweep` already gone), `internal/invoice/payment_service.go` (field/ctor/panic + 6 broadcasts), `internal/estimate/service.go`, `internal/session/service.go` + `service_items.go`; their `*_test.go` helpers + assertions.

- [ ] **Step 1:** Remove from each: `events`/`realtime` imports, fields, nil-hub panic, the `hub` ctor param, every `s.events.*` and `s.hub.Broadcast(...)`. New signatures: `invoice.NewService(db, sessions SessionLinker)`, `invoice.NewPaymentService(db)`, `estimate.NewService(db)`, `session.NewService(db, invoices InvoiceChecker)`. Keep all non-broadcast logic.
- [ ] **Step 2:** Update test helpers to new signatures; delete `hub.Subscribe`/event-assertion blocks; remove `realtime` imports; fix call sites. **Note the nested constructors:** `invoice/service_test.go:72` builds `NewService(conn, hub, session.NewService(conn, hub, NewInvoices(conn)))` → `NewService(conn, session.NewService(conn, NewInvoices(conn)))`; `session/service_test.go` (`:18,:57,:170,:211,:248`) builds `NewService(conn, hub, invoice.NewInvoices(conn))` → `NewService(conn, invoice.NewInvoices(conn))`. Drop the `hub`/`realtime.NewHub()` from the inner *and* outer calls.
- [ ] **Step 3:** Verify: `go test ./internal/invoice/... ./internal/estimate/... ./internal/session/...` → `ok`; `gofmt -l` empty.
- [ ] **Step 4:** Commit: `refactor(billing): drop SSE broadcasts from invoice/estimate/session/payment services`

### Task 3 — Delete `internal/realtime/` + `internal/events/`; rewire `internal/app`

**Files:** delete `internal/realtime/*.go` + `internal/events/events.go` + `internal/app/events_test.go`; modify `internal/app/app.go` (`realtime` import; `hub := realtime.NewHub()` ~:149; `hub` arg in all nine `New*Service(...)` ~:154-162) and `internal/app/server.go` (`realtime` import; `Events` field ~:37; `/events` route ~:123-124); fix all remaining `internal/app/*_test.go` that build `realtime.NewHub()`.

- [ ] **Step 1:** `rm internal/realtime/hub.go internal/realtime/hub_test.go internal/realtime/events_handler.go internal/realtime/events_handler_test.go internal/events/events.go internal/app/events_test.go`.
- [ ] **Step 2:** `app.go` — remove the `realtime` import + `hub := realtime.NewHub()`; drop the `hub` arg from all nine `New*Service(...)` calls (coordinate with the Phase 1/2 edits already applied in this region).
- [ ] **Step 3:** `server.go` — remove the `realtime` import, `Events` field, and `/events` route block.
- [ ] **Step 4:** In every remaining `internal/app/*_test.go` (`business_profile_test.go`, `catalogue_test.go`, `catalogue_import_test.go`, `clients_test.go`, `estimates_test.go`, `invoices_test.go`, `payers_test.go`, `payments_test.go`, `sessions_test.go`, `tax_rates_test.go`, `validation_e2e_test.go`): remove `realtime` import + `hub := realtime.NewHub()`, drop `hub` from constructions, delete any `hub.Subscribe`/assertions.
- [ ] **Step 5:** Verify: `CGO_ENABLED=0 go build ./cmd/tallyo` success; `go test ./...` all `ok`; `go vet ./...` clean; `gofmt -l .` empty; `grep -rn 'realtime\|events\.Notifier' internal/ --include='*.go'` → empty.
- [ ] **Step 6:** Update CLAUDE.md: remove the "broadcasts an SSE event from the service after commit" convention and the `internal/events/` + `internal/realtime/` Platform bullets (prose only, minimal).
- [ ] **Step 7:** Commit: `feat(app): delete realtime/SSE subsystem and events.Notifier`

### Task 4 — Add the SPA poll helper + test

**Files:** new `web/src/lib/realtime/poll.ts`, `web/src/lib/realtime/poll.test.ts`. (Vitest env is `node`; the test stubs `window`/`document`.)

- [ ] **Step 1:** Create `web/src/lib/realtime/poll.ts`:
  ```ts
  /**
   * Polling replacement for the removed SSE subscription. Calls refetch once
   * immediately, then on a fixed interval and on visibility/focus regain.
   * Returns a cleanup that stops everything. SSR-safe (no-op without window).
   * ponytail: fixed 30s interval + focus refetch; tune only if stale or chatty.
   */
  const POLL_INTERVAL_MS = 30_000;

  export function startPolling(refetch: () => void): () => void {
  	if (typeof refetch !== 'function') throw new Error('startPolling: refetch must be a function');
  	if (typeof window === 'undefined' || typeof document === 'undefined') return () => {};

  	refetch();
  	const interval = setInterval(refetch, POLL_INTERVAL_MS);
  	const onVisible = (): void => {
  		if (document.visibilityState === 'visible') refetch();
  	};
  	document.addEventListener('visibilitychange', onVisible);
  	window.addEventListener('focus', refetch);

  	return () => {
  		clearInterval(interval);
  		document.removeEventListener('visibilitychange', onVisible);
  		window.removeEventListener('focus', refetch);
  	};
  }
  ```
- [ ] **Step 2:** Create `web/src/lib/realtime/poll.test.ts` (fake timers + minimal window/document stubs):
  ```ts
  import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

  type Handler = () => void;
  function installDom() {
  	const dl = new Map<string, Set<Handler>>();
  	const wl = new Map<string, Set<Handler>>();
  	const add = (m: Map<string, Set<Handler>>) => (t: string, cb: Handler) => {
  		let s = m.get(t); if (!s) m.set(t, (s = new Set())); s.add(cb);
  	};
  	const rm = (m: Map<string, Set<Handler>>) => (t: string, cb: Handler) => { m.get(t)?.delete(cb); };
  	const fire = (m: Map<string, Set<Handler>>, t: string) => { for (const cb of m.get(t) ?? []) cb(); };
  	const doc = { visibilityState: 'visible' as DocumentVisibilityState, addEventListener: add(dl), removeEventListener: rm(dl) };
  	const win = { addEventListener: add(wl), removeEventListener: rm(wl) };
  	(globalThis as unknown as { window: unknown }).window = win;
  	(globalThis as unknown as { document: unknown }).document = doc;
  	return {
  		fireFocus: () => fire(wl, 'focus'),
  		fireVisibility: () => fire(dl, 'visibilitychange'),
  		docCount: () => dl.get('visibilitychange')?.size ?? 0,
  		winCount: () => wl.get('focus')?.size ?? 0
  	};
  }

  describe('startPolling', () => {
  	let dom: ReturnType<typeof installDom>;
  	beforeEach(() => { vi.useFakeTimers(); dom = installDom(); });
  	afterEach(() => {
  		vi.useRealTimers();
  		delete (globalThis as Record<string, unknown>).window;
  		delete (globalThis as Record<string, unknown>).document;
  	});

  	it('refetches immediately, on interval, on focus/visibility; cleanup stops all', async () => {
  		const { startPolling } = await import('./poll');
  		const refetch = vi.fn();
  		const stop = startPolling(refetch);
  		expect(refetch).toHaveBeenCalledTimes(1);
  		vi.advanceTimersByTime(30_000);
  		expect(refetch).toHaveBeenCalledTimes(2);
  		dom.fireFocus();
  		expect(refetch).toHaveBeenCalledTimes(3);
  		dom.fireVisibility();
  		expect(refetch).toHaveBeenCalledTimes(4);
  		stop();
  		expect(dom.docCount()).toBe(0);
  		expect(dom.winCount()).toBe(0);
  		vi.advanceTimersByTime(60_000);
  		dom.fireFocus();
  		expect(refetch).toHaveBeenCalledTimes(4);
  	});

  	it('is a no-op without a window (SSR)', async () => {
  		delete (globalThis as Record<string, unknown>).window;
  		delete (globalThis as Record<string, unknown>).document;
  		const { startPolling } = await import('./poll');
  		const refetch = vi.fn();
  		const stop = startPolling(refetch);
  		expect(refetch).not.toHaveBeenCalled();
  		expect(typeof stop).toBe('function');
  		stop();
  	});
  });
  ```
- [ ] **Step 3:** Verify: `cd web && npx vitest run src/lib/realtime/poll.test.ts` → 2 passed; `cd web && npm run check` → 0/0.
- [ ] **Step 4:** Commit: `feat(web): add interval+focus poll helper to replace SSE`

### Task 5 — Swap stores to polling; delete `events.ts`; clean layout

**Files:** `web/src/lib/stores/{collection,sessions,businessProfile}.svelte.ts`; delete `web/src/lib/realtime/events.ts`; `web/src/routes/[tenant]/+layout.svelte`. Keep the public method names (`ensureSubscribed`, `subscribe`) so existing call sites across components/routes need no edits — only the method *bodies* change.

- [ ] **Step 1:** `collection.svelte.ts` — swap `import { onEntity }` → `import { startPolling } from '$lib/realtime/poll';`; rewrite `ensureSubscribed` (idempotent guard on `registered`):
  ```ts
  function ensureSubscribed(): void {
  	if (registered) return;
  	registered = true;
  	startPolling(() => { if (lastParams !== null) void query(lastParams); else void load(); });
  }
  ```
- [ ] **Step 2:** `sessions.svelte.ts` — swap import; `ensureSubscribed` → `startPolling(() => { void load(); })`; update doc comment to "polls".
- [ ] **Step 3:** `businessProfile.svelte.ts` — swap import; `subscribe` → `startPolling(() => { void load(); })`; fix the SSE-echo comment.
- [ ] **Step 4:** `+layout.svelte` — remove the `openEvents/closeEvents` import; delete `onDestroy(() => closeEvents())` (drop unused `onDestroy` import if applicable), the `closeEvents()`/`openEvents()` calls, and the stale comment. (Polling is owned by the stores.)
- [ ] **Step 5:** `rm web/src/lib/realtime/events.ts`.
- [ ] **Step 6:** Verify: `cd web && grep -rn 'onEntity\|openEvents\|closeEvents\|realtime/events\|EventSource' src` → empty; `cd web && npm run check` → 0/0; `cd web && npx vitest run` → all pass.
- [ ] **Step 7:** Commit: `feat(web): poll stores instead of SSE; remove EventSource client`

**Phase 3 exit:** `grep -rn 'realtime\|events\.Notifier' internal --include='*.go'` empty; SPA grep for `EventSource`/`onEntity`/`realtime/events` empty; full Go + frontend gate green.

---

<!-- ============================ PHASE 4 ============================ -->

## Phase 4 — Migrate SQLite → Postgres

**Prerequisite:** Phases 1–3 merged (recurring + overdue queries already deleted — nothing to port). **Strictly sequential:** Task 0 → 1 → 2 → 3 → 4 → 5 → 6 → 7.

### Task 0 — Stand up a test Postgres + skip-when-unset helper

**Files:** new `internal/db/testdb.go`; read sites `internal/app/auth_test.go:22` (`openMigratedDB`), `internal/numbering/numbering_test.go:18` (`setup`).

- [ ] **Step 1:** Provision Postgres (macOS, no Docker):
  ```bash
  brew install postgresql@17 && brew services start postgresql@17 && createdb tallyo_test
  ```
  (Docker alt: `docker run -d --name tallyo-pg -e POSTGRES_PASSWORD=pg -p 5432:5432 postgres:17 && docker exec tallyo-pg psql -U postgres -c 'CREATE DATABASE tallyo_test'`.)
- [ ] **Step 2:** `export TEST_DATABASE_URL="postgres://localhost:5432/tallyo_test?sslmode=disable"` (Docker form includes `postgres:pg@`).
- [ ] **Step 3:** Write `internal/db/testdb.go`:
  ```go
  package db

  import (
  	"database/sql"
  	"os"
  	"testing"
  )

  // OpenTestDB returns a migrated connection to TEST_DATABASE_URL, skipping the
  // test when it is unset so `go test ./...` still runs without a database.
  func OpenTestDB(t *testing.T) *sql.DB {
  	t.Helper()
  	dsn := os.Getenv("TEST_DATABASE_URL")
  	if dsn == "" {
  		t.Skip("TEST_DATABASE_URL not set; skipping Postgres-backed test")
  	}
  	conn, err := Open(dsn) // post-Task-1 signature
  	if err != nil {
  		t.Fatalf("OpenTestDB: %v", err)
  	}
  	if err := Migrate(conn); err != nil {
  		t.Fatalf("OpenTestDB migrate: %v", err)
  	}
  	t.Cleanup(func() { _ = conn.Close() })
  	return conn
  }
  ```
  (Won't compile until Task 1 gives `Open(dsn)`. Add a `TRUNCATE`-on-open cleanup when the first shared-DB collision appears.)
- [ ] **Step 4:** Point `auth_test.go:24` and `numbering_test.go:20` at `appdb.OpenTestDB(t)` (drop the temp-file path args + unused `path/filepath` import). Leave `migrate_test.go`/`sqlite_test.go` for later tasks.
- [ ] **Step 5:** Commit: `test(db): add OpenTestDB helper gated on TEST_DATABASE_URL`

### Task 1 — Swap driver + connection (modernc → pgx/stdlib)

**Files:** `internal/db/sqlite.go` → `internal/db/postgres.go`; `internal/app/app.go:122-142`; `go.mod`/`go.sum`; delete `internal/db/sqlite_test.go`.

- [ ] **Step 1:** `go get github.com/jackc/pgx/v5@latest && go mod tidy` (pgx/stdlib + pgconn ship in the module).
- [ ] **Step 2:** `git mv internal/db/sqlite.go internal/db/postgres.go`; replace contents:
  ```go
  package db

  import (
  	"database/sql"
  	"fmt"
  	"time"

  	_ "github.com/jackc/pgx/v5/stdlib"
  )

  // Open opens a Postgres connection (pgx via database/sql) at dsn. One database
  // holds control + all tenants' data; tenancy is logical (WHERE tenant_id = $n).
  func Open(dsn string) (*sql.DB, error) {
  	if dsn == "" {
  		return nil, fmt.Errorf("Open: empty dsn")
  	}
  	conn, err := sql.Open("pgx", dsn)
  	if err != nil {
  		return nil, fmt.Errorf("open postgres: %w", err)
  	}
  	conn.SetMaxOpenConns(8)
  	conn.SetMaxIdleConns(8)
  	conn.SetConnMaxLifetime(30 * time.Minute)
  	return conn, nil
  }
  ```
  (Delete `DataDir()`.)
- [ ] **Step 3:** Rewrite `internal/app/app.go:122-142` to read `DATABASE_URL`:
  ```go
  dsn := EnvOr("DATABASE_URL", "")
  if dsn == "" {
  	return fmt.Errorf("DATABASE_URL is required")
  }
  database, err := appdb.Open(dsn)
  if err != nil {
  	return fmt.Errorf("open db: %w", err)
  }
  if err := appdb.Migrate(database); err != nil {
  	return fmt.Errorf("migrate: %w", err)
  }
  logger.Info("database connected")
  ```
  Drop `filepath` import + `cfg.DataDir` reads. If the `--data-dir`/`DATA_DIR`/`DataDir` field is now unreferenced, remove the flag + field too. `grep -rn "DataDir\|DATA_DIR\|--data-dir" cmd/ internal/` → 0.
- [ ] **Step 4:** Add the Cloud SQL socket DSN form as a comment in `postgres.go`: `postgres://USER:PASSWORD@/DBNAME?host=/cloudsql/PROJECT:REGION:INSTANCE`.
- [ ] **Step 5:** `git rm internal/db/sqlite_test.go` (pragma/`DataDir` tests, obsolete).
- [ ] **Step 6:** Verify: `go build ./internal/db/... ./internal/app/...` compiles.
- [ ] **Step 7:** Commit: `feat(db): replace modernc sqlite with pgx/v5 Postgres driver`

### Task 2 — Migration DDL audit (affinities → Postgres types)

**Files:** `internal/db/migrations/control/00001_control.sql` (sessions ~:49-54); `internal/db/migrations/tenant/00001_tenant.sql` (REAL columns); `internal/db/migrations/tenant/00003_catalogue.sql:24`; `internal/db/migrations/tenant/00005_catalogue_merge.sql:19,72,98`.

- [ ] **Step 1:** Replace the `control/00001_control.sql` sessions DDL with the postgresstore schema:
  ```sql
  CREATE TABLE sessions (
  	token  text PRIMARY KEY,
  	data   bytea NOT NULL,
  	expiry timestamptz NOT NULL
  );
  CREATE INDEX sessions_expiry_idx ON sessions (expiry);
  ```
- [ ] **Step 2:** Convert every money/quantity `REAL` → `double precision` across the live tenant migrations (tax_rates.rate, invoices/estimates subtotal/tax/total, line_items/estimate_line_items quantity/unit_price/line_total, payments.amount, catalogue_items.unit_price). Then `grep -rni "REAL" internal/db/migrations/` → 0 and `grep -rni "BLOB" internal/db/migrations/` → 0.
- [ ] **Step 3:** Leave `INTEGER`-as-bool columns as `INTEGER` (`is_current`, `taxable`, `tax_rates.is_default`, `users.is_platform_admin`, any `0/1` flag); confirm no slice expects `bool` and sqlc keeps generating `int32`/`int64`.
- [ ] **Step 4:** Commit: `refactor(db): port migration DDL to Postgres types (bytea, double precision, timestamptz)`

### Task 3 — sqlc engine + placeholders + casts; regenerate

**Files:** `sqlc.yaml:3`; all `internal/db/queries/*.sql`; casts at `payments.sql:5`, `invoices.sql:84`, `invoices.sql:89`; numbering MAX `invoices.sql:73`/`estimates.sql:74`.

- [ ] **Step 1:** `sqlc.yaml` `engine: "sqlite"` → `"postgresql"`.
- [ ] **Step 2:** Rewrite bare `?` → `$1,$2,…` per statement across every query file (keep existing `sqlc.arg(name)`); regenerate after each file to catch numbering errors.
- [ ] **Step 3:** `CAST(... AS REAL)` → `CAST(... AS double precision)` at the three sites; `grep -rn "AS REAL" internal/db/queries/` → 0; `grep -rn "date('now')" internal/db/queries/` → 0 (already deleted).
- [ ] **Step 4:** Confirm the numbering MAX `substr/CAST` expressions compile under postgresql (substr 1-indexed in both; offset unchanged).
- [ ] **Step 5:** `"$(go env GOPATH)/bin/sqlc" generate && go build ./internal/db/...` → success.
- [ ] **Step 6:** Verify money columns stayed `float64`: `grep -ni "float64" internal/db/gen/models.go` shows Amount/Total/Subtotal/UnitPrice/Rate/LineTotal/Quantity as `float64` (not `pgtype.Numeric`).
- [ ] **Step 7:** Commit: `feat(db): regenerate sqlc for postgresql engine`

### Task 4 — goose dialect + postgresstore

**Files:** `internal/db/migrate.go:48`; `internal/auth/session.go:8,19`; `go.mod`.

- [ ] **Step 1:** `migrate.go:48` `goose.SetDialect("sqlite3")` → `"postgres"` (keep the two-sequence model + `SetTableName`).
- [ ] **Step 2:** `go get github.com/alexedwards/scs/postgresstore@latest`; in `internal/auth/session.go` swap the import to `postgresstore` and `sqlite3store.New(db)` → `postgresstore.New(db)`; `go mod tidy`; `grep -rn "sqlite3store" internal/ go.mod` → 0.
- [ ] **Step 3:** Verify the two goose sequences + the `IF NOT EXISTS` tenant audit_log apply to one PG DB: `dropdb tallyo_test && createdb tallyo_test && TEST_DATABASE_URL=... go test ./internal/app/... -run TestSetup -count=1` (or the relevant setup test) → success, both version tables present, no duplicate-table error.
- [ ] **Step 4:** Commit: `feat(db): switch goose dialect and scs store to Postgres`

### Task 5 — `numbering.isRetryable` + concurrency harness (TDD)

**Files:** `internal/numbering/numbering.go:61-65` (+imports); `internal/numbering/numbering_test.go`.

- [ ] **Step 1 (RED):** Port `numbering_test.go` `setup` to `appdb.OpenTestDB(t)` + a Postgres `doc_test` table (`bigserial`/`UNIQUE(tenant_id, number)`), rewrite SQL to `$n` (substr offset unchanged). Run `TEST_DATABASE_URL=... go test ./internal/numbering/ -run TestConcurrentCreate -race -count=1` → FAIL (a `23505` surfaces as `*pgconn.PgError` whose `.Error()` lacks "unique"/"constraint", so the old substring `isRetryable` returns false).
- [ ] **Step 2 (GREEN):** Rewrite `isRetryable`:
  ```go
  import (
  	"context"
  	"errors"
  	"fmt"

  	"github.com/jackc/pgx/v5/pgconn"
  )

  // isRetryable: transient Postgres conflicts worth retrying — 23505 unique_violation
  // (a concurrent creator took our number) or 40001 serialization_failure.
  func isRetryable(err error) bool {
  	var pgErr *pgconn.PgError
  	if !errors.As(err, &pgErr) {
  		return false
  	}
  	return pgErr.Code == "23505" || pgErr.Code == "40001"
  }
  ```
  Drop the `strings` import; update the SQLITE_BUSY/WAL doc comments.
- [ ] **Step 3:** `TEST_DATABASE_URL=... go test ./internal/numbering/ -race -count=1` → all PASS (incl. 16-worker no-collision, two-tenant independence; `retry_test.go` still green since a plain error is not a `*pgconn.PgError`).
- [ ] **Step 4:** Commit: `feat(numbering): retry on pgconn SQLSTATEs 23505/40001`

### Task 6 — Rewrite `migrate_test.go` for Postgres catalog views

**Files:** `internal/db/migrate_test.go`.

- [ ] **Step 1:** Swap every `Open(filepath.Join(t.TempDir(), ...))` for `OpenTestDB(t)`; since the DB is shared, each mutating test first `conn.Exec("TRUNCATE tenants, payers, clients, invoices, line_items CASCADE")`.
- [ ] **Step 2:** Table-existence checks → `information_schema.tables` with `$1`:
  ```go
  err := conn.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)`, tbl).Scan(&ok)
  ```
- [ ] **Step 3:** Confirm `"recurring_templates"` is absent from the expected-tables list (it was removed in Phase 1 Task 3; this task is the SQLite→PG rewrite of the rest of the file).
- [ ] **Step 4:** Column checks → `information_schema.columns` (replace `PRAGMA table_info(...)`); keep assertions (`catalogue_item_id` present; `item_id`/`custom_item_id`/`price_list_version_id`/`clients.pricing_tier_id` absent).
- [ ] **Step 5:** Convert insert/constraint `?` → `$n`; FK/CHECK/CASCADE tests pass natively under Postgres.
- [ ] **Step 6:** `TEST_DATABASE_URL=... go test ./internal/db/ -count=1` → all PASS.
- [ ] **Step 7:** Commit: `test(db): rewrite migration tests for Postgres catalog views`

### Task 7 — Final gate

- [ ] **Step 1:** Full gate:
  ```bash
  go build ./... && go vet ./... && gofmt -l . && \
  TEST_DATABASE_URL="postgres://localhost:5432/tallyo_test?sslmode=disable" go test ./... && \
  CGO_ENABLED=0 go build ./cmd/tallyo && (cd web && npm run check)
  ```
  All succeed; `gofmt -l .` empty; DB tests run (not skipped). Then run `go test ./...` *without* `TEST_DATABASE_URL` and confirm DB tests `--- SKIP`.
- [ ] **Step 2:** Grep clean:
  ```bash
  grep -rn "modernc.org/sqlite" . --include='*.go' go.mod   # 0 (go mod tidy if lingering)
  grep -rn "sqlite3store" internal/ go.mod                  # 0
  grep -rn "_pragma\|_txlock" internal/                     # 0
  grep -rni "BLOB" internal/db/migrations/                  # 0
  grep -rn "AS REAL" internal/db/queries/                   # 0
  grep -rn "date('now')" internal/db/queries/               # 0
  ```
- [ ] **Step 3:** Commit: `chore(db): finalize SQLite→Postgres migration gate`

---

## Plan-level acceptance (spec §7)

- `go test ./...` passes against Postgres (incl. numbering concurrency); `go vet ./...` + `gofmt -l .` clean; `CGO_ENABLED=0 go build ./cmd/tallyo` succeeds; `cd web && npm run check` clean.
- No `modernc.org/sqlite`/`sqlite3store`/`_pragma`/`_txlock`/`BLOB`/`CAST(... AS REAL)`/`date('now')` residue.
- `isRetryable` uses pgconn SQLSTATEs; `internal/realtime/` + `internal/events/` gone; no notifier params; SPA polls.
- No background work (`sweep.go` gone); invoice API returns raw status; SPA computes overdue.
- Recurring fully removed (code/identifiers; incidental prose excepted).
- **Next:** Plan 2 (Dockerfile + docker-compose), then Plan 3 (OpenTofu + Terragrunt).
