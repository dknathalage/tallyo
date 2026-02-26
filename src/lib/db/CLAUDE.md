# src/lib/db/

Database layer. Runs SQL.js (SQLite compiled to WASM) in the browser with IndexedDB for persistence.

## Files

- `connection.svelte.ts` — Singleton DB connection manager; loads WASM, persists to IndexedDB, exposes reactive state
- `schema.ts` — SQL table definitions (CREATE TABLE statements)
- `migrate.ts` — Schema migration runner (version-based)
- `audit.ts` — Audit log helper for tracking data mutations

## Directories

- `queries/` — SQL query modules per entity with co-located tests
