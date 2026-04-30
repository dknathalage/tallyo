# src/routes/

SvelteKit file-based routing. Each `+page.svelte` or `+page.md` is a page.

## Files

- `+layout.svelte` — Root layout with shared concerns (CSS, theme, i18n, PWA, announcer)
- `+layout.ts` — Layout config (prerender, SSR settings)

## Directories

- `(app)/` — App route group (wrapped in FileGate + AppShell, requires database)
  - `+page.svelte` — Dashboard with stats cards and quick actions
  - `invoices/` — Invoice list, detail view, edit, and create pages
  - `estimates/` — Estimate list, detail view, edit, and create pages
  - `clients/` — Client list, detail view, and create pages
  - `catalog/` — Catalog item list, detail view, and create pages
  - `rate-tiers/` — Rate tier management page
  - `settings/` — Business profile and app settings page
- `(docs)/` — Docs route group (standalone layout, no database dependency)
  - `docs/` — Documentation pages rendered from mdsvex markdown
