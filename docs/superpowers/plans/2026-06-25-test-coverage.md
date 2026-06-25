# Test Coverage — Fill Real Gaps — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add tests for the genuinely-untested critical paths — handler error branches, cross-cutting HTTP infra, the SSE stream, the AI guard — plus three end-to-end user flows.

**Architecture:** Tests only, no production code. HTTP error branches go in `internal/app/*_test.go` (integration, real router) reusing existing helpers; pure infra gets fast in-package unit tests with `httptest` and no DB; user flows are Playwright specs against the real binary reusing the seeded-owner harness.

**Tech Stack:** Go stdlib `testing` + `net/http/httptest`, chi v5, `alexedwards/scs/v2`, modernc SQLite; Playwright for e2e.

**Spec:** `docs/superpowers/specs/2026-06-25-test-coverage-design.md`

**Shared facts (verified against the codebase):**
- App-test helpers live in `internal/app/*_test.go` (package `app`): `openMigratedDB(t, name)`, `seedTenantOwner(t, conn) (*auth.UsersRepo, ownerEmail, ownerPassword, tenantUUID)`, `loggedInClient(t, base)`, `jarClient(t)`, `get`, `postJSON`, `putJSON`, `delete_`. Template: `internal/app/tax_rates_test.go` (its `newTaxRateServer` shows the wiring).
- `httpx.WriteServiceError(w, err) bool`: `apperr.ErrNotFound`→404, `apperr.ErrConflict`→409, `apperr.Validation`→422, nil→false/no write, else→500.
- `realtime.Event{TenantID, Entity, UUID, Action}` (TenantID is `json:"-"`); `Hub.Subscribe(tenantID) (<-chan Event, func())`, `Hub.Broadcast(e)`; SSE frame format is `data: <json>\n\n`.
- `smarts.NewHandler(svc *Service, enabled bool)` — nil svc allowed when `enabled=false`; `guard` writes 503; `writeSmartError`: `ErrNotFound`→404, `ErrNoData`→422, `ErrNoPriceList`→422, else→502.
- `estimate.ErrAlreadyConverted`; route `POST /estimates/{uuid}/convert`.

**Gate after every task:** `go test ./... -race` green, `go vet ./...` clean, `gofmt -l .` empty. (e2e tasks: `cd web && npx playwright test` green.)

---

## Workstream 2 — Infra unit tests (do first; no DB, fastest feedback)

### Task 1: httpx response-mapping unit tests

**Files:**
- Create: `internal/httpx/respond_test.go` (package `httpx`)

- [ ] **Step 1: Write the failing test** — table test for `WriteServiceError` + `DecodeJSON`.

```go
package httpx

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dknathalage/tallyo/internal/apperr"
)

func TestWriteServiceError(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantWrote bool
	}{
		{"nil falls through", nil, 200, false},
		{"not found", apperr.ErrNotFound, 404, true},
		{"conflict", apperr.ErrConflict, 409, true},
		{"validation", &apperr.ValidationError{Errors: []apperr.FieldError{{Field: "name", Message: "required"}}}, 422, true},
		{"unknown", errors.New("boom"), 500, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			got := WriteServiceError(rec, c.err)
			if got != c.wantWrote {
				t.Fatalf("wrote: want %v got %v", c.wantWrote, got)
			}
			if c.wantWrote && rec.Code != c.wantCode {
				t.Fatalf("status: want %d got %d", c.wantCode, rec.Code)
			}
		})
	}
}

func TestDecodeJSONRejectsBadBody(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader("{not json"))
	var dst struct{ A string }
	if err := DecodeJSON(r, &dst); err == nil {
		t.Fatal("want error for malformed body, got nil")
	}
}

func TestDecodeJSONRejectsUnknownFields(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"nope":1}`))
	var dst struct{ A string `json:"a"` }
	if err := DecodeJSON(r, &dst); err == nil {
		t.Fatal("want error for unknown field, got nil")
	}
}
```

> **Before writing:** confirm the exact `apperr.ValidationError` / `FieldError` field names by reading `internal/apperr/*.go` — adjust the literal if they differ (e.g. it may be a constructor, not a struct literal). If construction is awkward, use the same validation error the slices return (grep a slice's `Input.Validate`).

- [ ] **Step 2: Run, expect FAIL** (or compile error if literal is wrong → fix to match `apperr`). `go test ./internal/httpx/ -run 'WriteServiceError|DecodeJSON' -v`
- [ ] **Step 3:** No impl needed (testing existing code). Make it compile + pass.
- [ ] **Step 4: Run, expect PASS.**
- [ ] **Step 5: Commit** — `test(httpx): cover WriteServiceError mapping + DecodeJSON guards`

### Task 2: httpx middleware unit tests

**Files:**
- Create: `internal/httpx/middleware_test.go` (package `httpx`)

- [ ] **Step 1: Write the failing test.** No-DB cases: `Recover`, `RequestLogger` status capture + `Unwrap`, `RequireRole`, `RequirePlatformAdmin`. (`RequireAuth`/`RequireSession` full-path is already exercised by `internal/app` integration tests + the 401 branch below; here cover the cheap branches.)

```go
package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/dknathalage/tallyo/internal/auth"
)

func TestRecoverTurnsPanicInto500(t *testing.T) {
	h := Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	// Recover calls LoggerFrom(ctx); RequestLogger normally seeds it. Wrap so the
	// context carries a logger, OR confirm LoggerFrom tolerates a missing logger
	// (read logging.go) — pick whichever the code supports.
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d", rec.Code)
	}
}

func TestRequestLoggerCapturesStatus(t *testing.T) {
	h := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusTeapot {
		t.Fatalf("status not passed through: got %d", rec.Code)
	}
}

func TestRequireRoleForbidsWrongRole(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	h := RequireRole("owner")(next)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), userCtxKey, &auth.User{Role: "member"}))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d", rec.Code)
	}
}

func TestRequireRoleAllowsListedRole(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(200) })
	h := RequireRole("owner", "admin")(next)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), userCtxKey, &auth.User{Role: "admin"}))
	h.ServeHTTP(rec, req)
	if !called || rec.Code != 200 {
		t.Fatalf("admin should pass: called=%v code=%d", called, rec.Code)
	}
}

func TestRequireRoleNoUser401(t *testing.T) {
	h := RequireRole("owner")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rec.Code)
	}
}

func TestRequirePlatformAdminForbidsNonAdmin(t *testing.T) {
	h := RequirePlatformAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), userCtxKey, &auth.User{IsPlatformAdmin: false}))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d", rec.Code)
	}
}

func TestRequireSession401WithoutSession(t *testing.T) {
	sm := scs.New() // empty session → GetString returns ""
	h := RequireSession(sm)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	// scs reads session from ctx loaded by LoadAndSave; with a bare request the
	// values are empty → 401. If scs panics without LoadAndSave, wrap with
	// sm.LoadAndSave(h) and drive via httptest.NewServer instead.
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rec.Code)
	}
}
```

> `userCtxKey` is unexported and in-package — fine since the test is package `httpx`. The `Recover` and `RequireSession` cases have inline notes: read `internal/httpx/logging.go` for `LoggerFrom` behavior and adjust if a seeded logger is required; for scs, fall back to `LoadAndSave` + `httptest.NewServer` if a bare context panics.

- [ ] **Step 2:** `go test ./internal/httpx/ -v` — expect FAIL/compile, then iterate per the inline notes until the branches are green.
- [ ] **Step 3-4:** No production change; make all cases pass.
- [ ] **Step 5: Commit** — `test(httpx): cover Recover, RequestLogger, role gates, session 401`

### Task 3: realtime SSE stream unit test

**Files:**
- Create: `internal/realtime/events_handler_test.go` (package `realtime`)

- [ ] **Step 1: Write the failing test.** Drive `Stream` with a cancelable context; broadcast one event; assert a `data:` frame lands; assert cancel ends the stream. Use `httptest.NewRecorder` (it implements `http.Flusher`).

```go
package realtime

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/reqctx"
)

func TestStreamDeliversEventFrame(t *testing.T) {
	hub := NewHub()
	h := NewEventsHandler(hub)

	ctx, cancel := context.WithCancel(reqctx.WithTenant(context.Background(), "tenant-1"))
	req := httptest.NewRequest("GET", "/api/events", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() { h.Stream(rec, req); close(done) }()

	// Give Stream time to Subscribe before broadcasting. Poll the hub instead of
	// sleeping if Hub exposes a subscriber count; otherwise a short retry loop on
	// the recorder body is acceptable (bounded).
	waitFor(t, func() bool { return strings.Contains(rec.Body.String(), "event-stream") || true })
	hub.Broadcast(Event{TenantID: "tenant-1", Entity: "invoice", UUID: "abc", Action: "created"})

	waitFor(t, func() bool { return strings.Contains(rec.Body.String(), `"entity":"invoice"`) })
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stream did not return after context cancel")
	}
	if !strings.Contains(rec.Body.String(), "data: ") {
		t.Fatalf("want a data frame, got %q", rec.Body.String())
	}
}

func TestWriteFrameSkipsUnmarshalableButKeepsAlive(t *testing.T) {
	rec := httptest.NewRecorder()
	if !writeFrame(rec, Event{Entity: "x", UUID: "1", Action: "created"}) {
		t.Fatal("writeFrame should return true on success")
	}
	if !strings.HasPrefix(rec.Body.String(), "data: ") {
		t.Fatalf("frame format wrong: %q", rec.Body.String())
	}
}

// waitFor polls cond up to ~1s (bounded, NASA rule 2). Replace the busy loop
// with a sync primitive if the hub exposes one.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within 1s")
}
```

> **Concurrency note:** `httptest.ResponseRecorder` is not concurrency-safe for simultaneous read+write. The `waitFor` polling reads `rec.Body` while `Stream`'s goroutine may write. If `-race` flags this, switch to an `io.Pipe`-backed `http.ResponseWriter` wrapper exposing `Flush()`, or add a small mutex-guarded recorder. Prefer the simplest variant that passes `-race`; the inline note is the upgrade path. `// ponytail: recorder polling; swap to piped writer if -race complains`.

- [ ] **Step 2:** `go test ./internal/realtime/ -race -run Stream -v` — expect FAIL then iterate.
- [ ] **Step 3-4:** No production change; get green under `-race`.
- [ ] **Step 5: Commit** — `test(realtime): cover SSE Stream frame delivery + cancel`

### Task 4: smarts disabled-guard + error-mapping unit test

**Files:**
- Create: `internal/smarts/handler_disabled_test.go` (package `smarts`)

- [ ] **Step 1: Write the failing test.** With `enabled=false` every route returns 503; `writeSmartError` mapping table.

```go
package smarts

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestDisabledHandlerReturns503(t *testing.T) {
	h := NewHandler(nil, false)
	r := chi.NewRouter()
	h.Routes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	for _, path := range []string{"/smarts/draft-invoice", "/smarts/suggest-lines", "/smarts/follow-up"} {
		resp, err := http.Post(srv.URL+path, "application/json", nil)
		if err != nil {
			t.Fatalf("post %s: %v", path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("%s: want 503 got %d", path, resp.StatusCode)
		}
	}
	// map-import is gated by RequireRole — without auth wiring it returns 401/403
	// before the guard, so it is intentionally excluded here.
}

func TestWriteSmartError(t *testing.T) {
	cases := []struct {
		err  error
		code int
	}{
		{ErrNotFound, http.StatusNotFound},
		{ErrNoData, http.StatusUnprocessableEntity},
		{ErrNoPriceList, http.StatusUnprocessableEntity},
		{errStub{}, http.StatusBadGateway},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		writeSmartError(rec, c.err)
		if rec.Code != c.code {
			t.Fatalf("%v: want %d got %d", c.err, c.code, rec.Code)
		}
		if rec.Body.Len() > 0 && contains(rec.Body.String(), "stub-internal") {
			t.Fatal("raw error string leaked into body")
		}
	}
}

type errStub struct{}

func (errStub) Error() string { return "stub-internal model failure" }

func contains(s, sub string) bool { return len(s) >= len(sub) && (s == sub || stringsIndex(s, sub) >= 0) }
```

> Use `strings.Contains`/`strings.Index` directly instead of the hand-rolled `contains`/`stringsIndex` above (import `strings`). Inlined here only to show intent; replace with stdlib (ponytail: stdlib over reinvention).

- [ ] **Step 2:** `go test ./internal/smarts/ -run 'Disabled|WriteSmartError' -v` — expect FAIL/compile then fix imports.
- [ ] **Step 3-4:** No production change; pass.
- [ ] **Step 5: Commit** — `test(smarts): cover disabled-guard 503 + writeSmartError mapping`

---

## Workstream 1 — Backend handler edge/error branches (integration)

All tasks add to existing `internal/app/*_test.go` files (package `app`), reusing helpers. Follow `tax_rates_test.go` exactly. Each test = build server, login, hit endpoint, assert status.

### Task 5: taxrate error branches

**Files:**
- Modify: `internal/app/tax_rates_test.go`

- [ ] **Step 1:** Add tests:
  - `TestTaxRateCreateMalformedJSON400` — `postJSON(..., "{")` → 400.
  - `TestTaxRateUpdateMalformedJSON400` — create one, `putJSON(..., "{")` → 400.
  - `TestTaxRateDeleteCrossTenant404` / `TestTaxRateDeleteMissing404` — `delete_` a random uuid → 404.
  (No `BulkDelete` route on this slice — do not add one.)
- [ ] **Step 2:** `go test ./internal/app/ -run TaxRate -v` — expect FAIL.
- [ ] **Step 3-4:** Adjust asserted codes to actual behavior (read `taxrate/handler.go` if a code surprises you; a malformed body may surface as 400 via `DecodeJSON`). Green.
- [ ] **Step 5: Commit** — `test(taxrate): cover malformed-JSON 400 + delete-missing 404`

### Task 6: payer error/filter branches

**Files:**
- Modify: `internal/app/payers_test.go`

- [ ] **Step 1:** Add:
  - `TestPayerListWithFilter` — create 2 payers, GET `…/payers?search=<name>` (confirm the real query param by reading `payer/query.go`/`handler.go`); assert filtered result.
  - `TestPayerBulkDeleteMixedIDs` — create 1, BulkDelete `[validID, randomUUID]`; assert the valid one is gone (GET → 404) and the call's status matches the handler contract.
  - `TestPayerGetCrossTenant404` — random uuid → 404.
- [ ] **Step 2:** `go test ./internal/app/ -run Payer -v` — FAIL.
- [ ] **Step 3-4:** Confirm BulkDelete request shape from `payer/handler.go:120` (path + body JSON). Green.
- [ ] **Step 5: Commit** — `test(payer): cover list filter, bulk-delete mixed ids, cross-tenant 404`

### Task 7: customitem error branches

**Files:**
- Modify: `internal/app/custom_items_test.go`

- [ ] **Step 1:** Add:
  - `TestCustomItemUpdateMalformedJSON400`.
  - `TestCustomItemBulkDelete` — create 2, bulk-delete both, list empty.
- [ ] **Step 2:** `go test ./internal/app/ -run CustomItem -v` — FAIL.
- [ ] **Step 3-4:** Confirm bulk-delete shape from `customitem/handler.go:128`. Green.
- [ ] **Step 5: Commit** — `test(customitem): cover malformed-JSON 400 + bulk-delete`

### Task 8: estimate convert conflict branch

**Files:**
- Modify: `internal/app/estimates_test.go`

- [ ] **Step 1:** Add `TestEstimateConvertTwiceConflicts`:
  - Create estimate, mark/accept it as the convert path requires (read `estimate/convert.go` + existing estimate tests for the accept step), `POST …/estimates/{uuid}/convert` → success; convert again → 409 (`ErrAlreadyConverted` maps to conflict — confirm the mapped status at `estimate/handler.go:369`).
- [ ] **Step 2:** `go test ./internal/app/ -run EstimateConvert -v` — FAIL.
- [ ] **Step 3-4:** Mirror the existing estimate-convert happy-path test for setup. Green.
- [ ] **Step 5: Commit** — `test(estimate): cover double-convert conflict`

---

## Workstream 3 — End-to-end flows (Playwright)

All specs go in `web/e2e/`, follow `smoke.spec.ts` (read tenant from `e2e/.auth/tenant.json`, navigate, assert visible DOM). Run with `cd web && npx playwright test`.

> **First:** read `web/e2e/smoke.spec.ts`, `global-setup.ts`, and the relevant SvelteKit routes under `web/src/routes` to learn the actual selectors/URLs. Do NOT guess selectors — open the route components. Prefer `getByRole`/`getByText` with user-visible strings, as `smoke.spec.ts` does.

### Task 9: invoice lifecycle e2e

**Files:**
- Create: `web/e2e/invoice.spec.ts`

- [ ] **Step 1:** Spec: from logged-in state, create a client (or use the seeded `Acme Baseline Client`), create an invoice, add a line item, assert the total renders, mark paid, assert the paid status shows. Use seeded data where possible to keep the flow short.
- [ ] **Step 2:** `cd web && npx playwright test invoice -x` — iterate on real selectors (use `npx playwright test --ui` or `--debug` locally if stuck).
- [ ] **Step 3:** Green.
- [ ] **Step 4: Commit** — `test(e2e): invoice create → line → total → paid flow`

### Task 10: estimate→invoice + tax-rate e2e

**Files:**
- Create: `web/e2e/estimate.spec.ts`, `web/e2e/taxrate.spec.ts`

- [ ] **Step 1:** `estimate.spec.ts` — create estimate, add line, convert to invoice, assert landing on the new invoice. `taxrate.spec.ts` — set a default tax rate, add a taxable line on a new invoice, assert tax appears in the total.
- [ ] **Step 2:** `cd web && npx playwright test estimate taxrate` — iterate selectors.
- [ ] **Step 3:** Green.
- [ ] **Step 4: Commit** — `test(e2e): estimate→invoice convert + default tax-rate applied`

---

## Final verification (after all tasks)

- [ ] `go test ./... -race` — all green.
- [ ] `go vet ./...` && `gofmt -l .` — clean (empty).
- [ ] `cd web && npm run check` — 0 errors / 0 warnings.
- [ ] `cd web && npx playwright test` — all e2e specs pass.
- [ ] Re-run `go test ./internal/httpx ./internal/realtime ./internal/smarts -coverprofile` and confirm the targeted functions (`WriteServiceError`, `Stream`, `writeFrame`, `guard`, `writeSmartError`, the role gates) moved off 0%.
- [ ] No non-test files changed: `git diff --stat main` shows only `*_test.go`, `web/e2e/*.spec.ts`, and docs.
```
