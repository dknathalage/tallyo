#!/usr/bin/env python3
"""Build release notes from git log + quality metrics."""
import os
import re
import subprocess
import sys

def get_commits_since_last_tag():
    try:
        result = subprocess.run(
            ['git', 'describe', '--tags', '--abbrev=0', 'HEAD^'],
            capture_output=True, text=True
        )
        prev_tag = result.stdout.strip() if result.returncode == 0 else None
        log_range = f'{prev_tag}..HEAD' if prev_tag else 'HEAD'

        result = subprocess.run(
            ['git', 'log', log_range, '--pretty=format:%s|%h', '--no-merges'],
            capture_output=True, text=True
        )
        return result.stdout.strip().split('\n') if result.stdout.strip() else []
    except Exception as e:
        print(f"Warning: {e}", file=sys.stderr)
        return []

def categorise(commits):
    groups = {
        'Features': [], 'Bug Fixes': [], 'Performance': [],
        'Refactor': [], 'Documentation': [], 'CI/CD': [], 'Miscellaneous': []
    }
    prefix_map = {
        'feat': 'Features', 'fix': 'Bug Fixes', 'perf': 'Performance',
        'revert': 'Bug Fixes', 'refactor': 'Refactor', 'docs': 'Documentation',
        'ci': 'CI/CD', 'build': 'CI/CD', 'chore': 'Miscellaneous', 'style': 'Miscellaneous'
    }
    for line in commits:
        if not line or '|' not in line:
            continue
        msg, sha = line.rsplit('|', 1)
        m = re.match(r'^(\w+)(\(.+?\))?!?:\s*(.+)$', msg)
        subject = m.group(3) if m else msg
        group = prefix_map.get(m.group(1).lower(), 'Miscellaneous') if m else 'Miscellaneous'
        groups[group].append(f'- {subject} (`{sha}`)')
    return {k: v for k, v in groups.items() if v}

commits = get_commits_since_last_tag()
categorised = categorise(commits)

changelog = ''
for group, items in categorised.items():
    changelog += f'### {group}\n' + '\n'.join(items) + '\n\n'
changelog = changelog.strip() or '_No notable changes._'

test_count = os.environ.get('TEST_COUNT', 'N/A')
stmts  = os.environ.get('COVERAGE_STMTS', 'N/A')
funcs  = os.environ.get('COVERAGE_FUNCS', 'N/A')
branch = os.environ.get('COVERAGE_BRANCH', 'N/A')
lines  = os.environ.get('COVERAGE_LINES', 'N/A')

notes = f"""## What's Changed

{changelog}

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
cd invoices && npm install && npm run build
PORT=3002 HOST=0.0.0.0 node build/index.js
```

Database auto-created at `~/.<package-name>/<package-name>.db`. Health: `GET /health`
"""

open('RELEASE_NOTES.md', 'w').write(notes)
print(f"Release notes written ({sum(len(v) for v in categorised.values())} commits categorised)")
