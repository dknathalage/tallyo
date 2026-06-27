# Tallyo SaaS Subscriptions — Design

Date: 2026-06-27
Status: Approved (design + spec review)

## Goal

Charge tenants a recurring fee to use Tallyo. One paid plan with a free trial
that requires a credit card up front. Stripe Billing owns plans, cards, the
trial clock, dunning, invoices, and the customer self-service portal. Tallyo
stores the minimum mirror state needed to gate access and is driven by Stripe
webhooks as the source of truth.

Out of scope (deliberately deferred — YAGNI): multiple tiers, per-seat pricing,
proration logic, locally-stored Stripe invoices, a Stripe OpenTofu provider.

## Context

- Multi-tenant invoicing SaaS. Postgres control plane + DB-per-tenant.
- Auth: Firebase / GCIP stateless bearer JWT. `users` rows link a firebase uid
  to a `(tenant, uid)` membership with role `owner|admin|member`.
- `tenants` table (control DB) already has `status active|suspended`.
- `internal/httpx.ResolveTenant` middleware already loads the tenant per request
  and 403s a suspended tenant — the entitlement gate folds in here.
- `internal/httpx.RequireRole("owner")` exists for owner-only routes.
- Env config helpers: `app.EnvOr`, `app.EnvBool` (`internal/app/app.go`).
- Signup: `SignupHandler` (`internal/app/auth_handlers.go`) provisions
  tenant + owner + business profile for the bearer identity.
- Secrets: Google Secret Manager via `infra/modules/secrets`, one resource pair
  per secret, injected into Cloud Run.

## Decisions

| Decision | Choice |
|---|---|
| Provider | Stripe Billing (hosted Checkout + Customer Portal + webhooks) |
| Plan | Single paid plan |
| Trial | Card required up front; Stripe-driven; `trial_period_days = TRIAL_DAYS` |
| `TRIAL_DAYS` | Default 90, configurable via env |
| Lapse enforcement | Read-only everything (writes → 402); reads + export still work |
| `past_due` (dunning) | Still entitled + grace banner |
| Source of truth | Stripe webhooks, not the post-Checkout redirect |
| SaaS subscription state | Columns on `tenants` (1 tenant = 1 sub), not a new table |
| Stripe product/price IaC | Dashboard, not a tofu provider (one price, one webhook) |
| Rollout | Behind `BILLING_ENABLED` feature gate; off → everyone entitled |

## Data model

New control-DB migration `internal/db/migrations/control/00003_subscriptions.sql`.
Add to `tenants`:

- `stripe_customer_id TEXT` — null until Checkout completes
- `stripe_subscription_id TEXT` — null until Checkout completes
- `subscription_status TEXT NOT NULL DEFAULT 'none'`
  — `none | trialing | active | past_due | canceled`
- `trial_end TEXT` — display only, set from webhook
- `current_period_end TEXT` — display only, set from webhook

`none` = tenant created but Checkout never completed.

Index `CREATE INDEX idx_tenants_stripe_customer ON tenants(stripe_customer_id)` —
the tenant-agnostic webhook resolves the tenant by `stripe_customer_id` (or by
`client_reference_id`/metadata carried on the Checkout Session; see §5).

**sqlc plumbing:** the entitlement fields must ride along with the existing
`ResolveTenant` read (no extra query). That means: add the new columns to the
`SELECT *`-equivalent in `internal/db/queries/tenants.sql` (they come free with
`SELECT *`, but pin explicit columns if switching), add a
`GetTenantByStripeCustomer` query, regenerate sqlc (`task sqlc`), and add the
new fields to the `auth.Tenant` struct + its `GetByUUID`/status mappers.

## Entitlement (the whole gate)

One pure function, unit-tested in isolation:

```
entitled(status) = status == "active"
                || status == "trialing"
                || status == "past_due"   // grace during dunning
```

`none` and `canceled` → not entitled. Stripe flips `trialing → active` (or
`→ past_due → canceled`) and the webhook syncs `subscription_status`, so Tallyo
never computes the trial clock itself.

## Components

### 1. Subscription store
A small repo over the new `tenants` columns in the control DB (read entitlement
fields; write the webhook-synced fields). Follows the existing `auth.TenantsRepo`
shape.

### 2. Entitlement folded into ResolveTenant
`ResolveTenant` already loads the tenant. Have it also stash an `entitled` bool
in `reqctx` (zero extra DB read). When `BILLING_ENABLED` is off, `entitled` is
always true.

### 3. RequireSubscription middleware (`internal/httpx`)
Applied to tenant-scoped routes. If `!entitled` AND method ∈
{POST, PUT, PATCH, DELETE} → `402 Payment Required`. GET always passes. Billing
routes are mounted *without* this middleware so a lapsed tenant can still pay.

### 4. Trial start in signup
`SignupHandler` creates the tenant with `subscription_status='none'`, then the
frontend routes the new owner straight to Stripe Checkout (`mode=subscription`,
the configured price, `trial_period_days=TRIAL_DAYS`, card captured). No local
trial bookkeeping.

### 5. Stripe handlers (`internal/billing` or new `internal/subscription`)
- `POST /api/t/{tenantUUID}/billing/checkout` — owner-only. Creates a Checkout
  Session, returns the redirect URL.
- `GET  /api/t/{tenantUUID}/billing/portal` — owner-only. Returns a Customer
  Portal session URL. Portal owns cancel / card update / invoice history — no
  code from us.
- `GET  /api/t/{tenantUUID}/billing` — current status for the UI
  (`subscription_status`, `trial_end`, `entitled`).
- `POST /api/stripe/webhook` — tenant-agnostic, mounted OUTSIDE auth and any
  body-consuming middleware (needs the raw body for signature verification).
  Verifies the Stripe signature with `STRIPE_WEBHOOK_SECRET`. Handles:
  - `checkout.session.completed` → link `stripe_customer_id` +
    `stripe_subscription_id`, set status.
  - `customer.subscription.updated` / `.deleted` → sync `subscription_status`,
    `trial_end`, `current_period_end`.

  **Tenant resolution + race/idempotency:** set `client_reference_id` (and
  metadata `tenant_id`) on the Checkout Session so EVERY handler can map back to
  a tenant — `checkout.session.completed` reads `client_reference_id`;
  `subscription.*` handlers resolve via `GetTenantByStripeCustomer`, and if the
  customer isn't linked yet (update arrived before completed), self-heal by
  fetching the subscription from Stripe to read its metadata/customer and link.
  **Idempotency:** webhook delivery is at-least-once and can be out of order —
  store the Stripe event/subscription `created`(or `current_period_end`)
  timestamp and no-op any event not newer than stored state; duplicate
  deliveries are therefore idempotent.

Uses the `stripe-go` SDK (justified dependency: hand-rolling signed API calls and
webhook signature verification is the wrong rung).

### 6. Config (env)
- `BILLING_ENABLED` (bool, default false) — feature gate
- `STRIPE_SECRET_KEY`
- `STRIPE_WEBHOOK_SECRET`
- `STRIPE_PRICE_ID`
- `TRIAL_DAYS` (int, default 90)

Read via `app.EnvOr` / `app.EnvBool`. Use the `feature-gate` skill for `BILLING_ENABLED`.

### 7. Frontend
- Billing settings page: status + trial days left; `Subscribe` → Checkout,
  `Manage billing` → Portal.
- New tenant with `status='none'` → onboarding wall routing straight to Checkout
  (not a dismissible banner).
- Global banner: trial countdown while `trialing`; grace warning while
  `past_due`; "subscribe to continue" when read-only.
- A write that returns 402 shows a subscribe CTA (owner) or
  "ask your account owner" (member).

### 8. Infra
Cross-module, all copying existing patterns:
- `infra/modules/secrets` — add `stripe_secret_key` + `stripe_webhook_secret`
  vars + GSM secret/version resource pairs (clone the `anthropic` pair); export
  their ids in `outputs.tf`.
- `infra/modules/cloud-run` — add the two secret vars + `variables.tf` entries
  for `billing_enabled`, `stripe_price_id`, `trial_days`; reference the secrets
  in the container env block in `main.tf`; grant the runtime SA
  `secretAccessor` on the two new secrets (the IAM binding near
  `cloud-run/main.tf:30`).
- Terragrunt — pass the new secret outputs + plain values through
  `_envcommon/secrets.hcl`, `_envcommon/cloud-run.hcl`, and per-env inputs.

No Stripe tofu provider; create the product + price once in the Stripe dashboard
(grab `STRIPE_PRICE_ID`) and register the webhook endpoint (grab
`STRIPE_WEBHOOK_SECRET`) manually.

## Data flow

1. Owner signs up → tenant created `status='none'` → frontend redirects to
   Checkout.
2. Owner enters card → Stripe starts the trial → redirect back.
3. `checkout.session.completed` webhook → status `trialing`, ids + `trial_end`
   stored. Tenant now entitled.
4. After `TRIAL_DAYS`, Stripe charges → `customer.subscription.updated`
   (`active`) webhook syncs state.
5. Payment fails → Stripe dunning → `past_due` (still entitled, grace banner);
   exhausted → `canceled` → not entitled → writes return 402.
6. Owner can hit the Portal anytime to fix the card or cancel.

## Error handling

- Webhook signature invalid → 400, no state change.
- Webhook for an unknown customer/subscription → 200 (ack) + log; never 500 a
  webhook Stripe will retry into a storm.
- Checkout/Portal Stripe API error → 502 + log; frontend shows a retry.
- `BILLING_ENABLED=false` → all billing routes return 404/disabled and
  everyone is entitled (clean dark-ship + local dev).

## Testing

- Entitlement function: each status in/out (`none`, `trialing`, `active`,
  `past_due`, `canceled`).
- `RequireSubscription` middleware: GET passes when lapsed; POST/PUT/PATCH/DELETE
  → 402 when lapsed; all methods pass when entitled; billing route exempt;
  gate disabled → all pass.
- Webhook handler: valid signature applies state for each handled event type;
  invalid signature → 400; unknown subscription → 200 + no-op.
- Webhook idempotency: duplicate delivery of the same event is a no-op;
  out-of-order (stale-timestamp) event does not clobber newer state.
- Webhook self-heal: `subscription.updated` arriving before
  `checkout.session.completed` still links the tenant (via Stripe lookup).
- Signup: new tenant lands in `status='none'`.
