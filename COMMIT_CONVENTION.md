# Commit Convention

This project uses [Conventional Commits](https://www.conventionalcommits.org/).

## Format

```
<type>(<optional scope>): <description>

[optional body]

[optional footer(s)]
```

## Types

| Type | Description |
|------|-------------|
| `feat` | A new feature |
| `fix` | A bug fix |
| `docs` | Documentation changes |
| `style` | Code style changes (formatting, no logic change) |
| `refactor` | Code refactoring (no new features or bug fixes) |
| `test` | Adding or fixing tests |
| `chore` | Maintenance tasks (deps, config, etc.) |
| `perf` | Performance improvements |
| `ci` | CI/CD changes |
| `build` | Build system changes |
| `revert` | Reverting a previous commit |

## Examples

```bash
feat: add PDF export for estimates
fix: correct invoice total calculation with tax
docs: update README with Docker instructions
chore: upgrade SvelteKit to v2
test: add coverage for recurring template creation
ci: add release workflow for v* tags
```

## Enforcement

Commits are validated by [commitlint](https://commitlint.js.org/) via the `.husky/commit-msg` hook.
