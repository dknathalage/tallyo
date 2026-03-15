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

Opens at http://localhost:5173. Database created automatically at `~/.invoices/invoices.db`.

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
| `DB_PATH` | `~/.invoices/invoices.db` | SQLite database path |

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
