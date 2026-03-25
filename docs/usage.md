# Usage Guide

## Quick start

Initialize the repo once:

```bash
stack init --trunk main --remote origin
```

Create or adopt branches:

```bash
stack create feature/a
stack track feature/b --parent feature/a
```

Inspect before you mutate:

```bash
stack status
stack tui
```

## Normal flow

1. `stack status`
2. `stack restack` if parent branches moved
3. `stack submit <branch>` to push and create or update the PR
4. `stack queue <branch>` only when the branch already targets trunk and is healthy
5. `stack sync` after merges or GitHub-side base changes

## Repair flow

Use `stack sync` first when local metadata and GitHub disagree.

Use `stack sync --apply` only for clean classified repairs. If the CLI reports a manual-review case, keep it manual.

If a restack stops for conflicts:

```bash
stack continue
stack abort
```

`stack abort` clears the recorded operation and leaves stack metadata on the original parent when the move or restack did not complete.

## Important guardrails

- Parents must be trunk or another tracked branch.
- `move`, `restack`, `submit`, and `queue` all preview before destructive work unless you pass `--yes`.
- `sync` does not guess ambiguous merged-parent repairs.
- `queue` only uses `gh`; there is no GraphQL path in this repo.
