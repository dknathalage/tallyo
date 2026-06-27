# Marketing Landing Page — Design

**Date:** 2026-06-27
**Branch:** `feat/admin-and-landing` (off `feat/saas-subscriptions`)
**Status:** Approved design, pending plan

## Purpose

A public marketing page at `/` for logged-out visitors: explain Tallyo, show
pricing tiers, drive signup. Today `/` either redirects a logged-in user to
their tenant or (logged-out) bounces to `/login`. This gives logged-out
visitors something to convert on.

## Pattern

**Pricing-Focused Landing** (ui-ux-pro-max). Section order:

1. **Hero** — value proposition + primary CTA → signup.
2. **Pricing** — 3 **display-only** tiers, mid-tier highlighted with a "most
   popular" badge. An optional monthly/annual toggle changes only the displayed
   numbers (pure marketing copy). See "Pricing reality" below — the backend has
   one price, so all tier CTAs go to the same place.
3. **Feature grid** — the core jobs Tallyo does (invoices, estimates, clients,
   tax, catalogue).
4. **FAQ** — addresses common objections.
5. **Final CTA** — repeat signup.

Sticky nav with a CTA. (`primary-action`: one primary CTA per section.)

## Routing & Shell

- Landing is the root `+page.svelte`. `/` is added to the root layout's
  `PUBLIC_PATHS` so logged-out visitors render the page instead of being
  redirected to `/login`.
- Existing root-layout behaviour is preserved: a **logged-in** user hitting `/`
  still redirects to their first tenant (the redirect fires in `bootstrap()`
  after `loadSession()`). The landing is for logged-out traffic. Note: a
  logged-in user may briefly see the landing mount before the async redirect
  fires — acceptable; not worth a guard. (`isPublic` is exact-match for root, so
  adding `'/'` to `PUBLIC_PATHS` is sufficient.)
- Reuses `app.css` tokens (teal/amber, IBM Plex). Marketing polish (larger hero
  type, amber accents) may exceed in-app chrome but stays on-brand.

## Pricing reality — tiers are display-only

The backend supports **exactly one Stripe price** (`STRIPE_PRICE_ID`), and
checkout is **trial-first** (`TrialPeriodDays`, default 90). There is no tier,
plan, or billing-cadence concept anywhere in Go, the frontend billing API, or
infra. Confirmed by spec review against `internal/subscription/config.go`,
`stripe.go`, `handler.go`, and infra.

Decision: tiers are **marketing copy only**. Every tier CTA does the same thing:

> Pricing CTA → `/signup` → existing signup → existing in-app billing flow
> starts the single trial/checkout.

Consequences for the build:
- **No** `?plan=` param, **no** plan selection, **no** signup changes for plan.
  Signup stays as-is (posts `{ businessName, name }`).
- `startCheckout()` and the backend `Checkout` handler are unchanged — they take
  no plan and start the one configured price. Nothing new is wired to Stripe.
- The monthly/annual toggle, if kept, is **display-only** — it changes shown
  numbers, not what checkout does.

`// ponytail:` real multi-tier billing (multiple Stripe prices + plan param
through signup→checkout→infra) is a separate, larger product effort. Deferred.
When it lands, the landing CTAs get a `?plan=` and signup learns to carry it.

## SEO / rendering — deliberate limitation

The whole app is a deliberate SPA: root `+layout.ts` sets `ssr = false` and
`prerender = false`. The landing therefore renders client-side like every other
route.

`// ponytail:` SPA landing has no prerendered HTML for crawlers. Prerendering
just `/` requires a separate marketing layout group that re-enables
`ssr`/`prerender` independent of the app shell. **Deferred** — add when SEO/ad
traffic actually matters. Not built now.

## Components

Reuse `Button`, `Card`, `Badge`. New, landing-only, small:
- `PricingCard` — tier name, price, feature list, CTA. Highlight variant for the
  popular tier.
- A simple FAQ `<details>`/`<summary>` accordion — native element, no JS, no
  library. (`progressive-disclosure`.)

## UX rules applied (ui-ux-pro-max)

- `prefers-reduced-motion` respected; no autoplay video.
- Pricing uses tabular figures (`number-tabular`) so prices don't shift.
- Mobile-first; pricing cards stack on small screens, no horizontal scroll.
- Annual/monthly toggle has a visible label, not color-only state.
- Hero/section contrast meets 4.5:1 in both light and dark mode (tokens already
  pass).

## Testing

- Routing (the one real behavioural change): a logged-out visit to `/` renders
  the landing instead of bouncing to `/login` — assert `/` is treated public.
- If the monthly/annual display toggle is kept, its price-math (the only
  non-trivial logic) gets a small assert-based test. Otherwise no test — the
  page is static markup.

No signup test — signup is unchanged (tiers are display-only).

## Explicitly skipped (YAGNI)

- Blog, testimonials carousel, competitor comparison table — add with real copy
  and customers.
- i18n / localisation.
- Prerender/SSR for SEO (deferred, see above).
- A CMS — pricing/features are hard-coded until they change often enough to hurt.
