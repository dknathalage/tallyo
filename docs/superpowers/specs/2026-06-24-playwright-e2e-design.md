# Playwright E2E for Tallyo — Design

**Date:** 2026-06-24
**Status:** Approved (brainstorm)
**Scope:** A committed, local-only Playwright end-to-end suite that boots the real
single binary, logs in via API-seeded session state, drives the embedded SvelteKit
SPA, and asserts on visible outcomes. CI and cross-browser are explicitly out of scope.

## Goal

Give Claude (and humans) a repeatable way to actually exercise the UI: launch the
shipped artifact, reach real screens with real data, click through flows, and catch
regressions. Tests run against the **real `tallyo` binary** (Go + embedded SPA +
SQLite) — the thing that ships — not the Vite dev harness.

## Non-Goals

- CI integration (no GitHub Actions / pipeline wiring yet).
- Cross-browser coverage (Chromium only).
- Visual / screenshot snapshot assertions (flaky; assert on roles + text instead).
- Parallel sharding (single worker until the suite is measurably slow).

These are deliberate omissions, not oversights. Add when a concrete need appears.

## Architecture

### Layout

```
web/
  e2e/
    fixtures.ts        # API seed helpers (signup, create client / price-list item / ...)
    global-setup.ts    # signup first user -> save storageState.json + tenant uuid
    *.spec.ts          # the actual tests
  playwright.config.ts
```

Playwright is added as a `web/` devDependency. Two entry points:

- `npm run test:e2e` (in `web/package.json`) — the underlying command.
- `task test:e2e` (in `Taskfile.yml`) — repo-level passthrough.

### Binary boot

`playwright.config.ts` uses Playwright's `webServer` option to build and run the
real binary before the suite, and tear it down after:

- Build: the SPA **must** be built and embedded before `go build`, or the binary
  serves a stale `web/build`. So `webServer.command` runs `task build:web` (or
  `npm run build`) **then** `CGO_ENABLED=0 go build -o ../bin/tallyo-e2e ./cmd/tallyo`
  — both steps, not just the Go build.
- Run: `./bin/tallyo-e2e --data-dir <fresh temp dir> --port <fixed test port>`.
- Playwright waits on the port (`url` + `reuseExistingServer: false`), then kills the
  process on teardown.

A **fresh temp `--data-dir` per run** means an empty SQLite DB, which means the first
request hits the **first-run signup** path automatically. No reset endpoint, no
pre-baked database file, no migration-staleness risk.

> Implementation note: Playwright's `webServer.command` is a single shell command.
> The build+run can be a tiny shell one-liner or a small `web/e2e/launch.sh`; the
> temp data dir can be created inline (`mktemp -d`) so each `npm run test:e2e`
> invocation is clean. This is a detail for the plan, not the design.

### Seeding (global-setup)

`web/e2e/global-setup.ts` seeds through the **real `/api`** using plain `fetch` —
no UI clicking for fixtures:

1. `POST /api/signup` — the first user becomes the owner; the response sets the
   session cookie.
2. Read the tenant uuid from the **signup response body** (the plan must confirm
   exactly where the uuid surfaces — note `/api/t/<uuid>/auth/me` cannot be the
   source, since it already requires the uuid; avoid a chicken-and-egg seed).
3. A few `POST /api/t/<uuid>/...` calls to seed baseline data (at minimum: one
   client and one price-list item, so invoice/estimate screens have something to
   work with).
4. Persist the authenticated session via Playwright `storageState.json`, and stash
   the tenant uuid where specs can read it (e.g. an env var or a small JSON written
   next to `storageState.json`).

Tests load `storageState.json` and start **already logged in**, navigating to
`/<tenant-uuid>/...`.

### Tests

Specs drive the SPA by ARIA role / visible text and assert on visible outcomes:

```ts
test('create invoice', async ({ page }) => {
  await page.goto(`/${tenant}/invoices`);
  await page.getByRole('button', { name: 'New invoice' }).click();
  // ...fill, save...
  await expect(page.getByText('INV-')).toBeVisible();
});
```

Any per-test data beyond the global baseline is seeded via `fixtures.ts` helpers
(API calls), not by clicking through setup screens. Only the behaviour **under
test** is exercised through the UI.

## Data Flow

```
playwright.config.ts (webServer)
  -> build SPA + go build -> run tallyo against temp --data-dir on test port
global-setup.ts
  -> POST /api/signup (first-run owner) -> cookie + tenant uuid
  -> POST /api/t/<uuid>/... baseline data
  -> write storageState.json + tenant uuid
*.spec.ts
  -> reuse storageState (logged in) -> goto /<uuid>/... -> click/assert
teardown
  -> kill binary, drop temp data dir
```

## Error Handling / Robustness

- **Port conflicts:** fixed test port distinct from dev (`5173`/`8080`); rely on
  `reuseExistingServer: false` so a stale server is never silently reused.
- **Server-not-ready:** Playwright `webServer.url` health-wait gates the suite start.
- **Seed failures:** global-setup asserts non-2xx responses loudly (fail fast — a
  bad seed should abort the run, not produce confusing test failures downstream).
- **State leakage:** fresh temp data-dir per run guarantees isolation; no
  cross-run contamination.

## Testing the Tests

The first spec doubles as the smoke test: if global-setup, binary boot, login state,
and one navigation+assertion all pass, the harness itself is proven. Start with one
real flow (e.g. create-invoice or the dashboard landing) before expanding.

## Open Implementation Details (for the plan, not the design)

- Exact signup/me endpoint shape and where the tenant uuid surfaces.
- Which baseline entities the seed needs for the first target spec.
- `webServer.command` form (inline vs `launch.sh`) and temp-dir creation.
- Fixed test port value.
