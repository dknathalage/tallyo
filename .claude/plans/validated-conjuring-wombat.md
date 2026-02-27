# Plan: Move SvelteKit to root, VitePress to /docs/

## Context

The current deployment puts VitePress docs at `/invoices/` (root) and the SvelteKit app at `/invoices/console/`. This causes:
- VitePress router intercepts clicks to the app, showing its 404 page
- PWA installs with the VitePress landing page instead of the SvelteKit app
- Service worker `non-precached-url` errors from scope/path mismatches

**Goal:** Swap the hierarchy — SvelteKit at `/invoices/` (root), VitePress docs at `/invoices/docs/`.

## Changes

### 1. `svelte.config.js` — SvelteKit base path
```diff
- base: dev ? '' : '/invoices/console'
+ base: dev ? '' : '/invoices'
```

### 2. `vite.config.ts` — Vite base path + SW denylist
```diff
- const basePath = dev ? '' : '/invoices/console';
+ const basePath = dev ? '' : '/invoices';
```
```diff
- navigateFallbackDenylist: [],
+ navigateFallbackDenylist: [/^\/invoices\/docs\//],
```
The denylist prevents the service worker from intercepting docs navigations and serving the SvelteKit SPA fallback.

### 3. `src/docs/.vitepress/config.ts` — VitePress base + nav link
```diff
- base: '/invoices/',
+ base: '/invoices/docs/',
```
```diff
- { text: 'Open App', link: '/console/', target: '_self' }
+ { text: 'Open App', link: 'https://dknathalage.github.io/invoices/', target: '_self' }
```
VitePress's `normalizeLink` prepends base to all non-external links, so an absolute URL is required to link outside the docs.

### 4. `src/docs/index.md` — Hero "Open App" button
```diff
- link: /console/
+ link: https://dknathalage.github.io/invoices/
```

### 5. `.github/workflows/deploy.yml` — Deploy assembly
Flip the directory layout:
```diff
- mkdir -p deploy/console
- cp -r src/docs/.vitepress/dist/* deploy/
- cp -r build/* deploy/console/
+ mkdir -p deploy/docs
+ cp -r build/* deploy/
+ cp -r src/docs/.vitepress/dist/* deploy/docs/
```
Update 404.html to skip docs paths and redirect everything else to the SvelteKit SPA:
```
if path starts with "/invoices/docs" → do nothing
else if path starts with "/invoices" → redirect to /invoices/ (SPA)
```

### 6. `src/docs/CLAUDE.md` — Fix deployed path
```diff
- deployed at `/invoices/web/`.
+ deployed at `/invoices/docs/`.
```

## Files that need NO changes
- `src/app.html` — SPA redirect handler is path-generic
- `src/lib/components/layout/Navbar.svelte` — uses `base` from `$app/paths`, adapts automatically
- `src/lib/components/pwa/ReloadPrompt.svelte` — no hardcoded paths
- PWA manifest (`start_url: '.'`, `scope: '.'`) — resolves correctly to `/invoices/`

## Verification
1. `npm run build` — SvelteKit builds with `/invoices` base
2. `npm run docs:build` — VitePress builds with `/invoices/docs/` base
3. `npm test` — all tests pass
4. Simulate deploy assembly locally and confirm `deploy/index.html` is SvelteKit, `deploy/docs/index.html` is VitePress
