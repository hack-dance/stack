---
name: stack-cli
description: Use for the `stack` GitHub CLI for stacked PRs and landing orchestration. Covers graph setup, stacked-PR workflows, landing branches, verification, superseded PR handling, closeout, and queue handoff for the real merge target.
---

# stack CLI

Use this skill to operate `stack` safely: make the graph explicit, then choose how it lands.

## Decide the path first

There are two supported paths:

- standard stacked PR flow: each tracked branch lands as its own PR
- landing workflow: one composed landing branch and landing PR become the real merge target

If you are starting from existing PRs, first make the graph explicit, then continue with one of those two paths.

## Operating rules

- Prefer `stack status` first when repo state is unclear.
- Treat stack metadata as the source of truth; use `sync` to refresh GitHub-backed state.
- Preview before mutating when possible.
- Stop on ambiguity. Do not guess parentage, merge bases, repair actions, or the real merge target when the CLI surfaces a manual-review case.
- Prefer preview over `--yes` unless the user wants unattended execution.
- Use ordinary `git` and `gh` around `stack`; do not invent a parallel hosted workflow.

## Core command map

- Setup
- `init`: initialize repo metadata
- `create <branch>`: create a tracked branch
- `track <branch> --parent <parent>`: adopt an existing local branch
- `adopt pr <number> --parent <parent>`: adopt an existing PR head
- Health and repair
- `status`: graph, PR, verification, and landing health
- `sync [--apply]`: refresh PR state and apply clean repairs
- `restack [branch] | --all`: rebase tracked branches onto parents
- `continue`: resume an interrupted restack
- `abort`: abandon an interrupted restack
- `move <branch> --parent <parent>`: change a branch parent
- Standard PR flow
- `submit [branch] | --all`: push tracked branches and create or update PRs
- Landing workflow
- `compose <name>`: create a landing branch
- `verify add <branch>`: attach verification evidence
- `verify list <branch>`: inspect verification records
- `supersede --landing <branch> --prs ...`: mark original PRs as superseded
- `closeout <landing-branch> [--apply]`: plan or apply post-merge closure work
- Queue handoff
- `queue <branch>`: hand off the real merge target

## Standard stacked-PR workflow

Typical flow:

1. `stack init --trunk main --remote origin`
2. `stack create feature/base`
3. `stack create feature/child`
4. `stack status`
5. `stack restack` when a parent moved
6. `stack submit --all`
7. `stack queue feature/base`
8. `stack sync` after merges

## Existing PR pile setup

Typical flow:

1. `stack init --trunk main --remote origin`
2. `stack adopt pr 353 --parent main`
3. `stack adopt pr 354 --parent pr/353`
4. `stack status`
5. `stack sync`
6. `stack move`, `stack restack`, and `stack submit` until the graph matches intent

Do not ask `stack` to infer the dependency graph. The operator still chooses the parent chain.

Once the graph is explicit, continue with Standard stacked-PR workflow or Landing workflow.

## Landing workflow

Typical flow:

1. make the graph explicit with the setup flow above
2. `stack compose discovery-core --from pr/353 --to pr/364 --ticket LNHACK-66 --ticket LNHACK-74 --open-pr`
3. `stack verify add stack/discovery-core --type sim --run-id run-123 --passed`
4. `stack supersede --landing stack/discovery-core --prs 353,354,363,364 --close-after-merge`
5. `stack queue stack/discovery-core`
6. `stack closeout stack/discovery-core`
7. `stack closeout stack/discovery-core --apply` after the landing PR merges

Important operator rules:

- use explicit `--ticket` flags during compose; closeout no longer guesses tickets from branch names
- keep verification on the landing branch, not only in chat or in a PR body

## Before queue

- Queue only the real merge target: the bottom tracked PR in standard flow, or the landing PR in landing workflow.
- Prefer `submit` before `queue`.
- If queue reports stale remote or stale PR head state, rerun `submit`.
- If verification exists, queue requires the latest verification to pass and still match the current head.

## Repair loop

1. inspect with `stack status`
2. refresh with `stack sync`
3. apply only clean repairs if they are obvious and supported
4. restack, resubmit, or recompose only after the graph is consistent again

If a restack stops mid-flight:

- use `stack continue` from the same worktree after resolving conflicts
- use `stack abort` if you need to clear the operation journal and recover instead

## Queue and GitHub guardrails

- `queue` is for a healthy trunk-bound PR or recorded landing PR, not for any arbitrary branch in the graph
- when a landing batch exists, original source PRs are traceability-only; if queue redirects you to the landing PR, trust that
- if a PR is closed, merged, draft, or on the wrong base, repair that before queue handoff

## Repo-specific sandbox note

On machines with multiple `gh` accounts, pin `GH_TOKEN`; do not rely on the active account:

```bash
gh auth switch -u roodboi
TOKEN="$(gh auth token)"
GH_TOKEN="$TOKEN" scripts/sandbox/seed-fixtures.sh
```

## What to avoid

- Do not bypass stack metadata with direct remote mutations unless the user explicitly wants a manual repair step.
- Do not override the CLI’s manual-review cases with guesses.
- Do not treat `stack tui` as an edit surface; it is read-only.
