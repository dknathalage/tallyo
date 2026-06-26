# Tallyo Containerization Plan (Plan 2 of 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to run this plan. Work top-to-bottom, one task at a time; tick each checkbox (`- [ ]`) as its step completes and only move on once its verification output matches. Do not batch tasks.

**Goal:** Produce a single OCI image that builds the SvelteKit SPA, embeds it into the cgo-free `cmd/tallyo` Go binary, and runs it on distroless — plus a `docker-compose.yml` that brings up Postgres 17 + the app so one `docker compose up` yields a working, auto-migrated local stack. The exact same image is what Plan 3 pushes to Artifact Registry and runs on Cloud Run.

**Architecture:** Multi-stage Docker build. Stage 1 (`node:22-alpine`) compiles `web/` → `web/build`. Stage 2 (`golang:1.26`) builds `CGO_ENABLED=0 go build -o /tallyo ./cmd/tallyo` with `web/build` present so `//go:embed all:build` succeeds. Stage 3 (`gcr.io/distroless/static:nonroot`) ships just `/tallyo`, runs as non-root, listens on 8080. The app is Postgres-only (Plan 1 merged): it reads `DATABASE_URL`, runs goose migrations on startup, has no background work, and serves the embedded SPA via the SPA catch-all. `docker-compose.yml` wires a `postgres:17` service (named volume + `pg_isready` healthcheck) to the `app` service via `DATABASE_URL=postgres://tallyo:tallyo@db:5432/tallyo?sslmode=disable`, with `depends_on: db: condition: service_healthy`.

**Tech Stack:** Docker multi-stage build (BuildKit), `node:22-alpine`, `golang:1.26`, `gcr.io/distroless/static:nonroot`, `postgres:17`, Docker Compose v2. App: Go 1.26 single binary (`cmd/tallyo`), SvelteKit static SPA embedded via `//go:embed`, Postgres via `DATABASE_URL`.

**Spec:** docs/superpowers/specs/2026-06-26-postgres-gcp-migration-design.md

---

## Assumptions (read before starting)

- **Plan 1 MUST be merged first (enforced gate, not just an assumption).** This plan produces a *correct* image only against the Postgres app. If run on the current SQLite code, `docker build` still succeeds (modernc is pure-Go, so `CGO_ENABLED=0` passes) and the container starts — but it ignores `DATABASE_URL`, opens a SQLite file, and never touches the compose Postgres. That failure is silent. **Before Task 1, run the gate below; do not proceed (and do not use Task 0's "deferred to CI" path) until it passes:**
  ```bash
  cd /Users/dknathalage/repos/tallyo
  grep -rq "DATABASE_URL" internal/app && grep -q "jackc/pgx" go.mod \
    && ! grep -q "modernc.org/sqlite" go.mod \
    && echo "Plan 1 merged — OK to containerize" \
    || { echo "STOP: Plan 1 not merged (app is still SQLite). Finish Plan 1 first."; exit 1; }
  ```
- **Port is 8080.** `cmd/tallyo/main.go` defaults `--port` to `8080` (verified). The app does not read a `PORT` env var; the binary listens on `8080` unless `--port` is passed. The Dockerfile `EXPOSE`s 8080 and compose maps `8080:8080`.
- **SPA embed contract.** `web/embed.go` is `//go:embed all:build` (verified) → the Go build **requires** `web/build` to exist in the build context at compile time. Stage ordering must guarantee that. `web/package-lock.json` exists (verified) so `npm ci` works.
- **No Docker on the dev machine.** Verification commands that need Docker must run wherever Docker is available (CI or a machine with Docker installed). See Task 0.
- **Module path** `github.com/dknathalage/tallyo`; the Dockerfile uses `./cmd/tallyo` so the module path is not hard-coded.

---

## File-structure map

All new, at repo root; no existing code modified:

```
tallyo/
  Dockerfile             # NEW — 3-stage: SPA build → Go build (embeds SPA) → distroless runtime
  .dockerignore          # NEW — keep build context small; exclude data/, .git, node_modules, legacy Electron tree, *.db
  docker-compose.yml     # NEW — postgres:17 (volume + healthcheck) + app (built from Dockerfile, DATABASE_URL → db)
```

---

## Task 0 — Prerequisite: a Docker-capable environment

**Files:** none.

The dev machine has no Docker. Every `docker`/`docker compose` command below MUST run where Docker is available. Authoring Tasks 1–3 needs no Docker; only verification does.

- [ ] **Step 1:** `docker version && docker compose version` → both print client+server with no error.
- [ ] **Step 2 (if absent):** macOS — `brew install --cask docker` then launch Docker.app once; or `brew install colima docker docker-compose && colima start`. Re-run `docker version` until the `Server:` block appears.
- [ ] **Step 3:** Run the **Plan-1 gate** from the Assumptions block above; it must print "OK to containerize". If it fails, STOP — finish Plan 1 first (a pre-migration image is silently wrong). The deferred-to-CI path below is allowed ONLY after the gate passes.
- [ ] **Step 4:** If no Docker environment is reachable (but the Plan-1 gate passed), author Tasks 1–3, commit, and mark their Docker-only verifications **deferred to CI** in the commit bodies.

---

## Task 1 — `.dockerignore`

Author first so it constrains the Task 2 build context (prevents stale local `web/build`, `data/`, `*.db` leaking into the image).

**Files:** Create `/Users/dknathalage/repos/tallyo/.dockerignore`.

- [ ] **Step 1:** Create `.dockerignore`:
  ```gitignore
  # --- VCS / tooling ---
  .git
  .gitignore
  .github
  .dockerignore
  *.md
  docs/

  # --- runtime data / local DB (never copy into the image) ---
  data/
  *.db
  *.db-shm
  *.db-wal
  *.sqlite
  *.sqlite3

  # --- node / SPA artifacts (stage 1 runs a clean build from source) ---
  node_modules/
  web/node_modules/
  web/build/
  web/.svelte-kit/
  web/.vite/
  .npm

  # --- Go local build outputs ---
  bin/
  /tallyo
  *.test
  *.out
  coverage*

  # --- legacy Electron/SvelteKit tree (superseded, not part of the image) ---
  src/
  electron/
  drizzle/
  package.json
  package-lock.json
  pnpm-lock.yaml
  yarn.lock
  svelte.config.js
  vite.config.*

  # --- editor / OS noise ---
  .vscode/
  .idea/
  .DS_Store
  *.log
  .env
  .env.*
  docker-compose.yml
  Dockerfile
  ```
  Note: excluding `web/build/` is intentional — stage 1 rebuilds the SPA from source. Excluding root `package.json`/`vite.config.*`/`svelte.config.js` targets the **legacy** root Electron tooling; the active SPA tooling under `web/` is preserved (only `web/node_modules`, `web/build`, `web/.svelte-kit` excluded).
- [ ] **Step 2:** `test -f .dockerignore && grep -qE '^data/$' .dockerignore && grep -qE '^src/$' .dockerignore && grep -qE '^web/node_modules/$' .dockerignore && echo OK` → `OK`.
- [ ] **Step 3:** Commit: `build: add .dockerignore for container build context`

---

## Task 2 — Multi-stage `Dockerfile`

**Files:** Create `/Users/dknathalage/repos/tallyo/Dockerfile`.

- [ ] **Step 1:** Create `Dockerfile`:
  ```dockerfile
  # syntax=docker/dockerfile:1

  # --- Stage 1 — build the SvelteKit SPA -> web/build (consumed by //go:embed).
  FROM node:22-alpine AS web
  WORKDIR /src/web
  COPY web/package.json web/package-lock.json ./
  RUN npm ci
  COPY web/ ./
  RUN npm run build

  # --- Stage 2 — build the cgo-free Go binary with the SPA present so
  # //go:embed all:build in web/embed.go can embed web/build.
  FROM golang:1.26 AS build
  WORKDIR /src
  COPY go.mod go.sum ./
  RUN go mod download
  COPY . .
  # web/build is .dockerignore'd from the host context on purpose; it comes
  # from stage 1, never from the host.
  COPY --from=web /src/web/build ./web/build
  ENV CGO_ENABLED=0 GOOS=linux GOFLAGS=-trimpath
  RUN go build -ldflags="-s -w" -o /tallyo ./cmd/tallyo

  # --- Stage 3 — minimal runtime. distroless/static:nonroot: no shell, non-root
  # uid 65532, ships CA certs for outbound TLS (Anthropic API).
  FROM gcr.io/distroless/static:nonroot AS final
  WORKDIR /
  COPY --from=build /tallyo /tallyo
  EXPOSE 8080
  USER nonroot:nonroot
  ENTRYPOINT ["/tallyo"]
  ```
  Notes: stage ordering is load-bearing — stage 2 overlays `web/build` from stage 1 *before* `go build` (embed fails to compile without it; `.dockerignore` ensures the embedded SPA is the freshly-built one). `distroless/static:nonroot` fits a cgo-free static binary and includes CA certs for the Smarts HTTPS calls.
- [ ] **Step 2:** `grep -c '^FROM ' Dockerfile` → `3`. (If `hadolint` available: `hadolint Dockerfile` → no errors.)
- [ ] **Step 3 (Docker):** `cd /Users/dknathalage/repos/tallyo && docker build -t tallyo .` → successful 3-stage build, exit 0.
- [ ] **Step 4 (Docker):** `docker image inspect tallyo --format '{{.Config.Entrypoint}} {{.Config.ExposedPorts}} {{.Config.User}}'` → `[/tallyo] map[8080/tcp:{}] nonroot:nonroot`.
- [ ] **Step 5:** Commit: `build: add multi-stage Dockerfile (SPA + cgo-free binary on distroless)`

---

## Task 3 — `docker-compose.yml`

**Files:** Create `/Users/dknathalage/repos/tallyo/docker-compose.yml`.

- [ ] **Step 1:** Create `docker-compose.yml`:
  ```yaml
  # Local dev stack: Postgres 17 + the Tallyo app, mirroring the cloud topology.
  # One `docker compose up` yields a working stack; the app runs goose migrations
  # on startup. The `app` image is byte-for-byte what Plan 3 runs on Cloud Run —
  # only DATABASE_URL differs (cloud uses the Cloud SQL unix socket; spec §1.1).

  services:
    db:
      image: postgres:17
      environment:
        POSTGRES_USER: tallyo
        POSTGRES_PASSWORD: tallyo
        POSTGRES_DB: tallyo
      volumes:
        - tallyo_pgdata:/var/lib/postgresql/data
      healthcheck:
        test: ["CMD-SHELL", "pg_isready -U tallyo -d tallyo"]
        interval: 5s
        timeout: 5s
        retries: 10
        start_period: 10s
      ports:
        - "5432:5432"

    app:
      build:
        context: .
        dockerfile: Dockerfile
      image: tallyo
      environment:
        DATABASE_URL: "postgres://tallyo:tallyo@db:5432/tallyo?sslmode=disable"
        LOG_FORMAT: "text"
        LOG_LEVEL: "info"
      depends_on:
        db:
          condition: service_healthy
      ports:
        - "8080:8080"
      restart: unless-stopped

  volumes:
    tallyo_pgdata:
  ```
  Note: `DATABASE_URL` is the compose form from spec §1.1 (host `db` = the Postgres service). `depends_on: condition: service_healthy` + `pg_isready` means the app starts only once Postgres accepts connections, so startup migrations succeed first try.
- [ ] **Step 2 (Docker):** `docker compose config >/dev/null && echo OK` → `OK`.
- [ ] **Step 3 (Docker):** end-to-end:
  ```bash
  cd /Users/dknathalage/repos/tallyo && docker compose up -d --build && sleep 8 \
    && curl -fsS localhost:8080/ -o /dev/null && echo HTTP_OK \
    && docker compose logs app | grep -i migrat
  ```
  Expected: both containers up/healthy; `HTTP_OK`; at least one migration log line. (If the app needs longer after db-healthy, raise `sleep` to 15 and retry curl; non-zero curl → inspect `docker compose logs app`.)
- [ ] **Step 4 (Docker):** `docker compose down -v` → containers removed, `Volume tallyo_pgdata Removed`.
- [ ] **Step 5:** Commit: `build: add docker-compose stack (postgres:17 + app, auto-migrate on up)`

---

## Plan-level acceptance

- Files exist at repo root: `Dockerfile`, `.dockerignore`, `docker-compose.yml` (real contents, no placeholders); no existing code modified.
- `.dockerignore` excludes `data/`, `.git`, `node_modules/`, `web/node_modules/`, `web/build/`, the legacy Electron tree (`src/`, `electron/`, `drizzle/`), and `*.db`/`*.sqlite*`.
- `docker build -t tallyo .` succeeds (SPA build → Go build with SPA embedded → distroless), exit 0; cgo-free; final image `gcr.io/distroless/static:nonroot` running `/tallyo` as `nonroot`, `EXPOSE 8080`.
- `docker compose up -d` brings up `postgres:17` (healthy) then `app`; `curl localhost:8080/` returns the SPA (200); logs show startup migrations; `docker compose down -v` cleans up.
- Same image is portable to Cloud Run (Plan 3) — only `DATABASE_URL` differs.
- If Docker was unavailable, Docker-gated steps are marked deferred-to-CI in commit bodies; files + non-Docker validations are committed.
