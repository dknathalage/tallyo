# Pricing + Website + App Alignment

**Date:** 2026-06-28
**Status:** Design — pending review

## Problem

The pricing copy, the marketing landing page, and the application are out of
sync, and the chosen monetization model has changed.

1. **Pricing copy is fictional.** `web/src/lib/pricing.ts` and the landing page
   advertise three tiers (Starter $0 / Professional $29 / Business $79) with a
   display-only monthly/annual toggle. The backend has exactly **one** Stripe
   price and no tier concept — the tiers are pure marketing copy that maps to
   nothing.
2. **The landing page promises features that do not exist:** recurring
   invoices, API access, and tax reports have no implementation. It also uses
   payment-collection language ("get paid faster", "send reminders") for a
   product that deliberately has **no payment processing**.
3. **The new model is a single paid plan, not three tiers**, with a different
   price point, a shorter trial, and a cheaper AI model for margin.

## Decision (confirmed with owner)

- **One paid plan**, no free tier, no upsell tiers.
- **AUD pricing:** $19 AUD/month or $190 AUD/year (~17% off, "2 months free").
- **30-day trial, card required** (keep the existing card-required Checkout
  mechanism; only the duration changes, 90 → 30).
- **AI model = Claude Haiku 4.5** (was Opus 4.8) — ~5x cheaper input/output,
  raising gross margin from ~60% to ~90% on AI-drafting usage.
- **The landing monthly/annual toggle is real intent** and must carry through
  signup to the first checkout (not a marketing-only display).

## Non-goals

- No payment processing (Stripe Billing for subscriptions only; not collecting
  invoice payments on behalf of users).
- No per-tier feature gating (single plan ⇒ entitlement is trial-vs-active
  only, which already exists).
- No new app features. Copy is being corrected to match what exists, not the
  reverse.
- No double-entry accounting, reconciliation, payroll, or tax computation.

## Real vs advertised features (audit result)

**Exist (keep in copy):** invoices, estimates/quotes, clients, payers, product
catalogue, tax-rate fields, AI draft assist (`internal/smarts`), PDF export,
team members via invites (`internal/auth/invites`).

**Do not exist (cut from copy):** recurring invoices, API access, tax reports.

**Remove (no payments):** "get paid faster", "send reminders", and any other
payment-collection language.

## Design

### 1. Pricing copy — `web/src/lib/pricing.ts`

Collapse the three-tier structure to a single plan.

- Remove the `Tier` type and the `starter`/`professional`/`business` price maps
  and feature arrays.
- Export a single monthly and annual AUD price (`$19` / `$190`), plus a helper
  for the displayed per-period string.
- **Display format:** monthly shows `$19/month`; annual shows the per-month
  equivalent `$15.83/mo, billed annually` (matches the existing toggle UX and
  makes the discount vs $19 visually obvious). `$190` appears as the billed
  total in the toggle/fine print.
- Keep the existing unit test file (`pricing.test.ts`) updated to the new shape.

### 2. Website / landing — `web/src/routes/+page.svelte`

- Pricing section: three `PricingCard`s → **one** card. Keep the
  monthly/annual toggle.
- Feature list = real features only (see audit above).
- Rewrite feature blurbs to invoicing language, not payment-collection
  language. Cut recurring-invoices / API / tax-reports mentions.
- CTAs continue to point at `/signup`, **plus** they record the toggle choice
  (see §4 flow) so the cadence carries through.

### 3. App / backend fixes

**Trial duration (90 → 30):**
- `internal/subscription/config.go`: change `DefaultTrialDays` to `30`.
- `infra/.../cloud-run.hcl`: set `trial_days = 30` (currently `90`).

**AI model → Haiku:**
- `internal/smarts/llm.go`: `defaultModel = string(anthropic.ModelClaudeHaiku4_5_…)`.
- The existing `supportsTuning` guard already disables adaptive thinking and the
  effort/output-config params for Haiku (Haiku rejects both with a 400), so no
  other code change is required. Confirm with the existing smarts tests.

**Annual checkout (the only new backend work):**
- `internal/subscription/config.go`: add `PriceIDAnnual` (env
  `STRIPE_PRICE_ID_ANNUAL`). The existing `PriceID` becomes the monthly price.
- `internal/subscription/stripe.go`: `Client` holds both price IDs;
  `CheckoutInput` gains `Plan` (`"monthly"` | `"annual"`, default `"monthly"`);
  `CreateCheckoutSession` selects the price by `Plan`.
- `internal/subscription/handler.go`: `Checkout` reads the desired plan from the
  request. **Transport: query param on the existing POST** —
  `POST .../billing/checkout?plan=annual`. No request body / JSON parsing is
  added (the endpoint takes no body today). `startCheckout()` in `billing.ts`
  appends `?plan=` to the POST URL. Unknown/empty → monthly.

### 4. Annual cadence flow (landing toggle → first checkout)

The card-required trial means the first Stripe Checkout happens after signup,
when `RequireSubscription` routes a `none`-status tenant to
`settings/billing`. The landing cadence choice is threaded as follows:

1. Landing page: on CTA click, write the toggle value to
   `sessionStorage` under `tallyo_plan` (`"monthly"` | `"annual"`), then
   navigate to `/signup`. Chosen because it survives the post-signup auth
   redirect within the same tab without threading a query param through every
   SvelteKit loader.
2. `web/src/routes/[tenant]/settings/billing/+page.svelte`: reads
   `tallyo_plan` from `sessionStorage` to **default** its own monthly/annual
   toggle. The toggle remains user-changeable — the billing page is the source
   of truth for what gets sent to checkout; the landing choice only pre-seeds
   it.
3. The billing page passes the selected `plan` to `POST .../billing/checkout`.

### 5. Stripe dashboard / ops (manual, outside code)

- Create two recurring prices in AUD: monthly ($19) and annual ($190).
- Populate `STRIPE_PRICE_ID` (monthly) and `STRIPE_PRICE_ID_ANNUAL` secrets.
- Set `trial_days = 30`.
- Flip `billing_enabled = true` once the prices, secrets, and webhook exist.

## Affected files

| File | Change |
|------|--------|
| `web/src/lib/pricing.ts` | single-plan AUD prices; drop tiers |
| `web/src/lib/pricing.test.ts` | update to new shape |
| `web/src/routes/+page.svelte` | one pricing card; real features; cut payment language; CTA writes `tallyo_plan` |
| `web/src/routes/[tenant]/settings/billing/+page.svelte` | read `tallyo_plan`, default cadence toggle, pass `plan` to checkout |
| `web/src/lib/api/billing.ts` | `startCheckout(plan)` appends `?plan=` to the POST URL |
| `internal/subscription/config.go` | `DefaultTrialDays = 30`; add `PriceIDAnnual` |
| `internal/subscription/stripe.go` | two prices; `CheckoutInput.Plan`; price selection |
| `internal/subscription/handler.go` | read `plan` query param |
| `internal/smarts/llm.go` | `defaultModel` → Haiku 4.5 |
| `infra/live/.../cloud-run.hcl` | `trial_days = 30` |

## Testing

- `pricing.test.ts` updated and passing for the single-plan shape.
- New Go unit test: `CreateCheckoutSession` selects the annual price when
  `Plan == "annual"` and the monthly price otherwise (including empty/unknown).
- Existing smarts tests confirm Haiku takes the no-tuning path
  (`supportsTuning("claude-haiku-4-5") == false`).
- Existing subscription webhook/store tests stay green (trial-duration change is
  config-only).
- Manual: landing toggle = annual → signup → billing page defaults to annual →
  checkout uses the annual price.

## Risks / open items

- **Haiku draft quality.** Invoice drafting is structured extraction; Haiku is
  expected to suffice. If quality regresses on real catalogues, the model is a
  one-line revert. Validate against representative drafts before shipping wide.
- **AUD-only display to a global audience.** Acceptable for now (Stripe charges
  in AUD); revisit currency/PPP localization later if non-AU signups grow.
- **`sessionStorage` handoff** is per-tab and cleared if the user opens signup
  in a new tab. Acceptable: the billing-page toggle is always changeable, so the
  worst case is the user re-picks cadence at checkout.
