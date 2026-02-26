# src/lib/import/

Multi-format file import processing logic. Supports CSV and Excel files.

- `parse-file.ts` — Detect file type and parse into rows
- `map-columns.ts` — Auto-map and manually map source columns to app fields
- `diff-catalog.ts` — Compute add/update/delete diff for catalog imports
- `commit-catalog.ts` — Apply catalog diff to the database
