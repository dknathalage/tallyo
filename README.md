# Tallyo

Self-hosted invoice manager. Single Go binary serving an embedded web UI, SQLite-backed, cgo-free.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/dknathalage/tallyo/refs/heads/main/install.sh | bash
```

Downloads the prebuilt `tallyo` binary for your platform from the latest GitHub release and installs it to `~/.local/bin`. Re-run to update.

Requires `curl` and `tar`. Set `TALLYO_BIN` to override the install dir.

## Run

```bash
tallyo --port 8080
```

Runs migrations on startup, then serves the UI at `http://localhost:8080`.

## Options

| Flag | Default | Description |
|------|---------|-------------|
| `--port <n>` | `8080` | HTTP listen port |
| `--data-dir <path>` | OS app data dir | Where the SQLite db lives |
| `--secure-cookie` | off | Mark the session cookie `Secure` (HTTPS only) |
| `-h, --help` | | Show help |

`DATA_DIR` env var is also respected. The default data dir is `os.UserConfigDir()/Tallyo` (e.g. `~/Library/Application Support/Tallyo` on macOS).

## Data

The SQLite database (`tallyo-go.db`) lives in the data dir. Back up that directory to back up your invoices.

## Develop

```bash
git clone https://github.com/dknathalage/tallyo.git
cd tallyo

# Build the SPA first (the Go build embeds web/build):
cd web && npm install && npm run build && cd ..

# Run the server:
go run ./cmd/tallyo --port 8080

# Frontend dev with hot reload (Vite proxies /api -> :8080):
cd web && npm run dev
```

Build the single binary:

```bash
CGO_ENABLED=0 go build -o tallyo ./cmd/tallyo
```

## License

AGPL-3.0 — see LICENSE.
