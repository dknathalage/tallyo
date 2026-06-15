# Tallyo → NDIS-only, Multi-tenant — Design Spec

**Date:** 2026-06-16
**Status:** Approved (brainstorming)
**Author:** Don Athalage (with Claude)

## 1. Purpose & Positioning

Pivot Tallyo from a generic invoice manager into an **NDIS-only, multi-tenant SaaS** for Australian disability-support providers.

- **Target:** sole-trader → small NDIS providers.
- **Billing workflow:** **direct invoicing to plan-managers / self-managed participants** only. No PRODA/myplace bulk-claim, no agency-managed channel in this scope. (Bulk-claim is a deliberate future scope; see Non-Goals.)
- **Core value:** NDIS-compliant invoices — correct support-item codes, price-guide caps enforced, plan-window validation, GST-free defaults — so claims aren't auto-rejected.
- **Hosting:** SaaS multi-tenant, single Go binary + SQLite.

### Why these choices (from research)
- The NDIS price guide (Support Catalogue) is national law; the applicable price cap is keyed to the **service-delivery date**, not the invoice date. Stale caps auto-reject.
- Price caps are **geographic** (national / remote / very-remote).
- Most NDIS money flows via plan-managed (66%) and self-managed (27%) participants, who are invoiced directly — not through the agency bulk-claim portal.

## 2. Non-Goals (explicit out-of-scope for this spec)

- PRODA/myplace **bulk payment request** CSV generation and agency-managed claiming.
- Plan **budget / fund-balance tracking** by support category.
- Migration of any existing data (pre-launch; fresh schema).
- Generic catalog-import wizard with column mapping (being removed).
- Custom per-tenant pricing tiers (being removed).

## 3. Architecture Overview

Unchanged layering: handlers → services → repositories → sqlc gen. Every DB mutation audited via `audit.WithTx` and broadcasts an SSE event after commit. Two new cross-cutting concerns: **tenant scoping** and **NDIS validation**.

### 3.1 Tenancy model

- **Shared SQLite DB + `tenant_id` column** on every tenant-owned table (logical isolation).
- **Enforcement choke point:** the repository layer. Session carries `tenant_id`; auth middleware loads it into request context; **every repository method takes a `tenantID` argument and every query filters `WHERE tenant_id = ?`**. Handlers/services never build a query without passing tenant scope.
- **Global (NOT tenant-scoped) tables:** `catalog_versions`, `support_items`, `support_item_prices` — the NDIS Support Catalogue is shared national reference data.
- **Platform admin:** `users.is_platform_admin` flag. Platform admins manage the global catalogue and may operate cross-tenant **only** in the catalog-admin area. All other access is tenant-scoped, including for platform admins.

### 3.2 Onboarding

- Remove the existing single-org first-run setup flow.
- Add public **`/signup`**: a single transaction creates `tenant` + owner `user` + `business_profile` (including geographic **zone**), then logs the user in.
- **Invites** become tenant-scoped: an owner/admin invites users into *their* tenant. Reuse existing invite plumbing + `tenant_id`.
- **Tenant roles:** `owner | admin | member`. `owner` (created at signup) + `admin` may invite/manage users and edit business settings; `member` may manage participants/invoices/estimates/payments but not users or settings. `is_platform_admin` is orthogonal (platform operator, not a tenant role).
- **Tenant status enforcement:** `tenants.status = 'suspended'` blocks login for that tenant's users and is skipped by the per-tenant sweeps (§8). `'active'` is normal.

## 4. Data Model (fresh goose baseline)

> Pre-launch: this replaces the existing schema with a fresh NDIS-native baseline. All `id INTEGER PRIMARY KEY AUTOINCREMENT`, `uuid TEXT UNIQUE`, `created_at`/`updated_at TEXT` unless noted.

### 4.1 Tenancy / auth

```
tenants       id, uuid, name, status ('active'|'suspended'), created_at, updated_at
users         id, uuid, tenant_id FK→tenants, email, password_hash, name,
              is_platform_admin INT DEFAULT 0, created_at, updated_at
              UNIQUE(tenant_id, email)
sessions      (scs-managed; session data carries tenant_id + user_id)
invites       id, uuid, tenant_id FK, email, token, role, expires_at, accepted_at, created_at
```

### 4.2 Tenant-owned business data (all carry `tenant_id FK→tenants`)

```
business_profile  tenant_id (1:1), name, abn, address, email, phone,
                  zone ('national'|'remote'|'very_remote') DEFAULT 'national',
                  logo, ... existing fields
plan_managers     (was payers) tenant_id, name, email, phone, address, metadata
participants      (was clients) tenant_id, name, ndis_number,
                  plan_start DATE, plan_end DATE, mgmt_type ('plan'|'self'),
                  plan_manager_id FK→plan_managers NULL (null when self-managed),
                  email, phone, address, metadata
custom_items      tenant_id, name, rate REAL, unit, gst_free INT
tax_rates         tenant_id, name, rate REAL, is_default INT
invoices          tenant_id, number, participant_id FK, plan_manager_id FK (bill-to snapshot),
                  status, issue_date, due_date, subtotal, tax, total, notes
                  UNIQUE(tenant_id, number)
line_items        tenant_id, invoice_id FK, support_item_id FK NULL, custom_item_id FK NULL,
                  code (snapshot), description (snapshot), service_date DATE,
                  catalog_version_id (pinned), unit, quantity REAL, unit_price REAL,
                  gst_free INT, line_total REAL
estimates         tenant_id, ... (parallel to invoices)
estimate_line_items  tenant_id, ... (parallel to line_items)
payments          tenant_id, invoice_id FK, amount, paid_at, method, reference
recurring_templates  tenant_id, ... (existing shape) + NDIS-aware line template
audit_log         tenant_id, user_id, entity, entity_id, action, changes, created_at
```

### 4.3 Global NDIS catalogue (NO `tenant_id`)

```
catalog_versions    id, uuid, label ('2025-26 v1.1'), effective_from DATE,
                    effective_to DATE NULL, source_filename, created_at
support_items       id, uuid, catalog_version_id FK, code, name, unit,
                    support_category ('Core'|'CB'|'Capital'), registration_group,
                    claim_type, gst_free INT, metadata
                    UNIQUE(catalog_version_id, code)
support_item_prices id, support_item_id FK, zone ('national'|'remote'|'very_remote'),
                    price_cap REAL NULL,  -- NULL = quotable item (no fixed cap)
                    UNIQUE(support_item_id, zone)
```

Some NDIS items are **quotable** ("Price Limit: Quote") with no fixed cap — `price_cap` is NULL for these; the validation engine skips the over-cap assertion (§6 step 4) when the cap is NULL.

Catalog versions are immutable history. The version whose `[effective_from, effective_to|∞]` contains a given service date is authoritative for that date.

## 5. NDIS Catalogue Ingest (platform-admin)

- Platform admin uploads the official **NDIS Support Catalogue XLSX**.
- **Fixed-format parser** keyed to known NDIA column headers (no column-mapping wizard). Reuses `excelize` + the `importer` package plumbing.
- Creates a new `catalog_version` (label + `effective_from` from the form), then bulk-upserts `support_items` + `support_item_prices` (one price row per zone).
- Audited; broadcasts an SSE event. Parsing is validated: reject the upload if required columns are missing or rows fail to parse (no partial-version state — wrap in one transaction).

## 6. Invoice Line Validation Engine (service layer — the core differentiator)

On line-item create/update, for a **support-item** line, in order:

1. Resolve `catalog_version` where `effective_from ≤ service_date ≤ effective_to|∞`. (Error if none.)
2. Find `support_item` by `code` within that version; snapshot `code`/`description`; pin `catalog_version_id`.
3. Look up `price_cap` for the **tenant's configured zone** (from `business_profile.zone`). A single tenant-wide zone is **intentional** for this scope; per-line/per-participant zone is deferred (the per-zone price rows in §5 already support it later).
4. **Assert `unit_price ≤ price_cap`** — block over-cap (the #1 rejection cause). **Skip this assertion when `price_cap` is NULL** (quotable item).
5. **Assert `service_date ∈ [participant.plan_start, participant.plan_end]`** — block out-of-plan.
6. Default `gst_free` from the support item.

For a **custom-item** line: skip steps 1–5; still validate quantity/price ≥ 0 and recompute totals.

> **Money type (2026-06-16 decision):** money stays `REAL` (float), per user decision. Mitigation: line totals, subtotal, tax, and total MUST be rounded defensively to the cent at every computation boundary to limit cumulative float drift in NDIS reconciliation. Revisit if reconciliation errors surface.

Per NASA rule 5: ≥2 assertions per non-trivial function; validate at the service boundary. Validation errors return structured, field-level messages to the UI.

## 7. Strip Plan (remove generic features)

- **rate_tiers, fully:** drop `rate_tiers` + `catalog_item_rates` tables and `clients.pricing_tier_id`; delete `service/rate_tier.go`, `repository/rate_tier.go`, `http/rate_tiers.go`, `queries/rate_tiers.sql`, `queries/catalog_item_rates.sql`, web `/rate-tiers` route, and their tests.
- **Generic catalog-import wizard:** delete `web/src/routes/catalog/import`, `importer/detection.go` (column auto-detect), and the mapping portions of `http/import.go`. Replace with the §5 fixed platform-admin upload.

## 8. Cross-cutting Multi-tenant Impacts

- **Numbering** (`internal/numbering`): document-number sequences are **per tenant**; uniqueness `(tenant_id, number)`; tx-scoped allocation keyed by tenant.
- **Realtime** (`internal/realtime`): SSE hub keyed by `tenant_id`; `/api/events` only delivers events for the subscriber's tenant.
- **Audit** (`internal/audit`): every entry stamped with `tenant_id` + `user_id`.
- **Sweeps** (`main.go` hourly ticker for overdue + recurring): iterate per tenant.
- **Structured logging** (§8.1): replace all stdlib `log.Printf`/`log.Fatal` with `log/slog`.

### 8.1 Structured logging

Replace the current scattered stdlib `log.Printf` calls (`main.go`, `http/middleware.go`, `http/respond.go`, `http/auth.go`, `http/invoices.go`) with **`log/slog`** (Go 1.26 stdlib, no new dependency).

- **Setup (`main.go`):** one root `slog.Logger` with a **JSON handler** in production, **text handler** for dev; level configurable via `--log-level` flag / `LOG_LEVEL` env (default `info`). Set as `slog.SetDefault`.
- **Request-scoped logger:** `RequestLogger` middleware generates a `request_id` (e.g. UUID), builds a child logger with `request_id`, and stores it in the request context. After auth resolves the session, enrich the context logger with `tenant_id` and `user_id`. Handlers/services retrieve the logger from context via a helper (`httpapi.LoggerFrom(ctx)`), falling back to default.
- **Standard fields:** request logs carry `method`, `path`, `status`, `duration_ms`, `request_id`, and (when authenticated) `tenant_id`, `user_id`. Panics (recover middleware) log at `error` with the stack.
- **Leveling:** `error` for unexpected failures, `warn` for recoverable/expected-but-notable (e.g. failed login), `info` for lifecycle (startup, shutdown, sweep summaries), `debug` for verbose tracing. No secrets/PII (no passwords, tokens, NDIS numbers) in logs.
- **Multi-tenant payoff:** every log line is filterable by `tenant_id` — essential for SaaS support/debugging.

## 9. Frontend Changes

- Route renames: `clients`→`participants`, `payers`→`plan-managers`, `catalog`→`support-catalog`.
- Remove `rate-tiers` route and `catalog/import` wizard.
- **Support-catalog view:** browse versions + items (read-only for tenants); platform-admin sees an upload control.
- **Signup page** (`/signup`) capturing business name + zone; remove first-run setup screens.
- **Invoice editor:** line picker searches support items by code/name, shows the applicable cap, adds a `service_date` field per line, GST-free toggle; surfaces validation errors inline.
- **Settings:** editable `zone`.

## 10. Testing Strategy

- Go stdlib tests per service/repo. **Heaviest coverage on the validation engine:** over-cap rejection, out-of-plan rejection, version resolution by service date (boundary dates), zone-based cap selection, custom-item path.
- **Cross-tenant isolation tests** on every repo: tenant A cannot read or mutate tenant B's rows; queries scoped correctly.
- Signup transaction test (tenant + owner + profile atomic).
- Per-tenant numbering concurrency test (`-race`).
- SSE tenant-scoping test (events don't leak across tenants).
- Catalogue parser test against a sample NDIA XLSX fixture (incl. missing-column rejection).
- Structured-logging test: request logger attaches `request_id`/`tenant_id`/`user_id`; assert no secrets/PII fields emitted (capture with a `slog` test handler).
- `svelte-check` 0 errors / 0 warnings.

## 11. Compliance / Legal Note

A direct-invoicing tool (no bulk-claim submission) is unlikely to be classed as an "NDIS digital platform provider" under the 1 July 2026 mandatory-registration rule. **Revisit this legal question before adding any bulk-claim feature** (deferred).

## 12. Open Items (deferred, not blocking)

- Free-to-paid pricing/packaging of the SaaS itself.
- Email verification + signup abuse guard.
- Operator-provisioned tenants (only self-serve in this scope).
- Bulk-claim / agency-managed channel.
- Plan budget tracking.
