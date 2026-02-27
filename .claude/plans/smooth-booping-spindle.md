# Plan: Replace VitePress with Native SvelteKit Docs Routes

## Context

The PWA (SvelteKit) and docs (VitePress) are two separate apps stitched together at deploy time under one GitHub Pages site. This causes recurring conflicts: service worker scope collisions, routing interception, and a fragile `404.html` with conditional logic. Each new route risks re-triggering these issues.

**Goal:** Eliminate VitePress entirely. Render all 11 doc pages as native SvelteKit routes using mdsvex, giving us one build, one router, one service worker.

---

## Approach

Use **SvelteKit route groups** to split the app into two layout branches:

- `(app)/` — existing app pages, wrapped in `FileGate > AppShell` (database-dependent)
- `(docs)/` — docs pages, wrapped in a new `DocsShell` layout (no database dependency)

Route groups don't affect URLs — app stays at `/`, `/invoices`, etc.; docs live at `/docs/*`.

Use **mdsvex** to preprocess `.md` files as Svelte components so authors keep writing markdown.

---

## Implementation Steps

### Phase 1: Dependencies & Config

1. **`package.json`** — Add `mdsvex`, `@tailwindcss/typography`. Remove `vitepress`. Remove scripts: `docs:dev`, `docs:build`, `docs:preview`, `build:all`.
2. **`svelte.config.js`** — Add mdsvex preprocessor, add `.md` to `extensions`.
3. **`src/app.css`** — Add `@plugin "@tailwindcss/typography"` for prose styling of markdown content.
4. **`vite.config.ts`** — Remove `navigateFallbackDenylist: [/^\/invoices\/docs\//]` from workbox config.

### Phase 2: Route Group Restructure

5. **`src/routes/+layout.svelte`** — Strip down to shared concerns only: CSS import, theme/i18n init, PWA head tag, `LiveAnnouncer`, `ReloadPrompt`. Remove `FileGate` and `AppShell`.
6. **`src/routes/(app)/+layout.svelte`** — New file. Wraps children in `FileGate > AppShell` (the logic removed from root layout).
7. **Move all existing app routes** into `(app)/`:
   - `+page.svelte` → `(app)/+page.svelte`
   - `catalog/`, `clients/`, `estimates/`, `invoices/`, `settings/` → `(app)/catalog/`, etc.
8. **Verify app works** — no URL changes, all navigation and active states still correct.

### Phase 3: Docs Components

9. **`src/lib/components/docs/DocsNavbar.svelte`** — Simpler nav bar with: logo, "Home"/"Getting Started"/"Guides" links, "Open App" button, theme toggle, mobile hamburger.
10. **`src/lib/components/docs/DocsSidebar.svelte`** — Static sidebar with 2 sections (Introduction: 4 items, Guides: 7 items). Highlights active page via `page.url.pathname`.
11. **`src/lib/components/docs/Callout.svelte`** — Replaces VitePress `::: tip` / `::: warning` containers. Props: `type="tip"|"warning"`. Only 2 usages exist.

### Phase 4: Docs Routes

12. **`src/routes/(docs)/+layout.svelte`** — Docs layout: `DocsNavbar` + sidebar + `<main class="prose dark:prose-invert">`.
13. **`src/routes/(docs)/docs/+page.svelte`** — Hero landing page (converted from VitePress `layout: home` frontmatter in `index.md`).
14. **Move and rename each markdown file** to SvelteKit route locations:
    - `src/docs/getting-started.md` → `src/routes/(docs)/docs/getting-started/+page.md`
    - `src/docs/features.md` → `src/routes/(docs)/docs/features/+page.md`
    - `src/docs/architecture.md` → `src/routes/(docs)/docs/architecture/+page.md`
    - `src/docs/guides/*.md` → `src/routes/(docs)/docs/guides/*/+page.md` (7 files)
15. **Edit 2 markdown files** to replace container syntax with `<Callout>` component:
    - `features/+page.md`: replace `::: tip` block
    - `guides/settings/+page.md`: replace `::: warning` block

### Phase 5: Navigation & Cleanup

16. **`src/lib/components/layout/Navbar.svelte`** — Add a "Docs" link to the app navbar.
17. **`.github/workflows/deploy.yml`** — Remove `npm run docs:build`, remove docs assembly (`mkdir deploy/docs`, `cp` VitePress dist), simplify 404.html to just `cp deploy/index.html deploy/404.html`.
18. **`.husky/pre-commit`** — Remove `npm run docs:build` line.
19. **Delete `src/docs/`** entirely (all content moved or replaced).

### Phase 6: Update Documentation

20. Update `CLAUDE.md` and `architecture.md` to remove VitePress references and document the new `(app)`/`(docs)` route groups.

---

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Markdown preprocessor | mdsvex | Standard for SvelteKit, preserves .md authoring |
| Layout isolation | Route groups `(app)` / `(docs)` | No URL changes, clean layout separation |
| Container syntax | `<Callout>` Svelte component | Only 2 usages — simpler than a remark plugin |
| Prose styling | `@tailwindcss/typography` | Handles all markdown elements (tables, code, lists) |
| Search | Deferred | 11 small pages — sidebar nav is sufficient. Can add Pagefind later |
| Docs landing page | Svelte component (not .md) | VitePress hero frontmatter can't be expressed in plain markdown |

---

## What Gets Removed

- `vitepress` dependency
- `src/docs/.vitepress/` (config, theme, dist)
- `navigateFallbackDenylist` workaround in `vite.config.ts`
- Conditional 404.html logic in deploy workflow
- `docs:dev`, `docs:build`, `docs:preview`, `build:all` scripts
- `npm run docs:build` in pre-commit hook

---

## Verification

1. `npm run dev` — navigate to `/docs`, `/docs/getting-started`, `/docs/guides/invoices`, etc. Verify sidebar, prose styling, dark mode, callout components.
2. Navigate between app and docs — confirm no full page reload, `FileGate` only activates on app routes.
3. `npm run build` — single build produces all routes including docs.
4. `npm run test` — existing tests still pass.
5. Service worker — verify docs pages are cached by the SW like any other app route (no denylist).

---

## Risks

- **mdsvex + Svelte 5 compatibility** — mdsvex v0.12+ supports Svelte 5. Verify at install time.
- **Route group migration** — Moving files into `(app)/` changes file paths but not URLs. Must test all navigation.
- **Tailwind Typography + CSS 4** — Verify `@plugin` import syntax works with `@tailwindcss/typography`.
