---
name: stack-cli
description: Use for the `stack` GitHub stacked-PR CLI. Covers safe init, track, create, status, restack, submit, sync, move, and queue workflows with preview-first, repairable operations and no GraphQL.
---

# stack CLI

Use this skill when helping someone operate the `stack` CLI safely.

## Operating rules

- Prefer `stack status` first when the repo state is unclear.
- Treat stack metadata as the source of truth; use `sync` to refresh GitHub-backed state.
- Keep write flows previewable and repairable. If a command can be checked before mutation, do that.
- Use `git` and `gh` only. Do not suggest GraphQL or backend shortcuts.
- When a command offers confirmation or `--yes`, assume the safer preview path unless the user explicitly wants unattended execution.
- Stop on ambiguity. Do not guess parentage, merge bases, or repair actions when the CLI surfaces a manual-review case.

## Core flow map

- `init`: initialize repo-level stack metadata.
- `create <branch>`: create a new tracked branch on top of the current branch.
- `track <branch> --parent <parent>`: adopt an existing branch into the stack graph.
- `status`: inspect stack health, hierarchy, and cached PR state.
- `restack [branch] | --all`: preview and rebase tracked branches onto their configured parents.
- `continue`: resume an interrupted restack after conflicts are resolved.
- `abort`: abandon an interrupted restack and restore the original branch when possible.
- `submit [branch] | --all`: fetch, optionally restack, preview the push plan, then push and create or update one normal PR per branch.
- `sync [--apply]`: refresh cached PR metadata and report or apply only clean repairs.
- `move <branch> --parent <parent>`: change a branch parent, preview the rewrite, and restack affected descendants.
- `queue <branch>`: hand one healthy bottom-of-stack PR to GitHub auto-merge or merge queue.

## Safe usage patterns

- Start with `stack status` or `stack status --json` to understand the graph and current drift.
- Use `stack init` once per repo before tracking branches.
- Use `stack create` for new stack branches and `stack track` for existing ones; always make the parent explicit.
- Before restack-heavy work, confirm the recorded anchor exists and the branch is tracked.
- Prefer `stack submit` before `stack queue`; queue handoff expects a fresh push, matching PR base, and matching head commit.
- Use `stack sync` after merges or PR changes to reconcile local metadata with GitHub.
- Use `stack sync --apply` only for clean, classified repairs. If the tool marks a case as ambiguous or manual review, keep it manual.

## Repair loop

When state drifts:

1. Inspect with `stack status`.
2. Refresh with `stack sync`.
3. Apply only clean repairs if they are obvious and supported.
4. Restack or resubmit only after the graph is consistent again.

If a restack stops mid-flight, use `stack continue` from the same worktree after resolving conflicts. Use `stack abort` if you need to clear the operation journal and recover instead.

## Queue and submit guardrails

- `submit` may push branch tips, create PRs, edit PR bases, and refresh PR metadata. Preview first.
- `queue` is only for a tracked branch that already targets trunk, has a PR, has a pushed remote branch, and has a current head that matches local state.
- For queue handoff, prefer the default merge strategy unless the user asked for `squash` or `rebase`.
- If the CLI reports stale remote state, re-run `submit` before `queue`.
- If a PR is closed, merged, draft, or on the wrong base, repair that state before handing it to queue.

## What to avoid

- Do not recommend GraphQL-based workflows.
- Do not bypass stack metadata with direct remote mutations unless the user explicitly wants a manual repair step.
- Do not override the CLI’s manual-review cases with guesses.
- Do not present `stack tui` as an edit surface; it is read-only.
