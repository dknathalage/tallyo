# Tallyo Smarts — Design

**Status:** approved design, pre-implementation
**Date:** 2026-06-24
**Supersedes:** the existing `internal/agent/` slice (to be deleted wholesale)

## 1. Overview

Add tasteful, user-initiated AI automation to Tallyo — "Smarts". A Smart is a
**button labeled by an outcome** (not a prompt box, not a chatbot, not an agent
loop). The user taps it; deterministic code gathers facts; one or more LLM calls
fill a fixed schema (grounding against live tenant data via a read-only `search`
tool where needed); deterministic code validates and applies the result; the user
lands in an **editable draft**. They never type a prompt.

This is the "workflow that breaks down work, makes LLM calls, performs actions"
the requester asked for. The breakdown is the fixed pipeline **gather → propose →
apply** — there is no autonomous agent loop, no persisted conversation, no
approval gate. The editable draft *is* the review.

### What we are NOT building

- No chatbot / prompt box the user types into.
- No agent harness: no tool registry, no plan phase, no approval/checkpoint
  tables, no token budget bookkeeping, no SSE progress stream.
- No `conversations` / `messages` / `steps` tables. A Smart is one HTTP request
  with a bounded set of LLM calls inside it; nothing persists but the result of
  `apply`, written through the normal audited service layer.

### Core invariant

**The model proposes structured data; deterministic code validates and applies
it.** The model picks *which* catalogue code, *what* quantity, *what* description,
*what* message text. It never prices, never writes to the DB directly, never gets
an approval gate. Everything touching money or the database is the existing,
already-trusted service + validator code. The model cannot misprice.

## 2. What gets scrapped

Delete `internal/agent/` entirely:

- `smarts.go`, `config.go`, `deps.go`, `extract.go`,
  `smart_import_shifts.go`, `smart_divide_session.go`,
  `smart_draft_propose.go`, `smarts_handler.go`, and `internal/agent/llm/`.
- Its tests.

Remove its wiring in `internal/app` (the `smartsHandler` / `sessionDivider`
branch around `app.go:184-191`, plus the `/shifts/import` route in
`server.go:165`). The `/sessions/{sessionUUID}/divide` route is owned by the
session slice — registered in `internal/session/handler.go:56` via the
`session.SessionDivider` interface; remove that interface, the `Divide` handler
(`handler.go:20,338`), and its route too.

Frontend: remove the "Divide with AI" button in `SessionForm.svelte` and the
`importShifts` / `divideSession` bindings in `web/src/lib/api/sessions.ts`.

The `features.agent` flag is **renamed** to `features.smarts`, still computed as
`cfg.FeatureSmarts && apiKey != ""` and exposed at `GET /api/features`. Smarts
endpoints return 503 and the buttons hide when no `ANTHROPIC_API_KEY` is set.

## 3. LLM client (built fresh)

New thin wrapper `internal/smarts/llm.go` over `github.com/anthropics/anthropic-sdk-go`
(already a dependency). Not a reuse of the old `agent/llm` package — a fresh,
minimal client.

- Model: `claude-opus-4-8` (overridable via `ANTHROPIC_MODEL`).
- Thinking: adaptive (`thinking: {type: "adaptive"}`).
- Effort: from `ANTHROPIC_EFFORT` (default `high`); validated against the
  allowed set.
- Streaming: used for any call so large `max_tokens` never hits an HTTP timeout;
  accumulate to the final message.
- Prompt caching: ephemeral cache_control on the stable system prefix + tool
  definitions (cheap, since the system + tool schema repeat across calls).

Two entry points — that is the whole surface:

```go
// Propose: one forced-single-tool call. The model MUST emit `toolName` with
// input matching `schema`. Returns the raw tool input JSON.
func (c *Client) Propose(ctx context.Context, p ProposeRequest) (json.RawMessage, error)

// ProposeGrounded: a BOUNDED read-tool loop. The model may call the read-only
// `search` tool repeatedly to ground specifics, then MUST emit the final
// `commit` tool. Capped at maxCalls (default 6). Returns the commit input JSON.
func (c *Client) ProposeGrounded(ctx context.Context, p GroundedRequest) (json.RawMessage, error)
```

`ProposeRequest` / `GroundedRequest` carry: system prompt, user content, the
forced tool schema(s), and (for grounded) a `SearchFunc` callback the loop
invokes when the model calls `search`. The loop is `for i := 0; i < maxCalls; i++`
— a statically bounded counter, no `while(true)` (NASA rule 2).

## 4. The seamless tenant-scoped search capability

Grounding is "give capability, not answers": instead of the app pre-resolving
"measure X → code Z" and handing the model a candidate list, we expose **one
read-only `search` tool** and let the model map facts → specifics itself.

### 4.0 Prerequisite: make the catalogue genuinely tenant-scoped

**The catalogue is not tenant-scoped today.** Despite CLAUDE.md and the query
comments claiming "per-tenant", `price_list_versions` and `items`
(`internal/db/migrations/tenant/00003_catalogue.sql`) carry **no `tenant_id`
column** — a latent bug left over from the DB-per-tenant → single-DB collapse
(commit `c812968`). All tenants currently share one global catalogue, and
`ResolveVersionForDate` / `SearchItems` have no tenant predicate.

This work fixes that as a **prerequisite migration** (decided during design):

1. New tenant goose migration adds `tenant_id INTEGER NOT NULL` to both
   `price_list_versions` and `items`, with an index on `(tenant_id, ...)` used by
   the lookups. Backfill: if exactly one tenant exists, stamp all rows with it;
   otherwise the migration assigns rows to their owning tenant if derivable, else
   the operator reloads catalogues post-migration (document this — a fresh
   clean-break data model means most installs have one tenant).
2. Every catalogue query (`ResolvePriceListVersionForDate`, `SearchItems`,
   `ListItems`, `GetItemByCode`, the import writes) gains a `tenant_id = ?`
   predicate; the repo methods take the tenant from `reqctx` like every other
   tenant repo. `sqlc generate` regenerates `internal/db/gen`.
3. Update CLAUDE.md + `docs/data-model.md` to match (the "scoped per tenant by
   `tenant_id`" claim becomes true).

After this, catalogue reads are tenant-scoped exactly like clients/sessions, and
the search tool below is genuinely "for the given tenant".

### 4.1 All-fields search

**The search covers all searchable fields, scoped to the tenant, seamlessly.**
Today `pricelist.SearchItems` (repository.go:155) LIKE-matches only `code` OR
`name`. We extend it to match across **all searchable columns** — `code`,
`name`, `category`, `unit` — so the model can find an item by any field a human
could, in one call. With §4.0 in place the query is guarded by both the resolved
version and the tenant:

```sql
-- internal/db/queries/items.sql (extend SearchItems)
-- match across all searchable fields; q is the LIKE pattern, escaped.
WHERE i.tenant_id = ?
  AND i.price_list_version_id = ?
  AND ( i.code     LIKE ? ESCAPE '\'
     OR i.name     LIKE ? ESCAPE '\'
     OR i.category LIKE ? ESCAPE '\'
     OR i.unit     LIKE ? ESCAPE '\' )
ORDER BY i.code
LIMIT 50
```

`// ponytail: LIKE across columns. If a tenant's catalogue grows past a few
thousand items and search feels slow, switch to SQLite FTS5 over a generated
search column — same call signature, no API change.`

The `search` tool exposed to the model returns `[]{code, name, unit, category,
unitPrice}` for the resolved price-list version (resolved by service date via
`ResolveVersionForDate`, now tenant-scoped per §4.0). If no version resolves for
the date (`ResolveVersionForDate` returns `(nil, nil)`, repository.go:115), the
Smart aborts before any LLM call with a 422 "no price list loaded for this date".
The model never sees `tenant_id` or int PKs.

The same broad, tenant-scoped read philosophy applies to the Smarts that resolve a
**client** by name: they call `client.Service.List(ctx, search)` (service.go:37),
which already does tenant-scoped search across client fields. No new per-field
tools — one `search` per entity kind the Smart needs to ground against.

## 5. Slice layout

`internal/smarts/` — a vertical slice following the repo conventions
(handler → service → deps; no other slice imported directly; cross-domain
behaviour through consumer-declared interfaces wired in `internal/app`).

| file | role |
|---|---|
| `llm.go` | the fresh Anthropic wrapper (§3). |
| `service.go` | `Service` holding the LLM client + the consumer interfaces (§6). Constructed in `internal/app`. |
| `handler.go` | `Routes(r chi.Router)` registers one `POST` per Smart under the tenant group. 503 when disabled. The "registry" is this curated route list — no plugin dispatch machinery. |
| `draft_invoice.go` | Smart: draft invoice from a client's unbilled sessions. |
| `suggest_lines.go` | Smart: suggest line items for an open invoice/estimate. |
| `draft_followup.go` | Smart: draft an overdue-invoice follow-up message. |
| `map_import.go` | Smart: map price-list import headers → fields. |

Each Smart file is `gather` (read facts) → `propose`/`proposeGrounded` (LLM) →
`apply` (validate + act), each function short (≤60 lines; NASA rule 4), at least
two boundary assertions per non-trivial function (rule 5).

## 6. Consumer interfaces (grounded in real signatures)

Declared in `internal/smarts/service.go`, satisfied by existing repos/services,
wired in `internal/app`. No `smarts` import in any other slice.

```go
// Unbilled sessions for a client (session slice).
type SessionReader interface {
    ListUnbilledForClient(ctx context.Context, clientID int64) ([]session.Session, error)
}

// Tenant-scoped, all-fields catalogue search + version resolution (pricelist).
type CatalogueSearcher interface {
    ResolveVersionForDate(ctx context.Context, serviceDate string) (*pricelist.PriceListVersion, error)
    SearchAllFields(ctx context.Context, versionID int64, query string) ([]*pricelist.Item, error)
    GetItemByCode(ctx context.Context, versionID int64, code string) (*pricelist.Item, error)
}

// Draft-invoice creation through the trusted service path (invoice).
// in.Status = "draft"; items already validated by the smart's apply.
type InvoiceDrafter interface {
    Create(ctx context.Context, in invoice.InvoiceInput, items []billing.LineItemInput) (*invoice.Invoice, error)
}

// Reads for the follow-up smart (invoice).
type InvoiceReader interface {
    GetByUUID(ctx context.Context, uuid string) (*invoice.Invoice, error)
}

// Client reads (client). List is already tenant-scoped + searchable.
type ClientReader interface {
    Get(ctx context.Context, uuid string) (*client.Client, error)
    List(ctx context.Context, search string) ([]*client.Client, error)
}
```

(Method names like `ListUnbilledForClient` / `SearchAllFields` may need a thin
public wrapper on the existing repo — the underlying queries exist
(`ListRecordedUnbilled`, `SearchItems`); the plan phase resolves the exact
exported surface.)

`apply` resolves the model's `code`s to prices deterministically from the
catalogue, then writes through `invoice.Service.Create`. **`Create` is itself
self-validating** — it already calls `s.validator.Validate` internally and
overwrites `in.Tax` with the computed value (service.go:141-143), so the pricing
invariant is enforced there regardless of what the Smart does. The Smart *also*
runs `billing.LineValidator.Validate(ctx, tenantID, clientID, items)`
(validation.go:129) up front, but only to get **early, structured error feedback**
for the bounded re-propose loop (§7.1) — not because pre-validation is what
enforces correctness. Either way tax is computed, the model's numbers discarded.
`Create` audits and broadcasts the SSE event post-commit.

## 7. The four Smarts

### 7.1 Draft invoice from sessions (big Smart, grounded)

- **Surface:** client view / sessions list. Available when the client has
  unbilled (`recorded`) sessions.
- **gather:** `SessionReader.ListUnbilledForClient(clientID)` → the sessions and
  their free-text notes; resolve the catalogue version for the latest service date.
- **propose (grounded loop):** system prompt + the session notes; the model calls
  `search` (all-fields, tenant-scoped) to find catalogue codes, then emits the
  final `draft_invoice` tool: `{ items: [{code, description, unit, quantity,
  taxable, serviceDate}] }`. ≤6 calls.
- **apply:** map proposed `code`s → `LineItemInput` (resolve `ItemID` /
  `UnitPrice` deterministically from the catalogue by code — the model never sets
  price), run `LineValidator.Validate`, then `InvoiceDrafter.Create` with
  `InvoiceInput{Status:"draft", ClientID, IssueDate, DueDate}`. On validation
  failure, re-propose once (×2 total) feeding the error back; then give up with a
  typed error.
- **result:** return the new invoice UUID; frontend navigates the user into the
  editable draft invoice.

### 7.2 Suggest line items (small Smart, grounded)

- **Surface:** open invoice/estimate editor.
- **gather:** the editor's free-text note + (optionally) the client's recent line
  history; resolve catalogue version.
- **propose (grounded loop):** model searches the catalogue, emits
  `suggest_lines` → `{ items: [...] }`. No write.
- **apply:** resolve codes → prices deterministically, `ValidateFilling` (the
  fill-tolerant validator, validation.go:137), return the suggested lines.
- **result:** silent autofill into the line editor with a subtle AI marker;
  fully editable. No modal.

### 7.3 Draft overdue follow-up (small Smart, no grounding)

- **Surface:** an overdue invoice.
- **gather:** `InvoiceReader.GetByUUID` + `ClientReader.Get` → amount, due date,
  number, client name. No catalogue search.
- **propose:** single forced-tool `draft_followup` → `{ subject, body }`. Polite,
  factual reminder; the model writes the prose only.
- **apply:** trivial — assert non-empty, return the draft.
- **result:** editable draft in a compose box. (Sending email is out of scope for
  this design — the draft is copy/paste-ready; wiring an SMTP/send path is a
  separate feature.)

### 7.4 Map price-list import (small Smart, no grounding)

- **Surface:** the existing price-list import mapping wizard, after
  `…/price-list/import/inspect` returns headers + a row sample.
- **gather:** the detected headers + sample rows (already in hand from inspect).
- **propose:** single forced-tool `map_columns` → `{ mappings: [{header,
  field}], categoryGuesses?: [...] }` over the fixed set of target fields the
  importer understands.
- **apply:** validate each `field` is a known target; drop unknowns; return the
  mapping.
- **result:** pre-fills the mapping wizard; the user adjusts and commits via the
  existing `…/import/commit`.

## 8. Result UX summary

- **Big Smart** (draft invoice) → create the draft artifact, drop the user into
  it fully editable.
- **Small Smarts** (suggest lines, follow-up, map import) → autofill a field /
  wizard with a subtle AI marker; no modal, no streaming panel.
- Every result is editable; the user always adjusts. (Matches the requester's
  "backseat features, user always adjusts".)

## 9. Errors, gating, safety

- **Gating:** all routes behind owner/admin + tenant context, and behind the
  `features.smarts` flag (env flag AND key present). 503 + hidden buttons when off.
- **Error surfacing:** `apply` returns plain typed errors → normal
  `httpx.WriteError`. Raw model/tool error strings never reach the UI. Model
  failure → 502; validation failure after bounded re-propose → 422 with a
  human-readable message.
- **Bounded everything:** grounded loop ≤6 calls; re-propose ≤2; every loop has a
  static counter.
- **Tenant isolation:** every gather/search/write guards `WHERE tenant_id = ?`
  via `reqctx`. The model never sees tenant IDs or int PKs.
- **Audit + realtime:** writes go through existing services, so they are audited
  and broadcast SSE events post-commit for free.

## 10. Testing

- Go unit tests per Smart with a **fake LLM client** (the `Client` is an
  interface at the service boundary) returning canned tool JSON — assert `apply`
  resolves prices from the catalogue (not from the model), computes tax via the
  validator, and writes a draft. No live API calls in tests.
- A test for the all-fields search query: seed items differing only in `category`
  / `unit`, assert each is found.
- `svelte-check` clean for the frontend button additions.
- `go test ./...`, `go vet`, `gofmt`, cgo-free build all clean.

## 11. Out of scope / YAGNI

- No email/SMS sending for the follow-up Smart (draft only).
- No estimate/recurring Smarts yet (the plumbing generalizes; add later).
- No per-tenant API keys (operator's single `ANTHROPIC_API_KEY`).
- No streaming progress UI, no usage metering tables.
- No multi-phase agent pipelines — `gather → propose → apply` is the whole shape.
