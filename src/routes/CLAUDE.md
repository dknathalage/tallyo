# src/routes/

SvelteKit file-based routing. Each `+page.svelte` is a page.

## Files

- `+layout.svelte` — Root layout wrapping all pages (AppShell, DB gate)
- `+layout.ts` — Layout config (prerender, SSR settings)
- `+page.svelte` — Dashboard with stats cards and quick actions

## Directories

- `invoices/` — Invoice list, detail view, edit, and create pages
- `clients/` — Client list, detail view, and create pages
- `catalog/` — Catalog item list, detail view, and create pages
- `settings/` — Business profile and app settings page
