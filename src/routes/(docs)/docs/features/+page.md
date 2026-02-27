<script>
import Callout from '$lib/components/docs/Callout.svelte';
</script>

# Features

## Dashboard

The home page shows an overview of your invoicing activity with stats cards for revenue, invoice counts, outstanding amounts, and pending estimates. Quick action buttons let you jump to common tasks.

<Callout type="tip">
Dashboard totals are calculated in your default currency. Invoices in other currencies are excluded and noted separately.
</Callout>

## Invoice Management

- Create and edit invoices with multiple line items
- Link invoices to clients from your client list
- Automatic invoice number generation
- Status tracking (draft, sent, paid, overdue)
- Search and filter your invoice list
- Bulk selection and actions

## Estimates & Quotes

- Create estimates with line items, client, and payer details
- Five-status workflow: draft → sent → accepted → rejected → expired
- Convert accepted estimates to invoices with one click
- PDF export with business branding
- Search, filter by status, and bulk actions
- CSV import and export

## Client Management

- Maintain a directory of clients with contact details
- View all invoices associated with a client
- Search and filter clients
- Bulk operations

## Product Catalog

- Maintain a catalog of products and services
- Autocomplete search when adding invoice line items
- Browse catalog via a modal picker
- Rate tiers for flexible pricing

## Multi-Currency

- Support for 20 currencies (USD, EUR, GBP, AUD, CAD, JPY, CHF, and more)
- Set a default currency in your business profile
- Choose a currency per invoice or estimate
- Locale-aware formatting with proper symbols and decimal handling

## Import & Export

- **CSV and Excel import** with a multi-step wizard:
  - File selection
  - Column mapping (auto-detected + manual override)
  - Merge/replace strategy selection
  - Diff preview before committing changes
- **CSV export** for invoices, clients, and catalog items
- **Database backup** via Settings page

## PDF Generation

- Generate professional PDF invoices using jsPDF
- Includes your business logo, address, and branding
- Auto-formatted line item table with totals
- Download or preview directly in the browser

## Settings

- Business profile (name, address, contact, tax ID, default currency)
- Logo upload
- Custom metadata key-value pairs
- Full database export and import

## Payers / Bill-To

- Maintain a directory of payer entities separate from clients
- Link a default payer to a client for automatic population
- Override payer details per invoice or estimate
- Payer snapshots are captured at document creation time

## Dark Mode

- Three theme options: light, dark, and system (follows OS preference)
- Toggle from the navbar
- All pages and PDF previews respect the selected theme

## Internationalization

- Locale-aware date and number formatting via the Intl API
- Translation-ready architecture with structured message keys
- Language preference saved in your browser

## Audit Logging

- Every data change is recorded with a timestamp, action type, and field-level diffs
- View change history on invoice, estimate, client, and payer detail pages
- Bulk operations are grouped under a shared batch ID
