# src/lib/csv/

CSV import/export business logic. Uses PapaParse for parsing.

## Export

- `export-invoices.ts` — Export invoices to CSV
- `export-clients.ts` — Export clients to CSV
- `export-catalog.ts` — Export catalog items to CSV
- `download.ts` — Trigger browser file download

## Import

- `import-invoices.ts` — Parse and insert invoices from CSV
- `import-clients.ts` — Parse and insert clients from CSV
- `import-catalog.ts` — Parse and insert catalog items from CSV
- `parse.ts` — Generic CSV parsing wrapper

## Shared

- `columns.ts` — Column name definitions for each entity
- `types.ts` — TypeScript types for CSV operations
