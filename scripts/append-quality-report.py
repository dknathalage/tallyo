#!/usr/bin/env python3
"""Appends quality report to a GitHub release body via the API."""
import os
import json
import subprocess
import sys

tag      = os.environ['RELEASE_TAG']
token    = os.environ['GITHUB_TOKEN']
repo     = 'dknathalage/invoices'

test_count = os.environ.get('TEST_COUNT', 'N/A')
stmts      = os.environ.get('COVERAGE_STMTS', 'N/A')
funcs      = os.environ.get('COVERAGE_FUNCS', 'N/A')
branch     = os.environ.get('COVERAGE_BRANCH', 'N/A')
lines      = os.environ.get('COVERAGE_LINES', 'N/A')

# Fetch current release body
result = subprocess.run(
    ['gh', 'api', f'repos/{repo}/releases/tags/{tag}'],
    capture_output=True, text=True
)
if result.returncode != 0:
    print(f"Could not fetch release {tag}: {result.stderr}", file=sys.stderr)
    sys.exit(1)

release = json.loads(result.stdout)
release_id = release['id']
current_body = release.get('body', '')

quality_report = f"""
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
git clone https://github.com/{repo}.git
cd invoices
npm install && npm run build
PORT=3002 HOST=0.0.0.0 node build/index.js
```

Database auto-created at `~/.invoices/invoices.db` on first run.
Health check: `GET /health`
"""

new_body = current_body.rstrip() + '\n' + quality_report

# Update the release
update = subprocess.run(
    ['gh', 'api', f'repos/{repo}/releases/{release_id}',
     '-X', 'PATCH',
     '-F', f'body={new_body}'],
    capture_output=True, text=True
)

if update.returncode == 0:
    print(f"Quality report appended to release {tag}")
else:
    print(f"Failed to update release: {update.stderr}", file=sys.stderr)
    sys.exit(1)
