# SaaS Subscriptions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Charge tenants a recurring fee via Stripe Billing — card-required, Stripe-driven 90-day trial, webhooks as source of truth, lapse → read-only, all behind a `BILLING_ENABLED` feature gate.

**Architecture:** Subscription state lives as columns on the existing control-DB `tenants` table (1 tenant = 1 sub). A pure `entitled(status)` function is the whole gate; `ResolveTenant` stashes the entitlement bool in request context (no extra DB read), and a new `RequireSubscription` middleware returns 402 on write methods when not entitled. Stripe owns Checkout, the Customer Portal, the trial clock, and dunning; a tenant-agnostic, signature-verified webhook syncs status back. Stripe secrets flow through the existing GSM `secrets` module into Cloud Run.

**Tech Stack:** Go 1.x, chi router, sqlc + goose (Postgres control plane), `github.com/stripe/stripe-go/v8x`, SvelteKit frontend, OpenTofu + Terragrunt infra.

**Spec:** `docs/superpowers/specs/2026-06-27-saas-subscriptions-design.md`

**Module path:** `github.com/dknathalage/tallyo`

> **Test prerequisite:** DB-backed tests (`internal/db`, `internal/auth`, `internal/subscription` store/handler, `internal/app` routes) call `appdb.OpenTestDB(t)` (`internal/db/testdb.go`), which `t.Skip`s when `TEST_DATABASE_URL` is unset. **A SKIP is not a PASS** — export `TEST_DATABASE_URL` (Postgres) before claiming any DB task green. The auth tests' `mustTenantDB(t)` wrapper (`internal/auth/tenants_test.go:13`) is the harness to copy. Note: a tenant's public "uuid" IS `tenants.id` — there is no separate uuid column.

---

## File Structure

New Go package `internal/subscription` owns billing logic (kept out of `internal/billing`, which is invoice math):
- `internal/subscription/entitlement.go` — pure `Entitled(status)` + status constants
- `internal/subscription/config.go` — `Config` loaded from env
- `internal/subscription/store.go` — repo over the new `tenants` columns
- `internal/subscription/stripe.go` — thin `stripe-go` wrapper (checkout/portal/webhook parse)
- `internal/subscription/handler.go` — HTTP handlers + `Routes`
- `internal/subscription/webhook.go` — webhook event dispatch (idempotent, self-healing)
- plus `_test.go` siblings

Modified:
- `internal/db/migrations/control/00003_subscriptions.sql` (new)
- `internal/db/queries/tenants.sql`, `internal/db/gen/*` (regenerated)
- `internal/auth/tenants.go` (Tenant struct + mappers)
- `internal/reqctx/*.go` (entitled bool)
- `internal/httpx/middleware.go` (ResolveTenant fold-in + RequireSubscription)
- `internal/app/server.go`, `internal/app/app.go` (wiring)
- `web/src/routes/[tenant]/settings/billing/+page.svelte` (+ layout banner, api 402)
- `infra/modules/secrets/*`, `infra/modules/cloud-run/*`, `infra/live/_envcommon/*`

---

## Task 1: DB migration + sqlc plumbing for subscription columns

**Files:**
- Create: `internal/db/migrations/control/00003_subscriptions.sql`
- Modify: `internal/db/queries/tenants.sql`
- Modify: `internal/auth/tenants.go` (Tenant struct + mappers)
- Regen: `internal/db/gen/*`

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up
ALTER TABLE tenants ADD COLUMN stripe_customer_id     TEXT;
ALTER TABLE tenants ADD COLUMN stripe_subscription_id TEXT;
ALTER TABLE tenants ADD COLUMN subscription_status    TEXT NOT NULL DEFAULT 'none';
ALTER TABLE tenants ADD COLUMN trial_end              TEXT;
ALTER TABLE tenants ADD COLUMN current_period_end     TEXT;
ALTER TABLE tenants ADD COLUMN subscription_synced_at TEXT; -- last webhook event ts, for idempotency
CREATE INDEX idx_tenants_stripe_customer ON tenants (stripe_customer_id);

-- +goose Down
DROP INDEX idx_tenants_stripe_customer;
ALTER TABLE tenants DROP COLUMN subscription_synced_at;
ALTER TABLE tenants DROP COLUMN current_period_end;
ALTER TABLE tenants DROP COLUMN trial_end;
ALTER TABLE tenants DROP COLUMN subscription_status;
ALTER TABLE tenants DROP COLUMN stripe_subscription_id;
ALTER TABLE tenants DROP COLUMN stripe_customer_id;
```

- [ ] **Step 2: Add queries** to `internal/db/queries/tenants.sql` (existing `GetTenant`/`GetTenantByUUID` use `SELECT *` so the new columns come along for free — verify after regen). Add:

```sql
-- name: GetTenantByStripeCustomer :one
SELECT * FROM tenants WHERE stripe_customer_id = $1;

-- name: UpdateTenantSubscription :exec
UPDATE tenants SET
    stripe_customer_id     = $1,
    stripe_subscription_id = $2,
    subscription_status    = $3,
    trial_end              = $4,
    current_period_end     = $5,
    subscription_synced_at = $6,
    updated_at             = $7
WHERE id = $8;
```

- [ ] **Step 3: Regenerate sqlc**

Run: `task sqlc`
Expected: `internal/db/gen` `Tenant` struct gains the 6 new fields (nullable ones as `sql.NullString`); new `GetTenantByStripeCustomer` + `UpdateTenantSubscription` methods exist. No diff churn elsewhere.

- [ ] **Step 4: Extend `auth.Tenant`** in `internal/auth/tenants.go` — add `SubscriptionStatus`, `TrialEnd`, `CurrentPeriodEnd`, `StripeCustomerID`, `StripeSubscriptionID` fields and map them in `GetByUUID` (and any other `gen.Tenant → Tenant` mapper). Keep JSON tags camelCase to match existing.

- [ ] **Step 5: Build + existing tests pass**

Run: `task test` (or `go build ./... && go test ./internal/auth/... ./internal/db/...`)
Expected: PASS.

- [ ] **Step 6: Commit** — `feat(db): add subscription columns to tenants`

---

## Task 2: Entitlement pure function (TDD)

**Files:**
- Create: `internal/subscription/entitlement.go`
- Test: `internal/subscription/entitlement_test.go`

- [ ] **Step 1: Failing test**

```go
package subscription

import "testing"

func TestEntitled(t *testing.T) {
	cases := map[string]bool{
		"active": true, "trialing": true, "past_due": true,
		"none": false, "canceled": false, "": false, "bogus": false,
	}
	for status, want := range cases {
		if got := Entitled(status); got != want {
			t.Errorf("Entitled(%q) = %v, want %v", status, got, want)
		}
	}
}
```

- [ ] **Step 2: Run, verify fail** — `go test ./internal/subscription/ -run TestEntitled` → FAIL (undefined Entitled).

- [ ] **Step 3: Implement**

```go
// Package subscription holds SaaS billing: entitlement, Stripe integration, and
// the webhook that syncs Stripe state back onto the control-DB tenants table.
package subscription

// Subscription status values mirrored from Stripe (plus local "none").
const (
	StatusNone     = "none"     // signed up, Checkout never completed
	StatusTrialing = "trialing"
	StatusActive   = "active"
	StatusPastDue  = "past_due" // dunning — grace, still entitled
	StatusCanceled = "canceled"
)

// Entitled reports whether a tenant in the given subscription status may perform
// write actions. trialing/active/past_due are entitled; none/canceled are not.
// Stripe owns the trial clock, so there is no time math here.
func Entitled(status string) bool {
	switch status {
	case StatusActive, StatusTrialing, StatusPastDue:
		return true
	default:
		return false
	}
}
```

- [ ] **Step 4: Run, verify pass.**
- [ ] **Step 5: Commit** — `feat(subscription): entitlement function`

---

## Task 3: Config from env

**Files:**
- Create: `internal/subscription/config.go`
- Test: `internal/subscription/config_test.go`

- [ ] **Step 1: Failing test** — `LoadConfig` reads env with defaults: `Enabled=false`, `TrialDays=90` when unset; `TrialDays` parses an int; `Enabled` true when `BILLING_ENABLED=true`.

```go
func TestLoadConfigDefaults(t *testing.T) {
	c := LoadConfig()
	if c.Enabled { t.Error("default Enabled should be false") }
	if c.TrialDays != 90 { t.Errorf("default TrialDays = %d, want 90", c.TrialDays) }
}
func TestLoadConfigTrialDays(t *testing.T) {
	t.Setenv("TRIAL_DAYS", "14")
	if c := LoadConfig(); c.TrialDays != 14 { t.Errorf("TrialDays = %d, want 14", c.TrialDays) }
}
```

- [ ] **Step 2: Run, verify fail.**
- [ ] **Step 3: Implement** — `Config{Enabled bool; SecretKey, WebhookSecret, PriceID string; TrialDays int}`. Use `app.EnvOr`/`app.EnvBool`? Note: importing `internal/app` from `internal/subscription` would create a cycle (app imports subscription). So duplicate the tiny env read here with `os.Getenv` + `strconv.Atoi` (default 90 on parse error). `// ponytail: local env reads to avoid app→subscription import cycle`.

- [ ] **Step 4: Run, verify pass.**
- [ ] **Step 5: Commit** — `feat(subscription): env config`

---

## Task 4: Entitlement in request context + ResolveTenant fold-in

**Files:**
- Modify: `internal/reqctx/` (add `WithEntitled`/`EntitledFrom`)
- Modify: `internal/httpx/middleware.go` (`ResolveTenant` signature + stash)
- Test: `internal/reqctx/*_test.go`, update `internal/httpx/resolvetenant_test.go`

- [ ] **Step 1: Failing test for reqctx** — round-trip `WithEntitled(ctx,true)` → `EntitledFrom` returns `(true,true)`; bare ctx → `(false,false)`.

- [ ] **Step 2: Implement reqctx** — add `entitledKey` to the const block, `WithEntitled`/`EntitledFrom` mirroring the existing `WithUser`/`UserFrom` pattern.

- [ ] **Step 3: Fold into ResolveTenant** — `ResolveTenant` already has the loaded `tenant`. Add a `billingEnabled bool` parameter (passed from wiring). After resolving the tenant:

```go
entitled := !billingEnabled || subscription.Entitled(tenant.SubscriptionStatus)
ctx = reqctx.WithEntitled(ctx, entitled)
```

(`internal/httpx` importing `internal/subscription` is fine — no cycle.) Update the existing `ResolveTenant` callers/tests for the new param.

- [ ] **Step 4: Run** `go test ./internal/reqctx/... ./internal/httpx/...` → PASS.
- [ ] **Step 5: Commit** — `feat(httpx): stash entitlement in ResolveTenant`

---

## Task 5: RequireSubscription middleware (TDD)

**Files:**
- Modify: `internal/httpx/middleware.go`
- Test: `internal/httpx/middleware_test.go`

- [ ] **Step 1: Failing tests**
  - entitled ctx + POST → next runs (200)
  - not-entitled ctx + GET → next runs (200)
  - not-entitled ctx + POST/PUT/PATCH/DELETE → 402
  - missing entitled key (gate disabled path never set it) → treat as entitled (next runs)

- [ ] **Step 2: Run, verify fail.**

- [ ] **Step 3: Implement**

```go
// RequireSubscription blocks write methods for tenants without an entitled
// subscription, returning 402. Reads always pass. Chain AFTER ResolveTenant
// (which sets the entitled flag). Mount only on the non-billing route group so a
// lapsed tenant can still reach Checkout/Portal.
func RequireSubscription(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entitled, ok := reqctx.EntitledFrom(r.Context())
		if ok && !entitled {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				WriteError(w, http.StatusPaymentRequired, "subscription required")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 4: Run, verify pass.**
- [ ] **Step 5: Commit** — `feat(httpx): RequireSubscription middleware`

---

## Task 6: Subscription store (TDD)

**Files:**
- Create: `internal/subscription/store.go`
- Test: `internal/subscription/store_test.go` (use `appdb.OpenTestDB(t)` — copy the `mustTenantDB` harness from `internal/auth/tenants_test.go:13`; needs `TEST_DATABASE_URL`)

- [ ] **Step 1: Failing test** — `Store.Apply(ctx, update)` writes the 6 fields; `GetByStripeCustomer` round-trips; a second `Apply` with an OLDER `SyncedAt` is a no-op (idempotency/out-of-order guard).

- [ ] **Step 2: Run, verify fail.**

- [ ] **Step 3: Implement** — `Store{db *sql.DB}` wrapping `gen`. `Apply` reads current `subscription_synced_at`; if the incoming event timestamp is not newer, return nil without writing. Otherwise call `UpdateTenantSubscription`. Provide `GetByStripeCustomer(ctx, custID) (tenantID string, found bool, err error)`.

- [ ] **Step 4: Run, verify pass.**
- [ ] **Step 5: Commit** — `feat(subscription): control-DB store with idempotent apply`

---

## Task 7: Stripe wrapper

**Files:**
- Create: `internal/subscription/stripe.go`
- Modify: `go.mod` (add `github.com/stripe/stripe-go/v8x`)
- Test: `internal/subscription/stripe_test.go` (only the pure bits — event parsing/signature handled in Task 9)

- [ ] **Step 1: Add dep** — `go get github.com/stripe/stripe-go/v82` (or current major). Run `go mod tidy`.

- [ ] **Step 2: Implement** a `Client` wrapping the SDK with:
  - `CreateCheckoutSession(ctx, in CheckoutInput) (url string, err error)` — `mode=subscription`, `LineItems` = configured price qty 1, `SubscriptionData.TrialPeriodDays = TrialDays`, `ClientReferenceID = tenantID`, `Metadata{"tenant_id": tenantID}`, success/cancel URLs.
  - `CreatePortalSession(ctx, customerID, returnURL) (url string, err error)`
  - `GetSubscription(ctx, subID) (*stripe.Subscription, error)` — for webhook self-heal.

- [ ] **Step 3: Build** — `go build ./internal/subscription/` → PASS.
- [ ] **Step 4: Commit** — `feat(subscription): stripe-go client wrapper`

---

## Task 8: Billing HTTP handlers + routes

**Files:**
- Create: `internal/subscription/handler.go`
- Test: `internal/subscription/handler_test.go`

- [ ] **Step 1: Failing tests** — checkout/portal require `owner` role (member → 403, asserted via the route wiring in Task 11 or a handler-level check); `GET billing` returns `{status, trialEnd, entitled}` from the resolved tenant; with `Enabled=false` handlers are not mounted (404).

- [ ] **Step 2: Implement `Handler`** with `Checkout`, `Portal`, `Status` methods and a `Routes(r chi.Router)` that registers:
  - `r.With(httpx.RequireRole("owner")).Post("/billing/checkout", h.Checkout)`
  - `r.With(httpx.RequireRole("owner")).Get("/billing/portal", h.Portal)`
  - `r.Get("/billing", h.Status)`

  `Checkout` reads `reqctx.MustTenant`, calls `Client.CreateCheckoutSession`, returns `{url}`. `Portal` looks up `stripe_customer_id` (402/409 if none yet), creates a portal session. `Status` reads the tenant fields + `subscription.Entitled`.

- [ ] **Step 3: Run tests, verify pass.**
- [ ] **Step 4: Commit** — `feat(subscription): billing handlers`

---

## Task 9: Webhook handler (TDD — the risky one)

**Files:**
- Create: `internal/subscription/webhook.go`
- Test: `internal/subscription/webhook_test.go`

- [ ] **Step 1: Failing tests** (use `webhook.GenerateTestSignedPayload` from stripe-go to forge valid signatures, and a fake/stub Store):
  - invalid signature → 400, no store write
  - `checkout.session.completed` → links customer+sub, sets status from payload (resolve tenant via `client_reference_id`)
  - `customer.subscription.updated` → syncs status/trial_end/current_period_end (resolve via `GetByStripeCustomer`)
  - `customer.subscription.deleted` → status `canceled`
  - duplicate delivery of same event → second is a no-op (store idempotency)
  - stale (older timestamp) event after a newer one → no clobber
  - `subscription.updated` for an unknown customer → self-heal: calls `Client.GetSubscription`, reads `metadata.tenant_id`, links; if still unresolvable → 200 + log, no write
  - unhandled event type → 200, no write

- [ ] **Step 2: Run, verify fail.**

- [ ] **Step 3: Implement `Webhook(w, r)`** — read raw body (`io.ReadAll`), `webhook.ConstructEvent(body, sig, webhookSecret)`; on error 400. Switch on `event.Type`; build an `update` with the Stripe `subscription` timestamp as `SyncedAt`; resolve tenant id (client_reference_id for checkout, customer lookup + self-heal for subscription events); call `Store.Apply`. Always return 200 for handled/ignored/unknown-but-acked events (never 500 into Stripe's retry storm; 500 only on genuine internal DB error so Stripe retries).

- [ ] **Step 4: Run all subscription tests, verify pass.**
- [ ] **Step 5: Commit** — `feat(subscription): idempotent self-healing webhook`

---

## Task 10: Signup default status

**Files:**
- Modify: `internal/app/auth_handlers.go` / `internal/auth/tenants.go` (only if needed)

- [ ] **Step 1:** The `00003` migration defaults `subscription_status='none'`, so `TenantsRepo.Create`/`Signup` already produce `none` with no code change. Add/confirm a test asserting a freshly signed-up tenant has `SubscriptionStatus == "none"`.
- [ ] **Step 2:** Run the signup test → PASS.
- [ ] **Step 3: Commit** (if any change) — `test(auth): signup tenant starts with status none`

---

## Task 11: Wire routes + app deps

**Files:**
- Modify: `internal/app/server.go`
- Modify: `internal/app/app.go`
- Test: `internal/app/` route tests (extend existing patterns)

- [ ] **Step 1:** In `app.go`, load `subscription.LoadConfig()`; if `cfg.Enabled`, build `subscription.NewClient(cfg)`, `subscription.NewStore(database)`, `subscription.NewHandler(...)`; add `Subscription *subscription.Handler` and `BillingEnabled bool` to `Deps`. Pass `BillingEnabled` into `httpx.ResolveTenant(deps.Users, deps.Tenants, deps.BillingEnabled)`.

- [ ] **Step 2:** In `server.go`:
  - Mount the webhook OUTSIDE auth, at the top of the `/api` tree (before RequireAuth groups), raw-body safe: `if deps.Subscription != nil { api.Post("/stripe/webhook", deps.Subscription.Webhook) }`.
  - Inside `api.Route("/t/{tenantUUID}", ...)` after `ResolveTenant`: register billing routes directly on `pr` (NO RequireSubscription) — `if deps.Subscription != nil { deps.Subscription.Routes(pr) }`.
  - Wrap ALL the other tenant-scoped handlers in a sub-group that adds the gate:
    ```go
    pr.Group(func(g chi.Router) {
        if deps.BillingEnabled { g.Use(httpx.RequireSubscription) }
        // move BusinessProfile, Payers, TaxRates, Clients, Catalogue, Invoices,
        // Sessions, Estimates, Payments, Smarts, invites, features here
    })
    ```
    `auth/me` and `GET /billing` stay readable (GET passes the gate anyway, but keep `me` outside for clarity).

- [ ] **Step 3:** Add a route test: not-entitled tenant → `POST /api/t/{uuid}/invoices` returns 402; `POST /api/t/{uuid}/billing/checkout` still reaches the handler; `GET` routes still 200.

- [ ] **Step 4:** Run `go test ./internal/app/...` → PASS.
- [ ] **Step 5: Commit** — `feat(app): wire subscription routes + gate`

---

## Task 12: Frontend

**Files:**
- Create: `web/src/routes/[tenant]/settings/billing/+page.svelte`
- Modify: `web/src/lib/api/*` (handle 402), `web/src/routes/[tenant]/+layout.svelte` (banner), signup redirect
- Test: extend Playwright e2e if a billing flow stub exists; otherwise a Vitest component test for banner states

- [ ] **Step 1:** Billing settings page: fetch `GET /api/t/{tenant}/billing`; show status + trial days left (from `trialEnd`); `Subscribe` button → `POST .../billing/checkout` then `window.location = url`; `Manage billing` → `GET .../billing/portal` then redirect. Owner-only UI (hide for non-owners).

- [ ] **Step 2:** Layout banner: `trialing` → "Trial: N days left"; `past_due` → grace warning; not entitled (`none`/`canceled`) → "Subscribe to continue" with CTA to billing page.

- [ ] **Step 3:** API client: on `402`, surface a "subscription required" toast/modal — owner sees Subscribe CTA, member sees "ask your account owner".

- [ ] **Step 4:** Signup flow: after successful signup, if billing enabled, route the new owner to the billing page / straight to Checkout (onboarding wall).

- [ ] **Step 5:** Run `cd web && npm run check && npm test` → PASS.
- [ ] **Step 6: Commit** — `feat(web): billing page, banner, 402 handling`

---

## Task 13: Infra

**Files:**
- Modify: `infra/modules/secrets/{variables,main,outputs}.tf`
- Modify: `infra/modules/cloud-run/{variables,main,outputs}.tf`
- Modify: `infra/live/_envcommon/secrets.hcl`, `infra/live/_envcommon/cloud-run.hcl`, per-env inputs

- [ ] **Step 1:** `secrets` module — add `stripe_secret_key` + `stripe_webhook_secret` vars (sensitive, default ""), clone the `anthropic` `google_secret_manager_secret` + `_version` pair (version `count` guarded on non-empty), export the secret ids in `outputs.tf`.

- [ ] **Step 2:** `cloud-run` module — add vars for the two secret ids + `billing_enabled` (bool), `stripe_price_id`, `trial_days`; reference the two secrets in the container env (secret-backed env, matching how `anthropic`/db password are injected); add plain env vars for the non-secret three; grant the runtime SA `roles/secretmanager.secretAccessor` on the two new secrets — clone `google_secret_manager_secret_iam_member.anthropic` (`cloud-run/main.tf:28-33`), threading each new secret id as a variable like `var.anthropic_secret_id`, AND add the two new iam_member resources to the service's `depends_on` list (`cloud-run/main.tf:131-135`).

- [ ] **Step 3:** Terragrunt — thread the secret outputs + plain values through `_envcommon/secrets.hcl`, `_envcommon/cloud-run.hcl`, and the per-env `terragrunt.hcl` inputs (dev + prd). Leave secret VALUES blank in committed HCL (populated out-of-band).

- [ ] **Step 4:** Validate — `cd infra/modules/secrets && tofu init -backend=false && tofu validate`; same for `cloud-run`. Expected: success.

- [ ] **Step 5: Commit** — `feat(infra): stripe secrets + cloud-run billing env`

- [ ] **Step 6 (manual, document in PR, NOT code):** In the Stripe dashboard create the product + recurring price → set `STRIPE_PRICE_ID`; register the `/api/stripe/webhook` endpoint subscribed to `checkout.session.completed`, `customer.subscription.updated`, `customer.subscription.deleted` → set `STRIPE_WEBHOOK_SECRET`. Populate both GSM secret values.

---

## Done when
- `BILLING_ENABLED=false` (default) → app behaves exactly as today, all routes open.
- With it on: new tenant must complete Checkout to write; trial runs 90 days via Stripe; lapse → writes 402, reads OK; `past_due` → grace banner; Portal handles cancel/card; webhook keeps state in sync idempotently.
- `task test` and `cd web && npm test` green.
