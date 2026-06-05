# Contributing to Tallyo

Thank you for your interest in contributing! Here's how to get started.

## Getting Started

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/my-feature`
3. Make changes + add tests
4. Ensure the gate passes (see below)
5. Commit using [Conventional Commits](https://www.conventionalcommits.org/)
6. Push and open a Pull Request

## Development Setup

```bash
# Build the SPA (the Go build embeds web/build):
cd web && npm install && npm run build && cd ..

# Run the server:
go run . --port 8080
```

For frontend hot reload: `cd web && npm run dev` (Vite proxies `/api` -> :8080).

## Code Standards

- Go 1.26, cgo-free (`CGO_ENABLED=0`).
- Handlers call services, services call repositories, repositories call sqlc gen — never skip layers.
- Every DB mutation is audited (`audit.WithTx`) and broadcasts an SSE event after commit.
- sqlc source SQL lives in `internal/db/queries/`; never hand-edit `internal/db/gen/`.
- Frontend: Svelte 5 runes (`$state`, `$derived`, `$effect`), Tailwind 4.
- Follow the coding rules in `CLAUDE.md` (NASA Power of 10, adapted).

## Pull Request Checklist

- [ ] `go test ./...` passes (add `-race` for the full gate)
- [ ] `go vet ./...` and `gofmt -l .` are clean
- [ ] `cd web && npm run check` passes (0 errors / 0 warnings)
- [ ] Code follows existing patterns
- [ ] PR description explains what and why
