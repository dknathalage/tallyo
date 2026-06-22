# Tenant Wiring Fix + UUID-Named Tenant DB Files — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the `tenantPath(shifts): no active tenant set` error on tenant pages, and name per-tenant SQLite files by tenant UUID instead of integer id.

**Architecture:** (1) Frontend — publish the active tenant in a SvelteKit universal `load()` on the `[tenant]` layout, which runs before any child component mounts, eliminating the race against page `onMount` data loads. (2) Backend — the `tenantdb.Registry` resolves `id → uuid` from the control DB (cached) and names files `tenants/tenant-<uuid>.db`.

**Tech Stack:** Go 1.26 (modernc SQLite, sqlc), SvelteKit SPA (Svelte 5 runes, `adapter-static`, `ssr=false`).

---

## Context

After the DB-per-tenant refactor:

1. **Frontend race:** tenant-scoped fetches build URLs via `tenantPath()` in `web/src/lib/api/client.ts`, which reads a module global `_activeTenant`. That global is set ONLY by the layout `$effect` (`web/src/routes/[tenant]/+layout.svelte`). The `[tenant]` home page loads data in its own `onMount` (`web/src/routes/[tenant]/+page.svelte:12` → `shifts.load()` → `tenantPath('shifts')`). No happens-before guarantee between the effect and the child `onMount` → when the load wins, `_activeTenant` is null and `tenantPath` throws.

2. **File naming:** tenant DB files are named `tenant-<id>.db` (control-DB int id). They should use the tenant UUID: `tenant-<uuid>.db`.

## File Structure

- **New** `web/src/routes/[tenant]/+layout.ts` — universal `load` that sets the active tenant before child mount.
- **Modify** `web/src/routes/[tenant]/+layout.svelte` — drop the redundant `setActiveTenant` call from the `$effect` (single source of truth = `load`).
- **Modify** `internal/tenantdb/registry.go` — `id → uuid` cache + `pathLocked` building `tenant-<uuid>.db`.
- **Modify** `internal/tenantdb/registry_test.go` — seed `tenants` rows so id→uuid resolves; assert the UUID file name.

---

## Task 1: Name tenant DB files by UUID (backend)

**Files:**
- Modify: `internal/tenantdb/registry.go`
- Test: `internal/tenantdb/registry_test.go`

- [ ] **Step 1: Write/extend the failing test**

In `registry_test.go`, seed `tenants` rows after `MigrateControl` (so the registry can resolve id→uuid), and add an assertion that the file on disk is UUID-named:

```go
// in newReg(t), after MigrateControl(control):
for id := int64(1); id <= 10; id++ {
	if _, err := control.Exec(
		`INSERT INTO tenants (id, uuid, name, status, created_at, updated_at)
		 VALUES (?, ?, ?, 'active', '2026-01-01', '2026-01-01')`,
		id, fmt.Sprintf("uuid-%d", id), fmt.Sprintf("Tenant %d", id)); err != nil {
		t.Fatalf("seed tenant %d: %v", id, err)
	}
}
```

```go
func TestForTenantID_FileNamedByUUID(t *testing.T) {
	reg := newReg(t)
	if _, err := reg.ForTenantID(3); err != nil {
		t.Fatalf("ForTenantID(3): %v", err)
	}
	if _, err := os.Stat(filepath.Join(reg.dataDir, "tenants", "tenant-uuid-3.db")); err != nil {
		t.Fatalf("expected tenant-uuid-3.db: %v", err)
	}
}
```

Add imports `fmt` and `os` to the test.

- [ ] **Step 2: Run it; verify it fails**

Run: `go test ./internal/tenantdb/ -run TestForTenantID -v`
Expected: FAIL (file is `tenant-3.db`, not `tenant-uuid-3.db`; or id→uuid lookup missing).

- [ ] **Step 3: Implement id→uuid resolution + UUID file name**

In `registry.go`: add `uuids map[int64]string` to `Registry` (init in `New`). Replace `path(id)` with:

```go
func (r *Registry) pathLocked(id int64) (string, error) {
	uuid, ok := r.uuids[id]
	if !ok {
		if err := r.control.QueryRowContext(context.Background(),
			"SELECT uuid FROM tenants WHERE id = ?", id).Scan(&uuid); err != nil {
			return "", fmt.Errorf("tenantdb: resolve uuid for tenant %d: %w", id, err)
		}
		r.uuids[id] = uuid
	}
	return filepath.Join(r.dataDir, "tenants", fmt.Sprintf("tenant-%s.db", uuid)), nil
}
```

`ForTenantID` calls `pathLocked(id)` (under the existing `r.mu` lock) instead of `path(id)`.

- [ ] **Step 4: Run tests; verify pass**

Run: `go test ./internal/tenantdb/... -v`
Expected: PASS.

- [ ] **Step 5: Gates + commit**

Run: `gofmt -l . | grep -v '^web/'` (empty); `go vet ./...`; `go test ./... -race`; `CGO_ENABLED=0 go build ./cmd/tallyo`.

```bash
git add internal/tenantdb/
git commit -m "refactor(tenantdb): name tenant DB files by UUID, not integer id"
```

---

## Task 2: Race-proof active-tenant wiring (frontend)

**Files:**
- Create: `web/src/routes/[tenant]/+layout.ts`
- Modify: `web/src/routes/[tenant]/+layout.svelte`

- [ ] **Step 1: Add the layout `load` that publishes the tenant before mount**

Create `web/src/routes/[tenant]/+layout.ts`:

```ts
import type { LayoutLoad } from './$types';
import { setActiveTenant } from '$lib/api/client';

// Publish the active tenant BEFORE any child component mounts / onMount runs, so
// tenant-scoped fetches (tenantPath) never race the layout effect. Universal
// load re-runs whenever params.tenant changes, covering tenant switching.
export const load: LayoutLoad = ({ params }) => {
	setActiveTenant(params.tenant ?? null);
	return {};
};
```

- [ ] **Step 2: Remove the redundant setter from the layout effect**

In `web/src/routes/[tenant]/+layout.svelte`, inside the `$effect` `untrack(() => { ... })` block, delete the `setActiveTenant(uuid);` line (now owned by `load`). Keep `closeEvents()`, `loadMe`, `features.load`, `openEvents`. Remove the `setActiveTenant` import if no longer referenced.

- [ ] **Step 3: Type-check**

Run: `cd web && npm run check`
Expected: 0 errors / 0 warnings (`$types` provides `LayoutLoad`; unused-import check clean).

- [ ] **Step 4: Build the SPA (so the Go binary embeds the fix)**

Run: `cd web && npm run build`
Expected: emits `web/build` with no errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/routes/[tenant]/+layout.ts web/src/routes/[tenant]/+layout.svelte web/build
git commit -m "fix(web): set active tenant in [tenant] layout load to avoid tenantPath race"
```

---

## Verification (end-to-end)

- [ ] Backend smoke: boot with a fresh data dir, sign up, confirm the UUID-named file:

```bash
rm -rf /tmp/tw-smoke && CGO_ENABLED=0 go build -o /tmp/tallyo ./cmd/tallyo
/tmp/tallyo --port 18098 --data-dir /tmp/tw-smoke &
sleep 2
curl -s -X POST localhost:18098/api/signup -H 'Content-Type: application/json' \
  -d '{"businessName":"TW","name":"O","email":"tw@x.com","password":"password123","zone":"national"}' >/dev/null
ls /tmp/tw-smoke/tenants/        # expect tenant-<uuid>.db (NOT tenant-1.db)
```

- [ ] Frontend manual: `cd web && npm run dev` (proxy to the running server), sign up / log in, land on the Shifts page — the shift list loads with **no** `no active tenant set` error in the console; "Add shift" navigates to `/{uuid}/shifts/new`.

- [ ] Full gate: `go test ./... -race`, `go vet ./...`, `gofmt -l .` clean, `"$(go env GOPATH)/bin/sqlc" generate` no drift.
