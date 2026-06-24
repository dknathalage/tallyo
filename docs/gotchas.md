# Gotchas

Hard-won traps. Read before touching the listed areas.

## Frontend

### Seed lazily-mounted modal forms with `$effect.pre`, not `$effect`

`Modal.svelte` mounts its body lazily (`{#if open}`), so any `bind:value` input
inside it is created the instant `open` flips true. If you seed the bound state
in a plain `$effect` (a *post-render* user effect), the input mounts **before**
the seed runs and is left on its stale/empty value — e.g. a `<select>` stuck on
"— select —" despite a `presetX` prop.

Use `$effect.pre` so seeding runs **before** the modal body renders. See
`SessionForm.svelte` (seed block) — preselecting the client of an ad-hoc session
created from a client page.

Scope it to the lazy/modal host only. Don't blanket-swap a component's seed
`$effect` for `$effect.pre`: a host that mounts the body eagerly (e.g. the
inline full-page route) wants the original post-render `$effect` seed-once-on-
mount. `SessionForm` keeps both — `$effect.pre` for the modal open-transition,
plain `$effect` for the inline host.

## Smarts (AI)

### A configured model must support both adaptive thinking AND the effort param

`internal/smarts/llm.go` calls Anthropic with adaptive thinking (in
`ProposeGrounded`, used by draft-invoice) and the effort/output-config param
(everywhere `ANTHROPIC_EFFORT` is set). Some models — e.g. `claude-haiku-4-5` —
reject **both** with a `400` (`adaptive thinking is not supported on this model`
/ `This model does not support the effort parameter`), which the handler masks
as a `502`. So pointing `ANTHROPIC_MODEL` at such a model silently broke 3 of 4
Smarts.

`supportsTuning(model)` now gates both params off for models that don't accept
them (currently a `haiku` name check), so cheaper models work. If you add a new
model override and a Smart starts returning `502`, check the server log for a
real `400` and extend `supportsTuning`.

### Signup response `tenantId` is empty — read the tenant uuid from the session

`POST /api/signup` returns the owner JSON, but `TenantsRepo.Signup` skips the
`fillTenantUUID` backfill every other user-returning path runs, so the response's
`tenantId` is `""`. To get the new tenant's uuid, call `GET /api/auth/session`
and read `tenants[0].id`. The e2e seed helper (`web/e2e/fixtures.ts`,
`signupOwner`) does exactly this.

## Testing

### Live Smarts e2e: catalogue `effectiveFrom` defaults to the commit date

A price-list version's `effectiveFrom` defaults to the day it's imported. The
draft-invoice / suggest-lines Smarts resolve the catalogue version by the
session's service date, so a session dated **before** today yields
`422 no price list in effect for that date`. When seeding for a live Smarts test,
date the session on/after the price-list's effective date (i.e. today).
