# Tallyo Smarts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the scrapped `internal/agent` slice with a curated set of user-initiated AI "Smarts" (button → gather → propose → apply → editable draft), and make the price-list catalogue genuinely tenant-scoped so the grounding search is "for the given tenant".

**Architecture:** A fresh `internal/smarts` vertical slice with a thin Anthropic SDK wrapper exposing two entry points — a forced-single-tool `Propose` and a bounded read-tool-loop `ProposeGrounded`. Each Smart is `gather → propose → apply`; deterministic code prices, validates (via existing `billing`/`invoice` services), and writes. No agent loop, no chat, no persisted conversation. A prerequisite migration adds `tenant_id` to the catalogue tables and scopes every catalogue query.

**Tech Stack:** Go 1.26, chi, SQLite (modernc) + sqlc + goose, `github.com/anthropics/anthropic-sdk-go` (already in go.mod), model `claude-opus-4-8`, SvelteKit SPA.

**Spec:** `docs/superpowers/specs/2026-06-24-tallyo-smarts-design.md`

---

## File map

**Delete:** `internal/agent/` (whole dir, incl. `llm/`).

**Phase 0 — scrap & rename**
- Modify: `internal/app/app.go` (remove agent wiring; rename FeatureAgent→FeatureSmarts)
- Modify: `internal/app/server.go` (remove `/shifts/import`; rename features key)
- Modify: `internal/session/handler.go` (remove `SessionDivider`, `Divide`, divide route)
- Modify: `internal/session/service.go` if it references the divider (verify)
- Modify: `internal/config/*` (FeatureAgent→FeatureSmarts; env `TALLYO_FEATURE_AGENT`→`TALLYO_FEATURE_SMARTS`)
- Modify: `web/src/lib/api/sessions.ts` (remove `divideSession`, `importShifts`)
- Modify: `web/src/lib/components/SessionForm.svelte` (remove "Divide with AI" button)
- Modify: `web/src/lib/stores/features.svelte.ts` (`agent`→`smarts`)

**Phase 1 — catalogue tenancy**
- Create: `internal/db/migrations/tenant/00004_catalogue_tenant.sql`
- Modify: `internal/db/queries/items.sql`, `internal/db/queries/price_list_versions.sql`
- Regen: `internal/db/gen/` via sqlc
- Modify: `internal/pricelist/repository.go` (thread tenantID; extend SearchItems to all fields)
- Modify: `internal/pricelist/service.go`, `internal/pricelist/handler.go` (pass tenant from reqctx)
- Modify: `internal/billing/validation.go` (pass tenantID to catalogue calls)
- Modify any other callers the compiler flags.

**Phase 2-5 — smarts slice**
- Create: `internal/smarts/llm.go`, `service.go`, `handler.go`
- Create: `internal/smarts/draft_invoice.go`, `suggest_lines.go`, `draft_followup.go`, `map_import.go`
- Create: tests `internal/smarts/*_test.go`
- Modify: `internal/app/app.go`, `internal/app/server.go` (wire + routes)

**Phase 6 — frontend**
- Modify: `web/src/lib/api/smarts.ts` (new), relevant route/components for buttons
- Modify: invoice/session/import views to surface buttons

**Phase 7 — docs**
- Modify: `CLAUDE.md`, `docs/data-model.md`

---

## Phase 0 — Scrap the old agent slice

### Task 0.1: Delete the agent package

- [ ] **Step 1: Delete the directory**

```bash
git rm -r internal/agent
```

- [ ] **Step 2: Build to surface all break points**

Run: `CGO_ENABLED=0 go build ./... 2>&1 | head -40`
Expected: FAIL — undefined references in `internal/app` (agent wiring) and possibly `internal/session` (divider). These are the edit sites for the next tasks.

### Task 0.2: Remove agent wiring in app + rename flag

**Files:** Modify `internal/app/app.go`, `internal/config/*`

- [ ] **Step 1:** In `internal/app/app.go`, delete the block that builds `llmClient`, `smarts`, `smartsHandler`, `sessionDivider` (around lines 184-191) and any `import` of `internal/agent`. Where `sessionSvc`/`invoiceSvc` are constructed, drop the `sessionDivider` argument if the session service took one (verify signature — see Task 0.4).
- [ ] **Step 2:** Rename `cfg.FeatureAgent` → `cfg.FeatureSmarts` in config (struct field + env parse: `TALLYO_FEATURE_AGENT` → `TALLYO_FEATURE_SMARTS`, default true). The agent-specific env (`ANTHROPIC_MODEL`/`ANTHROPIC_EFFORT`/`ANTHROPIC_API_KEY`) is re-read by the new smarts client in Phase 2 — for now just remove the `agent.Config` construction.
- [ ] **Step 3:** Compute the feature flag as `featureSmarts := cfg.FeatureSmarts && os.Getenv("ANTHROPIC_API_KEY") != ""` and pass it to the features handler (replaces the old agent flag).

### Task 0.3: Remove the `/shifts/import` route + rename features key

**Files:** Modify `internal/app/server.go`

- [ ] **Step 1:** Delete the `POST .../shifts/import` route registration (server.go:~165) and the `smartsHandler` parameter it used.
- [ ] **Step 2:** In the `GET /api/features` handler, rename the JSON key `"agent"` → `"smarts"`, value = the `featureSmarts` bool.

### Task 0.4: Remove the divide Smart from the session slice

**Files:** Modify `internal/session/handler.go`, `internal/session/service.go`

- [ ] **Step 1:** In `internal/session/handler.go`, delete the `SessionDivider` interface (line ~20), the `Divide` handler method (line ~338), and the `POST .../sessions/{sessionUUID}/divide` route (line ~56). Remove the divider field from the handler struct + constructor.
- [ ] **Step 2:** If `session.Service`/handler constructor took a `SessionDivider`, drop that parameter. Update the `app.go` call site accordingly.

- [ ] **Step 3: Build clean**

Run: `CGO_ENABLED=0 go build ./...`
Expected: PASS

- [ ] **Step 4: Test**

Run: `go test ./... 2>&1 | tail -20`
Expected: PASS (some session/agent tests removed with the dir; fix any dangling test referencing deleted symbols by deleting those tests).

### Task 0.5: Remove frontend AI bits

**Files:** Modify `web/src/lib/api/sessions.ts`, `web/src/lib/components/SessionForm.svelte`, `web/src/lib/stores/features.svelte.ts`

- [ ] **Step 1:** Remove `divideSession` and `importShifts` from `sessions.ts`.
- [ ] **Step 2:** Remove the "Divide with AI" button + its handler/state from `SessionForm.svelte`.
- [ ] **Step 3:** Rename `features.agent` → `features.smarts` in `features.svelte.ts` (and any consumer).

- [ ] **Step 4: Check**

Run: `cd web && npm run check`
Expected: 0 errors / 0 warnings.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "refactor: scrap internal/agent slice; rename feature flag agent→smarts"
```

---

## Phase 1 — Make the catalogue tenant-scoped

> Today `price_list_versions` and `items` carry no `tenant_id` (a latent bug from the DB-per-tenant collapse). This phase adds it and scopes every catalogue query so reads/writes are per-tenant, mirroring how `session`/`client`/`invoice` repos take an explicit `tenantID`.

### Task 1.1: Migration adding tenant_id + indexes

**Files:** Create `internal/db/migrations/tenant/00004_catalogue_tenant.sql`

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up
-- Catalogue tables were created without tenant_id during the DB-per-tenant →
-- single-DB collapse. Add it so the price list is genuinely per-tenant. Backfill
-- assigns existing rows to the sole tenant when exactly one exists (the common
-- clean-break case); multi-tenant installs reload catalogues post-migration.
ALTER TABLE price_list_versions ADD COLUMN tenant_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE items ADD COLUMN tenant_id INTEGER NOT NULL DEFAULT 0;

UPDATE price_list_versions SET tenant_id = (SELECT id FROM tenants ORDER BY id LIMIT 1)
  WHERE (SELECT COUNT(*) FROM tenants) = 1;
UPDATE items SET tenant_id = (SELECT id FROM tenants ORDER BY id LIMIT 1)
  WHERE (SELECT COUNT(*) FROM tenants) = 1;

CREATE INDEX idx_plv_tenant_effective ON price_list_versions (tenant_id, effective_from, effective_to);
CREATE INDEX idx_items_tenant_version ON items (tenant_id, price_list_version_id);

-- +goose Down
DROP INDEX idx_items_tenant_version;
DROP INDEX idx_plv_tenant_effective;
ALTER TABLE items DROP COLUMN tenant_id;
ALTER TABLE price_list_versions DROP COLUMN tenant_id;
```

> Note: confirm the `tenants` table name + PK (`SELECT id FROM tenants`) against `internal/db/migrations/control/`. If the control table is named differently, adjust the subquery.

### Task 1.2: Scope every catalogue query by tenant + all-fields search

**Files:** Modify `internal/db/queries/items.sql`, `internal/db/queries/price_list_versions.sql`

> **Both INSERTs need it.** `items.sql` has `CreateItem` (used by test seeds) AND `UpsertItem` (used by `Ingest`) — add `tenant_id` column + value to **both**.

- [ ] **Step 1:** Add `tenant_id = ?` to every read/write in both files. Use `sqlc.arg(tenant_id)` named args so generated params are clear. Key changes:

`price_list_versions.sql`:
```sql
-- name: ListPriceListVersions :many
SELECT * FROM price_list_versions WHERE tenant_id = ? ORDER BY effective_from DESC;

-- name: ResolvePriceListVersionForDate :one
SELECT * FROM price_list_versions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND effective_from <= sqlc.arg(service_date)
  AND (effective_to IS NULL OR effective_to >= sqlc.arg(service_date))
ORDER BY effective_from DESC LIMIT 1;

-- name: GetCurrentPriceListVersion :one  -- add tenant_id = ?
-- name: CloseOpenPriceListVersions :exec -- add tenant_id = ?
-- name: CreatePriceListVersion :one      -- add tenant_id column + value
```
Add `tenant_id` to the `CreatePriceListVersion` INSERT column list + `VALUES`. `GetPriceListVersionByUUID` / `GetPriceListVersion` / `…IDByUUID` also gain `AND tenant_id = ?` (a UUID is globally unique but scoping defends against cross-tenant uuid guessing).

`items.sql`:
```sql
-- name: ListItems :many
SELECT * FROM items WHERE tenant_id = ? AND price_list_version_id = ? ORDER BY code;

-- name: SearchItems :many  -- ALL searchable fields, tenant-scoped
SELECT * FROM items
WHERE tenant_id = sqlc.arg(tenant_id) AND price_list_version_id = sqlc.arg(version_id)
  AND ( (code     LIKE sqlc.arg(q) ESCAPE '\')
     OR (name     LIKE sqlc.arg(q) ESCAPE '\')
     OR (category LIKE sqlc.arg(q) ESCAPE '\')
     OR (unit     LIKE sqlc.arg(q) ESCAPE '\') )
ORDER BY code LIMIT 50;

-- name: GetItemByCode :one -- add tenant_id = ?
-- name: GetItemIDByUUID :one / GetItem :one -- add tenant_id = ?
-- name: CreateItem / UpsertItem -- add tenant_id column + value
-- name: CountItems / DeleteItemsForVersion -- add tenant_id = ?
```

- [ ] **Step 2: Regenerate sqlc**

Run: `"$(go env GOPATH)/bin/sqlc" generate`
Expected: `internal/db/gen` updated; no errors.

### Task 1.3: Thread tenantID through the pricelist repository

**Files:** Modify `internal/pricelist/repository.go`

- [ ] **Step 1:** Add a `tenantID int64` first param (after `ctx`) to every method: `ListVersions`, `GetVersion`, `GetVersionByUUID`, `ResolveVersionForDate`, `ListItems`, `ResolveVersionIDByUUID`, `SearchItems`, `GetItemByCode`, `Ingest`. Pass it into the matching gen params. For `SearchItems`, populate the single `Q` arg (one `like`) — the gen struct now has one `Q` field, not separate Code/Name.
- [ ] **Step 2:** In `Ingest`, set `TenantID: tenantID` on `CreatePriceListVersionParams` and every `UpsertItemParams`.
- [ ] **Step 3:** In `toItem`, set `Category`/`Unit` as before (no struct change; `tenant_id` stays internal, not exposed in the `Item` JSON view).

### Task 1.4: Fix all callers (service, handler, validator, importer)

**Files:** Modify `internal/pricelist/service.go`, `internal/pricelist/handler.go`, `internal/billing/validation.go`, plus whatever the compiler flags.

- [ ] **Step 1:** `pricelist.Service` methods read the tenant **internally** via `reqctx.MustTenant(ctx)` (mirroring `invoice.Service.Create`, service.go:142) and pass it to the now-`tenantID`-taking repo methods. Do **not** add a `tenantID` param to the public service methods — keeps the handler/HTTP surface unchanged. `MustTenant` **panics if no tenant is in ctx** (reqctx.go:100) — safe because every pricelist + smarts route mounts under the `/api/t/{tenantUUID}` group.
- [ ] **Step 2:** In `billing/validation.go`, `Validate`/`ValidateFilling` already carry `tenantID` (public signatures unchanged — invoice/estimate/session callers compile as-is). Thread `tenantID` into the **private helper chain** that calls the catalogue: add `tenantID int64` to `validateLine` (validation.go:215), `validateSupportLine` (:241), and `resolveVersion` (:248), and pass it from `validate()` (which has it in scope) down through to `cat.GetItemByCode` / `ResolveVersionForDate`.
- [ ] **Step 3:** Build and fix every remaining call site. Grep first: `grep -rn '\.ResolveVersionForDate(\|\.SearchItems(\|\.GetItemByCode(\|\.ListItems(\|\.Ingest(\|\.GetVersion\|\.ResolveVersionIDByUUID(' internal/ | grep -v _test` to enumerate them.

Run: `CGO_ENABLED=0 go build ./...`
Expected: PASS after all call sites updated.

### Task 1.5: Test the tenant-scoped all-fields search

**Files:** Create/extend `internal/pricelist/repository_test.go`

- [ ] **Step 1: Write a failing test** — seed two tenants, each with a version + items that differ only by `category` and `unit`; assert `SearchItems(tenantA, ...)` matching a category substring returns tenant A's item and NOT tenant B's, and that a `unit`-only match is found.

```go
func TestSearchItemsAllFieldsTenantScoped(t *testing.T) {
    // helper to open an in-memory migrated DB + seed two tenants/catalogues
    // ... seed tenantA item{code:"AAA",category:"Therapy",unit:"hour"}
    //     seed tenantB item{code:"BBB",category:"Therapy",unit:"hour"}
    repo := NewItems(database)
    got, err := repo.SearchItems(ctx, tenantA, verA, "Therapy") // category match
    if err != nil { t.Fatal(err) }
    if len(got) != 1 || got[0].Code != "AAA" { t.Fatalf("want only AAA, got %v", got) }
    got, _ = repo.SearchItems(ctx, tenantA, verA, "hour")        // unit match
    if len(got) != 1 { t.Fatalf("unit search should match, got %d", len(got)) }
}
```

- [ ] **Step 2:** Run, watch it fail, implement (already done in 1.2/1.3), run pass.

Run: `go test ./internal/pricelist/ -run TestSearchItemsAllFields -v`
Expected: PASS

- [ ] **Step 3: Full gate + commit**

```bash
go test ./... && go vet ./... && gofmt -l .
git add -A && git commit -m "feat(pricelist): tenant-scope the catalogue; all-fields search"
```

---

## Phase 2 — The Smarts LLM client

### Task 2.1: Define the client interface + Anthropic wrapper

**Files:** Create `internal/smarts/llm.go`

- [ ] **Step 1: Write the client.** Define a `Proposer` interface (so tests can fake it) and an `anthropicClient` implementing it.

```go
package smarts

import (
    "context"
    "encoding/json"

    "github.com/anthropics/anthropic-sdk-go"
    "github.com/anthropics/anthropic-sdk-go/option"
)

const (
    defaultModel = anthropic.ModelClaudeOpus4_8
    maxTokens    = 8000
    maxToolCalls = 6 // bounded grounding loop
)

// Tool is a forced or read-only tool definition we hand the model.
type Tool struct {
    Name        string
    Description string
    Schema      map[string]any // JSON Schema (object)
}

// ProposeRequest forces a single tool call and returns its input JSON.
type ProposeRequest struct {
    System string
    User   string
    Force  Tool
}

// GroundedRequest lets the model call a read-only `search` tool (bounded) then
// MUST emit `commit`. Search is executed by the SearchFunc callback.
type GroundedRequest struct {
    System     string
    User       string
    Search     Tool
    Commit     Tool
    SearchFunc func(ctx context.Context, input json.RawMessage) (string, error)
}

// Proposer is the boundary the Smarts depend on (faked in tests).
type Proposer interface {
    Propose(ctx context.Context, r ProposeRequest) (json.RawMessage, error)
    ProposeGrounded(ctx context.Context, r GroundedRequest) (json.RawMessage, error)
}

type anthropicClient struct {
    sdk    anthropic.Client
    model  anthropic.Model
    effort string
}

func newAnthropicClient(apiKey, model, effort string) *anthropicClient {
    m := defaultModel
    if model != "" {
        m = anthropic.Model(model)
    }
    return &anthropicClient{
        sdk:    anthropic.NewClient(option.WithAPIKey(apiKey)),
        model:  m,
        effort: effort,
    }
}
```

- [ ] **Step 2:** Implement `Propose` — one streaming request with `ToolChoice` forcing `Force.Name`, effort via `output_config`. **Do NOT set `Thinking`** — adaptive thinking and a forced `tool_choice` are mutually exclusive (the old `agent/llm/anthropic.go:67` only enabled thinking when no tool was forced). Build the tool with `anthropic.ToolParam{Name, Description, InputSchema: anthropic.ToolInputSchemaParam{Properties: schema["properties"]}}`, wrap in `ToolUnionParam{OfTool:&t}`, set `ToolChoice: anthropic.ToolChoiceParamOfTool(name)`. Stream + `message.Accumulate` to the final message, find the `ToolUseBlock`, **return `json.RawMessage(block.Input)`** (`ToolUseBlock.Input` is already `json.RawMessage` in v1.50.2 — do NOT use `block.JSON.Input.Raw()`, which is field metadata). Two assertions: non-empty system, exactly-one forced tool block found.

- [ ] **Step 3:** Implement `ProposeGrounded` — a manual loop `for i:=0; i<maxToolCalls; i++`: send `messages` with both tools and **adaptive `Thinking`** (no forced tool here, so thinking is allowed); on `StopReasonToolUse` walk blocks: if a `search` tool_use, call `SearchFunc`, append the assistant turn (`resp.ToParam()`) + a `tool_result` user turn (`anthropic.NewToolResultBlock(block.ID, result, false)`), continue; if a `commit` tool_use, return `json.RawMessage(block.Input)`. If the loop exhausts without commit, return a typed `errNoCommit`. Cap enforced by the counter (NASA rule 2).

### Task 2.2: Build verification (no live API)

- [ ] **Step 1:** Build.

Run: `CGO_ENABLED=0 go build ./internal/smarts/`
Expected: PASS (compiles against the SDK).

- [ ] **Step 2: Commit**

```bash
git add internal/smarts/llm.go && git commit -m "feat(smarts): Anthropic client wrapper (Propose + grounded loop)"
```

---

## Phase 3 — Service, consumer interfaces, wiring

### Task 3.1: Service + interfaces

**Files:** Create `internal/smarts/service.go`

- [ ] **Step 1:** Declare the consumer interfaces (§6 of the spec) and the `Service` holding `Proposer` + deps + `hub`. Constructor `NewService(llm Proposer, sessions SessionReader, cat CatalogueSearcher, invoices InvoiceDrafter, invRead InvoiceReader, clients ClientReader)`. Panic on nil deps (assertion).

```go
type SessionReader interface {
    ListUnbilledForClient(ctx context.Context, tenantID, clientID int64) ([]*session.Session, error)
}
type CatalogueSearcher interface {
    ResolveVersionForDate(ctx context.Context, tenantID int64, serviceDate string) (*pricelist.PriceListVersion, error)
    SearchItems(ctx context.Context, tenantID, versionID int64, query string) ([]*pricelist.Item, error)
    GetItemByCode(ctx context.Context, tenantID, versionID int64, code string) (*pricelist.Item, error)
}
type InvoiceDrafter interface {
    Create(ctx context.Context, in invoice.InvoiceInput, items []billing.LineItemInput) (*invoice.Invoice, error)
}
type InvoiceReader interface {
    GetByUUID(ctx context.Context, uuid string) (*invoice.Invoice, error)
}
type ClientReader interface {
    Get(ctx context.Context, uuid string) (*client.Client, error)
}
```

> `ItemsRepo` already satisfies `CatalogueSearcher` after Phase 1 (its methods now take tenantID). `session` needs a public `ListUnbilledForClient` wrapping `ListRecordedUnbilled` — add it in Task 3.2. `invoice.Service` satisfies `InvoiceDrafter`+`InvoiceReader`; `client.Service` satisfies `ClientReader`.

### Task 3.2: Add the session public reader

**Files:** Modify `internal/session/service.go` (or repository)

- [ ] **Step 1:** Add `func (s *Service) ListUnbilledForClient(ctx context.Context, tenantID, clientID int64) ([]*Session, error)` delegating to `s.repo.ListRecordedUnbilled(ctx, tenantID, clientID)` (repository.go:246 returns `[]*Session`).

### Task 3.3: Handler + routes (stubs first)

**Files:** Create `internal/smarts/handler.go`; modify `internal/app/app.go`, `server.go`

- [ ] **Step 1:** `Handler` wraps `*Service` + an `enabled bool`. `Routes(r chi.Router)` registers `POST /smarts/draft-invoice`, `/smarts/suggest-lines`, `/smarts/follow-up`, `/smarts/map-import` under the tenant group, owner/admin gated. Each handler: if `!enabled` → 503; else decode, call the Smart, `httpx.WriteJSON`.
- [ ] **Step 2:** In `app.go`, build the smarts client only when `featureSmarts`: `llm := smarts.NewAnthropicClient(apiKey, model, effort)`, `smartsSvc := smarts.NewService(llm, sessionSvc, catRepo, invoiceSvc, invoiceSvc, clientSvc)`, `smartsHandler := smarts.NewHandler(smartsSvc, true)`; else `smarts.NewHandler(nil, false)`. Register `smartsHandler.Routes` in the tenant route group.

- [ ] **Step 3: Build + commit**

```bash
CGO_ENABLED=0 go build ./... && git add -A && git commit -m "feat(smarts): service, consumer interfaces, handler stubs, wiring"
```

---

## Phase 4 — The four Smarts (TDD: fake Proposer)

> Each Smart test uses a `fakeProposer` returning canned tool JSON; assert `apply` resolves prices from the catalogue (model never prices), computes tax via the validator, and produces the right result shape.

### Task 4.1: Draft invoice from sessions

**Files:** Create `internal/smarts/draft_invoice.go`, `internal/smarts/draft_invoice_test.go`

- [ ] **Step 1: Write the failing test.** Fake catalogue with item `{code:"CONSULT", unitPrice:100, taxable:true}`; fake sessions for the client; `fakeProposer.ProposeGrounded` returns `{"items":[{"code":"CONSULT","description":"...","quantity":2,"unit":"hour","serviceDate":"2026-06-01"}]}`. Fake `InvoiceDrafter.Create` records its args. Assert: Create called with `Status:"draft"`, one line whose `UnitPrice==100` (from catalogue, NOT model), `Code=="CONSULT"`; returns the invoice UUID.

- [ ] **Step 2:** Run → fail (function undefined).

- [ ] **Step 3: Implement `DraftInvoiceFromSessions(ctx, clientUUID) (string, error)`:**
  - resolve tenant via reqctx; `clients.Get(clientUUID)` → clientID; `sessions.ListUnbilledForClient(tenant, clientID)`; assert non-empty (else 422 "no unbilled sessions").
  - `cat.ResolveVersionForDate(tenant, latestDate)`; if nil → 422 "no price list for date".
  - build system+user from session notes; `SearchFunc` = closure calling `cat.SearchItems(tenant, version.ID, q)` and JSON-encoding results; `ProposeGrounded` with the `draft_invoice` commit schema.
  - parse commit → for each proposed item, `cat.GetItemByCode(tenant, version.ID, code)`; **derive `UnitPrice` from the catalogue item**, build `billing.LineItemInput{ItemID:&item.UUID, PriceListVersionID:&version.UUID, Code, Description, Unit, Quantity, UnitPrice:item.UnitPrice, Taxable:item.Taxable}`.
  - `billing.LineValidator.Validate(ctx, tenant, clientID, items)` for early feedback; on error re-propose once (×2) with the error appended; then `invoices.Create(InvoiceInput{ClientID, Status:"draft", IssueDate:today, DueDate:today+N}, items)`.
  - return `inv.UUID`.

- [ ] **Step 4:** Run → pass.

- [ ] **Step 5: Commit** `feat(smarts): draft invoice from sessions`.

### Task 4.2: Suggest line items

**Files:** Create `internal/smarts/suggest_lines.go`, `_test.go`

- [ ] **Step 1: Failing test** — given a free-text note + catalogue, fake proposer returns suggested items; assert returned lines carry catalogue prices and pass `ValidateFilling`. No invoice write.
- [ ] **Step 2-4:** Implement `SuggestLines(ctx, in SuggestInput) ([]billing.LineItemInput, error)` — same grounded flow as 4.1 but `apply` only resolves prices + `ValidateFilling` and returns the lines (no Create). Run → pass.
- [ ] **Step 5: Commit** `feat(smarts): suggest line items`.

### Task 4.3: Draft overdue follow-up

**Files:** Create `internal/smarts/draft_followup.go`, `_test.go`

- [ ] **Step 1: Failing test** — fake `InvoiceReader.GetByUUID` + `ClientReader.Get`; fake `Propose` returns `{"subject":"...","body":"..."}`; assert non-empty subject/body returned, and the prompt included the invoice number + amount + client name (assert via the fake capturing the user content).
- [ ] **Step 2-4:** Implement `DraftFollowUp(ctx, invoiceUUID) (FollowUp, error)` — gather invoice+client, single forced-tool `Propose`, assert non-empty, return `{Subject, Body}`. Run → pass.
- [ ] **Step 5: Commit** `feat(smarts): draft overdue follow-up`.

### Task 4.4: Map price-list import

**Files:** Create `internal/smarts/map_import.go`, `_test.go`

- [ ] **Step 1: Failing test** — given headers `["Item Code","Description","Price"]` + sample rows + the importer's known target fields, fake `Propose` returns `{"mappings":[{"header":"Item Code","field":"code"},...]}`; assert unknown target fields are dropped and known ones pass through.
- [ ] **Step 0 (prereq):** `internal/importer/mapping.go` keeps the valid targets in an **unexported** `validTargets` (`name`(required)`, code, unit, category, unitPrice, taxable`). Export them: add `func TargetFields() []string` (or `var TargetFields = []string{...}`) and have `validTargets` derive from it. Commit this as a tiny standalone change first.
- [ ] **Step 2-4:** Implement `MapImport(ctx, in MapInput) (MapResult, error)` — single forced-tool `Propose` over `importer.TargetFields()`; validate each `field` against that set, drop unknowns. Run → pass.
- [ ] **Step 5: Commit** `feat(smarts): map price-list import columns`.

### Task 4.5: Wire the four handlers to the service methods

**Files:** Modify `internal/smarts/handler.go`

- [ ] **Step 1:** Each route decodes its request (`{clientId}`, `{note,clientId}`, `{invoiceId}`, `{headers,rows}`), calls the matching service method, `WriteJSON`. Map typed errors → status (502 model, 422 validation/no-data). Build + `go test ./internal/smarts/...`.
- [ ] **Step 2: Commit** `feat(smarts): wire Smart handlers`.

---

## Phase 5 — Full backend gate

### Task 5.1: Gate

- [ ] **Step 1:** `go test ./... -race`
- [ ] **Step 2:** `go vet ./...` ; `gofmt -l .` (empty)
- [ ] **Step 3:** `CGO_ENABLED=0 go build ./cmd/tallyo`
- [ ] **Step 4: Commit** any fixups.

---

## Phase 6 — Frontend buttons

### Task 6.1: API bindings

**Files:** Create `web/src/lib/api/smarts.ts`

- [ ] **Step 1:** Functions: `draftInvoice(clientId)`, `suggestLines(payload)`, `draftFollowUp(invoiceId)`, `mapImport(headers, rows)` — each POSTs to the matching `/api/t/{tenant}/smarts/...` route, typed against the existing API helpers. Gate calls on `features.smarts`.

### Task 6.2: Surface the buttons (gated on `features.smarts`)

**Files:** Modify the client/sessions view (draft invoice), invoice/estimate editor (suggest lines), overdue invoice view (follow-up), import wizard (map import).

- [ ] **Step 1:** Add a single tasteful button per surface labeled by outcome ("Draft invoice", "Suggest items", "Draft reminder", "Auto-map"), each with a loading state, hidden when `!features.smarts`.
- [ ] **Step 2:** Draft-invoice button → on success navigate to `/{tenant}/invoices/{uuid}`. Suggest-lines → autofill the line editor with a subtle AI marker. Follow-up → fill a compose box. Map-import → pre-fill the wizard mapping.

- [ ] **Step 3: Check + commit**

```bash
cd web && npm run check && npm run build && cd ..
git add -A && git commit -m "feat(web): surface Smarts buttons (gated on features.smarts)"
```

---

## Phase 7 — Docs

### Task 7.1: Update CLAUDE.md + data-model

**Files:** Modify `CLAUDE.md`, `docs/data-model.md`

- [ ] **Step 1:** In `CLAUDE.md`: replace the `internal/agent` description with `internal/smarts` (curated Smarts: gather→propose→apply, editable draft, no agent loop). Update the price-list section — now genuinely tenant-scoped (`tenant_id` on `price_list_versions`+`items`). Update the env vars (`TALLYO_FEATURE_SMARTS`).
- [ ] **Step 2:** In `docs/data-model.md`: add `tenant_id` to the catalogue tables in the Mermaid ERD.
- [ ] **Step 3: Commit** `docs: update for smarts slice + tenant-scoped catalogue`.

---

## Done criteria

- `go test ./... -race`, `go vet`, `gofmt -l .`, `CGO_ENABLED=0 go build ./cmd/tallyo`, `cd web && npm run check && npm run build` all clean.
- With `ANTHROPIC_API_KEY` unset: smarts routes 503, buttons hidden. With it set: each Smart produces an editable draft; the draft-invoice Smart's lines are priced from the catalogue (never the model); a second tenant cannot see the first tenant's catalogue via search.
