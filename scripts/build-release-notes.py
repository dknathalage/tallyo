#!/usr/bin/env python3
"""Build release notes combining git-cliff changelog + quality report."""
import os
import sys

changelog_path = 'CHANGELOG_RELEASE.md'
if not os.path.exists(changelog_path):
    print(f"ERROR: {changelog_path} not found", file=sys.stderr)
    sys.exit(1)

changelog = open(changelog_path).read().strip()

test_count = os.environ.get('TEST_COUNT', 'N/A')
stmts      = os.environ.get('COVERAGE_STMTS',  'N/A')
funcs      = os.environ.get('COVERAGE_FUNCS',  'N/A')
branch     = os.environ.get('COVERAGE_BRANCH', 'N/A')
lines      = os.environ.get('COVERAGE_LINES',  'N/A')

notes = f"""{changelog}

---

## \U0001f9ea Quality Report

| Metric | Value |
|--------|-------|
| \u2705 Tests passing | **{test_count}** |
| \U0001f4ca Statement coverage | {stmts}% |
| \U0001f4ca Function coverage | {funcs}% |
| \U0001f4ca Branch coverage | {branch}% |
| \U0001f4ca Line coverage | {lines}% |

## \U0001f680 Quick Start

```bash
git clone https://github.com/dknathalage/invoices.git
cd invoices
npm install && npm run build
PORT=3002 HOST=0.0.0.0 node build/index.js
```

Database auto-created at `~/.invoices/invoices.db` on first run.

## \U0001f3e5 Health Check

```
GET /health -> {{"status":"ok","db":"connected"}}
```
"""

open('RELEASE_NOTES.md', 'w').write(notes)
print("Release notes written to RELEASE_NOTES.md")
