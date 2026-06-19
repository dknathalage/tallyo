---
name: feature-gate
description: Use when adding, removing, or toggling a feature gate in Tallyo — env-controlled flags that turn backend behavior and/or frontend UI on or off. Triggers include "gate this feature", "put X behind a flag", "feature toggle", "env flag", "make X optional".
---

# Feature Gate

Env-controlled on/off switches for a feature. One flag flows through three places with one consistent name:

```
TALLYO_FEATURE_<NAME>   (env, exported)  →  Config.Feature<Name>  (Go bool)  →  features.<name>  (JSON / frontend)
```

- **Env source:** exported process env only. The app does NOT load `.env` at runtime — `.env` is dev convenience. In dev: `set -a; source .env; set +a` before `go run`. In prod, set the var via systemd/compose/etc.
- **Default OFF** for new/experimental features. Default ON only when gating already-shipped behavior you want to be able to disable.
- Naming: env `TALLYO_FEATURE_AGENT` ↔ `Config.FeatureAgent` ↔ JSON key `agent`.

## Checklist

1. Backend: add the `EnvBool` helper (first gate only), add a `Config` field, default it in `main`, use it to gate wiring/behavior.
2. Frontend (only if the gate affects UI): ensure `/api/features` exists, add the key, guard the UI via the store.
3. Document the new var in `.env` (commented) and `README`/`CLAUDE.md` if user-facing.

---

## 1. Backend

### `EnvBool` helper — add once, in `internal/app/app.go` next to `EnvOr`

```go
// EnvBool returns the boolean value of env var key, or def when unset/empty or
// unparseable. Accepts 1/t/T/TRUE/true/0/f/false (strconv.ParseBool).
func EnvBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
```

(Add `"strconv"` to the imports.)

### Config field — `internal/app/app.go`, in `type Config struct`

```go
FeatureAgent bool
```

### Default it — `cmd/tallyo/main.go`, in the `app.Config{...}` literal

```go
FeatureAgent: app.EnvBool("TALLYO_FEATURE_AGENT", false),
```

### Use it — `internal/app/app.go` wiring

Prefer **not mounting the route** when off (the endpoint then 404s — no half-on state):

```go
if cfg.FeatureAgent {
	agentHandler.Routes(r)
}
```

For behavior inside a service, pass the bool into the service constructor and branch on it. Do not read `os.Getenv` outside `main`/`EnvBool` — the resolved `Config` is the single source of truth.

---

## 2. Frontend (skip if the gate is backend-only)

### `/api/features` endpoint

If it does not exist yet, create it. It returns the gate state as camelCase JSON so the SPA can render conditionally. Build it from `Config` in `internal/app` (alongside the router assembly in `server.go`) — it does not need its own slice:

```go
// inside the /api group, available to authed users
r.Get("/features", func(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{
		"agent": cfg.FeatureAgent,
	})
})
```

When it already exists, just add your key to the map.

### Frontend store — `web/src/lib/stores/features.ts`

Create once; fetched a single time on app load:

```ts
import { api } from '$lib/api';

// ponytail: plain module-level runes; one fetch, app-wide. No per-component reload.
const state = $state<Record<string, boolean>>({});
let loaded = false;

export async function loadFeatures(): Promise<void> {
	if (loaded) return;
	loaded = true;
	Object.assign(state, await api.get<Record<string, boolean>>('/features'));
}

export const features = state;
```

Call `loadFeatures()` in the root layout's load/`onMount`. Guard UI:

```svelte
{#if features.agent}
	<AgentPanel />
{/if}
```

Match the actual `$lib/api` client shape in this repo — read it before writing, it may differ from `api.get`.

---

## Removing a gate

Reverse the checklist: delete the `Config` field, the `main` default, the `/api/features` key, the store usage, and the env var line. Inline whichever branch (on or off) is now permanent. Leaving a dead flag around is the failure mode — grep `TALLYO_FEATURE_<NAME>` and `Feature<Name>` to confirm none remain.

## Verify

- `go build ./cmd/tallyo` clean; `go vet ./...`; `gofmt -l .` empty.
- Toggle proof: run with the var unset (default) and with `TALLYO_FEATURE_X=true`, confirm the route/UI appears only when on.
- `cd web && npm run check` if you touched the frontend.
