# GH Stack CLI Design

## Goal

Define a Graphite-like stack CLI we can build for this repo using standard Git plus the GitHub CLI (`gh`), without requiring a hosted backend or synthetic remote branch scheme.

The tool should make it practical to:

- create and manage explicit branch stacks
- open and update one GitHub PR per branch
- restack descendants safely after rebases and partial merges
- hand off the bottom of a stack to GitHub merge queue safely
- recover from conflicts and stale local metadata

## Problem

GitHub does not have a first-class “stack” object.

Today, a “stack” on GitHub is only an informal combination of:

- local Git branches or commits
- a chain of PR base branches
- some human understanding of dependency order

That creates a few recurring failures:

- branch ancestry and PR base relationships drift apart
- rebasing a mid-stack branch leaves children stale
- branch renames and partial merges break PR linkage
- GitHub merge queue validates integration order, but it is not stack-aware
- restacking and retargeting are manual, slow, and easy to get wrong

We need a repo-local tool that makes these transitions explicit and deterministic.

## Research Takeaways

### Graphite

Graphite’s core model is a parent-linked branch DAG managed locally by the CLI. `gt restack` makes Git ancestry match Graphite’s parent metadata, `gt submit` pushes branches and creates or updates one PR per branch, and the merge queue logic is separate from stack editing.

Useful behaviors to copy:

- explicit parent tracking
- automatic descendant restacking after branch edits
- idempotent submit
- safe force-push behavior
- conflict continuation commands

Useful behaviors to avoid in V1:

- backend-dependent state
- partial-stack merge orchestration that depends on temporary remote branches
- trying to replace GitHub’s merge queue

### ghstack

`ghstack` is the most invasive design reviewed. It creates synthetic remote `base`, `head`, and `orig` branches per diff and lands by cherry-picking the canonical `orig` refs back onto trunk.

Useful lesson:

- synthetic refs can make stack identity robust

Why not copy it:

- too much remote branch churn
- unnatural GitHub PR layout
- harder to explain and debug
- overkill for a repo-local branch-stack workflow

### spr

`spr` treats each commit as the stack unit and syncs one PR per commit. Its merge trick is notable: it finds the top mergeable PR, retargets that PR to trunk, merges it, and then closes the lower PRs because their commits were effectively merged by the top PR.

Useful lesson:

- submit can be derived from local commit order

Why not copy it:

- we want branch stacks, not commit stacks
- branch-level ownership maps better to our Codex runner output
- merge-queue handoff is clearer when each branch remains a normal PR

### git-spice

`git-spice` is the closest reference design for this repo. It keeps explicit local branch metadata, restacks branches with normal Git rebases, and uses normal GitHub `createPullRequest` and `updatePullRequest` behavior to keep PR bases in sync.

This is the closest V1 fit:

- explicit local state
- normal remote branches
- one normal PR per branch
- no service backend required

### GitHub merge queue

GitHub merge queue is not stack management. It is speculative integration and validation on temporary `merge_group` branches. It is FIFO, validates queued changes against the latest base branch plus changes ahead in queue, and removes entries that fail checks or conflict.

Design consequence:

- our CLI should not attempt to be a merge queue
- our CLI should prepare stacks for GitHub merge queue and advance stacks after downstack merges

## Non-Goals

- Building a custom merge queue service
- Replacing GitHub PR review UX
- Commit-stack workflows where each commit is a PR
- Synthetic remote base/head/orig branches
- Branch rename support after a PR already exists
- Automatic background merge orchestration in V1

## Design Principles

1. Use branches as the stack unit, not commits.
2. Store stack state locally and explicitly.
3. Keep remote branches and PRs normal and legible in GitHub.
4. Prefer deterministic rebases over heuristics.
5. Always use `--force-with-lease` when rewriting published branch history.
6. Keep stack editing separate from merge-queue handoff.
7. Make recovery and repair first-class.
8. Be worktree-safe.

## Recommended V1

Build a small repo-local CLI, tentatively named `stack`, that uses:

- `git` for branch creation, rebasing, ancestry checks, and pushing
- `gh` for PR creation, inspection, retargeting, updating, and merge-queue handoff
- per-worktree local metadata stored under the active Git dir

The key design choice is to make the stack graph explicit instead of inferring it from current Git ancestry or branch naming.

## State Model

Store state at:

```text
$(git rev-parse --git-dir)/stack-cli/state.json
```

This keeps state:

- local to the clone/worktree
- out of the repository contents
- isolated across concurrent QA-runner worktrees

Suggested shape:

```json
{
  "version": 1,
  "trunk": "main",
  "defaultRemote": "origin",
  "branches": {
    "codex/discovery-1": {
      "parent": "main",
      "remote": "origin",
      "prNumber": 401,
      "prUrl": "https://github.com/org/repo/pull/401",
      "headRef": "codex/discovery-1",
      "baseRef": "main",
      "lastSubmittedHeadOid": "abc123",
      "lastRestackedParentOid": "def456"
    },
    "codex/discovery-2": {
      "parent": "codex/discovery-1",
      "remote": "origin",
      "prNumber": 402,
      "prUrl": "https://github.com/org/repo/pull/402",
      "headRef": "codex/discovery-2",
      "baseRef": "codex/discovery-1",
      "lastSubmittedHeadOid": "ghi789",
      "lastRestackedParentOid": "abc123"
    }
  }
}
```

Why record `lastRestackedParentOid`:

- it gives us a stable rebase anchor
- it lets `restack` use `git rebase --onto <newParent> <oldParentOid> <branch>`
- it avoids relying only on `merge-base`, which becomes ambiguous after repeated rebases and partial merges

## Core Mental Model

Each tracked branch has:

- a parent branch, which is either another tracked branch or trunk
- a Git branch tip
- an optional GitHub PR

The invariant we want:

- the parent branch tip is the logical base of the child branch
- the GitHub PR base ref matches the parent branch name
- the local branch history is rebased so the parent tip is in its ancestry

If any of those drift, the stack is unhealthy.

## Command Surface

### `stack init`

Initializes local state for the current worktree.

Example:

```bash
stack init --trunk main --remote origin
```

Underlying operations:

- `git rev-parse --git-dir`
- write `state.json`

### `stack create`

Creates a new tracked branch on top of the current branch and records the current branch as the parent.

Example:

```bash
stack create codex/search-1
```

Underlying operations:

```bash
git switch -c codex/search-1
```

State mutation:

- `parent = currentBranch`

### `stack track`

Adopts an existing branch into the stack graph.

Example:

```bash
stack track codex/search-2 --parent codex/search-1
```

Use this to import branches created outside the tool.

### `stack log`

Prints the stack graph and health.

Example:

```bash
stack log
stack log --long
stack log --json
```

Health checks should include:

- parent exists locally
- parent tip matches `lastRestackedParentOid` or is ahead of it
- parent tip is ancestor of child
- PR exists and base ref matches parent
- remote branch exists

### `stack restack`

Rebases one branch or a whole subtree so each branch is based on its configured parent.

Examples:

```bash
stack restack
stack restack codex/search-2
stack restack --all
```

Preferred algorithm per branch:

1. Resolve `parent`.
2. Resolve `currentParentOid = git rev-parse <parent>`.
3. Resolve `oldParentOid` from metadata.
4. If `currentParentOid == oldParentOid` and parent is already ancestor, skip.
5. Else run:

```bash
git rebase --onto <parent> <oldParentOid> <branch>
```

Fallback if `oldParentOid` is missing or invalid:

```bash
git rebase --onto <parent> $(git merge-base <parent> <branch>) <branch>
```

After success:

- update `lastRestackedParentOid`
- recurse into descendants in topological order

### `stack submit`

Pushes the stack and creates or updates one PR per tracked branch.

Examples:

```bash
stack submit
stack submit codex/search-1
stack submit --all
```

V1 behavior:

1. Restack targeted branches unless `--no-restack`.
2. Traverse in parent-to-child order.
3. For each branch:
   - push branch with `--force-with-lease`
   - create PR if none exists
   - otherwise retarget PR base if needed

Underlying operations:

```bash
git push --force-with-lease origin <branch>:refs/heads/<branch>
gh pr create --base <parent> --head <branch> --fill
gh pr edit <pr> --base <parent>
gh pr view <pr> --json number,url,baseRefName,headRefName,state,isDraft
```

Important rule:

- once a branch has a PR, do not rename the branch in V1

### `stack sync`

Synchronizes local metadata with remote and repairs obvious drift after merges.

Examples:

```bash
stack sync
stack sync --prune
```

V1 responsibilities:

- `git fetch --prune`
- inspect tracked PRs with `gh pr view`
- detect merged or closed parent PRs
- reparent children of merged branches to trunk
- optionally prune fully merged local branches from metadata
- offer or perform restack after reparenting

Key case:

- if `A <- B <- C` and `A` is merged, `B.parent` becomes `main`
- `C.parent` remains `B`

### `stack move`

Changes the parent of a branch and restacks the affected subtree.

Example:

```bash
stack move codex/search-3 --parent codex/search-1
```

This is the cleanest “unstack” primitive.

### `stack delete`

Removes a branch from the stack graph and reparents its children to the deleted branch’s parent.

Examples:

```bash
stack delete codex/search-2
stack delete codex/search-2 --close-pr
stack delete codex/search-2 --delete-branch
```

Algorithm:

1. Determine `parent(B)`.
2. For each child of `B`, set `parent(child) = parent(B)`.
3. Restack each child.
4. Optionally close `B`’s PR.
5. Remove `B` from metadata.
6. Optionally delete the local branch.

This is the main V1 answer to “unstack”.

### `stack unlink`

Drops the PR association while keeping the branch tracked.

Example:

```bash
stack unlink codex/search-2
```

Use this when a PR was created incorrectly and should be recreated.

### `stack queue`

Hands the bottom of a stack to GitHub’s merge queue or auto-merge flow.

Examples:

```bash
stack queue codex/search-1
stack queue --cascade codex/search-1
```

V1 should only guarantee safe handoff for the currently ready branch.

Preferred underlying command:

```bash
gh pr merge <pr> --auto --match-head-commit <headSha>
```

Behavior when the target branch requires merge queue:

- GitHub either enables auto-merge or adds the PR to merge queue
- `--match-head-commit` gives us a stale-head safety check

Optional advanced mode:

- use `gh api graphql` with `EnqueuePullRequestInput.expectedHeadOid` when we need explicit queue operations

### `stack continue` / `stack abort`

Resumes or aborts a conflicted restack operation.

This should be built in from the start.

Store the current operation queue at:

```text
$(git rev-parse --git-dir)/stack-cli/op.json
```

Suggested shape:

```json
{
  "type": "restack",
  "pending": [
    { "branch": "codex/search-2", "parent": "codex/search-1" },
    { "branch": "codex/search-3", "parent": "codex/search-2" }
  ]
}
```

If a rebase conflicts:

- stop immediately
- leave Git’s rebase state intact
- tell the user to resolve conflicts and run `stack continue`

`stack continue` should:

1. run `git rebase --continue`
2. update metadata for the branch that just completed
3. continue the remaining queued descendant restacks

## Underlying Git And GH Primitives

This design only needs a small set of primitives.

### Git

- `git switch -c <branch>`
- `git rev-parse <ref>`
- `git merge-base <a> <b>`
- `git merge-base --is-ancestor <a> <b>`
- `git rebase --onto <newBase> <oldBase> <branch>`
- `git push --force-with-lease origin <branch>:refs/heads/<branch>`
- `git fetch --prune origin`
- `git range-diff <oldBase>...<newBranch>`

### GH

- `gh pr create --base <parent> --head <branch> --fill`
- `gh pr edit <pr> --base <parent>`
- `gh pr view <pr> --json ...`
- `gh pr list --state open --json ...`
- `gh pr update-branch <pr> --rebase`
- `gh pr merge <pr> --auto --match-head-commit <sha>`

### Optional GraphQL via `gh api graphql`

Useful when `gh pr` subcommands are not enough:

- `enqueuePullRequest(expectedHeadOid, jump)`
- `enablePullRequestAutoMerge(expectedHeadOid, mergeMethod)`

We should treat GraphQL as an escape hatch, not the default path.

## Restack Algorithm Details

Given a branch graph:

```text
main <- A <- B <- C
```

If `A` changes, we want:

1. rebase `B` onto new `A`
2. rebase `C` onto new `B`

Topological order matters. The simplest rule is:

- always process parent before child

Pseudo-flow:

```text
restack(branch):
  ensure parent is healthy
  if branch already contains parent tip and metadata is current:
    skip
  else:
    rebase branch onto parent
    record new parent OID
  for child in children(branch):
    restack(child)
```

Why explicit metadata matters:

- pure ancestry checks do not tell us what the intended parent is
- after partial merges, GitHub may retarget child PRs in ways that do not match local intent
- local parent pointers let us repair drift deterministically

## Unstacking And Partial Merge Behavior

There are three distinct cases and the CLI should treat them separately.

### 1. Remove a branch from the middle of a stack

Use `stack delete` or `stack move`.

Example:

```text
main <- A <- B <- C
```

Delete `B`:

```text
main <- A <- C
```

Mechanically:

- change `C.parent` from `B` to `A`
- rebase `C` onto `A`
- retarget PR `C` base from `B` to `A`

### 2. Downstack PR merged

Example:

```text
main <- A <- B <- C
```

`A` merges.

Desired result:

```text
main <- B <- C
```

Mechanically:

- `B.parent = main`
- restack `B`
- retarget PR `B` base to `main`
- restack `C`

### 3. PR was wrong and must be recreated

Use `stack unlink` followed by `stack submit`.

Mechanically:

- keep branch metadata
- forget stored PR number
- create a new PR on next submit

## Merge Queue Strategy

GitHub merge queue should remain the source of truth for queue ordering and speculative validation.

Our CLI should do only three things:

1. ensure the bottom PR is correctly based and pushed
2. enqueue or auto-merge it safely
3. after it merges, advance the next PR in the stack

That means a stack merge with GitHub merge queue is sequential in V1:

```text
A queued
A merged
sync + restack B
B queued
B merged
sync + restack C
C queued
```

This is slower than Graphite’s native queue, but it matches GitHub’s capabilities and avoids hidden automation.

## Why This Design Instead Of A Closer Graphite Clone

### No synthetic remote branches

We do not need `ghstack`-style `base/head/orig` refs because:

- our review unit is a branch
- GitHub already supports PR base retargeting directly
- normal remote branches are easier to reason about

### No commit-stack model

We do not want `spr`’s “one commit, one PR” model because:

- our current workflow naturally produces one branch per issue slice
- QA and runner ownership map better to branch units

### No backend

We do not need hosted state in V1 because:

- the operator and Codex runner already share the repo
- worktree-local metadata is enough to manage stack shape
- GitHub remains the source of truth for remote PR state

## Failure Modes And Recovery

### Parent branch deleted locally

- mark branch unhealthy
- require `stack move` or `stack sync --repair`

### Parent PR merged remotely

- reparent child to trunk during `stack sync`

### Force-push race

- fail safely via `--force-with-lease`
- require operator re-fetch and retry

### PR base drift

- detect via `gh pr view`
- fix with `gh pr edit --base <expectedParent>`

### Rebase conflict

- stop at first conflict
- persist operation queue
- continue with `stack continue`

### Branch renamed after PR creation

- unsupported in V1
- error with an explicit repair path:
  - recreate branch with original name, or
  - `stack unlink` and recreate the PR

## Verification Plan

### Unit tests

- graph validation
- topological sort
- parent/child reparenting
- restack anchor selection
- operation queue persistence

### Fixture tests

Use throwaway Git repos to verify:

- create stack
- restack after trunk changes
- remove mid-stack branch
- merge downstack and sync
- recover from rebase conflicts

### GH integration tests

Use a sandbox repository to verify:

- `gh pr create --base`
- `gh pr edit --base`
- `gh pr merge --auto --match-head-commit`
- optional `gh api graphql` enqueue path

## Recommended Command Set For V1

Ship only:

- `stack init`
- `stack create`
- `stack track`
- `stack log`
- `stack restack`
- `stack submit`
- `stack sync`
- `stack move`
- `stack delete`
- `stack unlink`
- `stack queue`
- `stack continue`
- `stack abort`

Defer to V2:

- `stack split`
- `stack fold`
- `stack rename`
- navigation comments
- background cascade merge worker
- Linear integration

## Example Workflow

```bash
stack init --trunk main

git switch main
stack create codex/discovery-1
# edit, commit

stack create codex/discovery-2
# edit, commit

stack log
stack submit --all

# later, after edits to discovery-1
git switch codex/discovery-1
# amend / new commit
stack restack codex/discovery-1
stack submit codex/discovery-1

# merge flow
stack queue codex/discovery-1

# after PR 1 merges
stack sync
stack submit codex/discovery-2
stack queue codex/discovery-2
```

## Open Questions

- Should `stack submit` autofill PR title/body from commit messages, branch name, or PR template by default?
- Should `stack sync` automatically prune merged branches from metadata, or only suggest it?
- Should V1 support stacked draft PRs explicitly?
- Should we store extra repo-local metadata in Git config, or keep everything in the JSON file?
- Do we want a later GitHub Action that advances the next PR in the stack automatically after a downstack merge?

## Recommendation

Implement the V1 branch-stack tool using the `git-spice` shape, not the `ghstack` or `spr` shapes:

- explicit local branch graph
- normal remote branches
- normal GitHub PRs
- deterministic restack and retarget
- GitHub merge queue as external handoff

That gives us most of the practical value of Graphite’s CLI with a much smaller surface area and without needing to install Graphite in this repo.

## References

- Graphite CLI quick start: <https://graphite.com/docs/cli-quick-start>
- Graphite command reference: <https://graphite.com/docs/command-reference>
- Graphite restack docs: <https://graphite.com/docs/restack-branches>
- Graphite merge queue: <https://graphite.com/docs/graphite-merge-queue>
- Graphite external merge queue integration: <https://graphite.com/docs/external-merge-queue-integration>
- GitHub merge queue docs: <https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue>
- GitHub merge queue / merge_group webhook docs: <https://docs.github.com/en/webhooks/webhook-events-and-payloads#merge_group>
- GitHub GraphQL input objects: <https://docs.github.com/en/graphql/reference/input-objects>
- ghstack repository: <https://github.com/ezyang/ghstack>
- spr repository: <https://github.com/ejoffe/spr>
- git-spice repository: <https://github.com/abhinav/git-spice>
- Sapling stacks overview: <https://sapling-scm.com/docs/overview/stacks/>
- Mergify stacks docs: <https://docs.mergify.com/stacks/>
