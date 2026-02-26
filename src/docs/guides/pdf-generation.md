# PDF Generation

Invoice Manager generates professional PDF invoices directly in your browser using jsPDF.

## Generating a PDF

### Invoices

1. Open an invoice from the **Invoices** list
2. The PDF preview is shown on the invoice detail page
3. Click **Download PDF** to save the file

### Estimates

1. Open an estimate from the **Estimates** list
2. Click the **PDF** button on the estimate detail page
3. The PDF is downloaded as `estimate-EST-XXXX.pdf`

## What's Included

The generated PDF contains:

- **Header** — Your business name, logo, and contact details (from Settings)
- **Client details** — The linked client's name and address
- **Payer / Bill-To details** — If a payer is set, their name, contact, and address appear alongside the client
- **Document metadata** — Number, date, due date (or valid-until for estimates), status
- **Currency** — All amounts formatted in the document's selected currency
- **Line items table** — Description, quantity, unit price, and line total for each item
- **Totals** — Subtotal, tax, and grand total

## Customization

The PDF uses your business profile from the **Settings** page:

- Upload a logo to include it in the header
- Set your business name, address, and contact info
- Configure tax ID and other details

All changes to your business profile are reflected in future PDF exports immediately.
