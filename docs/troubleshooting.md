# Troubleshooting

## `stack` says metadata must be repaired before continuing

Run:

```bash
stack status
```

Look for:

- untracked parents
- duplicate PR linkage
- cycles in the graph

Repair the graph before trying `restack`, `move`, `submit`, `compose`, or
`queue`.

## `stack move` rejects the new parent

Parents must be:

- the configured trunk branch, or
- another tracked branch

If the local branch exists but is not tracked yet, adopt it first:

```bash
stack track feature/base --parent main
```

Or, if the real unit is an open PR:

```bash
stack adopt pr 353 --parent main
```

## `stack continue` or `stack abort` is required

The CLI found an interrupted rebase or a recorded operation journal.

Use:

```bash
stack continue
```

after resolving conflicts in the same worktree, or:

```bash
stack abort
```

to clear the operation and return to the original branch.

## `stack sync` reports manual review

That is intentional. The CLI only auto-applies clean merged-parent repairs.

Manual review is expected when:

- merged PR metadata drifted
- a parent was squash-merged or otherwise rewritten ambiguously
- the remote branch disappeared unexpectedly
- the tracked PR head or base disagrees with local intent

Start with:

```bash
stack status
stack sync
```

Then choose a repair path deliberately.

## `stack compose --open-pr` fails against GitHub

Check:

- `gh auth status`
- the branch was pushed to the expected remote
- the repo has GitHub permissions to create or edit PRs
- the landing branch does not already match multiple open PRs

If multiple `gh` accounts exist on the machine, pin `GH_TOKEN` to the intended
account before live checks instead of trusting active-account state alone.

## `stack queue` tells me to queue the landing PR instead

That is correct.

Once a landing batch exists, the original source PRs are traceability-only.
They are no longer the real merge targets.

Queue the landing branch instead:

```bash
stack queue stack/discovery-core
```

If `stack queue` stops, read the message literally. The most common causes are:

- you tried to queue a source PR after a landing PR already exists
- the landing PR head is stale
- the latest verification failed
- the latest verification does not match the current landing head

## `stack closeout` says no explicit tickets are recorded

That is a deliberate stop.

`stack` no longer guesses tickets from branch names during closeout. Record
explicit tickets when you compose the landing branch:

```bash
stack compose discovery-core --from pr/353 --to pr/364 --ticket LNHACK-66 --ticket LNHACK-74
```

If the landing branch already exists, repair the local landing metadata before
using closeout for ticket closure.

## `stack closeout --apply` will not close superseded PRs

Check:

- the landing PR is merged
- `stack supersede` was run with `--close-after-merge`
- the superseded PRs are still open

`closeout --apply` only closes original PRs when that post-merge closure was
made explicit earlier.

## GitHub operations fail even though `gh auth status` looks fine

If you see account-specific GraphQL failures, especially on live sandbox runs,
assume auth drift first.

Recommended check:

```bash
gh auth switch -u roodboi
TOKEN="$(gh auth token)"
GH_TOKEN="$TOKEN" scripts/sandbox/seed-fixtures.sh
```

Pinned `GH_TOKEN` is more reliable than relying on active-account state during
long-running scripts.

## `stack submit` or `stack queue` reports stale state

When queue handoff reports stale state, resubmit first:

```bash
stack submit <branch>
stack queue <branch>
```

If the CLI reports multiple open PRs for one head branch, it is refusing to
guess which live PR owns that branch. Close or retarget the duplicate until one
open PR remains for that head name, then rerun `stack submit`.

## Release automation does not update the tap

The release workflow needs:

- `RELEASE_PLEASE_TOKEN` if you want release PRs to trigger normal follow-on checks
- `HOMEBREW_TAP_GITHUB_TOKEN` with write access to `hack-dance/homebrew-tap`

If the GitHub release succeeds but the tap update does not, inspect the
`release` workflow log and verify the token can push to the tap repo.
