# Usage Guide

## Start a repo

Initialize the repo once:

```bash
stack init --trunk main --remote origin
```

Create new stack branches from the current branch:

```bash
stack create feature/base
stack create feature/child
```

Or adopt an existing branch and make the parent explicit:

```bash
stack track feature/child --parent feature/base
```

If you already have a larger set of open PRs and want to turn them into an
explicit stack after the fact, use
[adopting-existing-prs.md](adopting-existing-prs.md).

## Inspect first

Use `status` before you mutate anything:

```bash
stack status
stack tui
```

`stack tui` is a read-only dashboard for browsing the stack tree, branch health,
and cached PR state.

## The normal loop

1. Run `stack status`.
2. Run `stack restack` if a parent branch moved.
3. Run `stack submit <branch>` or `stack submit --all` to push and create or update PRs.
4. Run `stack queue <branch>` only when the bottom branch targets trunk and is healthy.
5. Run `stack sync` after merges or GitHub-side base changes.

For `stack queue`, GitHub repository auto-merge must be enabled. `stack` hands
off through `gh pr merge --auto`, then GitHub applies the repo's normal
auto-merge or merge-queue policy.

## Repair loop

Use `stack sync` first when local metadata and GitHub disagree.

Use `stack sync --apply` only for clean repairs. If the CLI reports a
manual-review case, keep it manual.

If a rebase or restack stops for conflicts:

```bash
stack continue
stack abort
```

`stack abort` clears the recorded operation and leaves stack metadata on the
clean recovery point.

## Guardrails

- parents must be trunk or another tracked branch
- `move`, `restack`, `submit`, and `queue` preview before destructive work unless you pass `--yes`
- `sync` stops on ambiguous merged-parent cases instead of guessing
- `queue` is only for a healthy bottom-of-stack PR
- `queue` requires GitHub repository auto-merge to be enabled
