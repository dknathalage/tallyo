# Estimates & Quotes

Estimates let you send prospective pricing to clients before committing to an invoice. Once accepted, an estimate can be converted into an invoice with a single click.

## Viewing Estimates

The **Estimates** page lists all your estimates in a table. Use the search bar to find estimates by number or client name, and filter by status using the status pills (All, Draft, Sent, Accepted, Rejected, Expired).

## Creating an Estimate

1. Click **New Estimate**
2. Select a client from the dropdown
3. Optionally set a **Payer / Bill-To** party
4. Choose a currency (defaults to your business profile currency)
5. Set the **Date** and **Valid Until** date
6. Add line items — search your catalog or enter custom items
7. Each line item has a description, quantity, rate, and calculated amount
8. Optionally add a tax rate and notes
9. Click **Save**

An estimate number is generated automatically in the format `EST-0001`, `EST-0002`, etc.

## Editing an Estimate

Open an estimate and click **Edit**. You can modify all fields including line items, client, payer, dates, and currency.

## Status Workflow

Estimates follow a five-status lifecycle:

| Status | Description |
|--------|-------------|
| **Draft** | Initial state — not yet shared with the client |
| **Sent** | Delivered to the client for review |
| **Accepted** | Client has agreed to the estimate |
| **Rejected** | Client has declined the estimate |
| **Expired** | The validity period has passed |

Change an estimate's status from its detail page using the **Status** dropdown. Bulk status changes are also available from the list page.

## Converting to an Invoice

Once an estimate is **Accepted**, a **Convert to Invoice** button appears on the detail page. Clicking it:

1. Creates a new invoice with the same client, line items, currency, tax, and notes
2. Copies the due date from the estimate's **Valid Until** date
3. Preserves the business, client, and payer snapshots
4. Sets the new invoice status to **Draft**
5. Links the estimate to the invoice (the button changes to **View Invoice**)

Only accepted estimates that have not already been converted can use this action.

## PDF Export

From the estimate detail page, click the **PDF** button to download a formatted PDF. The PDF includes:

- Your business name and logo
- **Service For** (client details) and **Bill To** (payer details) sections
- Estimate number, date, valid-until date, and status
- Line items table with description, quantity, rate, and amount
- Subtotal, tax breakdown, and total in the selected currency
- Notes (if present)

The file is named `estimate-EST-0001.pdf`.

## Bulk Actions

Select multiple estimates using the checkboxes on the list page to:

- **Change status** — Move all selected estimates to a chosen status
- **Delete** — Remove all selected estimates

## Import & Export

### Exporting

Click **Export** on the Estimates page to download a CSV of all estimates and their line items. Each line item is a separate row, grouped by estimate number.

### Importing

Click **Import** to open the import wizard. The wizard validates required fields (estimate number, client name, date, line description) and checks that statuses are valid. New clients are created automatically if they don't already exist. See the [Import & Export guide](./import-export) for the full wizard workflow.

## Change History

Every create, edit, status change, and conversion is recorded in the audit log. Click **Change History** on the estimate detail page to see a timestamped list of all modifications with field-level diffs.
