#!/usr/bin/env python3
"""Build release notes from git log + coverage data. No external tools needed."""
import os
import re
import subprocess
import sys

# --- Changelog from git log ---
def get_commits_since_last_tag():
    try:
        # Find previous tag
        result = subprocess.run(
            ['git', 'describe', '--tags', '--abbrev=0', 'HEAD^'],
            capture_output=True, text=True
        )
        prev_tag = result.stdout.strip() if result.returncode == 0 else None

        if prev_tag:
            log_range = f'{prev_tag}..HEAD'
        else:
            log_range = 'HEAD'

        result = subprocess.run(
            ['git', 'log', log_range, '--pretty=format:%s|%h', '--no-merges'],
            capture_output=True, text=True
        )
        return result.stdout.strip().split('\n') if result.stdout.strip() else []
    except Exception as e:
        print(f"Warning: could not get git log: {e}", file=sys.stderr)
        return []

def categorise(commits):
    groups = {
        'Features': [],
        'Bug Fixes': [],
        'Performance': [],
        'Refactor': [],
        'Tests': [],
        'Documentation': [],
        'CI': [],
        'Miscellaneous': [],
    }
    prefix_map = {
        'feat': 'Features',
        'fix': 'Bug Fixes',
        'perf': 'Performance',
        'refactor': 'Refactor',
        'test': 'Tests',
        'docs': 'Documentation',
        'ci': 'CI',
        'build': 'CI',
        'chore': 'Miscellaneous',
        'style': 'Miscellaneous',
    }
    for line in commits:
        if not line or '|' not in line:
            continue
        msg, sha = line.rsplit('|', 1)
        match = re.match(r'^(\w+)(\(.+?\))?!?:\s*(.+)$', msg)
        if match:
            prefix = match.group(1).lower()
            subject = match.group(3)
            group = prefix_map.get(prefix, 'Miscellaneous')
        else:
            subject = msg
            group = 'Miscellaneous'
        groups[group].append(f'- {subject} (`{sha}`)')

    return {k: v for k, v in groups.items() if v}

commits = get_commits_since_last_tag()
categorised = categorise(commits)

changelog_lines = []
for group, items in categorised.items():
    changelog_lines.append(f'### {group}')
    changelog_lines.extend(items)
    changelog_lines.append('')

changelog = '\n'.join(changelog_lines).strip() or '_No changes recorded._'

# --- Quality metrics from env ---
test_count = os.environ.get('TEST_COUNT', 'N/A')
stmts      = os.environ.get('COVERAGE_STMTS',  'N/A')
funcs      = os.environ.get('COVERAGE_FUNCS',  'N/A')
branch     = os.environ.get('COVERAGE_BRANCH', 'N/A')
lines      = os.environ.get('COVERAGE_LINES',  'N/A')

# --- Build final notes ---
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
cd invoices
npm install && npm run build
PORT=3002 HOST=0.0.0.0 node build/index.js
```

Database auto-created at `~/.invoices/invoices.db` on first run.
Check health at `/health`.
"""

open('RELEASE_NOTES.md', 'w').write(notes)
print("Release notes written to RELEASE_NOTES.md")
print(f"Commits categorised: {sum(len(v) for v in categorised.values())}")
