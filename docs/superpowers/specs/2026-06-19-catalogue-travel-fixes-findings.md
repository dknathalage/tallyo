# Catalogue / Travel-Billing Fixes — Findings & Plan

Date: 2026-06-19
Status: Investigated, NOT implemented (deferred by request)
Context: Surfaced after the AI-harness→Smarts teardown (merged to `main` @ `e3e5608`). With the draft-invoice flow no longer crashing, the *real* failure became visible: drafting an invoice from recorded shifts fails because driven km can't be billed.

Live symptom (server log):
```
draft invoice: could not produce a valid invoice after 3 attempts: create_invoice: the draft
does not cover every recorded shift … 2026-06-09: transport 36.00 km; …12 km; …64 km; …38 km
```
The model bills the support **hours** but never the **transport km**, so `verifyShiftsCovered` rejects every attempt → friendly 502, real reason logged.

Neither bug is caused by the Smarts refactor. The refactor *exposed* them (clear errors instead of a JSON crash) and, for Bug A, removed the model's `search_catalogue` escape hatch — so the gather must now hand the model the right code deterministically.

---

## Bug A — km is never billable (code bug, the visible failure)

`internal/agent/tools_shifts.go` `shiftCandidates()` resolves a shift's catalogue candidates:
```go
if sh.Km > 0 { add("transport") }      // ← wrong term
if sh.Hours > 0 { add("self-care"); … }
```
`SearchSupportItems` does `code/name LIKE %term%` ordered by code, limit 3. For `"transport"` the real 2025-26 catalogue returns:
- `02_050_0108_1_1` Specialised Transport To School — unit **D** (per day)
- `02_051_0108_1_1` Transport — unit **YR** (annual transport allowance)
- `04_590_0125_6_1` Activity Based Transport — unit **E**

None is per-kilometre, and the actual per-km item — **`01_799 Provider travel - non-labour costs`** (name = "travel", not "transport") — is never surfaced. So the model has no correct code, can't search (single-shot Smart), and omits/custom-lines the km. Hours work because `"self-care"` correctly surfaces `Assistance With Self-Care Activities` (unit H), matching the shift's hours.

**NDIS rule (2025-26, authoritative):** a worker driving to/with a participant claims vehicle running costs **per km** via **Provider travel – non-labour costs** (`01_799_*`), reasonable rate ~$0.99/km standard. This is distinct from *Activity Based Transport* (transporting the **participant** during an activity) and *`02_051` Transport* (the participant's own transport budget).
Sources: NDIS Pricing Arrangements & Price Limits 2025-26; NDIS "Travel claiming rules".

**Shift model gap:** `shift.Shift` has a single scalar `Km` with no field saying *whose* travel it is. `Measures[].Code` exists but the gather ignores it. So provider-drive vs participant-transport is not encoded — the common case (worker drove X km) maps to `01_799`.

### Chosen fix (Bug A) — surface BOTH travel families
In `shiftCandidates`, replace the km branch:
```go
if sh.Km > 0 {
    add("provider travel")          // 01_799 family — per-km, DEFAULT for worker-driven km
    add("activity based transport") // participant transport during an activity
}
```
(`%provider travel%` matches `Provider travel - non-labour costs`; `%activity based transport%` matches the ABT family. Each limit 3, deduped — gives the model the right, name-distinguished options.)

Also update `draftInvoiceSystem` (in `smart_draft_invoice.go`) to instruct: bill each shift's km as a per-km **Provider travel** coded line (default), using the provided travel candidate; use Activity Based Transport only when the note indicates the participant was transported.

Ship with a test (extend the deterministic shift-fixture or a `SearchForDate` stub) asserting a km>0 shift surfaces a `01_799` candidate and that a draft covering km succeeds.

**Open nuance (flag, not blocking):** `01_799` has many per-category variants (`_0102/_0104/_0106/…`); any has the same per-km cap, so price is fine, but category-correct variant selection is a refinement for later.

---

## Bug B — no `national` catalogue prices (stale data; code already correct)

`support_item_prices` in the live DB has rows for **only** `remote` (620) and `very_remote` (620) — **zero `national`**. Tenant 1's `business_profile.zone = 'national'` (the default). So `ResolveZonePrice(code,'national')` → nil for every code → `applyZonePrice` (`internal/billing/validation.go:288`) fails: *"no price is published for code … in zone national"*. This blocks ALL catalogue-priced invoicing for a national-zone tenant — not just AI, not just travel. (We never reached it in the live run because Bug A failed first at coverage.)

**Root cause: stale migration, not a code bug.**
- The committed `internal/db/migrations/00006_catalogue_2025_26.sql` is **correct**: 409 `national` rows incl. `01_799_0102_1_1 → 'national', 1`. The `e30b3f6` fix ("capture national price from per-state columns — no National column in NDIS sheet") added the per-state→national fallback in `catalog/service.go` (`nationalPriceColumns`, reads ACT…WA since the XLSX has no National column) AND regenerated `00006`.
- The **live DB applied goose version 6 at 2026-06-19 03:09:16 UTC** — ~19 min *before* even `077f01d` (the pre-fix migration) was committed at 03:24, and before `e30b3f6` at 03:28. goose tracks applied migrations by **version number only**, so it will never re-run `00006`, and the corrected file on disk never loads into this DB.
- `git show 077f01d:…/00006_*.sql` → 0 national rows (only remote/very_remote). The live DB ran a pre-fix working copy.

**Why only 409/620 have national:** 211 items are genuinely quote-only / no published flat price; they're not in the draft-from-shifts path (self-care + provider-travel both have national). Non-blocking.

### Chosen fix (Bug B) — recreate the dev DB
The corrected `00006` already produces national prices, so a fresh DB is correct:
```bash
# stop the server first, then:
rm "$HOME/Library/Application Support/Tallyo/tallyo-go.db"*   # removes db + -shm + -wal
go run . --port 8080   # migrations rebuild from scratch; corrected 00006 seeds national prices
```
Cost: wipes dev data (participants, the 4 shifts). Re-create the participant + re-import shifts (the import Smart) to retest the draft.

**Deployed-DB follow-up (out of scope here, recommended later):** an idempotent `00007` re-seed migration that inserts the missing `national` rows for the existing 2025-26 version (`INSERT … ON CONFLICT DO NOTHING`) — goose runs it on already-deployed DBs while staying a no-op on fresh installs. Editing `00006` in place will NOT repair an already-migrated DB.

**Do NOT add a `ResolveZonePrice` national→remote fallback** — remote caps are loading-adjusted (higher); falling back would over-price. The data is the fix.

---

## Residual decisions / notes
- **Travel cap value:** seed stores `price_cap = 1` for `01_799` (vs the ~$0.99/km figure in guidance). Verify the intended cap against the XLSX before relying on it for real billing.
- **Tenant-zone threading (deferred nice-to-have):** `shiftCandidates`/`gatherShiftContext` pass `zone=""` → defaults to `national`. After the re-seed this resolves correctly for a national tenant, but for a *remote* tenant the surfaced `priceCap` hint won't match apply-time pricing. Threading the real tenant zone into candidate resolution would make the hint accurate. Apply-time pricing is already correct (uses `tenantZone`).
- **Shift travel-type:** if both provider-drive and participant-transport km must be billable distinctly, consider a travel-type/code field on the shift; for now default km → `01_799` and surface ABT as a secondary candidate.

## Sequencing when picked up
1. New branch off `main` (e.g. `feat/catalogue-travel-fix`).
2. Bug A code fix + test (gather surfaces `01_799` and ABT; system prompt guides km→provider-travel).
3. Recreate dev DB (Bug B) and re-test the draft end-to-end (km billed, priced, draft opens).
4. (Optional, deployed) idempotent `00007` national re-seed migration.
