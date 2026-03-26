# Usage Guide

Use this guide for the standard stacked-PR loop.

If your real merge target should be one combined landing PR, read
[landing-workflow.md](landing-workflow.md) instead.

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

Or make an existing branch explicit:

```bash
stack track feature/child --parent feature/base
```

If you already have open PRs and want to adopt their heads directly, use
`stack adopt pr`:

```bash
stack adopt pr 353 --parent main
stack adopt pr 354 --parent pr/353
```

If the branch is stale and the parent moved since it was cut, `stack` records a
repairable restack anchor from shared history instead of blindly anchoring on
the current parent tip.

## Inspect first

Use `status` before you mutate anything:

```bash
stack status
stack tui
```

`stack tui` is a read-only dashboard for the tree, branch health, PR linkage,
and verification summaries.

## The standard loop

1. run `stack status`
2. run `stack restack` if a parent branch moved
3. run `stack submit <branch>` or `stack submit --all`
4. run `stack queue <branch>` only when the real merge target is a healthy trunk-bound PR
5. run `stack sync` after merges or GitHub-side base changes

When `stack submit` creates a new PR, it stays non-interactive by default:

- the PR title comes from the tip commit subject
- the PR body comes from the tip commit body
- if the tip commit body is empty, `stack` generates a deterministic fallback body
- if the tip commit subject is empty, `stack` falls back to the branch name

## Choose the right landing path

Use the standard loop in this file when each tracked branch should land as its
own PR.

Switch to [landing-workflow.md](landing-workflow.md) when:

- you already have a PR pile
- you want one combined landing PR
- the original PRs should remain traceability-only
- you need explicit verification and closeout on the landing branch

That path uses:

- `stack compose --ticket ... --open-pr`
- `stack verify add`
- `stack supersede --close-after-merge`
- `stack queue stack/...`
- `stack closeout --apply`

## Repair loop

Use `stack sync` first when local metadata and GitHub disagree:

```bash
stack sync
stack sync --apply
```

`stack sync --apply` only handles clean repairs. If the CLI reports a
manual-review case, keep it manual.

If a rebase or restack stops for conflicts:

```bash
stack continue
stack abort
```

`stack abort` clears the recorded operation and leaves stack metadata on the
last clean recovery point.

## Guardrails

- parents must be trunk or another tracked branch
- `move`, `restack`, `submit`, `compose`, `supersede`, `closeout --apply`, and `queue` preview before destructive work unless you pass `--yes`
- `sync` stops on ambiguous merged-parent cases instead of guessing
- `queue` is only for a healthy trunk-bound PR or recorded landing PR
- when verification exists, `queue` requires the latest verification to pass and still match the current head
- the repo must have GitHub auto-merge enabled before `stack queue` can hand off successfully
