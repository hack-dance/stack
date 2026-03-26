# Adopting existing PRs

`stack` is easiest when you start with a stack from the beginning, but that is
not the only useful case.

If your repo already has a large set of open PRs, you can use `stack` to turn
that pile into an explicit branch graph so it becomes easier to:

- test a composed set of changes locally
- see the intended landing order
- move related PRs into a dependency chain
- restack and repair larger sets when conflicts show up

This is especially useful when a team or agent workflow is producing many PRs in
parallel and you need to turn that output into something reviewable and
mergeable.

## What `stack` can do today

`stack` can adopt existing branches and existing PRs, but it does not infer the
dependency graph for you.

You still choose the parent chain. The tool then helps you keep that chain
consistent and repairable.

That means V1 is good for:

- grouping a set of related PRs under a chosen base branch
- reordering a chain once you know the intended dependency structure
- moving branches to new parents and retargeting their PR bases
- restacking large dependent sets after lower branches change or merge

It is not trying to guess the stack structure from commit history alone.

## Before you start

Make sure you have:

- a local clone of the repo
- local branches for the PR heads you want to organize
- a rough view of which PRs depend on which others

If the branches only exist on GitHub, fetch them first.

## A practical adoption workflow

### 1. Pick the scope

Do not start with all 30 PRs unless they really belong together.

Usually the better move is:

- make one stack per feature area or dependency chain
- keep independent PR groups separate
- pick a clear bottom branch for each group

You are trying to create legible review and merge order, not one giant tower.

### 2. Initialize the repo

```bash
stack init --trunk main --remote origin
```

### 3. Track the branches you want in the stack

Start at the bottom and work upward:

```bash
stack track feature/base --parent main
stack track feature/parser --parent feature/base
stack track feature/runtime --parent feature/parser
stack track feature/ui --parent feature/runtime
```

When the parent branch has moved since a child branch was originally cut,
`stack track` records a merge-base-style restack anchor instead of blindly
assuming the current parent tip. That makes stale existing PR heads adoptable
without breaking the first `stack restack`.

If the first parent chain is wrong, that is fine. Get a draft graph in place
first.

### 4. Inspect drift and PR linkage

```bash
stack status
stack sync
```

This tells you:

- which tracked branches are healthy
- which branches already have linked PRs
- which PR bases or heads disagree with your intended stack
- whether a branch needs `stack submit`, `stack restack`, or manual metadata repair next

### 5. Fix the shape

If a branch belongs elsewhere, move it:

```bash
stack move feature/ui --parent feature/base
```

If lower branches have already moved, restack:

```bash
stack restack --all
```

When the local stack shape looks right, update GitHub:

```bash
stack submit --all
```

That is the step that turns your intended parent graph into updated PR bases and
branch tips on GitHub.

When `stack submit` creates a PR during adoption, it uses the tip commit subject
and body by default. If the branch tip has no commit body yet, `stack` uses a
deterministic fallback body so the first adoption submit stays non-interactive
and reviewable.

## Large sets: how to keep them manageable

For a larger PR set, the safest pattern is:

1. group PRs into smaller dependency chains
2. adopt one chain at a time
3. run `stack status` after each batch
4. restack and submit the chain before moving to the next one

That gives you tighter feedback loops and makes conflicts easier to localize.

If you try to adopt everything at once, you can still do it, but debugging
parent mistakes gets slower and noisier.

If `stack submit` reports multiple open PRs for one head branch, stop and clean
that up before continuing. `stack` refuses to guess which open PR owns a reused
head name.

## Testing a composed set

Once the branches are tracked, the stack graph gives you a clearer local test
surface.

You can:

- check out the top branch in a chain and test the full composed set
- move or split branches when the integration shape is wrong
- restack after conflict resolution without manually retargeting every PR again

That is often the difference between “30 unrelated PRs” and “5 understandable
stacks.”

## Merge order

After adoption, the normal landing flow is:

1. make sure the bottom branch is healthy
2. run `stack submit` so the pushed heads and PR bases are current
3. run `stack queue <bottom-branch>`
4. after merge, run `stack sync`
5. repeat for the next bottom branch

That gives you a controlled path for landing larger change sets in order.

## Conflict handling on larger stacks

The main reason to adopt an explicit stack is conflict handling.

When a lower branch changes:

- `stack restack` moves the dependent branches in order
- `stack continue` and `stack abort` give you a recovery loop if a rebase stops
- `stack sync` helps after merges when PR bases or local metadata drift

You still resolve the Git conflicts, but the tool keeps the branch order and PR
bases aligned while you do it.

## A realistic expectation

For existing PRs, `stack` is best thought of as a repair and organization layer,
not a magic importer.

You choose the intended structure. Once that structure is explicit, `stack`
helps you keep it stable.
