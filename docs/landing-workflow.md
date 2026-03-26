# Landing workflow

Use this path when the graph is useful for organization, repair, and review,
but the real merge target should be one combined landing PR.

That is the common case when you already have a pile of open PRs, want to land
only a verified subset, and need the original PRs to become traceability-only
instead of queue targets.

## What this guide is for

This guide covers one continuous operator flow:

1. adopt or repair the existing PR graph
2. compose one strict landing branch
3. open or refresh the landing PR
4. attach verification to the landing branch
5. mark the original PRs as superseded
6. queue the landing PR
7. close out the original PRs and tickets after merge

If you are starting a clean stack and want each branch to land as its own PR,
read [usage.md](usage.md) instead.

## A concrete situation

Suppose you have four already-open PRs:

- `#353`
- `#354`
- `#363`
- `#364`

You want one verified landing PR.

You also know the combined working branch later picked up a follow-up commit
that should not ship yet. That is exactly why this flow exists. You keep the
graph explicit, but you choose one strict merge target instead of queueing the
original PRs directly.

## Step 1: make the graph explicit

If the PRs are already checked out locally, you can track the branches directly.
If not, use `stack adopt pr` and let the CLI fetch the PR head if needed.

Example:

```bash
stack init --trunk main --remote origin

stack adopt pr 353 --parent main
stack adopt pr 354 --parent pr/353
stack adopt pr 363 --parent pr/354
stack adopt pr 364 --parent pr/363
```

Then inspect drift:

```bash
stack status
stack sync
```

If the shape is wrong, fix it before you think about landing:

```bash
stack move pr/364 --parent pr/354
stack restack --all
stack submit --all
```

The point here is simple: make the intended graph explicit first. Do not try to
compose or queue while the parent chain is still ambiguous.

For more detail on adoption and repair, read
[adopting-existing-prs.md](adopting-existing-prs.md).

## Step 2: compose the strict landing branch

Once the graph is right, create a landing branch from the exact subset you want
to ship.

If you already know the contiguous bottom and top branches:

```bash
stack compose discovery-core \
  --from pr/353 \
  --to pr/364 \
  --ticket LNHACK-66 \
  --ticket LNHACK-74 \
  --open-pr
```

If you want an explicit ordered set instead:

```bash
stack compose discovery-core \
  --branches pr/353 \
  --branches pr/354 \
  --branches pr/363 \
  --branches pr/364 \
  --ticket LNHACK-66 \
  --ticket LNHACK-74 \
  --open-pr
```

What this does:

- creates an ordinary landing branch such as `stack/discovery-core`
- replays only the selected commits onto trunk
- stores explicit ticket metadata on the landing branch
- pushes the landing branch
- creates or refreshes the landing PR

That is the step that keeps later follow-up commits out of the first merge. If
the source graph moved, you can recompute the landing branch from the exact
range again instead of manually rebuilding a composed branch.

## Step 3: verify the landing branch

Verification belongs on the landing branch, not in chat or in a PR body that
people have to interpret from memory.

Examples:

```bash
stack verify add stack/discovery-core \
  --type sim \
  --run-id run-123 \
  --score 100 \
  --passed

stack verify add stack/discovery-core \
  --type deploy \
  --identifier deploy-42 \
  --passed \
  --note "safe to close after deploy"
```

Use `stack status` to see the latest verification and whether the branch head
has moved since that record:

```bash
stack status
```

If the head moved, `stack queue` will refuse the handoff until verification is
fresh again.

## Step 4: mark the originals as superseded

Once the landing PR exists, make it explicit that the originals are no longer
queue candidates.

```bash
stack supersede \
  --landing stack/discovery-core \
  --prs 353,354,363,364 \
  --close-after-merge
```

This does two things:

- comments on the original PRs so reviewers know they were replaced by the landing PR
- stores explicit superseded-PR metadata locally so `closeout` can act on it later

The important operator rule is plain:

- queue the landing PR
- do not queue the original PRs once they are traceability-only

## Step 5: queue the real merge target

Queue the landing PR, not the original source PRs:

```bash
stack queue stack/discovery-core
```

`stack queue` verifies:

- the landing branch targets trunk
- the local head matches the pushed head
- the landing PR head matches the current branch head
- the latest recorded verification passed and still matches the current head

If you accidentally try to queue one of the source PRs after a landing batch
exists, `stack` will stop and tell you to queue the landing PR instead.

`stack` only performs the handoff. GitHub decides whether that becomes
auto-merge or queue entry.

## Step 6: close out after merge

After the landing PR merges:

```bash
stack closeout stack/discovery-core
```

This shows:

- the landing PR
- the superseded original PRs
- tickets safe to close now
- tickets still blocked on deploy verification
- any remaining follow-up checks

If you opted into post-merge superseded PR closure:

```bash
stack closeout stack/discovery-core --apply
```

That closes the superseded original PRs after the landing PR is merged.

## Ticket handling

Ticket closure is now explicit.

Use repeated `--ticket` flags when composing the landing branch. `closeout`
uses those stored ticket refs instead of guessing from branch names.

That is deliberate. Ticket closure is operational state, not something the CLI
should infer from a branch naming convention and silently get wrong.

## When to use this flow

Use the landing workflow when:

- you already have a larger PR pile
- you want one verified landing PR
- you need strict control over which commits ship first
- the original PRs should remain as traceability, not queue targets

Do not use it when each branch should still land individually in order. In that
case, use the standard stacked-PR loop from [usage.md](usage.md).
