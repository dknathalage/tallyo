# Test Coverage — Fill Real Gaps

**Date:** 2026-06-25
**Status:** Approved (design), pending implementation plan

## Goal

Raise test coverage by filling **genuine gaps** in critical paths — not chasing a
coverage percentage. "Done" = the risky, currently-untested code paths
(handler error branches, cross-cutting HTTP infra, SSE, the AI guard) have tests,
and a few high-value end-to-end user flows are exercised against the real binary.

No percentage target. No tests written purely to move a number.

## Context — current state

The repo already has ~120 Go test files and a working Playwright e2e harness.
Per-package coverage is mostly 50–80%. The apparent "0%" handler coverage is a
**measurement artifact**: slice `handler.go` happy paths *are* exercised by the
`internal/app/*_test.go` integration tests (35–75% when measured with
`-coverpkg`), they just don't count toward the slice package's own number.

Therefore the real gaps are narrower than the raw per-package report suggests:

1. **Handler error/edge branches** not hit by existing integration tests.
2. **Cross-cutting HTTP infra** with genuine 0% — `httpx` middleware, `realtime`
   SSE stream, the `smarts` disabled-guard.
3. **End-to-end user flows** — only 2 Playwright specs exist (smoke, smarts).

## Non-Goals (out of scope)

- Frontend Vitest unit tests (api clients / stores) — explicitly deferred.
- Driving any package to a coverage threshold.
- Testing `smarts/llm.go` against the real Anthropic API (needs a key). Only the
  disabled-guard (503) and the `writeSmartError` mapping are tested.
- Refactoring production code. Tests only. If a path is untestable without a
  seam, note it; do not redesign the slice here.

## Architecture / boundary decisions

Tests live in their natural home, following the existing convention:

- **HTTP error branches → `internal/app/*_test.go` (integration).** These need the
  real router + `RequireSession` + `ResolveTenant` wiring. The existing
  `tax_rates_test.go` is the template (shared helpers: `openMigratedDB`,
  `seedTenantOwner`, `loggedInClient`, `postJSON`/`get`/`putJSON`/`delete_`,
  `jarClient`). New tests reuse these helpers — no new harness.
- **Pure infra → unit tests in the owning package**, no DB, plain `httptest`.
- **User flows → `web/e2e/*.spec.ts` (Playwright)** against the real single binary,
  reusing the seeded-owner `storageState` harness from `global-setup.ts`.

No duplication: a branch already covered by an existing integration test is not
re-tested. New tests target only the uncovered branches enumerated below.

## Workstream 1 — Backend edge/error branches (`internal/app`)

Add the missing branches per slice. Each test asserts the **status code** (and,
where relevant, that the mutation did/didn't happen). Concrete targets:

- **taxrate:** `BulkDelete` (no handler test exists); `Create` empty-name → 422;
  `Create`/`Update` malformed JSON → 400.
- **payer:** `List` with filter/sort query params (the uncovered List branch);
  `BulkDelete` with a mix of valid + missing ids; cross-tenant GET → 404.
- **customitem:** `BulkDelete`; `Update` malformed JSON → 400;
  `ResolveCustomItemIDs` exercised through the invoice draft path.
- **invoice / estimate:** conflict (409) path; delete-with-payment guard;
  estimate convert-to-invoice success + double-convert conflict.

These are integration tests through the HTTP layer, not service-level unit tests,
because the gap is in handler branch coverage.

## Workstream 2 — Untested infra (unit, in-package)

- **`internal/httpx/middleware_test.go`:**
  - `RequireAuth` / `RequireSession`: no session → 401; valid session → next runs.
  - `Recover`: handler panics → 500, panic is logged, connection not dropped.
  - `RequestLogger`: status code captured via the wrapper's `WriteHeader`;
    `Unwrap` returns the underlying `ResponseWriter`.
  - `RequirePlatformAdmin` / `RequireRole`: wrong role → 403; right role → next.
- **`internal/httpx/respond_test.go`:**
  - `WriteServiceError` mapping table: `apperr.ErrNotFound`→404,
    `apperr.ErrConflict`→409, a `Validation` error→422, unknown error→500, and
    the "did it write?" boolean return.
  - `DecodeJSON`: malformed body → error; oversized body → error.
- **`internal/realtime/events_handler_test.go`:**
  - `Stream`: client subscribes; a hub broadcast results in exactly one SSE frame
    on the wire (assert `data:` payload via `writeFrame`); request-context cancel
    ends the stream cleanly (goroutine returns).
- **`internal/smarts/handler_test.go`:**
  - With `enabled=false` (svc nil), every route returns 503 (`guard`).
  - `writeSmartError` mapping table: `ErrNotFound`→404, `ErrNoData`→422,
    `ErrNoPriceList`→422, any other error→502 (and asserts no raw model string
    leaks into the body).

## Workstream 3 — End-to-end flows (`web/e2e`)

Reuse the existing harness (seeded owner, logged-in `storageState`, real binary
on port 8099). Add:

- **`invoice.spec.ts`:** create client → create invoice → add a line item →
  verify the computed total → mark paid → status reflects paid.
- **`estimate.spec.ts`:** create estimate → add line → convert to invoice →
  land on the new invoice.
- **`taxrate.spec.ts`:** set a default tax rate → it is applied to a taxable line
  on a new invoice.

Specs follow `smoke.spec.ts`: read the seeded tenant from
`e2e/.auth/tenant.json`, navigate the SPA, assert on visible state. Local-only,
single worker, no CI (matches the current config).

## Error handling / testing philosophy

- Every test asserts an **observable outcome** (status code, response body, DB
  effect, or rendered DOM) — never just "didn't error".
- Table-driven where a mapping is under test (`WriteServiceError`,
  `writeSmartError`).
- Tests are hermetic: each gets a fresh migrated DB (`openMigratedDB`) or a fresh
  temp data dir (e2e). No shared mutable state, parallel-safe where the existing
  helpers already are.

## Execution

All three workstreams are independent (different files/packages) and will be
implemented in parallel. Each ends green: `go test ./... -race` clean,
`go vet ./...` / `gofmt -l .` clean, and `cd web && npx playwright test` passing
locally.

## Success criteria

- Workstream 1 & 2 tests added and passing; the enumerated branches are now
  exercised (verified by re-running `-coverprofile` and confirming the target
  functions moved off 0% / gained the named branches).
- Workstream 3: three new Playwright specs pass against the real binary.
- Full gate green: `go test ./... -race`, `go vet ./...`, `gofmt -l .`,
  `cd web && npm run check`.
- No production code changed (tests only), or any required test seam is minimal
  and called out in the implementation plan.
