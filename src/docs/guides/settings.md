# Settings

The Settings page manages your business profile and app-wide configuration.

## Business Profile

Configure the details that appear on your invoices:

- **Business name** — Your company or trading name
- **Address** — Full business address
- **Contact** — Phone, email, website
- **Tax ID** — ABN, GST number, or equivalent
- **Default currency** — The currency pre-selected on new invoices and estimates

## Logo

Upload your business logo. It will be included in the header of all generated PDF invoices. Supported formats include PNG, JPG, and SVG.

## Custom Metadata

Add arbitrary key-value pairs for any additional business information you want to track. Use the key-value editor to add, edit, or remove entries.

## Payers

Manage your **Payer / Bill-To** directory from the Settings page. Payers represent the party responsible for payment, which may differ from the client receiving the service.

- Add, edit, and delete payer records (name, email, phone, address, custom metadata)
- Link a payer to a client so invoices and estimates auto-populate the Bill-To section
- Payer details are captured as a snapshot on each document, so later edits to the payer record don't change existing invoices

## Rate Tiers

Define named pricing tiers (e.g. Standard, Premium, Wholesale) and set per-item rates for each tier. Assign a tier to a client so the correct rates are applied automatically when adding catalog items to their invoices.

## Theme & Language

- **Theme** — Choose between light, dark, or system (follows your OS setting). Toggle from the navbar icon.
- **Language** — Select your preferred locale for date and number formatting. The setting is saved in your browser.

## Database Management

### Export

Download a full backup of your database. This exports the complete SQLite database file, including all invoices, clients, catalog items, and settings.

### Import

Restore from a previously exported database backup. This replaces all current data with the imported database.

::: warning
Importing a database backup replaces all existing data. Make sure to export your current data first if you want to keep it.
:::
