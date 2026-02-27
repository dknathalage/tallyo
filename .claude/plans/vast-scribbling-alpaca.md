# Plan: Estimates/Quotes, Multi-Currency, i18n, Accessibility

## Context

The Invoice Manager is a local-first PWA with full invoice CRUD, client/catalog management, PDF export, CSV import/export, and audit logging. The user wants to add four major features to expand the app's capabilities. This plan covers the database, query, component, route, and utility changes for each.

## Implementation Order

**Multi-Currency → Estimates/Quotes → i18n → Accessibility**

- **Multi-Currency first**: Smallest surface area, touches core data layer. All subsequent features benefit from currency being in the schema from day one.
- **Estimates second**: Largest feature, but follows established invoice patterns. Having it done before i18n means string extraction captures everything in one pass.
- **i18n third**: Sweeping but non-structural. More efficient to extract strings after all features exist.
- **Accessibility last**: Purely additive markup changes. By doing it last, every component is in its final form for a single audit pass.

---

## Phase 1: Multi-Currency

### Database
- New `migration6_multiCurrency()` in `src/lib/db/migrate.ts`
  - `ALTER TABLE invoices ADD COLUMN currency_code TEXT DEFAULT 'USD'`
  - `ALTER TABLE business_profile ADD COLUMN default_currency TEXT DEFAULT 'USD'`
- Existing invoices get `'USD'` by default (backward compatible)

### Types (`src/lib/types/index.ts`)
- Add `currency_code: string` to `Invoice`
- Add `default_currency: string` to `BusinessProfile`
- Add `excluded_currency_count: number` to `DashboardStats` (count of invoices in non-default currencies)

### New Files
| File | Purpose |
|------|---------|
| `src/lib/utils/currency.ts` | `CURRENCIES` array (USD, EUR, GBP, AUD, CAD, JPY, etc.) + `CurrencyInfo` type |
| `src/lib/components/shared/CurrencySelect.svelte` | Reusable currency dropdown |

### Modified Files
| File | Change |
|------|--------|
| `src/lib/utils/format.ts` | `formatCurrency(amount, currencyCode = 'USD')` — parameterize currency |
| `src/lib/utils/pdf.ts` | Pass `currency_code` to all `formatPdfCurrency()` calls |
| `src/lib/db/queries/invoices.ts` | Add `currency_code` to create/update |
| `src/lib/db/queries/dashboard.ts` | Filter revenue/outstanding by default currency, count excluded |
| `src/lib/db/queries/business-profile.ts` | Add `default_currency` to save/get |
| `src/lib/csv/columns.ts` | Add `'currency_code'` to `INVOICE_COLUMNS` |
| `src/lib/csv/export-invoices.ts` | Include currency_code |
| `src/lib/csv/import-invoices.ts` | Parse currency_code (default: 'USD') |
| `src/lib/components/invoice/InvoiceForm.svelte` | Currency selector, init from business profile |
| `src/lib/components/invoice/LineItemRow.svelte` | Accept `currencyCode` prop |
| `src/routes/invoices/+page.svelte` | Pass currency to `formatCurrency()` |
| `src/routes/invoices/[id]/+page.svelte` | Pass currency to all currency formatting |
| `src/routes/+page.svelte` | Dashboard: show default-currency totals with excluded count note |
| `src/routes/settings/+page.svelte` | Default currency selector |

### Design Decisions
- Currency lives on invoices, not catalog items. Catalog rates stay in default currency. Users manually adjust rates for foreign-currency invoices. This avoids exchange rate complexity in a local-first app.
- **Dashboard**: Only aggregate invoices matching the default business currency. Show a note about excluded foreign-currency invoices (e.g., "3 invoices in other currencies not included").

---

## Phase 2: Estimates/Quotes

### Database
- New `migration7_estimates()` in `src/lib/db/migrate.ts`
- **`estimates` table**: mirrors `invoices` but with `estimate_number`, `valid_until` (instead of `due_date`), `converted_invoice_id` (FK → invoices), and status values: `draft | sent | accepted | rejected | expired`
- **`estimate_line_items` table**: mirrors `line_items` with FK to `estimates`
- Separate tables (not a `document_type` column) to keep invoice constraints clean and allow schema divergence
- **Free status transitions**: Any status can change to any other (same as invoices), no workflow restrictions enforced

### Types (`src/lib/types/index.ts`)
- `EstimateStatus = 'draft' | 'sent' | 'accepted' | 'rejected' | 'expired'`
- `Estimate` interface (mirrors Invoice with estimate-specific fields)
- `EstimateLineItem` interface
- Extend `DashboardStats` with `total_estimates`, `pending_estimates`, `recent_estimates`

### New Files
| File | Purpose |
|------|---------|
| `src/lib/utils/estimate-number.ts` | `generateEstimateNumber()` → EST-0001 pattern |
| `src/lib/db/queries/estimates.ts` | Full CRUD: getEstimates, getEstimate, getEstimateLineItems, createEstimate, updateEstimate, deleteEstimate, updateEstimateStatus, bulkDelete, bulkUpdateStatus, getClientEstimates, **convertEstimateToInvoice** |
| `src/lib/db/queries/estimates.test.ts` | Tests following invoices.test.ts mock pattern |
| `src/lib/utils/estimate-number.test.ts` | Tests for EST-XXXX generation |
| `src/lib/components/estimate/EstimateForm.svelte` | Form with `valid_until`, estimate statuses, currency selector |
| `src/lib/components/estimate/EstimateCard.svelte` | Summary card for estimate display |
| `src/lib/csv/export-estimates.ts` | CSV export for estimates |
| `src/lib/csv/import-estimates.ts` | CSV import for estimates |
| `src/routes/estimates/+page.svelte` | List page with search, status filters, bulk actions |
| `src/routes/estimates/new/+page.svelte` | Create estimate page |
| `src/routes/estimates/[id]/+page.svelte` | Detail page with "Convert to Invoice" button |
| `src/routes/estimates/[id]/edit/+page.svelte` | Edit estimate page |

### Key Feature: Convert Estimate to Invoice
- `convertEstimateToInvoice(estimateId)` in estimates.ts
- Only works on `accepted` estimates that haven't been converted
- Creates a new draft invoice with all line items, snapshots, and currency copied
- Sets `estimates.converted_invoice_id` to link back
- Audit-logged as `action: 'convert_to_invoice'`

### Modified Files
| File | Change |
|------|--------|
| `src/lib/db/queries/dashboard.ts` | Add estimate stats queries |
| `src/lib/components/shared/StatusBadge.svelte` | Add `accepted`, `rejected`, `expired` status colors |
| `src/lib/components/layout/Navbar.svelte` | Add "Estimates" nav link |
| `src/lib/csv/columns.ts` | Add `ESTIMATE_COLUMNS` |
| `src/lib/csv/types.ts` | Add estimate CSV types |
| `src/lib/utils/pdf.ts` | Add `exportEstimatePdf()`, refactor shared rendering into internal helper |
| `src/routes/+page.svelte` | Dashboard: estimate stats, recent estimates |
| `src/routes/clients/[id]/+page.svelte` | Add "Client Estimates" section |

---

## Phase 3: i18n (Internationalization)

### Architecture: Custom Store-Based Approach
- No external library — a simple Svelte 5 rune store with typed message keys
- JSON message files with `Messages` interface for compile-time safety
- Dynamic imports for non-English locales (keeps bundle small)
- Locale persisted in localStorage
- **English only at launch** — full infrastructure in place, additional locales added by creating a new `src/lib/i18n/{locale}.ts` file

### New Files
| File | Purpose |
|------|---------|
| `src/lib/i18n/types.ts` | `Messages` interface — all string keys (nav, dashboard, invoice, estimate, status, common, settings, client, catalog, validation, a11y) |
| `src/lib/i18n/en.ts` | English translations (~200-300 keys) |
| `src/lib/stores/i18n.svelte.ts` | `i18n` store: `locale`, `init()`, `setLocale()`, `t(key, values?)` with interpolation |

### Format Changes (`src/lib/utils/format.ts`)
- `formatCurrency()` reads `i18n.locale` to determine Intl locale string
- `formatDate()` reads `i18n.locale` for date formatting
- Locale-to-Intl mapping: `{ en: 'en-US', es: 'es-ES', fr: 'fr-FR', ... }`

### String Extraction (all files with hardcoded English text)
Replace all hardcoded strings with `i18n.t('key')` calls across:
- **Layout**: Navbar, AppShell, FileGate
- **Shared**: StatusBadge, EmptyState, BulkActionBar, SearchInput, ConfirmDialog, Modal, LogoUploader
- **Domain components**: InvoiceForm, EstimateForm, LineItemRow, InvoiceCard, EstimateCard, ClientForm, ClientCard, ClientSelect, CatalogForm, CatalogAutocomplete, CatalogBrowseModal, PayerForm
- **CSV/Import**: ImportExportBar, ImportPreviewModal, ImportWizardModal + steps
- **PWA**: ReloadPrompt
- **Routes**: All 14+ page files (dashboard, invoices, estimates, clients, catalog, settings)
- **PDF**: Labels in `pdf.ts` ("INVOICE", "ESTIMATE", "SERVICE FOR", "BILL TO", etc.)

### Settings Integration
- Language selector in `src/routes/settings/+page.svelte`
- Init in `src/routes/+layout.svelte` alongside `theme.init()`

---

## Phase 4: Accessibility (WCAG 2.1 AA)

### Skip-to-Content Link (`src/lib/components/layout/AppShell.svelte`)
- Visually hidden `<a href="#main-content">` that becomes visible on focus
- `<main id="main-content">` landmark

### Landmark Regions
- `Navbar.svelte`: Add `aria-label="Main navigation"` to `<nav>`
- Mobile menu: `aria-label="Mobile navigation"`, `aria-expanded` on toggle button

### Focus Management in Modals (`src/lib/components/shared/Modal.svelte`)
- **Focus trap**: Tab cycles within modal, Shift+Tab wraps backward
- **Initial focus**: Move to first focusable element on open
- **Restore focus**: Return to trigger element on close
- Apply same pattern to: ConfirmDialog, CatalogBrowseModal, ImportPreviewModal, ImportWizardModal

### Screen Reader Announcements
| File | Purpose |
|------|---------|
| `src/lib/stores/announcer.svelte.ts` | `announcer` store: `announce(msg, priority)` |
| `src/lib/components/shared/LiveAnnouncer.svelte` | `aria-live` regions (polite + assertive) |

Usage: bulk operations, status changes, form submissions, import completions.

### Form Accessibility
- `aria-describedby` linking inputs to error messages
- `role="alert"` on error messages
- `<fieldset>`/`<legend>` for form section grouping (From, Service For, Bill To)
- Apply to: InvoiceForm, EstimateForm, ClientForm, CatalogForm, PayerForm

### Keyboard Navigation Fixes
- Custom status dropdowns → proper `role="listbox"` + arrow key navigation
- Clickable table rows → add `tabindex="0"` + Enter/Space handlers (or use link elements)
- Mobile menu toggle → `aria-expanded`, `aria-controls`
- CatalogAutocomplete → proper combobox ARIA pattern (`role="combobox"`, `aria-expanded`, `aria-activedescendant`)

### Table Accessibility
All data tables need:
- `<caption class="sr-only">` describing the table
- `scope="col"` on `<th>` elements
- Descriptive `aria-label` on checkboxes ("Select invoice INV-0001")
- Files: dashboard, invoices list, invoice detail, estimates list, estimate detail, clients list, catalog list, settings (payers + tiers tables)

### Color & Motion
- **Contrast**: Verify all `text-gray-400 dark:` pairings meet 4.5:1; bump to `text-gray-300` where needed
- **Reduced motion**: Add `@media (prefers-reduced-motion: reduce)` rule in `src/app.css` to disable animations/transitions

---

## New Files Summary (20 files)

| File | Phase |
|------|-------|
| `src/lib/utils/currency.ts` | 1 |
| `src/lib/components/shared/CurrencySelect.svelte` | 1 |
| `src/lib/utils/estimate-number.ts` | 2 |
| `src/lib/utils/estimate-number.test.ts` | 2 |
| `src/lib/db/queries/estimates.ts` | 2 |
| `src/lib/db/queries/estimates.test.ts` | 2 |
| `src/lib/components/estimate/EstimateForm.svelte` | 2 |
| `src/lib/components/estimate/EstimateCard.svelte` | 2 |
| `src/lib/csv/export-estimates.ts` | 2 |
| `src/lib/csv/import-estimates.ts` | 2 |
| `src/routes/estimates/+page.svelte` | 2 |
| `src/routes/estimates/new/+page.svelte` | 2 |
| `src/routes/estimates/[id]/+page.svelte` | 2 |
| `src/routes/estimates/[id]/edit/+page.svelte` | 2 |
| `src/lib/i18n/types.ts` | 3 |
| `src/lib/i18n/en.ts` | 3 |
| `src/lib/stores/i18n.svelte.ts` | 3 |
| `src/lib/stores/announcer.svelte.ts` | 4 |
| `src/lib/components/shared/LiveAnnouncer.svelte` | 4 |
| `src/lib/utils/currency.test.ts` | 1 |

## Modified Files Summary (40+ files)

Core files touched by multiple phases:
- `src/lib/db/migrate.ts` — Phases 1, 2
- `src/lib/types/index.ts` — Phases 1, 2
- `src/lib/utils/format.ts` — Phases 1, 3
- `src/lib/utils/pdf.ts` — Phases 1, 2, 3
- `src/lib/components/shared/StatusBadge.svelte` — Phases 2, 3
- `src/lib/components/layout/Navbar.svelte` — Phases 2, 3, 4
- `src/routes/+page.svelte` (Dashboard) — Phases 1, 2, 3, 4
- `src/routes/settings/+page.svelte` — Phases 1, 3, 4

## Testing Strategy

- **Unit tests**: Follow existing Vitest + mock pattern from `invoices.test.ts`
- **New test files**: estimates.test.ts, estimate-number.test.ts, currency.test.ts, extend format.test.ts
- **Accessibility testing**: Manual VoiceOver + keyboard walkthrough, axe-core automated scan
- **Verification**: `npm run build` succeeds, `npm run test` passes after each phase
