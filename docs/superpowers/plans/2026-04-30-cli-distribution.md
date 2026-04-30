# CLI Distribution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert Tallyo from a Docker-deployed Turborepo monorepo into a globally installable npm CLI (`npm install -g tallyo` → `tallyo` opens app in browser).

**Architecture:** Flatten `apps/app/*` to repo root, drop turbo/husky/commitlint/Docker/CI, add a thin `bin/tallyo.js` orchestrator that resolves data dir, picks a free port, runs Drizzle migrations programmatically, boots the SvelteKit Node adapter in-process, and opens the browser.

**Tech Stack:** Node.js (ESM), SvelteKit (Node adapter), better-sqlite3, Drizzle ORM, `get-port`, `open`, Vitest.

**Reference:** Spec at `docs/superpowers/specs/2026-04-30-cli-distribution-design.md`.

---

## Task 1: Delete Docker, CI, and dev-tooling files

**Files:**
- Delete: `Dockerfile`, `docker-compose.yml`
- Delete: `.github/` (entire directory)
- Delete: `.husky/` (entire directory)
- Delete: `commitlint.config.js`, `COMMIT_CONVENTION.md`, `cliff.toml`
- Delete: `scripts/` (entire directory)
- Delete: `turbo.json`

- [ ] **Step 1: Remove files**

```bash
cd /Users/dknathalage/repos/random/tallyo
rm -rf Dockerfile docker-compose.yml .github .husky commitlint.config.js COMMIT_CONVENTION.md cliff.toml scripts turbo.json
```

- [ ] **Step 2: Verify**

```bash
ls -A | grep -E '^(Dockerfile|docker-compose|\.github|\.husky|commitlint|COMMIT_CONVENTION|cliff|scripts|turbo)'
```

Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "chore: remove docker, CI, husky, commitlint, turbo"
```

---

## Task 2: Flatten monorepo — move `apps/app/*` to repo root

**Files:**
- Move: `apps/app/src` → `src`
- Move: `apps/app/static` → `static`
- Move: `apps/app/drizzle` → `drizzle`
- Move: `apps/app/drizzle.config.ts` → `drizzle.config.ts`
- Move: `apps/app/svelte.config.js` → `svelte.config.js`
- Move: `apps/app/vite.config.ts` → `vite.config.ts`
- Move: `apps/app/tsconfig.json` → `tsconfig.json`
- Move: `apps/app/package.json` → `app-package.json` (temp; merged in Task 3)
- Move: any other top-level files in `apps/app/` (README, .gitignore, etc.) — inspect first
- Delete: `apps/` directory after move

- [ ] **Step 1: Inspect apps/app contents**

```bash
ls -A apps/app
```

Note any files not listed above; move them too.

- [ ] **Step 2: Move directories and configs**

```bash
git mv apps/app/src src
git mv apps/app/static static
git mv apps/app/drizzle drizzle
git mv apps/app/drizzle.config.ts drizzle.config.ts
git mv apps/app/svelte.config.js svelte.config.js
git mv apps/app/vite.config.ts vite.config.ts
git mv apps/app/tsconfig.json tsconfig.json
git mv apps/app/package.json app-package.json
```

If `ls -A apps/app` showed extra files (e.g. `.gitignore`, `README.md`, `.npmrc`), `git mv` them too. If a `.gitignore` exists in both root and `apps/app`, merge manually before deleting.

- [ ] **Step 3: Remove now-empty apps directory**

```bash
rmdir apps/app apps
```

If `rmdir` fails (residual files), inspect and either move or delete those files first.

- [ ] **Step 4: Verify project still loads**

```bash
ls src static drizzle drizzle.config.ts svelte.config.js vite.config.ts tsconfig.json app-package.json
```

Expected: all listed.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: flatten apps/app to repo root"
```

---

## Task 3: Merge `app-package.json` into root `package.json`

**Files:**
- Modify: `package.json` (replace contents)
- Delete: `app-package.json` (temp file from Task 2)

- [ ] **Step 1: Read both files**

```bash
cat package.json app-package.json
```

- [ ] **Step 2: Write merged `package.json`**

Replace `package.json` contents with:

```json
{
  "name": "tallyo",
  "version": "1.0.2",
  "description": "Self-hosted invoice manager. Run `tallyo` to launch.",
  "type": "module",
  "bin": {
    "tallyo": "bin/tallyo.js"
  },
  "files": [
    "bin",
    "build",
    "drizzle"
  ],
  "scripts": {
    "dev": "vite dev",
    "build": "vite build",
    "start": "node bin/tallyo.js",
    "test": "vitest run",
    "test:coverage": "vitest run --coverage",
    "check": "svelte-kit sync && svelte-check --tsconfig ./tsconfig.json"
  },
  "dependencies": {
    "@anthropic-ai/sdk": "^0.79.0",
    "@tailwindcss/typography": "^0.5.19",
    "@tailwindcss/vite": "^4.2.1",
    "better-sqlite3": "^12.8.0",
    "drizzle-orm": "^0.44.0",
    "get-port": "^7.1.0",
    "jspdf": "^4.2.0",
    "jspdf-autotable": "^5.0.7",
    "mdsvex": "^0.12.6",
    "open": "^10.1.0",
    "papaparse": "^5.5.3",
    "tailwindcss": "^4.2.1",
    "xlsx": "^0.18.5",
    "zod": "^4.3.6"
  },
  "devDependencies": {
    "@sveltejs/adapter-auto": "^7.0.0",
    "@sveltejs/adapter-node": "^5.5.4",
    "@sveltejs/kit": "^2.50.2",
    "@sveltejs/vite-plugin-svelte": "^6.2.4",
    "@types/better-sqlite3": "^7.6.12",
    "@types/papaparse": "^5.5.2",
    "@vitest/coverage-v8": "^4.1.0",
    "drizzle-kit": "^0.31.0",
    "svelte": "^5.51.0",
    "svelte-check": "^4.3.6",
    "typescript": "^5.9.3",
    "vite": "^7.3.1",
    "vitest": "^4.1.0"
  }
}
```

Notes:
- Drop `private: true`, `workspaces`, `packageManager`, `prepare`, all turbo/husky/commitlint deps.
- Add `get-port` and `open` to deps (needed by `bin/tallyo.js`).
- Drop `prepublishOnly` for now — keep manual `npm run build` before publish to avoid breaking `npm install` when `build/` isn't needed (e.g. dev clones). Document this in README.

- [ ] **Step 3: Delete temp file and lockfile**

```bash
rm app-package.json package-lock.json
```

- [ ] **Step 4: Reinstall**

```bash
npm install
```

Expected: clean install, generates new `package-lock.json`.

- [ ] **Step 5: Verify scripts work**

```bash
npm run check
npm test
npm run build
```

Expected: all pass. `build/` directory exists after build.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: merge app package.json into root, add get-port and open deps"
```

---

## Task 4: Confirm SvelteKit uses Node adapter and `HOST`/`PORT` env

**Files:**
- Verify: `svelte.config.js`

- [ ] **Step 1: Read `svelte.config.js`**

Confirm `@sveltejs/adapter-node` is the active adapter. If it's `adapter-auto`, switch:

```js
import adapter from '@sveltejs/adapter-node';
```

The Node adapter respects `HOST`, `PORT`, `ORIGIN`, `BODY_SIZE_LIMIT` env vars by default — no other config needed.

- [ ] **Step 2: Build and smoke-test**

```bash
npm run build
DATA_DIR=/tmp/tallyo-test PORT=3999 HOST=127.0.0.1 node build/index.js &
sleep 2
curl -sf http://127.0.0.1:3999/health
kill %1
rm -rf /tmp/tallyo-test
```

Expected: `{"status":"ok","db":"connected"}`.

- [ ] **Step 3: Commit (only if `svelte.config.js` changed)**

```bash
git add svelte.config.js
git commit -m "chore: ensure svelte uses node adapter"
```

---

## Task 5: Write `bin/tallyo.js` CLI entry

**Files:**
- Create: `bin/tallyo.js`

- [ ] **Step 1: Create `bin/` directory and write entry**

```bash
mkdir -p bin
```

Write `bin/tallyo.js`:

```js
#!/usr/bin/env node
import { mkdirSync } from 'node:fs';
import { homedir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { parseArgs } from 'node:util';
import getPort from 'get-port';
import open from 'open';
import Database from 'better-sqlite3';
import { drizzle } from 'drizzle-orm/better-sqlite3';
import { migrate } from 'drizzle-orm/better-sqlite3/migrator';

const __dirname = dirname(fileURLToPath(import.meta.url));
const pkgRoot = resolve(__dirname, '..');

const { values } = parseArgs({
  options: {
    port: { type: 'string' },
    'data-dir': { type: 'string' },
    'no-open': { type: 'boolean', default: false },
    help: { type: 'boolean', short: 'h', default: false },
    version: { type: 'boolean', short: 'v', default: false },
  },
});

if (values.help) {
  console.log(`Usage: tallyo [options]

Options:
  --port <n>           Port to bind (default: first free port from 3000)
  --data-dir <path>    Data directory (default: $DATA_DIR or ~/.tallyo)
  --no-open            Do not auto-open browser
  -h, --help           Show this help
  -v, --version        Show version
`);
  process.exit(0);
}

if (values.version) {
  const { default: pkg } = await import(pathToFileURL(join(pkgRoot, 'package.json')), { with: { type: 'json' } });
  console.log(pkg.version);
  process.exit(0);
}

const dataDir = values['data-dir'] ?? process.env.DATA_DIR ?? join(homedir(), '.tallyo');
mkdirSync(dataDir, { recursive: true });

const requestedPort = values.port ? Number(values.port) : undefined;
const port = await getPort({ port: requestedPort ?? [3000, 3001, 3002, 3003, 3004, 3005] });
if (requestedPort && port !== requestedPort) {
  console.error(`Port ${requestedPort} is in use.`);
  process.exit(1);
}

const dbPath = join(dataDir, 'tallyo.db');
const sqlite = new Database(dbPath);
const db = drizzle(sqlite);
migrate(db, { migrationsFolder: join(pkgRoot, 'drizzle') });
sqlite.close();

process.env.PORT = String(port);
process.env.HOST = '127.0.0.1';
process.env.DATA_DIR = dataDir;
process.env.NODE_ENV ??= 'production';

const url = `http://localhost:${port}`;
console.log(`Tallyo running at ${url}`);
console.log(`Data: ${dataDir}`);

const shutdown = () => process.exit(0);
process.on('SIGINT', shutdown);
process.on('SIGTERM', shutdown);

await import(pathToFileURL(join(pkgRoot, 'build', 'index.js')).href);

if (!values['no-open']) {
  open(url).catch(() => {});
}
```

- [ ] **Step 2: Make executable**

```bash
chmod +x bin/tallyo.js
```

- [ ] **Step 3: Build app**

```bash
npm run build
```

Expected: `build/index.js` exists.

- [ ] **Step 4: Smoke-test CLI with custom data dir**

```bash
DATA_DIR=/tmp/tallyo-cli-test node bin/tallyo.js --no-open --port 3998 &
sleep 2
curl -sf http://127.0.0.1:3998/health
kill %1
rm -rf /tmp/tallyo-cli-test
```

Expected: `{"status":"ok","db":"connected"}`.

- [ ] **Step 5: Smoke-test auto-port (no flag)**

```bash
DATA_DIR=/tmp/tallyo-cli-test2 node bin/tallyo.js --no-open &
PID=$!
sleep 2
# Note the port from stdout — should be 3000 or next free
kill $PID
rm -rf /tmp/tallyo-cli-test2
```

Expected: prints "Tallyo running at http://localhost:<port>".

- [ ] **Step 6: Smoke-test --help and --version**

```bash
node bin/tallyo.js --help
node bin/tallyo.js --version
```

Expected: help text; version `1.0.2`.

- [ ] **Step 7: Commit**

```bash
git add bin/tallyo.js package.json package-lock.json
git commit -m "feat: add tallyo CLI entry"
```

---

## Task 6: Verify global install via `npm link`

**Files:** none (verification only)

- [ ] **Step 1: Link**

```bash
npm link
```

- [ ] **Step 2: Run global command**

```bash
which tallyo
tallyo --version
```

Expected: path printed; version `1.0.2`.

- [ ] **Step 3: Run with --no-open and verify**

```bash
DATA_DIR=/tmp/tallyo-link-test tallyo --no-open --port 3997 &
sleep 2
curl -sf http://127.0.0.1:3997/health
kill %1
rm -rf /tmp/tallyo-link-test
```

Expected: health response.

- [ ] **Step 4: Unlink (cleanup)**

```bash
npm unlink -g tallyo
```

No commit needed; verification only.

---

## Task 7: Rewrite README

**Files:**
- Modify: `README.md` (replace contents)

- [ ] **Step 1: Write new README**

Replace `README.md` with:

```markdown
# Tallyo

Self-hosted invoice manager. Local-first, SQLite-backed.

## Install

\`\`\`bash
npm install -g tallyo
\`\`\`

## Run

\`\`\`bash
tallyo
\`\`\`

Picks the first free port starting at 3000, runs migrations, opens your browser.

## Options

| Flag | Default | Description |
|------|---------|-------------|
| `--port <n>` | first free from 3000 | Force a specific port |
| `--data-dir <path>` | `~/.tallyo` | Where the SQLite db and config live |
| `--no-open` | off | Don't auto-open the browser |
| `-h, --help` | | Show help |
| `-v, --version` | | Show version |

`DATA_DIR` env var is also respected.

## Data

Everything (database, config) lives in `~/.tallyo` by default. Back this directory up to back up your invoices.

## Develop

\`\`\`bash
git clone <repo>
cd tallyo
npm install
npm run dev          # Vite dev server at http://localhost:5173
npm run build        # Production build
npm test             # Vitest
npm link             # Use `tallyo` globally from working tree
\`\`\`

## License

See LICENSE.
```

(Use real backticks — the escaping above is for nesting.)

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: rewrite README for CLI distribution"
```

---

## Task 8: Update `CLAUDE.md` to reflect new structure

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update CLAUDE.md**

Rewrite the relevant sections:
- Drop `apps/app/` paths — use root paths (`src/lib/...`).
- Drop Turborepo references.
- Drop "Production Deployment", "Docker Compose", "Environment Variables" tables.
- Add a "Run" section pointing at `tallyo` CLI and `npm run dev`.
- Update commands list: drop `npm run --workspace=...`; keep `npm run dev/build/test/check`.

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for flattened CLI repo"
```

---

## Task 9: Final verification

- [ ] **Step 1: Clean build from scratch**

```bash
rm -rf node_modules build
npm install
npm run check
npm test
npm run build
```

Expected: all pass.

- [ ] **Step 2: End-to-end CLI smoke test**

```bash
DATA_DIR=/tmp/tallyo-final node bin/tallyo.js --no-open --port 3996 &
sleep 2
curl -sf http://127.0.0.1:3996/health
kill %1
rm -rf /tmp/tallyo-final
```

Expected: health OK.

- [ ] **Step 3: Inspect what would publish**

```bash
npm pack --dry-run
```

Expected tarball contains: `bin/`, `build/`, `drizzle/`, `package.json`, `README.md`, `LICENSE`. Should NOT contain `src/`, `static/`, `node_modules/`, `tests`, configs.

- [ ] **Step 4: Final commit (if anything changed)**

No code changes expected at this step. If verification surfaced issues, fix and commit.
