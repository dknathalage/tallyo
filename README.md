# 📄 Invoices

A self-hosted, open-source invoice management app built with SvelteKit and SQLite.

## Features

- Create and manage invoices, estimates, clients, catalog items
- PDF generation and export
- CSV import/export
- Recurring invoice templates
- Multi-currency support
- Business profile with logo
- Audit log for all changes
- Dark/light theme

## Tech Stack

- [SvelteKit](https://kit.svelte.dev/) + Svelte 5
- [SQLite](https://www.sqlite.org/) via [better-sqlite3](https://github.com/WiseLibs/better-sqlite3)
- [Tailwind CSS](https://tailwindcss.com/) v4
- [Vitest](https://vitest.dev/) for testing

## Getting Started

### Prerequisites

- Node.js 20+
- npm

### Installation

```bash
git clone https://github.com/dknathalage/invoices.git
cd invoices
npm install
```

### Development

```bash
npm run dev
```

Opens at http://localhost:5173. Database created automatically at `~/.<package-name>/<package-name>.db` (e.g., `~/.invoices/invoices.db`).

### Production

```bash
npm run build
PORT=3002 HOST=0.0.0.0 node build/index.js
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `3000` | HTTP port |
| `HOST` | `0.0.0.0` | Bind address |
| `NODE_ENV` | `development` | Environment |
| `DB_PATH` | `~/.<pkg-name>/<pkg-name>.db` | SQLite database path (derived from package.json `name`) |

## Docker

### Quick Start

```bash
docker compose up -d
```

The app will be available at http://localhost:3000. Data is persisted in a Docker volume.

### Environment Variables

You can customize the port mapping by setting `PORT` before running:

```bash
PORT=8080 docker compose up -d
```

See `.env.example` for all available configuration options.

### Data Volume

Invoice data (SQLite database) is stored in the `invoices-data` Docker volume mounted at `/data` inside the container. This ensures your data persists across container restarts and upgrades.

### Backup

To back up your data:

```bash
# Create a backup of the database
docker compose exec invoices cp /data/database.db /data/database.db.bak

# Or copy it to your host machine
docker compose cp invoices:/data/database.db ./invoices-backup.db
```

## Development Guide

### Project Structure

```
src/
  lib/
    components/   # Svelte UI components
    db/           # Database connection, schema, migrations
    repositories/ # Data access layer (interfaces + SQLite implementations)
    utils/        # Helpers (currency, formatting, PDF)
  routes/         # SvelteKit pages and server load functions
```

### Running Tests

```bash
npm test              # run all tests
npm run test:coverage # run with coverage report
```

### Commit Convention

This project uses [Conventional Commits](https://www.conventionalcommits.org/).
See [COMMIT_CONVENTION.md](COMMIT_CONVENTION.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT — see [LICENSE](LICENSE).
