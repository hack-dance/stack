# How stack works

## Stacked PRs in plain language

A stacked PR flow splits one larger change into a chain of smaller pull
requests.

Example:

- `feature/base` targets `main`
- `feature/child` targets `feature/base`
- `feature/grandchild` targets `feature/child`

That gives reviewers smaller PRs and clearer landing order. The tradeoff is that
the branches and PR bases need to move together when something lower in the
stack changes or merges.

## The model

`stack` treats branches as the stack unit.

- each tracked branch records an explicit parent
- each tracked branch can have one ordinary GitHub PR
- restacks move branches to match their recorded parents
- sync refreshes PR state and proposes repairs when GitHub no longer matches local intent

The repository still uses normal Git and GitHub objects. The tool adds a local
layer of stack intent and recovery logic on top.

## The normal loop

1. Create or track branches in a stack.
2. Use `stack status` to inspect the hierarchy and current health.
3. Use `stack restack` when a parent branch moved.
4. Use `stack submit` to push branches and create or update their PRs.
5. Use `stack queue` when the bottom PR is healthy and ready to land.
6. Use `stack sync` after merges or GitHub-side changes.

## Merge queue

Merge queue matters most at the bottom of the stack.

When the bottom PR is ready, `stack queue` checks that the local branch, pushed
branch, and PR head all match, then hands that PR to GitHub auto-merge or merge
queue. After the merge lands, `stack sync` helps advance the remaining stack to
the next safe state.

Because `stack` delegates that final step to GitHub, the repository must have
auto-merge enabled. On repos with merge queue configured, GitHub applies queue
policy after the handoff.

## How this differs from Graphite and similar tools

The important difference is workflow shape: `stack` keeps branches and PRs
ordinary, stores stack intent locally in the repo, and prefers explicit repair
when state drifts instead of hiding that drift.
