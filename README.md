# Tallyo

Self-hosted invoice manager. Local-first, SQLite-backed.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/dknathalage/tallyo/main/install.sh | bash
```

Requires `git`, `node`, `npm`. Installs source to `~/.tallyo-src` and a `tallyo` symlink in `~/.local/bin`. Re-run to update.

## Run

```bash
tallyo
```

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

```bash
git clone https://github.com/dknathalage/tallyo.git
cd tallyo
npm install
npm run dev          # Vite dev server at http://localhost:5173
npm run build        # Production build
npm test             # Vitest
npm link             # Use `tallyo` globally from working tree
```

## License

See LICENSE.
