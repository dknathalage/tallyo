# CLI Distribution Redesign

Date: 2026-04-30
Status: Approved

## Goal

Convert Tallyo from a Docker-deployed monorepo into a globally-installable npm CLI. Users run `npm install -g tallyo`, then `tallyo`, and the app opens in their browser.

## Non-goals

- CI/CD pipelines, release automation, commit hooks, conventional-commit enforcement.
- Docker, container orchestration, or remote deployment tooling.
- Auto-update mechanism (npm handles updates).
- Cross-platform install testing beyond what cross-platform deps provide.

## Repo structure (flattened)

Move `apps/app/*` to repo root. Drop the monorepo layer.

```
tallyo/
  bin/tallyo.js
  src/
  static/
  drizzle/
  drizzle.config.ts
  svelte.config.js
  vite.config.ts
  tsconfig.json
  package.json
  README.md
  LICENSE
```

Removed entirely:
`Dockerfile`, `docker-compose.yml`, `.github/`, `.husky/`, `commitlint.config.js`, `COMMIT_CONVENTION.md`, `cliff.toml`, `scripts/`, `turbo.json`, `apps/` (after move).

## CLI entry: `bin/tallyo.js`

Node ESM script with shebang `#!/usr/bin/env node`. Steps on invocation:

1. Parse args: `--port <n>`, `--data-dir <path>`, `--no-open`, `--help`, `--version`.
2. Resolve `DATA_DIR` (flag > `DATA_DIR` env > `~/.tallyo`). Create directory if missing.
3. Pick port: if `--port` given, use it (fail if taken). Otherwise use `get-port` starting at 3000, falling through if taken.
4. Run pending Drizzle migrations programmatically via `drizzle-orm/better-sqlite3/migrator`'s `migrate()` against `${DATA_DIR}/tallyo.db`, using bundled `drizzle/` folder.
5. Set `process.env.PORT`, `HOST=127.0.0.1`, `DATA_DIR`, `NODE_ENV=production`.
6. Dynamically `import('../build/index.js')` to boot the SvelteKit Node adapter in-process.
7. Print `Tallyo running at http://localhost:<port>`. Open URL via `open` package unless `--no-open`.
8. Handle `SIGINT` / `SIGTERM` for graceful exit.

New runtime deps: `get-port`, `open`. Existing deps `better-sqlite3` and `drizzle-orm` already present.

## package.json (merged, single root)

```json
{
  "name": "tallyo",
  "version": "1.0.2",
  "type": "module",
  "bin": { "tallyo": "bin/tallyo.js" },
  "files": ["bin", "build", "drizzle"],
  "scripts": {
    "dev": "vite dev",
    "build": "vite build",
    "start": "node bin/tallyo.js",
    "test": "vitest run",
    "test:coverage": "vitest run --coverage",
    "check": "svelte-kit sync && svelte-check --tsconfig ./tsconfig.json",
    "prepublishOnly": "npm run build"
  }
}
```

- Merge `devDependencies` and `dependencies` from `apps/app/package.json`.
- Drop: `turbo`, `husky`, `@commitlint/*`, `private: true` flag.
- `files` whitelist keeps the published tarball lean (built output + CLI + migrations only).
- `prepublishOnly` guards against publishing without a fresh build.

## Data directory

- Default `~/.tallyo` (preserves existing installs).
- Override via `--data-dir` flag or `DATA_DIR` env.
- Contains `tallyo.db` and `config.json`.

## Developer workflow

```
npm install
npm run dev      # Vite dev server (unchanged)
npm run build    # builds SvelteKit
npm link         # exposes `tallyo` globally from working tree
tallyo           # smoke-test the CLI
```

## End-user workflow

```
npm install -g tallyo
tallyo
# → Tallyo running at http://localhost:3000
# → browser opens automatically
```

Flags: `tallyo --port 4000`, `tallyo --data-dir /tmp/tally`, `tallyo --no-open`.

## Tests

Keep all 224 Vitest tests. Run with `npm test`. No coverage of CLI entry required (it is a thin orchestrator).

## README

Rewrite to cover: install, run, flags, data location. Remove Docker, deploy, health-check, env-var matrix sections.
