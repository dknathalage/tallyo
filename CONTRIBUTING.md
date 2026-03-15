# Contributing to Invoices

Thank you for your interest in contributing! Here's how to get started.

## Getting Started

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/my-feature`
3. Make changes + add tests
4. Ensure tests pass: `npm test`
5. Commit using [Conventional Commits](COMMIT_CONVENTION.md)
6. Push and open a Pull Request

## Development Setup

```bash
npm install
npm run dev
```

## Code Standards

- TypeScript strict mode
- Svelte 5 runes (`$state`, `$derived`, `$effect`)
- Import from `$lib/repositories` — never `$lib/db/queries` directly in routes
- All new features need tests

## Pull Request Checklist

- [ ] `npm run check` passes (0 errors)
- [ ] `npm test` passes
- [ ] Code follows existing patterns
- [ ] PR description explains what and why
