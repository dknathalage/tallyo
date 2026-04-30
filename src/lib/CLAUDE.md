# src/lib/

Shared library code imported across the app via `$lib/`.

## Files

- `index.ts` — Library barrel export

## Directories

- `components/` — Svelte UI components grouped by domain
- `csv/` — CSV import/export logic
- `db/` — Database layer (schema, migrations, queries, connection)
- `import/` — Multi-format file import processing (column mapping, diffing)
- `types/` — Shared TypeScript type definitions
- `utils/` — Formatting, PDF generation, invoice numbering helpers
- `assets/` — Static assets (favicon)
