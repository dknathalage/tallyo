# Plan: Extract Payers to Own Top-Level Route

## Context

Payers (bill-to parties) are currently managed inline on the Settings page. Since payers are independent entities (one payer can serve multiple clients), they deserve their own dedicated section like Clients have, with list/create/detail pages and a sidebar nav entry.

## Changes Overview

### 1. i18n — Add `payer` namespace & `nav.payers`

**`src/lib/i18n/types.ts`** — Add `payers: string` to `nav` section. Add new `payer` section after `client`:
- Keys: `title`, `newPayer`, `editPayer`, `noPayers`, `noPayersMessage`, `noResultsMessage`, `deleteConfirmTitle`, `deleteConfirmMessage`, `linkedClients`, `noLinkedClients`, `noLinkedClientsMessage`, `notFound`, `notFoundMessage`, `backToPayers`, `addNewPayerDesc`, `searchPlaceholder`, `bulkDeleteTitle`, `bulkDeleteMessage`, `createPayer`, `changeHistory`

**`src/lib/i18n/en.ts`** — Add `payers: 'Payers'` to `nav`. Add `payer` block with English values. Remove `settings.payers/payersDesc/addPayer/editPayer/noPayers/noPayersMessage/deletePayer/deletePayerMessage` keys (no longer used). Move `client.createPayer` to `payer.createPayer`.

### 2. DB Queries — Add `bulkDeletePayers` and `getPayerClients`

**`src/lib/db/queries/payers.ts`**:
- Add `bulkDeletePayers(ids: number[])` — follows `bulkDeleteClients` pattern from `clients.ts`
- Add `getPayerClients(payerId: number): Client[]` — `SELECT * FROM clients WHERE payer_id = ? ORDER BY name`
- Import `Client` type

### 3. Create Route Pages (follow Clients pattern)

**`src/routes/(app)/console/payers/+page.svelte`** — List page:
- Search bar + table (checkbox, name linked to detail, email, phone)
- Bulk select/delete with confirmation modal
- Empty state with CTA
- No CSV import/export (simpler entity, not needed)
- Based on `clients/+page.svelte` minus tier column and CSV

**`src/routes/(app)/console/payers/new/+page.svelte`** — Create page:
- Wraps existing `PayerForm` component, calls `createPayer()`, navigates to list
- Based on `clients/new/+page.svelte`

**`src/routes/(app)/console/payers/[id]/+page.svelte`** — Detail/edit page:
- View mode: email, phone, address, metadata key-value pairs
- Edit mode: toggles to `PayerForm`
- "Linked Clients" section: table showing clients with `payer_id` pointing to this payer (name linked to `/console/clients/{id}`, email, phone)
- Change History section (audit log, same pattern as client detail)
- Delete with confirmation
- Based on `clients/[id]/+page.svelte`

### 4. Sidebar Nav

**`src/lib/components/layout/Sidebar.svelte`** — Insert payer entry between Clients and Catalog in `navLinks` array with a building-office icon.

### 5. Remove Payers from Settings

**`src/routes/(app)/console/settings/+page.svelte`**:
- Remove payer imports (`getPayers`, `createPayer`, `updatePayer`, `deletePayer`, `PayerForm`, `Payer` type)
- Remove all payer state variables and functions (lines ~74-136)
- Remove payer section template, add/edit modal, and delete confirmation (lines ~237-309)

### 6. Update PayerForm submit label

**`src/lib/components/payer/PayerForm.svelte`** — Change `i18n.t('client.createPayer')` to `i18n.t('payer.createPayer')` on the submit button.

### 7. Update Documentation

- **`src/routes/(docs)/docs/guides/settings/+page.md`** — Replace "## Payers" section with cross-reference to the new Payers page
- **`src/routes/(docs)/docs/guides/clients/+page.md`** — Change "Manage payers from the **Settings** page" to "**Payers** page"

## Verification

1. `npm run build` — confirm no type errors
2. `npm run test` — all tests pass
3. Manual: navigate to `/console/payers` — list, create, view/edit/delete a payer
4. Manual: check sidebar shows Payers between Clients and Catalog
5. Manual: check Settings page no longer shows payer section
6. Manual: create a client linked to a payer, then check the payer detail page shows the client under "Linked Clients"
