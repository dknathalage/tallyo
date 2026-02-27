# Documentation Update Plan

## Context

The app has gained several major features (estimates/quotes, multi-currency, dark mode, i18n, payers, audit logging) that are not reflected in the VitePress docs. This plan brings the documentation in sync with the actual feature set.

## Changes

### 1. New file: `src/docs/guides/estimates.md`
Full guide covering: viewing, creating, editing, status workflow (draft→sent→accepted→rejected→expired), convert-to-invoice, PDF export, bulk actions, import/export, change history.

### 2. `src/docs/.vitepress/config.ts`
Add `{ text: 'Estimates', link: '/guides/estimates' }` to sidebar after Invoices.

### 3. `src/docs/index.md`
Add 4 feature cards to frontmatter: Estimates & Quotes, Multi-Currency, Dark Mode, Internationalization.

### 4. `src/docs/features.md`
Add new sections:
- Update Dashboard section (pending estimates, currency note)
- Estimates & Quotes (after Invoice Management)
- Multi-Currency (after Product Catalog)
- Payers / Bill-To (after Settings)
- Dark Mode
- Internationalization
- Audit Logging

### 5. `src/docs/getting-started.md`
- Add step 4 to First Steps: create an estimate
- Add "Customize Your Experience" section (theme, language, default currency)

### 6. `src/docs/architecture.md`
- Update project structure tree (add estimate/, payer/, stores/, i18n/ directories, estimates/ route)
- Add database tables reference table (all 12 tables)
- Add Pre-Commit Hooks section (Husky runs tests + docs:build)

### 7. `src/docs/guides/invoices.md`
Add sections: Currency, Payer/Bill-To, Change History.

### 8. `src/docs/guides/clients.md`
Add sections: Pricing Tier, Payer Linking, Change History.

### 9. `src/docs/guides/catalog.md`
Expand Rate Tiers section with how-it-works steps.

### 10. `src/docs/guides/settings.md`
- Add default currency to Business Profile
- Add new sections: Payers, Rate Tiers, Theme & Language

### 11. `src/docs/guides/pdf-generation.md`
- Split "Generating a PDF" into Invoices and Estimates subsections
- Add payer details and currency to "What's Included"

### 12. `src/docs/guides/import-export.md`
- Update export section to mention Estimates page
- Add Estimate Import & Export section

## Verification
Run `npm run docs:build` to ensure VitePress builds successfully with all new pages and sidebar links.
