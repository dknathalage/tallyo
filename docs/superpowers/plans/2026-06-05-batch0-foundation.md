# Domain Port — Batch 0: Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the cross-cutting refactors and shared infrastructure that every subsequent domain port depends on — an audit-transaction wrapper, standardized audit entries, atomic invite acceptance, an embedded-SPA self-check, a concurrency-safe numbering package, and shared frontend CRUD/store infra — so the 13× domain replication is structural, not hand-rolled.

**Architecture:** Extends the existing skeleton. Adds `repository.WithAudit` to make "mutation in a tx + audit row" a single enforced helper; refactors the three existing repos onto it. Makes invite acceptance one transaction. Adds `internal/numbering` (tx-scoped, retry-on-conflict). Adds frontend `crud` API helper + `createCollectionStore` rune factory wired to SSE. No new user-facing feature.

**Tech Stack:** Go 1.26 (database/sql, modernc sqlite, sqlc, scs), Svelte 5 runes + TS.

**Spec:** `docs/superpowers/specs/2026-06-05-domain-port-decomposition-design.md`

**Reference patterns:** `internal/repository/business_profile.go` (current Save tx+audit), `internal/auth/users.go` + `invites.go` (current per-repo tx+audit), `internal/http/invites.go` (current 3-step Accept), `internal/http/events.go` (SSE headers), `internal/realtime` (hub), `src/lib/db/number-generators.ts` (old numbering: `INV-%04d`, `EST-%04d` via racy MAX).

---

## File Structure

- Create: `internal/repository/audit_tx.go` — `WithAudit` wrapper + `Changes` helper
- Create: `internal/repository/audit_tx_test.go`
- Modify: `internal/repository/business_profile.go` — Save uses `WithAudit`
- Modify: `internal/auth/users.go`, `internal/auth/invites.go` — mutations use `WithAudit` (or a shared equivalent usable from the `auth` package — see Task 2 note)
- Create: `internal/numbering/numbering.go` (+ `_test.go`) — tx-scoped next-number
- Modify: `internal/http/invites.go` — `Accept` in one transaction; dup-email → 409
- Modify: `internal/auth/invites.go` and/or `users.go` — an `AcceptInTx` path
- Modify: `internal/http/events.go` — drop `Connection: keep-alive`
- Modify: `internal/http/static.go` or `cmd/tallyo/main.go` — embedded-SPA self-check
- Create: `web/src/lib/api/crud.ts` — generic REST CRUD helper over `/api/<resource>`
- Create: `web/src/lib/stores/collection.svelte.ts` — `createCollectionStore` rune factory (SSE-wired)

**Package-boundary note:** `WithAudit` lives in `internal/repository`, but `internal/auth` repos also need it. To avoid an import cycle (`auth` → `repository` is fine; `repository` must NOT import `auth`), put the wrapper in a NEUTRAL package both can import: create it in **`internal/audit`** as `audit.WithTx(ctx, db, entry, fn)` (audit already has no deps on auth/repository). Adjust Task 1 accordingly: the wrapper lives in `internal/audit`. (If a reviewer prefers a separate `internal/dbtx` package, that's acceptable — the constraint is no import cycle.)

---

## Task 1: `audit.WithTx` transaction wrapper + Changes helper

**Files:**
- Modify: `internal/audit/audit.go` (add `WithTx`, `Changes`)
- Test: `internal/audit/withtx_test.go`

- [ ] **Step 1: Write the failing test** `internal/audit/withtx_test.go`:
```go
package audit

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

func mustDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "w.db"))
	if err != nil { t.Fatalf("Open: %v", err) }
	if err := appdb.Migrate(conn); err != nil { t.Fatalf("Migrate: %v", err) }
	return conn
}

func TestWithTxCommitsAndAudits(t *testing.T) {
	conn := mustDB(t); defer conn.Close()
	ctx := context.Background()
	err := WithTx(ctx, conn, Entry{EntityType: "business_profile", EntityID: 1, Action: "update", Changes: Changes(map[string]any{"name": "Acme"})},
		func(tx *sql.Tx) error {
			_, e := tx.ExecContext(ctx, `INSERT INTO business_profile (id, uuid, name) VALUES (1, 'u', 'Acme')`)
			return e
		})
	if err != nil { t.Fatalf("WithTx: %v", err) }
	var n int
	conn.QueryRow(`SELECT COUNT(*) FROM business_profile`).Scan(&n)
	if n != 1 { t.Fatalf("profile rows=%d want 1", n) }
	conn.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE entity_type='business_profile'`).Scan(&n)
	if n != 1 { t.Fatalf("audit rows=%d want 1", n) }
}

func TestWithTxRollsBackOnFnError(t *testing.T) {
	conn := mustDB(t); defer conn.Close()
	ctx := context.Background()
	boom := errors.New("boom")
	err := WithTx(ctx, conn, Entry{EntityType: "business_profile", EntityID: 1, Action: "update"},
		func(tx *sql.Tx) error {
			tx.ExecContext(ctx, `INSERT INTO business_profile (id, uuid, name) VALUES (1, 'u', 'X')`)
			return boom
		})
	if !errors.Is(err, boom) { t.Fatalf("want boom, got %v", err) }
	var n int
	conn.QueryRow(`SELECT COUNT(*) FROM business_profile`).Scan(&n)
	if n != 0 { t.Fatalf("profile rows=%d want 0 (rolled back)", n) }
	conn.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&n)
	if n != 0 { t.Fatalf("audit rows=%d want 0 (rolled back)", n) }
}

func TestChangesProducesJSON(t *testing.T) {
	s := Changes(map[string]any{"name": "Acme", "n": 3})
	if s == "" || s[0] != '{' { t.Fatalf("Changes=%q", s) }
}
```

- [ ] **Step 2: Run → FAIL** (`go test ./internal/audit/ -run 'WithTx|Changes'`, undefined WithTx/Changes).

- [ ] **Step 3: Implement** in `internal/audit/audit.go`:
```go
// WithTx runs fn inside a transaction, writes the audit Entry in the SAME tx,
// and commits. Any error (begin, fn, audit, commit) rolls back. This is the
// canonical way to perform an audited mutation.
func WithTx(ctx context.Context, db *sql.DB, e Entry, fn func(*sql.Tx) error) error {
	if db == nil {
		return fmt.Errorf("audit WithTx: nil db")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("audit WithTx: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err // caller's error, unwrapped so errors.Is works
	}
	if err := Log(ctx, tx, e); err != nil {
		return fmt.Errorf("audit WithTx: log: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("audit WithTx: commit: %w", err)
	}
	return nil
}

// Changes marshals a map to a JSON string for the Entry.Changes field.
// Returns "{}" on marshal failure (never panics; audit must not break a mutation).
func Changes(m map[string]any) string {
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}
```
Add imports (`encoding/json` already likely present; add if needed). NOTE: `WithTx` takes `*sql.DB` (it owns the tx). For callers that already hold a tx, they keep using `Log(ctx, tx, e)` directly.

- [ ] **Step 4: Run → PASS** (`-race`), `go vet ./internal/audit/`, `gofmt -l internal/audit/`.

- [ ] **Step 5: Commit** `feat(audit): WithTx audited-mutation wrapper and Changes helper`.

---

## Task 2: Refactor existing repos onto `audit.WithTx`

**Files:**
- Modify: `internal/repository/business_profile.go` (Save)
- Modify: `internal/auth/users.go` (Create, Delete)
- Modify: `internal/auth/invites.go` (Create, MarkUsed)

Goal: replace the hand-rolled BeginTx/defer-rollback/Log/Commit blocks with `audit.WithTx`, preserving behavior. This is a pure refactor — existing tests must stay green (no test changes except if signatures shift, which they should not).

- [ ] **Step 1: Refactor `BusinessProfileRepo.Save`** to:
```go
func (r *BusinessProfileRepo) Save(ctx context.Context, in BusinessProfileInput) error {
	if in.Name == "" {
		return errors.New("save business profile: name is required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "business_profile", EntityID: 1, Action: "update",
		Changes: audit.Changes(map[string]any{"name": in.Name}),
	}, func(tx *sql.Tx) error {
		id, err := existingUuid(ctx, tx)
		if err != nil {
			return fmt.Errorf("read uuid: %w", err)
		}
		if err := gen.New(tx).UpsertBusinessProfile(ctx, buildParams(id, in)); err != nil {
			return fmt.Errorf("upsert: %w", err)
		}
		return nil
	})
}
```

- [ ] **Step 2: Run** `go test ./internal/repository/ -race` → still PASS (TestSaveThenGet, audit-row assertion, uuid preservation).

- [ ] **Step 3: Refactor `UsersRepo.Create` and `UsersRepo.Delete`** onto `audit.WithTx`, keeping the returned `*User` for Create. For Create, the `gen.CreateUser` returns the row inside the tx; capture it in a closure variable:
```go
func (r *UsersRepo) Create(ctx context.Context, email, hash, role string) (*User, error) {
	if email == "" || hash == "" {
		return nil, errors.New("create user: email and hash required")
	}
	var created gen.User
	err := audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "user", Action: "create",
		Changes: audit.Changes(map[string]any{"email": email, "role": role}),
	}, func(tx *sql.Tx) error {
		u, e := gen.New(tx).CreateUser(ctx, /* params */)
		if e != nil { return e }
		created = u
		return nil
	})
	if err != nil { return nil, err }
	// EntityID note: see Task 3's standardization — for now leave Entry.EntityID 0
	// (the changes carry the email). Acceptable; revisit when standardizing.
	return toUser(created), nil
}
```
NOTE: `audit.WithTx`'s Entry is built BEFORE the tx runs, so for Create the new user's id isn't known when the Entry is constructed. For entities whose id is generated mid-tx, set `EntityID` to 0 and rely on `Changes` (acceptable for the skeleton). Domains that need the real id in the audit row should write the audit `Log` manually inside their `fn` AFTER the insert (the wrapper's auto-log is for the common before-known-id case). DOCUMENT this tradeoff in a comment on `WithTx`. → Update Task 1's `WithTx` doc comment to state: "Entry is logged with whatever EntityID you pass; if the id is generated inside fn, log manually inside fn instead and pass a no-op Entry (Action empty) — WithTx skips the auto-log when Entry.Action == \"\"." Implement that skip: in `WithTx`, `if e.Action != "" { Log(...) }`.

- [ ] **Step 4: Refactor `InvitesRepo.Create` and `MarkUsed`** similarly. For Create, capture the returned invite; since invite id is generated mid-tx, either keep manual logging inside fn (pass empty-Action Entry to skip auto-log) OR accept EntityID 0. Keep behavior + existing tests green.

- [ ] **Step 5: Run** `go test ./internal/auth/ ./internal/repository/ -race` → all PASS. `go vet ./...`, `gofmt -l`.

- [ ] **Step 6: Commit** `refactor: route audited mutations through audit.WithTx`.

---

## Task 3: Atomic invite acceptance (single tx, dup-email → 409)

**Files:**
- Modify: `internal/auth/invites.go` (add `Accept`) or `internal/auth/users.go`
- Modify: `internal/http/invites.go` (Accept handler uses the atomic path)
- Test: `internal/auth/invites_test.go` (+ handler test in `internal/http/invites_test.go`)

Current `Accept` handler does Validate → users.Create → MarkUsed as 3 separate calls. Make it ONE transaction so a crash can't create a user without consuming the invite, and map a duplicate email to 409.

- [ ] **Step 1: Write failing test** in `internal/auth/invites_test.go`: `TestAcceptInviteAtomic` — create an owner + an invite; `repo.Accept(ctx, token, hash)` creates a member user AND marks the invite used in one call; a second `Accept` with the same token returns `ErrInviteInvalid`; `Accept` for an email that already exists returns a distinct `ErrEmailTaken` sentinel (and does NOT consume the invite — invite stays usable OR is consumed; pick: it should NOT create a duplicate and should surface a mappable error). Decide: on dup email, return `ErrEmailTaken`, leave invite unused.

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement `InvitesRepo.Accept(ctx, token, passwordHash string) (*User, error)`** (or a free function taking both repos). In ONE `db.BeginTx`:
  - re-validate the invite inside the tx (`SELECT ... WHERE token=?`): not found/expired/used → `ErrInviteInvalid`.
  - insert the user (role from invite); if the insert fails on the UNIQUE(email) constraint → return `ErrEmailTaken` (detect via the modernc error string containing "UNIQUE" / "constraint"); rollback.
  - mark the invite used.
  - write an audit row (`Log(ctx, tx, ...)`, entity "invite", action "accepted").
  - commit.
  Add `var ErrEmailTaken = errors.New("email already registered")`. Keep <60 lines (extract helpers).

- [ ] **Step 4: Update the HTTP `Accept` handler** (`internal/http/invites.go`) to call `repo.Accept`; map `ErrInviteInvalid` → 409 "invite invalid or already used", `ErrEmailTaken` → 409 "email already registered", other → 500, success → 201. Remove the old 3-step sequence.

- [ ] **Step 5: Run** `go test ./internal/auth/ ./internal/http/ -race` → PASS (existing invite handler tests + new atomic test; the double-accept→409 and accept-then-login tests must still pass).

- [ ] **Step 6: Commit** `fix(auth): atomic invite acceptance with dup-email 409`.

---

## Task 4: `internal/numbering` — concurrency-safe document numbers

**Files:**
- Create: `internal/numbering/numbering.go`, `internal/numbering/numbering_test.go`

Provides tx-scoped next-number generation with retry-on-conflict, parameterized by a typed Config (NO raw SQL strings from callers — avoids injection). Tested against a synthetic table (invoices/estimates don't exist until Batch 3/4).

- [ ] **Step 1: Write failing test** `internal/numbering/numbering_test.go`:
```go
package numbering

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

// test config + synthetic table
var testCfg = Config{Table: "doc_test", Column: "number", Prefix: "INV-", Pad: 4}

func setup(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "n.db"))
	if err != nil { t.Fatalf("Open: %v", err) }
	if _, err := conn.Exec(`CREATE TABLE doc_test (id INTEGER PRIMARY KEY AUTOINCREMENT, number TEXT NOT NULL UNIQUE)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	return conn
}

func TestNextStartsAtOne(t *testing.T) {
	conn := setup(t); defer conn.Close()
	tx, _ := conn.BeginTx(context.Background(), nil)
	defer tx.Rollback()
	n, err := Next(context.Background(), tx, testCfg)
	if err != nil { t.Fatalf("Next: %v", err) }
	if n != "INV-0001" { t.Fatalf("got %q want INV-0001", n) }
}

func TestNextIncrements(t *testing.T) {
	conn := setup(t); defer conn.Close()
	ctx := context.Background()
	for _, want := range []string{"INV-0001", "INV-0002", "INV-0003"} {
		tx, _ := conn.BeginTx(ctx, nil)
		n, err := Next(ctx, tx, testCfg)
		if err != nil { t.Fatalf("Next: %v", err) }
		if n != want { t.Fatalf("got %q want %q", n, want) }
		if _, err := tx.ExecContext(ctx, `INSERT INTO doc_test (number) VALUES (?)`, n); err != nil {
			t.Fatalf("insert: %v", err)
		}
		tx.Commit()
	}
}

// Concurrent creators must not collide: WithRetry retries the whole tx on UNIQUE conflict.
func TestConcurrentCreateNoCollision(t *testing.T) {
	conn := setup(t); defer conn.Close()
	ctx := context.Background()
	const workers = 12
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- WithRetry(ctx, 5, func() error {
				tx, err := conn.BeginTx(ctx, nil)
				if err != nil { return err }
				defer tx.Rollback()
				n, err := Next(ctx, tx, testCfg)
				if err != nil { return err }
				if _, err := tx.ExecContext(ctx, `INSERT INTO doc_test (number) VALUES (?)`, n); err != nil {
					return err
				}
				return tx.Commit()
			})
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil { t.Fatalf("worker: %v", e) }
	}
	var count int
	conn.QueryRow(`SELECT COUNT(DISTINCT number) FROM doc_test`).Scan(&count)
	if count != workers { t.Fatalf("distinct numbers=%d want %d", count, workers) }
}
```

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement `internal/numbering/numbering.go`**:
```go
package numbering

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Config names a document-number sequence. Table/Column come ONLY from
// predefined package configs (see Invoice/Estimate), never from request input,
// so building the query from them is safe.
type Config struct {
	Table  string
	Column string
	Prefix string // e.g. "INV-"
	Pad    int    // zero-pad width, e.g. 4
}

// Predefined configs used by the invoice/estimate domains (Batches 3-4).
var (
	Invoice  = Config{Table: "invoices", Column: "invoice_number", Prefix: "INV-", Pad: 4}
	Estimate = Config{Table: "estimates", Column: "estimate_number", Prefix: "EST-", Pad: 4}
)

// Next computes the next number for cfg WITHIN the given tx (so it is consistent
// with the insert that follows in the same tx). Caller must INSERT a row using
// the returned number in the SAME tx, and wrap the whole tx in WithRetry to
// survive a UNIQUE-constraint race.
func Next(ctx context.Context, tx *sql.Tx, cfg Config) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("numbering: nil tx")
	}
	// MAX over the integer suffix of values matching the prefix.
	q := fmt.Sprintf(
		`SELECT COALESCE(MAX(CAST(substr(%s, %d) AS INTEGER)), 0) FROM %s WHERE %s LIKE ?`,
		cfg.Column, len(cfg.Prefix)+1, cfg.Table, cfg.Column,
	)
	var max int
	if err := tx.QueryRowContext(ctx, q, cfg.Prefix+"%").Scan(&max); err != nil {
		return "", fmt.Errorf("numbering next: %w", err)
	}
	return fmt.Sprintf("%s%0*d", cfg.Prefix, cfg.Pad, max+1), nil
}

// WithRetry runs fn up to attempts times, retrying only on a UNIQUE-constraint
// error (the numbering race). Other errors return immediately.
func WithRetry(ctx context.Context, attempts int, fn func() error) error {
	if attempts < 1 {
		attempts = 1
	}
	var err error
	for i := 0; i < attempts; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !isUniqueViolation(err) {
			return err
		}
	}
	return fmt.Errorf("numbering: exhausted %d attempts: %w", attempts, err)
}

func isUniqueViolation(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unique") || strings.Contains(s, "constraint")
}
```
The bounded `for i := 0; i < attempts` loop satisfies NASA rule 2.

- [ ] **Step 4: Run** `go test ./internal/numbering/ -race` → PASS (incl the 12-worker no-collision test). `go vet`, `gofmt`.

- [ ] **Step 5: Commit** `feat(numbering): tx-scoped document numbers with retry-on-conflict`.

---

## Task 5: SSE header cleanup + embedded-SPA self-check

**Files:**
- Modify: `internal/http/events.go` (drop keep-alive header)
- Modify: `cmd/tallyo/main.go` (startup self-check)

- [ ] **Step 1: Drop the hop-by-hop header** in `events.go`: remove the
`w.Header().Set("Connection", "keep-alive")` line (it's invalid under HTTP/2 and
stripped by net/http). Keep `Content-Type: text/event-stream` and
`Cache-Control: no-cache`. Run `go test ./internal/http/ -race` → still PASS
(streaming test unaffected).

- [ ] **Step 2: Add an embedded-SPA self-check.** In `cmd/tallyo/main.go`, after
building `assets, err := fs.Sub(tallyoweb.Build, "build")`, verify the SPA shell
is present so a clean-clone/CI build that forgot `npm run build` fails loudly
rather than serving 500s:
```go
	if _, err := fs.Stat(assets, "200.html"); err != nil {
		return fmt.Errorf("embedded SPA missing 200.html — run `npm run build` in web/ before `go build`: %w", err)
	}
```
(import `io/fs` already present.)

- [ ] **Step 3: Verify** `go build ./...` succeeds (web/build currently has a real
build from Batch v2). `go vet ./...`, `gofmt -l` (non-web). Boot smoke:
`DATA_DIR=$(mktemp -d) go run ./cmd/tallyo --port 8092 &` then
`curl -s localhost:8092/api/setup/status` → `{"ownerExists":false}`; kill it.

- [ ] **Step 4: Commit** `chore(http): drop SSE keep-alive header; add embedded-SPA self-check`.

---

## Task 6: Frontend — generic CRUD API helper

**Files:**
- Create: `web/src/lib/api/crud.ts`

A thin typed wrapper over the existing `web/src/lib/api/client.ts` for the
standard list/get/create/update/delete shape every domain uses. (Components are
deferred — they emerge from Batch 1's clients/payers rather than being predicted.)

- [ ] **Step 1: Implement `web/src/lib/api/crud.ts`** using the existing
`apiGet/apiPost/apiPut/apiDelete` from `client.ts` (verify those exist; `apiDelete`
may need adding to client.ts — if so add it mirroring the others):
```ts
import { apiGet, apiPost, apiPut, apiDelete } from './client';

export interface Crud<T, TInput> {
	list(): Promise<T[]>;
	get(id: number): Promise<T>;
	create(input: TInput): Promise<T>;
	update(id: number, input: TInput): Promise<T>;
	remove(id: number): Promise<void>;
}

export function createCrud<T, TInput>(resource: string): Crud<T, TInput> {
	const base = `/api/${resource}`;
	return {
		list: () => apiGet<T[]>(base),
		get: (id) => apiGet<T>(`${base}/${id}`),
		create: (input) => apiPost<T>(base, input),
		update: (id, input) => apiPut<T>(`${base}/${id}`, input),
		remove: (id) => apiDelete<void>(`${base}/${id}`),
	};
}
```

- [ ] **Step 2: Add `apiDelete` to `client.ts`** if missing (mirror apiPut:
`credentials:'include'`, method DELETE, 401→login, parse error). Keep it small.

- [ ] **Step 3: Typecheck** `cd web && npm run check` → 0 errors/0 warnings.
`npm run build` → still emits `web/build/200.html`.

- [ ] **Step 4: Commit** `feat(web): generic CRUD API helper for domain stores`.

---

## Task 7: Frontend — `createCollectionStore` rune factory (SSE-wired)

**Files:**
- Create: `web/src/lib/stores/collection.svelte.ts`
- Test: `web/src/lib/stores/collection.test.ts` (vitest, if the project has vitest configured; else a `npm run check` type-level smoke)

A Svelte 5 runes factory holding a reactive collection for a domain, with
`load()`, and SSE-wired invalidation so a change to the domain's entity refetches.
This is the per-domain UI backbone (Batches 1-8 each instantiate one).

- [ ] **Step 1: Implement `web/src/lib/stores/collection.svelte.ts`**:
```ts
import { onEntity } from '$lib/realtime/events';
import { createCrud, type Crud } from '$lib/api/crud';

export function createCollectionStore<T extends { id: number }, TInput>(
	resource: string,
	entity: string
) {
	const crud: Crud<T, TInput> = createCrud<T, TInput>(resource);
	let items = $state<T[]>([]);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let registered = false;

	async function load() {
		loading = true;
		error = null;
		try {
			items = await crud.list();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	function ensureSubscribed() {
		if (registered) return;
		registered = true;
		onEntity(entity, () => { void load(); }); // SSE invalidation → refetch
	}

	return {
		get items() { return items; },
		get loading() { return loading; },
		get error() { return error; },
		crud,
		load,
		ensureSubscribed,
	};
}
```

- [ ] **Step 2: Typecheck/build** `cd web && npm run check` (0 errors) and
`npm run build` (emits 200.html). If vitest is configured (`web/package.json` has a
`test` script), add a small unit test that mounts the store with a mocked
`createCrud` and asserts `load()` populates `items`; otherwise rely on
`npm run check` + the Batch-1 integration where the store is first used.

- [ ] **Step 3: Commit** `feat(web): createCollectionStore rune factory wired to SSE`.

---

## Task 8: Batch 0 acceptance

- [ ] **Step 1: Full gates**
```bash
cd /Users/dknathalage/repos/tallyo
go test ./... -race
go vet ./...
gofmt -l . | grep -v '^web/' || echo "gofmt clean"
cd web && npm run check && npm run build && ls build/200.html
```
Expected: all green; `web/build/200.html` present.

- [ ] **Step 2: Regression smoke** — boot the binary, run the v2 flow
(setup→login→me→business-profile PUT/GET→SSE) to confirm the WithAudit refactor +
invite atomicity didn't break anything. Confirm an invite create→accept→login
still works and a duplicate-email accept returns 409.

- [ ] **Step 3: Commit** `chore: batch 0 foundation acceptance`.

---

## Done When

- `audit.WithTx` exists and the three existing repos route audited mutations through it; all their tests stay green.
- Invite acceptance is a single transaction; duplicate email → 409; double-accept → 409.
- `internal/numbering` generates collision-free numbers under concurrency (12-worker test green).
- SSE no longer sends `Connection: keep-alive`; the binary fails fast if the embedded SPA is missing.
- `web/src/lib/api/crud.ts` + `web/src/lib/stores/collection.svelte.ts` exist; `npm run check` clean.
- Full suite `go test ./... -race`, vet, gofmt clean; `npm run build` emits the SPA.

Batch 1 (rate_tiers, payers) instantiates this foundation: each domain becomes migration → sqlc → repository(WithAudit) → service(broadcast) → handlers → `createCollectionStore` + routes.
