# Troubleshooting

## `stack` says metadata must be repaired before continuing

Run:

```bash
stack status
```

Look for:

- untracked parents
- duplicate PR linkage
- cycles in the stack graph

Repair the graph before trying `restack`, `move`, `submit`, or `queue`.

## `stack move` rejects the new parent

Parents must be:

- the configured trunk branch, or
- another tracked branch

If the local branch exists but is not tracked yet, adopt it first:

```bash
stack track feature/base --parent main
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

- the merged PR metadata drifted
- a parent was squash-merged or otherwise rewritten ambiguously
- the remote branch disappeared unexpectedly
- the tracked PR head or base disagrees with local intent

Start with:

```bash
stack status
stack sync
```

Then choose a repair path deliberately.

Common outcomes:

- `run \`stack submit <branch>\`` when the remote branch or PR base/head is simply stale
- relink or clear local PR metadata when the tracked PR was closed, deleted, or points at the wrong head
- inspect the merged parent manually when GitHub-side history drift means `sync --apply` would have to guess

## `stack submit` or `stack queue` fails against GitHub

Check:

- `gh auth status`
- GitHub repository auto-merge is enabled
- the branch is pushed to the expected remote
- the tracked PR is open and on the expected base
- the local head still matches the pushed head

When queue handoff reports stale state, resubmit first:

```bash
stack submit <branch>
stack queue <branch>
```

If the CLI reports multiple open PRs for one head branch, it is refusing to
guess which live PR owns that branch. Close or retarget the duplicate until one
open PR remains for that head name, then rerun `stack submit`.

When `stack submit` creates a new PR, it uses the tip commit subject and body by
default. If the commit body is empty, the preview will show that `stack` is
using its generated fallback body instead.

## Release automation does not update the tap

The release workflow needs:

- `RELEASE_PLEASE_TOKEN` if you want release PRs to trigger normal follow-on checks
- `HOMEBREW_TAP_GITHUB_TOKEN` with write access to `hack-dance/homebrew-tap`

If the GitHub release succeeds but the tap update does not, inspect the `release` workflow log and verify the token can push to the tap repo.
