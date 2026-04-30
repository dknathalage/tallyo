# Tallyo

Self-hosted, open-source invoice management app distributed as a global npm CLI. SQLite via better-sqlite3 + Drizzle ORM.

## Tech Stack

- **Framework:** SvelteKit with Svelte 5, TypeScript (strict), Node adapter
- **Styling:** Tailwind CSS 4 via Vite plugin
- **Database:** SQLite via better-sqlite3 + Drizzle ORM
- **PDF:** jsPDF + autotable
- **Import/Export:** PapaParse (CSV), XLSX (Excel)
- **Testing:** Vitest
- **Distribution:** Global npm CLI (`tallyo`) via `bin/tallyo.js`

## Project Layout

- `bin/tallyo.js` — CLI entry: resolves data dir, picks port, runs migrations, boots SvelteKit Node server, opens browser
- `src/lib/` — Shared library: components, database, utilities
- `src/lib/db/` — Database connection, schema, migrations, query modules
- `src/lib/repositories/` — Data access layer (interfaces + SQLite implementations)
- `src/lib/utils/` — Helpers (currency, formatting, PDF)
- `src/routes/` — SvelteKit pages and server load functions
- `drizzle/` — Generated Drizzle migrations (shipped in published tarball)
- `build/` — SvelteKit Node adapter output (built before publish)

## Run

End users:

```bash
npm install -g tallyo
tallyo
```

Local dev:

```bash
npm run dev          # Vite dev server at http://localhost:5173
npm run build        # Production build into build/
npm link             # Use `tallyo` globally from working tree
```

## Commands

- `npm run dev` — Start dev server (http://localhost:5173)
- `npm run build` — Production build
- `npm test` — Run Vitest tests
- `npm run test:coverage` — Run tests with coverage report
- `npm run check` — TypeScript check
- `npx drizzle-kit generate` — Generate Drizzle migrations
- `npx drizzle-kit migrate` — Run Drizzle migrations

## Conventions

- Database queries live in `src/lib/db/queries/` with co-located `.test.ts` files
- Use repositories (`$lib/repositories`) in routes — never `$lib/db/queries` directly
- Components are grouped by domain under `src/lib/components/`
- All database mutations are audit-logged
- Commits follow Conventional Commits

## Database

- SQLite (better-sqlite3) + Drizzle ORM
- Migrations managed by Drizzle Kit (`npx drizzle-kit generate` / `npx drizzle-kit migrate`)
- Database file stored in `DATA_DIR` (default: `~/.tallyo/tallyo.db`); `bin/tallyo.js` runs migrations on startup

## Coding Rules (NASA Power of 10, adapted)

Apply to all new and modified code. Adapted from JPL's "Power of Ten" for safety-critical C; reinterpreted for TypeScript/Svelte.

1. **Simple control flow.** No `goto`, no recursion (unless provably bounded and justified in a comment). Prefer flat early-return over nested branching.
2. **Bounded loops.** Every loop must have a statically obvious upper bound. No `while (true)` without an explicit break condition tied to a bounded counter or external signal.
3. **No dynamic allocation after init.** In hot paths, avoid allocating new objects/arrays per iteration. Pre-size arrays, reuse buffers, and prefer iteration over `.map().filter().reduce()` chains when shape is fixed.
4. **Short functions.** Aim for ≤ 60 lines per function (one screen). Split when a function does more than one thing.
5. **Assertion density.** At least two runtime checks per non-trivial function — validate inputs at module boundaries (HTTP, DB, file I/O, user input). Use `throw new Error(...)` for invariant violations; never silently coerce.
6. **Smallest scope for data.** Declare variables at innermost scope. No module-level mutable state unless it represents a singleton resource (DB connection, i18n store). Prefer `const`; use `let` only when reassigned.
7. **Check every return value.** No ignored Promises, no swallowed errors. Every `await` is either inside a `try/catch` or its rejection is a documented programmer error. No bare `catch {}` — log or rethrow.
8. **Limit preprocessor / metaprogramming.** Avoid `eval`, `Function()`, dynamic `import()` of computed paths, and clever type gymnastics. Prefer explicit code over generated code.
9. **Restrict pointer/reference indirection.** Limit object-graph traversal to one level of optional chaining per expression. Destructure once at function entry rather than reaching deep into arguments throughout the body.
10. **Compile clean at max strictness.** Zero TypeScript errors, zero `svelte-check` warnings, zero ESLint warnings on every commit. `// @ts-ignore`, `// svelte-ignore`, and `any` require an inline comment explaining why.
