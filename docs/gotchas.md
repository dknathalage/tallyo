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
