# Strengthening the Notes→Invoice Agent Harness — Design

**Date:** 2026-06-17
**Status:** Approved (design); implementing
**Scope:** Reliability of the notes→invoice agent workflow ONLY. No general agent changes.

## Problem

The live real-API test (`TestAgentDraftsInvoiceFromNotesLive`) proved the pipeline
works but is unreliable with the configured model: it passed ~1/5. Two failure
modes, both structural (the harness is too passive, not the model's capability):

1. **Stall in prose** — the model ends its turn asking "shall I proceed?" instead
   of calling the gated `create_invoice` write tool. The execute loop just
   commits; nothing checks the planned write actually happened.
2. **Bad numbers** — the model passed `unitPrice: 0`; `create_invoice` trusts it
   (the NDIS validator only enforces `≤ cap`), yielding a silent `$0` invoice.

The fix is a stronger harness, scoped to this workflow.

## Pillar 1 — Server-authoritative pricing for coded lines

The genuine AI judgment is *which NDIS code* and *what quantity*. Price, GST flag
and unit are deterministic from (code + service date + tenant zone), so the
server owns them.

- A new validator mode `fillPrice` (off by default): for a support-item line it
  overwrites `unitPrice` with the resolved zone price cap. A quotable item (nil
  cap) keeps the caller-supplied price; if that is also `≤ 0`, it is a field
  error asking for a price.
- `LineValidator.Validate` stays as-is (fillPrice=false, used by HTTP/UI and
  estimates — humans keep sub-cap freedom). A second entry runs with
  fillPrice=true. `InvoiceService.CreateWithCatalogPricing` wraps Create using
  that mode. Zone is resolved from the tenant business profile, exactly as
  today's validation does — so the cap is correct per zone.
- The `create_invoice` agent tool calls `CreateWithCatalogPricing`. For a coded
  line the model supplies `{code, serviceDate, quantity}`; any `unitPrice` it
  sends is ignored. `gstFree`/`unit` are already snapshotted authoritatively from
  the catalogue by the validator.
- Wiring: `NewCreateInvoiceTool` is unchanged in signature (it already holds the
  `*InvoiceService`); only the method it calls changes.

## Pillar 2 — Stall recovery in `Execute` (nudge → force)

Track the write tool the plan declared (the first risky planned step under the
plan message — normally `create_invoice`). When the loop reaches `end_turn` and
**no tool_use for that write exists anywhere in the conversation history** (⇒ the
model stalled in prose):

1. **Stall 1:** persist a continuation user message — "You planned to create the
   invoice but have not called create_invoice yet. Call it now; the platform will
   gate it for approval. Do not ask for confirmation in prose." — and continue
   the loop under auto tool choice.
2. **Stall 2:** rebuild the next request forcing `tool_choice` = the write tool
   (reuses the existing `ForceTool` field), so the API guarantees the call.
3. Bounded by `maxStalls = 2`; after that, commit with a clear final message.

Detection is precise and avoids false positives:
- A suspended write never reaches `end_turn` (the loop returns at suspend).
- A post-approval resume already has the `create_invoice` tool_use in history.
- An errored `create_invoice` also has a tool_use in history (its `is_error`
  tool_result already drives a normal retry under auto choice — no nudge).

State added to `Execute`: the resolved `pendingWrite` tool name, a `stalls`
counter, and a `nextForce` string consumed by `buildRequest` per iteration.
When `pendingWrite` is empty (no planned write), behaviour is unchanged.

## Pillar 3 — Contract + prompt tightening (small)

- `create_invoice` schema/description: for a catalogue code, supply `code`,
  `serviceDate`, `quantity` and OMIT `unitPrice` (the platform applies the NDIS
  price); custom lines still supply a description + price.
- Reject `quantity ≤ 0` on a coded line with a structured `is_error`.
- Prompt: reinforce "call the write tool; do not stop to ask in prose" (already
  partly done).

## Pillar 4 — Post-draft completeness verification (added after live measurement)

Live measurement of Pillars 1–2 showed reliability rose from ~1/5 to 3/5; the two
residual failures were *line-shape* errors, not pricing: (run 2) the model omitted
all self-care lines; (run 4) it billed self-care as a custom $65 line instead of
the catalogue code. Neither engaged stall recovery (the model did call
create_invoice — just with wrong content).

The verified create_invoice tool (`NewCreateInvoiceToolVerified`, wired on the
notes→invoice path) runs a completeness check BEFORE persisting: it derives the
draft's service-date range from the coded lines, loads the participant's unbilled
notes in that range, and requires — for every note quantity tag (`transportKm`,
`supportHours` > 0) — a catalogue-CODED line on that service date with a matching
quantity. The code itself is not predicted (that stays the model's judgment);
only "a coded line of the right quantity on the right day exists" is enforced,
which is deterministic from the tags.

A gap returns a structured tool error listing each uncovered support, so the
model self-corrects and re-submits (bounded by the loop's MaxIterations). This
single check catches both residual failures: a missing line (no coded line of
that quantity) and a custom-substituted line (the line exists but carries no
code). Pre-persist, so a rejected draft never leaves an orphan invoice. Notes
without tags are not enforced (billed from prose; not deterministically
checkable). The plain `NewCreateInvoiceTool` (notes nil) skips the check, so
non-notes callers and existing tests are unaffected.

## Error handling

All new failures surface as the existing structured tool-error / field-error
shapes (`is_error` tool_result; `*ValidationError`) so the model self-corrects.
No panics; every error checked (NASA rules).

## Testing

- **Deterministic unit tests (real code, no model):**
  - validator `fillPrice`: coded line with `unitPrice:0`/omitted → resolved to
    the zone cap; quotable item with no price → field error; non-coded line
    unchanged.
  - `create_invoice` tool: model price ignored for coded lines; unknown
    code/date → `is_error`; `quantity ≤ 0` → `is_error`. Existing chain test
    still ⇒ `$1905.76`.
- **Harness state-machine tests (scripted `llm.Fake`):** these exercise OUR
  escalation logic, which cannot be triggered deterministically with a real
  model. Cases: stall→nudge→model acts; stall→nudge→stall→force→suspend→approve
  ⇒ invoice created; no-stall happy path unaffected; `maxStalls` bound respected.
- **Real-model acceptance:** the existing live test, expected to pass far more
  reliably. Unchanged assertions (8 lines, `$1905.76`).

## Efficiency pass (post-hardening)

After production hardening, three efficiency/reliability levers from the research:

1. **Prompt caching** (`internal/agent/llm/anthropic.go`): `cache_control: ephemeral`
   breakpoints on the last system block and last tool definition, so the large
   stable prefix (system prompt + tool schemas) is re-read cheaply on turns 2..N
   instead of re-sent at full price. History left uncached. ~60–80% input-token
   cut across a turn; zero behaviour change.
2. **Pre-loaded candidate codes** (`internal/agent/tools_notes.go`):
   `NewListParticipantNotesToolWithCatalog` attaches a small curated `candidates`
   set (code, unit, cap, gstFree) per note, derived from the structured tag
   (transportKm→"transport", supportHours→"self-care"), resolved for the note's
   service date. The model picks from a handful instead of free-form searching —
   shrinking its biggest error source. `search_catalogue` stays registered as a
   fallback; the prompt instructs preferring candidates.
3. **Skip the forced plan turn** (`Config.SkipPlan`, default on in `main.go` via
   `AGENT_SKIP_PLAN`): `Start` enters the execute loop directly, cutting a
   round-trip and restoring thinking on the first turn.

**Trade-off (skip-plan vs stall recovery):** Pillar 2 stall recovery is
*plan-driven* — `pendingWrite` comes from the plan's declared write. With the
plan skipped there is no write signal, and inferring one from the registry would
wrongly force a write on read-only requests. So SkipPlan intentionally trades
away automatic stall recovery (mitigated by candidates + the prompt). To get
both, a future workflow-typed notes→invoice entrypoint could set `pendingWrite`
explicitly without the plan round-trip.

## Out of scope

General agent reliability, re-planning, multi-write tasks, model selection,
estimates pricing, the human UI create path.
