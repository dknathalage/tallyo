# Import & Export

Tallyo supports bulk data operations via CSV and Excel files.

## Exporting Data

Export buttons are available on the Invoices, Clients, Catalog, and Estimates pages. Clicking **Export** downloads a CSV file with all records of that type.

## Importing Data

The import wizard guides you through a multi-step process:

### Step 1: File Selection

Choose a CSV or Excel (.xlsx) file from your device. The file is parsed locally in your browser — nothing is uploaded to a server.

### Step 2: Column Mapping

The wizard auto-detects column mappings based on header names. You can manually adjust which source columns map to which app fields. Saved mappings are remembered for future imports.

### Step 3: Import Mode

Choose how to handle conflicts:

- **Merge** — Add new records and update existing ones (matched by key fields)
- **Replace** — Clear existing data and import fresh

### Step 4: Preview Changes

Review a diff showing what will be added, updated, or removed before committing. This lets you verify the import before any data changes are made.

### Committing

Click **Import** to apply the changes. All modifications are recorded in the audit log.

## Supported Formats

- **CSV** — Comma-separated values (parsed with PapaParse)
- **Excel** — .xlsx files (parsed with SheetJS)

## Estimate Import & Export

Estimates support the same import/export workflow as invoices:

- **Export** — Click **Export** on the Estimates page to download a CSV with all estimates and their line items (one row per line item, grouped by estimate number)
- **Import** — Click **Import** to open the wizard. Required fields are estimate number, client name, date, and line description. Missing clients are created automatically. Statuses must be one of: draft, sent, accepted, rejected, or expired.

## Database Backup

For a full database backup, go to **Settings** and use the database export/import feature. This exports the entire SQLite database file.
