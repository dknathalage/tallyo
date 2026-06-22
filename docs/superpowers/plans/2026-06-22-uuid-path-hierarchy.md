# UUID Path Hierarchy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the UUID the only public identifier across the `/api` tree and the SvelteKit SPA — every entity addressed by `uuid` in paths and JSON, int64 PK kept internal-only — under a consistent shallow hierarchy; move the NDIS catalogue from the control DB to a tenant-owned resource.

**Architecture:** The schema already carries `uuid TEXT NOT NULL UNIQUE` on every tenant + control entity, and `google/uuid` already generates them on insert. So no schema migration for IDs. The work is: (1) route params `{id}`→`{uuid}`, (2) by-uuid sqlc lookups, (3) translate uuid↔int at the service boundary (path uuid → row; inbound `*Id` uuid bodies → int FK; outbound int FK → related uuid), (4) move catalogue tables control→tenant, (5) retype the SPA.

**Tech Stack:** Go 1.26, chi v5, modernc sqlite, sqlc, goose, `github.com/google/uuid`; SvelteKit SPA (TypeScript).

**Spec:** [`docs/superpowers/specs/2026-06-22-uuid-url-hierarchy-design.md`](../specs/2026-06-22-uuid-url-hierarchy-design.md)

**Parallelism note (for workflow/ralph):** Phase 2 slices are mutually independent (no slice imports another) — safe to fan out one subagent per slice. Phases 1, 3, 4, 5 are barriers: Phase 1 (helper) must land before Phase 2; Phase 4 (catalogue move) is independent of Phase 2 and can run alongside; Phase 5 (SPA) depends on the API shape from Phases 2+4.

---

## Phase 1 — Shared `ParseUUID` helper (barrier; do first)

**Files:**
- Create: `internal/httpx/parseuuid.go`
- Test: `internal/httpx/parseuuid_test.go`

- [x] **Step 1: Write the failing test**

```go
package httpx

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestParseUUID(t *testing.T) {
	cases := []struct {
		name   string
		param  string
		want   string
		wantOK bool
	}{
		{"valid", "3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c", "3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c", true},
		{"empty", "", "", false},
		{"not-a-uuid", "123", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("uuid", tc.param)
			r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
			got, ok := ParseUUID(r, "uuid")
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("ParseUUID=%q,%v want %q,%v", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
```

- [x] **Step 2: Run — expect FAIL** (`go test ./internal/httpx/ -run TestParseUUID -v`) — undefined: ParseUUID.

- [x] **Step 3: Implement**

```go
package httpx

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ParseUUID reads the named path param and validates it as a UUID.
// Returns the canonical lowercase string and true, or "" and false.
func ParseUUID(r *http.Request, name string) (string, bool) {
	raw := chi.URLParam(r, name)
	if raw == "" {
		return "", false
	}
	u, err := uuid.Parse(raw)
	if err != nil {
		return "", false
	}
	return u.String(), true
}
```

- [x] **Step 4: Run — expect PASS.**
- [x] **Step 5: `go vet ./internal/httpx/ && gofmt -l internal/httpx/` clean.**
- [x] **Step 6: Commit** — `feat(httpx): add ParseUUID path-param helper` (commit ab0d9c2)

---

## Phase 2 — Per-slice route + query + JSON switch to UUID

Each slice is independent. The **template below is taxrate (no FKs)** — fully worked. Every other slice follows it; slice-specific deltas are listed after.

### The translation contract (applies to every slice)

1. **Routes:** `{id}` → `{uuid}` in `Routes(r)`. Child-resource params get typed names (`{itemUUID}`, `{paymentUUID}`).
2. **Handler:** `httpx.ParseID(r)` → `httpx.ParseUUID(r, "uuid")`; on `!ok` → 400 `"invalid id"`.
3. **Service/repo public methods:** take `uuid string` instead of `id int64`. Internally, resolve to the row (the by-uuid query returns the full row incl. int PK) and keep using the int PK for any nested mutation/FK.
4. **sqlc queries:** change `WHERE tenant_id = ? AND id = ?` → `WHERE tenant_id = ? AND uuid = ?` for the public Get/Update/Delete. Joins and FK columns stay int (unchanged).
5. **JSON out:** the response struct's identifier becomes the uuid. Retag the existing `UUID string` field as `json:"id"` and **drop the int `ID int64` from JSON** (either remove the field if unused in Go, or tag it `json:"-"`). For every **FK field** (`PlanManagerID *int64 json:"planManagerId"`), join to the referenced table's `uuid` and expose that uuid string under the same JSON name; keep the int FK out of JSON.
6. **JSON in:** create/update bodies now carry related entities as **uuid strings** under the same field names. Resolve each inbound `*Id` uuid → int FK (a `GetXByUUID` lookup returning the int PK) before insert/update. Reject unknown/foreign uuids with a validation error.
7. **404 on miss:** an unknown uuid, or a uuid that exists in another tenant's DB, must 404 (the by-uuid query is already tenant-scoped via `tenant_id = ?`, so a foreign uuid simply returns no rows).

> **The API surface is the slice DTO structs** (e.g. `taxrate.TaxRate`, `participant.Participant`), NOT the sqlc `gen/models.go` structs. `gen` models keep their int fields (internal); the slice's response struct is what gets retagged. If a handler ever returns a `gen` model directly, replace it with the slice DTO.

> **Audit + SSE keep the int PK (learned from the taxrate template, commit 155a3bc).** Two internal uses of the int PK survive and must be preserved when the public method only has a uuid:
> 1. **`audit.WithTx` `Entry{EntityID: <int>}`** — resolve the row *inside the tx* (the by-uuid Get) to recover the int PK for the audit entry, then mutate by uuid. Missing row = silent no-op.
> 2. **Post-commit `realtime.Event.ID int64`** — for Update read it from the returned row; for Delete resolve the row first. **Leave `Event.ID` int here** — Phase 2.8 retypes the SSE payload.

> **Test harness (no pre-existing `handler_test.go`).** Slices have `testhelper_test.go` (`newTestDB` → migrated temp SQLite via `appdb.Open`+`appdb.Migrate`; `seedTenant` → int64 tenant id; `tctx(tenantID)` wraps `reqctx.WithTenant`). For an httptest handler test, mount `h.Routes` on a fresh `chi.NewRouter()` with a one-line middleware injecting `reqctx.WithTenant(req.Context(), tenantID)` (stands in for auth), hit via `httptest.NewServer`. Expect in-package churn: `repository_test.go`/`service_test.go` passing `.ID` (int) to Get/Update/Delete switch to `.UUID`.

### Task 2.0 (TEMPLATE): taxrate

**Files:**
- Modify: `internal/taxrate/handler.go`, `internal/taxrate/service.go`, `internal/taxrate/repository.go`
- Modify: `internal/db/queries/tax_rates.sql`
- Regenerate: `internal/db/gen/` (sqlc)
- Test: `internal/taxrate/handler_test.go` (create if absent)

- [x] **Step 1: Failing handler test** — `GET /tax-rates/{uuid}` returns the rate; an unknown uuid 404s; a non-uuid path 400s. Use `httptest` against the slice's `Routes`, seeding one tax rate via the repo. Assert the JSON `id` equals the seeded **uuid** (not an int).

- [x] **Step 2: Run — expect FAIL** (`go test ./internal/taxrate/ -v`).

- [x] **Step 3: Queries** — in `internal/db/queries/tax_rates.sql` change the by-id lookups to by-uuid:
```sql
-- name: GetTaxRate :one
SELECT * FROM tax_rates WHERE tenant_id = ? AND uuid = ?;

-- name: UpdateTaxRate :one
UPDATE tax_rates SET name = ?, rate = ?, is_default = ?, updated_at = ?
WHERE tenant_id = ? AND uuid = ? RETURNING *;

-- name: DeleteTaxRate :exec
DELETE FROM tax_rates WHERE tenant_id = ? AND uuid = ?;
```
(Match the existing column list; only the WHERE key changes.)

- [x] **Step 4: Regenerate sqlc** — `"$(go env GOPATH)/bin/sqlc" generate`. The params struct field flips from `ID int64` to `Uuid string`.

- [x] **Step 5: Service/repo** — change `Get/Update/Delete(ctx, id int64)` → `(ctx, uuid string)`; pass `Uuid: uuid` into the gen params.

- [x] **Step 6: Handler** — `ParseID` → `ParseUUID(r, "uuid")` in Get/Update/Delete.

- [x] **Step 7: JSON** — in `repository.go` `TaxRate` struct, retag `UUID string` as `json:"id"`, set `ID int64` to `json:"-"`.

- [x] **Step 8: Routes** — `internal/taxrate/handler.go`:
```go
r.Get("/tax-rates/{uuid}", h.Get)
r.Put("/tax-rates/{uuid}", h.Update)
r.Delete("/tax-rates/{uuid}", h.Delete)
```

- [x] **Step 9: Run — expect PASS.**
- [x] **Step 10: `go vet ./... && gofmt -l . && go test ./internal/taxrate/ -race` clean.**
- [x] **Step 11: Commit** — `refactor(taxrate): address by uuid (path + json + queries)`

### Task 2.1 — custom-items, plan-managers
Pure template copies (plan-managers has no inbound FK; custom-items none). Same 11 steps each. Plan-managers is referenced BY participants — its by-uuid lookup is also reused in 2.2.
- [x] custom-items (605be32)
- [x] plan-managers (c2e8ed7; + reusable `GetPlanManagerIDByUUID` query)

### Task 2.2 — participants (FK: plan_manager_id)
Template + FK translation:
- [x] Out: join `plan_managers` for its `uuid`; expose `planManagerId` as the plan-manager **uuid** (`PlanManagerUUID *string json:"planManagerId"`), drop the int. The enrichment join already fetches `PlanManagerName` — add `pm.uuid`.
- [x] In: create/update body `planManagerId` is a plan-manager uuid → resolve to int via a `GetPlanManagerIDByUUID` query before insert; nil/empty stays NULL.
- [x] `/participants/{participantUUID}`. **Note:** `/participants/{id}/stats` is registered by the **invoice** slice (`internal/invoice/handler.go`), not participant — change its route to `{participantUUID}` there and resolve participant uuid→int in the invoice slice. (Same for `/participants/{id}/shifts` if still nested — prefer the `?participant=` filter on shifts, Task 2.4.)
- [x] Filtered cross-link: `GET /participants?…` unchanged; participant's shifts/invoices are served by those slices' `?participant={uuid}` filters (Task 2.4/2.5).

### Task 2.3 — recurring (FK: participant_id, plan_manager_id, tax_rate_id, etc.)
- [x] Template + resolve each inbound FK uuid→int; expose each as the related uuid out.
- [x] Queries: `ListRecurringTemplates`/`GetRecurringTemplate` currently JOIN participant for name only. Add `participant.uuid` to those joins, and add `LEFT JOIN plan_managers pm ON r.plan_manager_id = pm.id` exposing `pm.uuid`. Add a `GetPlanManagerIDByUUID :one SELECT id FROM plan_managers WHERE tenant_id=? AND uuid=?` (reused by Task 2.2 inbound resolution too — declare it once in `plan_managers.sql`).
- [x] `/recurring/{recurringUUID}` and `POST /recurring/{recurringUUID}/generate`.

### Task 2.4 — shifts (FK: participant_id; owned child: line items)
- [x] Template for the shift itself; `/shifts/{shiftUUID}`, `/shifts/{shiftUUID}/status`, `/shifts/{shiftUUID}/divide`. **Note:** `POST /shifts/import` is currently wired in `internal/app/server.go` (not the shift `Routes`) → it has no `{id}`, so no path change, but keep it consistent; leave the wiring as-is unless trivially movable.
- [x] Add `?participant={participantUUID}` filter (resolve participant uuid→int, filter `shifts.participant_id`). Replaces the old nested participant→shifts read.
- [x] Child items nest one level: `/shifts/{shiftUUID}/items`, `/shifts/{shiftUUID}/items/{itemUUID}` (GET/POST/PATCH/DELETE). Resolve `shiftUUID`→shift row, then operate on items by `itemUUID` scoped to that shift's int id.
- [x] Out: shift JSON `id`=shift uuid, `participantId`=participant uuid; item `id`=item uuid. Catalogue refs (`supportItemId`, `catalogVersionId`) are already uuid TEXT — pass through unchanged.

### Task 2.5 — invoices (FK: participant_id; owned children: payments; embedded line items)
> **Inherited from Task 2.4 (commit fc8770a):** shared `billing.LineItem` is already retagged `ID json:"-"`, `UUID json:"id"` — embedded line-item `id` is already the uuid. **Do NOT re-tag the line-item id.** Reusable resolvers exist: `gen.GetParticipantIDByUUID`, `gen.GetShiftIDByUUID`/`GetShiftByID`. The `shiftId`/`invoiceId` int FKs still on `billing.LineItem` are a Phase 3 concern, not this task.
- [x] Template for invoice; `/invoices/{invoiceUUID}` + `/status`, `/pdf`, `/bulk-delete`, `/bulk-status`, `/draft-from-shifts` (body: shift **uuids** → resolve to int ids via `GetShiftIDByUUID`).
- [x] `?participant={participantUUID}` filter.
- [x] Out: `id`=invoice uuid, `participantId`=participant uuid; embedded `lineItems[].id`=line uuid (already present).
- [x] In: `lineItems` array — each line's catalogue refs already uuid; resolve inbound `participantId` uuid→int.
- [ ] Payments child: `/invoices/{invoiceUUID}/payments`, `DELETE /invoices/{invoiceUUID}/payments/{paymentUUID}`. Resolve invoiceUUID→invoice int id; payments keyed by paymentUUID.

### Task 2.6 — estimates (mirror of invoices)
- [x] Same as 2.5 minus payments, plus `/estimates/{estimateUUID}/duplicate` and `/convert`. `/convert` produces an invoice — return the new invoice's uuid.

### Task 2.7 — business-profile + auth/me + invites
- [x] business-profile is a singleton — no id in path, no change beyond confirming JSON `id`=uuid.
- [x] `auth/me` — unchanged (session-scoped).
- [x] Invites: `DELETE /settings/users/invites/{inviteUUID}` — invites already have a `uuid` column. Add `-- name: DeleteInviteByUUID :exec DELETE FROM invites WHERE tenant_id = ? AND uuid = ?` (the invites slice/handler lives in `internal/app`), wire the route, `ParseUUID`. Keep `GET /api/invites/{token}` and `POST /api/invites/{token}/accept` (token = accept secret) unchanged. The invite list response must expose `id`=invite uuid, not the int.

### Task 2.8 — platform structs that leak int ids (REQUIRED — spec: "int PK never crosses the API")
The review found three JSON surfaces outside the slices that still emit int ids:
- [x] **SSE** (`internal/realtime/hub.go`): `Event.ID int64 json:"id"`. The SPA uses the event to refetch by identifier — switch the broadcast payload to carry the entity **uuid** (services already have it post-commit). Retag/replace `ID` with a uuid string; tag any retained int `json:"-"`. Update every `svc` broadcast call site to pass the uuid.
- [x] **User** (`internal/auth/users.go`): `User.ID`/`User.TenantID` int64 are serialized (`/auth/me`). Tag `ID`→ use the user `uuid` as `json:"id"`; expose tenant as `tenantId` = tenant **uuid**; tag the ints `json:"-"`.
- [x] **EmailTenant / session** (`internal/auth/users.go`): the `/auth/login` 409 multi-tenant body and `/auth/session` expose `TenantID int64`. Drop it from JSON (`json:"-"`); keep only `TenantUUID` (retag as `json:"id"` or `tenantId`). The SPA already routes by tenant uuid.
- [ ] Tests: assert each of these three responses contains no integer id field. Commit — `refactor(api): stop leaking int ids via SSE/user/session`.

### Task 2.9 — retype the `internal/app` integration test suite to uuid (REQUIRED; do AFTER all slices migrate)
There is a full HTTP integration suite in `internal/app/*_test.go` (one file per resource: `tax_rates_test.go`, `custom_items_test.go`, `plan_managers_test.go`, `participants_test.go`, `recurring_test.go`, `shifts_test.go`, `invoices_test.go`, `estimates_test.go`, `payments_test.go`, `business_profile_test.go`, `invites_test.go`, plus `auth_test.go`/`signup_test.go` for Task 2.8). Each still asserts the **old int-id contract** and fails once its slice is migrated. They are knowingly RED through all of Phase 2 — this task makes `go test ./...` green again. Two mechanical fixes per file:
- [x] **Decode struct:** the local response struct decodes JSON `id` into `int64` — change it to `string` (the uuid). Address members by the returned uuid, not `strconv.Itoa(id)`.
- [x] **"Missing" cases:** tests that PUT/DELETE a non-existent int id (e.g. `"999"`) now hit `ParseUUID` → 400 instead of the intended 404. Use a well-formed random uuid (e.g. `uuid.NewString()`) for the not-found case so it parses then 404s.
- [x] Apply to every resource file above; for FK-bearing resources, the request bodies that sent int FK ids (`participantId` etc.) now send the related uuid.
- [x] Run `go test ./internal/app/ -race` → green. Commit — `test(app): retype integration suite to uuid contract` (4b3b7e0). **Done EXCEPT 3 tests left RED on a real production gap → Task 2.10.**

### Task 2.10 — bulk-delete by uuid for participant / plan-manager / custom-item (REAL BUG found by Task 2.9)
Task 2.9 found that `bulk-delete` for these three slices still accepts `[]int64` ids across the full stack (handler decodes `Ids []int64 json:"ids"`, `Service.BulkDelete(ctx, []int64)`, repo same) — the int PK crosses the API, violating the spec. invoice/estimate were already migrated to `[]string` uuid arrays; mirror them.
- [x] **participant:** handler decode `Ids []string`; resolve each uuid→int via existing `gen.GetParticipantIDByUUID`; keep service/repo int-based (or add `BulkDeleteByUUID`). Unknown uuid → 400.
- [x] **plan-manager:** same, reuse existing `gen.GetPlanManagerIDByUUID`.
- [x] **custom-item:** same, but **no `GetCustomItemIDByUUID` query exists** — add `-- name: GetCustomItemIDByUUID :one SELECT id FROM custom_items WHERE tenant_id=? AND uuid=?;` to `custom_items.sql` and `sqlc generate`.
- [x] The 3 RED tests (`TestParticipantBulkDelete204`, `TestPlanManagerBulkDelete204`, `TestCustomItemBulkDelete204`) already assert the uuid-array contract — they go green with no further test edits.
- [x] Gate: `go test ./internal/app/ ./internal/participant/ ./internal/planmanager/ ./internal/customitem/ -race`, `go build ./...`, `go vet ./...`, `gofmt -l .` all clean. Commit — `fix(api): bulk-delete by uuid for participant/plan-manager/custom-item`.

> **Gate note:** Per-slice tasks in Phase 2 only require the *slice* test + `go build ./...` to pass. The `internal/app` suite stays RED until Tasks 2.9+2.10. The full `go test ./...` gate is asserted at Task 2.10 and Phase 6.

---

## Phase 3 — Cross-slice read uuids (barrier after Phase 2)

Any enrichment join that surfaced an int FK for the SPA to link on must now surface the related **uuid**. Audit `internal/db/queries/*.sql` for `SELECT … <table>_id` exposed in list/detail DTOs and add the joined `uuid`.

- [x] Grep DTO structs for `*Id *int64`/`int64 \`json:"…Id"\`` still serialized; each is a miss from Phase 2 — fix in the owning slice. (Done 4a4231e: `billing.LineItem` shiftId/invoiceId → `json:"-"`; `shift.Shift.invoiceId` + `estimate.convertedInvoiceId` → uuid-join; authorUserId → `json:"-"`.)
- [x] Run `go test ./... -race`, `go vet ./...`, `gofmt -l .` — all clean. (FULL suite GREEN.)
- [x] Commit — `refactor(api): expose related-entity uuids in enrichment DTOs` (4a4231e)

### Task 3.1 — `customItemId` line-item FK by uuid (genuine leak found in Phase 3 audit)
`customItemId` is a real `custom_items` int FK on `billing.LineItem`/`LineItemInput` and `recurring.RecurringLine`, round-tripped by the SPA both ways — currently leaks the int. The resolver `gen.GetCustomItemIDByUUID` already exists (Task 2.10).
- [x] **Read:** add a `custom_items` uuid join to every line-item read so the row carries the custom-item uuid — central `line_items`, `estimate_line_items`, shift items. `billing.LineItemFromRow` currently gets a bare `gen.LineItem` (no join) — adjust its row source / add a joined variant. Expose `customItemId` as the custom-item uuid string (nil when none); int FK → `json:"-"`.
- [x] **Write:** on every line-item write path (invoice create/update lineItems[], estimate lineItems[], shift item add/patch, recurring lines), resolve inbound `customItemId` uuid → int via `gen.GetCustomItemIDByUUID`; unknown → 400; empty/null → NULL.
- [x] Tests: a line item with a custom item round-trips `customItemId` as the same uuid (create→get); unknown custom-item uuid → 400.
- [ ] Gate: `go test ./... -race`, `go build ./...`, `go vet ./...`, `gofmt -l .` all clean. Commit — `refactor(api): address line-item customItemId by uuid`.

---

## Phase 4 — Catalogue: control DB → tenant-owned (independent of Phase 2)

**Files:**
- Create: `internal/db/migrations/tenant/00003_catalogue.sql` (the 3 tables)
- Modify: `internal/db/migrations/control/00001_control.sql` (remove the 3 CREATE TABLEs + their Down drops)
- Modify: `sqlc.yaml` (the catalogue tables move from the control schema input to the tenant schema input)
- Delete: `internal/db/migrations/control/00002_catalogue_2025_26.sql`, `cmd/cataloguegen/`, `data/catalogue/`
- Modify: `internal/catalog/repository.go` (`reg.Control()` → `reg.Tenant()`), `internal/catalog/handler.go` (drop platform-admin gate → owner/admin)
- Regenerate `internal/db/gen/`

**Clean-break note:** this project edits migration files pre-release (CLAUDE.md: clean-break, fresh schema; dev `*.db*` are gitignored and disposable). So directly removing the tables from `00001_control.sql` and adding `00003_catalogue.sql` (tenant) is the intended path — no DROP-migration dance, just delete dev DBs and let them re-create.

- [x] **Step 0 (sqlc):** Read `sqlc.yaml`. The control schema input currently includes the catalogue tables (via `migrations/control` + the generated `00002_catalogue` file). After the move, ensure the **tenant** schema input includes `00003_catalogue.sql` and the **control** input no longer defines catalogue tables. Confirm no control-plane query JOINs catalogue (the review confirmed catalogue has no FK to control tables and nothing cross-tenant reads it). Regenerate and confirm `gen/` compiles before touching the repo.
- [x] **Step 1:** Cut the `catalog_versions`, `support_items`, `support_item_prices` CREATE TABLEs (keep their `uuid` columns) from `00001_control.sql` (Up and Down) into a new `00003_catalogue.sql` (tenant) with goose Up/Down.
- [x] **Step 2:** Repoint the catalog slice repository to `reg.Tenant()`. The `line_items.catalog_version_id`/`support_item_id` are already uuid TEXT — no change to pinning.
- [x] **Step 3:** Flip the ingest gate from `RequirePlatformAdmin` to `RequireRole(owner/admin)` in the route wiring (`internal/app/server.go`).
- [x] **Step 4:** Delete `cmd/cataloguegen/`, the generated control catalogue migration, and `data/catalogue/`. Keep `internal/catalog` `ParseXLSX`.
- [x] **Step 5:** Routes per spec; mark the ingest/create endpoints **DEFERRED** (leave `ParseXLSX` callable but no new upload wiring this pass). Read endpoints (`GET …/support-catalog/versions`, `…/versions/{versionUUID}/items`, `…/items/{itemUUID}/prices`) serve the now-tenant tables.
- [x] **Step 6:** sqlc generate; fix the catalog repo against the regenerated gen. `go build`, `go test ./internal/catalog/ -race`.
- [x] **Step 7:** Commit — `refactor(catalog): tenant-owned catalogue; drop global seed + cataloguegen`

**ponytail:** catalogue ingest stays deferred — routes reserved, no upload UI this pass. `ParseXLSX` retained so it drops in later without rework.

---

## Phase 5 — SPA retype (after Phases 2+4)

**Files:** `web/src/lib/api/*`, `web/src/routes/**`

- [x] **Step 1:** API client types — entity `id` is now a `string` (uuid); FK fields (`participantId`, `planManagerId`, …) are `string` uuids. Field names unchanged → store/component churn is bounded to the type and any `parseInt` removal.
- [x] **Step 2:** Rename route dirs `web/src/routes/.../[id]` → `[uuid]` (or keep `[id]` as the param name but feed it the uuid — **decide once, apply everywhere**; `[uuid]` matches the API and is clearer).
- [x] **Step 3:** Cross-entity links build from the related uuid now present in DTOs (e.g. invoice → participant link uses `invoice.participantId` which is now a uuid).
- [x] **Step 3b (login 409 inbound — flagged by Task 2.8):** the multi-tenant login flow now returns the tenant **uuid** as `id` in the 409 body; the SPA must send that tenant **uuid** on re-submit. Backend `loginRequest.TenantID int64 json:"tenantId"` is still int — change it to accept the tenant uuid and add a uuid→int resolution step in the login handler's `resolveCredentials`. SSE consumers now key off `event.id` as a uuid string; `/auth/me` `id`+`tenantId` and the session/409 tenant `id` are uuids.
- [x] **Step 4:** `cd web && npm run check` — 0 errors / 0 warnings. `npm run build`.
- [x] **Step 5:** Commit — `refactor(web): address entities by uuid (routes + api types)`

### Task 4.1 — catalogue read endpoints by uuid (last int-path surface)
Phase 4 left the catalogue read routes int-keyed: `GET …/support-catalog/versions/{id}/items` and `…/support-catalog/items/{itemId}/prices` still take int path params, and the catalogue read DTOs (`CatalogVersion`/`SupportItem`/`SupportItemPrice`) serialize int ids. This is the **only remaining int-path surface** — close it for "all paths uuid". The tables already have `uuid` columns (catalog_versions.uuid, support_items.uuid).
- [x] Routes: `{id}` → `{versionUUID}`, `{itemId}` → `{itemUUID}`; `ParseUUID`. Queries (support_items.sql / support_item_prices.sql) look up the version/item by uuid (resolve uuid→int internally, or filter by the uuid column directly).
- [x] DTOs: `CatalogVersion`/`SupportItem`/`SupportItemPrice` expose `id` = their uuid (int → `json:"-"`); any FK between them (support_items.catalog_version_id, prices.support_item_id) exposed as the related uuid or dropped if redundant (price is always fetched under its item).
- [x] SPA: `web/src/lib/api` catalogue types `id` → string; the catalogue browse pages build version/item URLs from the uuid.
- [x] Tests: a catalogue read by version/item uuid returns 200; unknown uuid → 404; non-uuid → 400. Frontend `npm run check` stays 0/0.
- [x] Gate: `go test ./... -race`, `go build ./...`, `CGO_ENABLED=0 go build ./cmd/tallyo`, `go vet ./...`, `gofmt -l .`, `cd web && npm run check && npm run build` — all clean. Commit — `refactor(catalog): address versions/items by uuid (read endpoints)`.

---

## Phase 6 — Docs + final gate

- [ ] Update `docs/data-model.md` ERD: catalogue moves control→tenant (it already shows uuid columns).
- [ ] Update `CLAUDE.md`: catalogue section (no more `cmd/cataloguegen`/generated seed; tenant-owned, owner/admin gated), and note the API addresses entities by uuid.
- [ ] Full gate: `go test ./... -race`, `go vet ./...`, `gofmt -l .` (empty), `CGO_ENABLED=0 go build ./cmd/tallyo`, `cd web && npm run check && npm run build`.
- [ ] Commit — `docs: catalogue tenant-owned + uuid addressing`

---

## Definition of done

- No `/api` path contains an int id; every member route is `/{...UUID}`.
- Every JSON `id` and `*Id` field is a uuid string; no int PK crosses the API.
- An entity uuid from tenant A 404s under tenant B.
- Catalogue tables live in the tenant DB; `cmd/cataloguegen` and the global seed are gone; `ParseXLSX` retained for the deferred ingest.
- SPA `npm run check` clean; full Go gate green.
