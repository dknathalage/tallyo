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
2. **Pricing** — 3 tiers, mid-tier highlighted with a "most popular" badge,
   monthly/annual toggle showing the ~20% annual discount.
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
  still redirects to their first tenant. The landing is for logged-out traffic.
- Reuses `app.css` tokens (teal/amber, IBM Plex). Marketing polish (larger hero
  type, amber accents) may exceed in-app chrome but stays on-brand.

## Pricing CTAs — how they wire to Stripe

A logged-out visitor has **no tenant and no auth**, so they cannot start a
Stripe checkout directly. The flow is:

> Pricing tier CTA → `/signup?plan=<tier>` → existing signup → in-app billing
> flow starts the trial/checkout for the chosen plan.

So "wired to Stripe" = the tier choice is carried into signup; the actual
Stripe checkout is the **existing** in-app billing flow from
`feat/saas-subscriptions` (no new checkout code on the landing page). Signup
reads `?plan=` and pre-selects the tier.

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

- One unit test: signup reads `?plan=` and pre-selects the matching tier
  (mirror existing `lib/*.test.ts` style).
- The monthly/annual toggle price math (the only non-trivial logic) gets a small
  assert-based test.
- Routing: a logged-out visit to `/` renders the landing (not a redirect) — the
  one behavioural change to the root layout.

## Explicitly skipped (YAGNI)

- Blog, testimonials carousel, competitor comparison table — add with real copy
  and customers.
- i18n / localisation.
- Prerender/SSR for SEO (deferred, see above).
- A CMS — pricing/features are hard-coded until they change often enough to hurt.
