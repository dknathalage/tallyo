# Plan: Sidebar Navigation, Rate Tiers Page, and Mobile Layout Fixes

## Context

The app currently uses a top horizontal navbar which doesn't scale well — 7+ links get cramped on tablet screens, and the mobile hamburger dropdown is basic. The user wants a sidebar navigation instead, with Rate Tiers promoted from a section within Settings to its own dedicated nav item. Additionally, several mobile layout issues (overflowing buttons, fixed-width line item inputs, cramped forms) need fixing.

---

## Phase 1: i18n Updates

**Files:** `src/lib/i18n/types.ts`, `src/lib/i18n/en.ts`

- Add `rateTiers: string` to the `nav` interface in types
- Add `sidebarNavigation: string` and `closeSidebar: string` to `a11y` interface
- Add corresponding values in `en.ts`: `rateTiers: 'Rate Tiers'`, `sidebarNavigation: 'Sidebar navigation'`, `closeSidebar: 'Close sidebar'`

---

## Phase 2: Create Sidebar Component

**New file:** `src/lib/components/layout/Sidebar.svelte`

Replace Navbar with a sidebar that has three responsive modes:
- **Desktop (lg:+)**: Fixed sidebar, always visible, `w-64` on the left
- **Tablet/Mobile (below lg:)**: Off-canvas sidebar with backdrop overlay, toggled via hamburger

Structure:
- `open` bindable prop (state managed by AppShell)
- Top: Logo + app name + close button (mobile only)
- Middle: Scrollable nav links with icons — Dashboard, Invoices, Estimates, Clients, Catalog, **Rate Tiers**, Settings, Docs
- Bottom: Theme toggle, DB filename, close DB button
- Each nav link gets a Heroicons outline icon for visual clarity
- Reuse existing `isActive()` logic and styling patterns from Navbar
- Close sidebar on nav link click (mobile/tablet)
- Backdrop click closes sidebar

---

## Phase 3: Update AppShell Layout

**File:** `src/lib/components/layout/AppShell.svelte`

- Replace `Navbar` import with `Sidebar`
- Pass `sidebarOpen` state as bindable prop to Sidebar
- Add a sticky mobile top bar (`lg:hidden`) with hamburger + logo
- Main content gets `lg:ml-64` to offset for the permanent desktop sidebar
- Keep skip-to-content link

---

## Phase 4: Clean Up Navbar

- Delete `src/lib/components/layout/Navbar.svelte` (only imported by AppShell)
- Update `src/lib/components/layout/CLAUDE.md` — replace Navbar reference with Sidebar

---

## Phase 5: Extract Rate Tiers to Dedicated Page

**New file:** `src/routes/(app)/console/rate-tiers/+page.svelte`

- Extract rate tier state/functions (lines 16-99) and template (lines 375-496) from Settings page
- Self-contained page with: header, tier table, add/edit modal, delete confirmation
- Same functionality, same imports from `$lib/db/queries/rate-tiers`

**Modify:** `src/routes/(app)/console/settings/+page.svelte`

- Remove rate tier imports, state, functions, and template sections
- Settings page retains only: Business Profile and Payers sections

---

## Phase 6: Mobile Layout Fixes

### 6a. LineItemRow.svelte
- Split into two rows on mobile: description (full width) on top, qty/rate/amount below
- Add inline labels (`sm:hidden`) for qty/rate/amount on mobile
- Duplicate remove button: one in description row (`sm:hidden`), one in qty row (`hidden sm:block`)

### 6b. InvoiceForm.svelte — Line Items Header
- Change column header from `flex` to `hidden sm:flex` (hide on mobile where items are stacked)

### 6c. InvoiceForm.svelte — Totals
- Change `w-72` to `w-full sm:w-72`

### 6d. Dashboard Header Buttons
- Add `flex-wrap` and `gap-4` to the header container

### 6e-6f. Invoice/Estimate Detail Action Buttons
- Add `flex-wrap` to action button containers
- Change outer header to `items-start` with `gap-4` for clean wrapping

### 6g-6h. Invoice/Estimate List Page Headers
- Add `flex-wrap` and `gap-4` to header containers

### 6i. KeyValueEditor.svelte
- Stack inputs vertically on mobile: `flex-col sm:flex-row`
- Key input: `w-full sm:w-1/3`

---

## Phase 7: Documentation Updates

- `src/lib/components/layout/CLAUDE.md` — Update Navbar → Sidebar
- `src/routes/CLAUDE.md` — Add `rate-tiers/` to app routes
- `src/routes/(app)/console/settings/CLAUDE.md` — Remove rate tiers mention
- New `src/routes/(app)/console/rate-tiers/CLAUDE.md`

---

## Files Modified (Summary)

| File | Change |
|------|--------|
| `src/lib/i18n/types.ts` | Add nav.rateTiers, a11y keys |
| `src/lib/i18n/en.ts` | Add matching values |
| `src/lib/components/layout/Sidebar.svelte` | **NEW** — sidebar navigation |
| `src/lib/components/layout/AppShell.svelte` | Sidebar layout with mobile top bar |
| `src/lib/components/layout/Navbar.svelte` | **DELETE** |
| `src/routes/(app)/console/rate-tiers/+page.svelte` | **NEW** — rate tiers page |
| `src/routes/(app)/console/settings/+page.svelte` | Remove rate tiers section |
| `src/lib/components/invoice/LineItemRow.svelte` | Mobile stacking |
| `src/lib/components/invoice/InvoiceForm.svelte` | Hide header on mobile, responsive totals |
| `src/routes/(app)/console/+page.svelte` | Wrap dashboard buttons |
| `src/routes/(app)/console/invoices/[id]/+page.svelte` | Wrap action buttons |
| `src/routes/(app)/console/estimates/[id]/+page.svelte` | Wrap action buttons |
| `src/routes/(app)/console/invoices/+page.svelte` | Wrap header |
| `src/routes/(app)/console/estimates/+page.svelte` | Wrap header |
| `src/lib/components/shared/KeyValueEditor.svelte` | Stack on mobile |
| CLAUDE.md files (4) | Documentation updates |

---

## Verification

1. `npm run dev` — verify sidebar renders, all nav links work, active state highlights correctly
2. Resize browser to test all three breakpoints (mobile < 768, tablet < 1024, desktop 1024+)
3. Test sidebar open/close on mobile (hamburger, backdrop click, nav click)
4. Visit `/console/rate-tiers` — verify CRUD works (list, create, edit, delete)
5. Visit `/console/settings` — verify rate tiers section is gone, business profile + payers still work
6. Test line item editing on mobile viewport — verify stacked layout
7. Test dashboard, detail pages, list pages on mobile — verify buttons wrap
8. `npm run build` — verify no build errors
9. `npm run test` — run existing tests
