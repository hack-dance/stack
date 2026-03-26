# Adopting existing PRs

Use this guide when the repo already has open PRs and you need to turn their
heads into an explicit branch graph.

That graph is the prerequisite for both supported landing paths:

- ordinary branch-by-branch stacked landing
- one composed landing branch and landing PR

This guide is about making the graph explicit. For the full combined-landing
operator flow, continue with [landing-workflow.md](landing-workflow.md).

## What `stack` can do today

`stack` can adopt existing branches and existing PRs, but it does not infer the
dependency graph for you.

You still choose the parent chain. The tool then helps you keep that chain
repairable.

That makes it good for:

- grouping a related PR set under a chosen base
- reordering the chain once you know the intended dependency structure
- restacking large dependent sets after lower branches change or merge
- deciding later whether the graph should land branch-by-branch or as one landing batch

## Before you start

Make sure you have:

- a local clone of the repo
- a rough view of which PRs depend on which others
- permission to fetch PR heads if they do not exist locally yet

## Practical adoption flow

### 1. Pick the scope

Do not start with all 30 PRs unless they really belong together.

Usually the better move is:

- one graph per feature area or dependency chain
- keep independent PR groups separate
- pick a clear bottom branch for each group

You are trying to create one legible review and merge shape, not one giant
tower.

### 2. Initialize the repo

```bash
stack init --trunk main --remote origin
```

### 3. Adopt the PR heads

Use `stack adopt pr` when the unit you care about is the PR itself:

```bash
stack adopt pr 353 --parent main
stack adopt pr 354 --parent pr/353
stack adopt pr 363 --parent pr/354
stack adopt pr 364 --parent pr/363
```

Use `stack track` when the local branches already exist and you do not need the
PR lookup or fetch path:

```bash
stack track feature/base --parent main
stack track feature/parser --parent feature/base
```

When the parent branch moved since a child branch was originally cut, adoption
records a merge-base-style restack anchor instead of assuming the current parent
tip. That makes stale PR heads repairable without poisoning the first restack.

### 4. Inspect drift and PR linkage

```bash
stack status
stack sync
```

This tells you:

- which tracked branches are healthy
- which branches already have linked PRs
- which PR bases or heads disagree with your intended graph
- whether the next step is `submit`, `restack`, or manual repair

### 5. Fix the shape

If a branch belongs elsewhere:

```bash
stack move pr/364 --parent pr/354
```

If lower branches already moved:

```bash
stack restack --all
```

When the local graph looks right, update GitHub:

```bash
stack submit --all
```

That is the step that makes GitHub match your intended parent graph.

## When to compose instead of submitting directly

Do not force every adopted PR set into a strict chain if the real goal is one
combined landing PR.

Compose a landing branch when:

- the originals should remain traceability-only
- you want to verify one strict subset before merge
- later follow-up commits should stay out of the first landing
- queueing the original PRs directly would create noise or ambiguity

That is the normal move for a larger existing PR pile.

## After adoption

From here you have two choices:

If each tracked branch should land as its own PR:

- continue with [usage.md](usage.md)

If the graph should end in one landing PR:

- continue with [landing-workflow.md](landing-workflow.md)

## Keep larger sets manageable

For a larger PR set, the safest pattern is:

1. group PRs into smaller dependency chains
2. adopt one graph at a time
3. run `stack status` after each batch
4. restack and submit before moving to the next graph

That gives tighter feedback loops and makes conflicts easier to localize.

## Realistic expectation

For existing PRs, `stack` is a repair and organization layer, not a magic
importer.

You choose the intended structure. Once that structure is explicit, `stack`
helps you keep it stable and use it as the basis for either ordinary stacked
landing or one explicit landing batch.
