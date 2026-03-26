# How stack works

## The core idea

`stack` keeps the graph explicit first. Then you choose how it lands.

That gives you two valid outcomes:

- a normal stacked PR flow where each tracked branch lands in order
- a landing workflow where the graph stays useful for organization and repair, but one composed landing PR becomes the real merge target

## The model

`stack` uses a small number of explicit objects:

- tracked branches
  - each tracked branch records one parent
  - each tracked branch can have one ordinary GitHub PR
- landing branches
  - an ordinary local branch such as `stack/discovery-core`
  - built from a selected portion of the tracked graph
  - may carry explicit tickets, verification records, and landing PR linkage
- verification records
  - attached to a tracked branch or landing branch
  - used by status, queue, and closeout
- superseded PR metadata
  - records which original PRs were replaced by a landing PR
  - used to keep traceability clear and to drive closeout

The repo still uses ordinary Git branches and ordinary GitHub PRs. `stack`
adds local intent and repair metadata on top.

## The standard stacked-PR loop

Use this when each tracked branch should land as its own PR:

1. create or track branches in a stack
2. inspect health with `stack status`
3. restack when a parent moved
4. submit the branches with `stack submit`
5. queue the healthy trunk-bound PR with `stack queue`
6. sync after merges with `stack sync`

## The landing workflow loop

Use this when the graph should end in one combined landing PR:

1. adopt or repair the graph
2. compose a strict landing branch with `stack compose`
3. attach verification with `stack verify add`
4. mark original PRs as superseded with `stack supersede`
5. queue the landing PR with `stack queue`
6. close out superseded PRs and tickets with `stack closeout`

The important distinction is operational, not theoretical:

- source PRs remain useful for traceability and review history
- the landing PR becomes the real merge target

## Merge queue

Merge queue only matters for the real merge target.

Sometimes that target is the bottom tracked PR. Sometimes it is a landing PR.

`stack queue` checks that:

- the PR targets trunk
- the local head matches the pushed head
- the PR head matches the current branch head
- the latest verification passed and still matches the current head when verification exists

Then it hands off through `gh pr merge --auto`.

`stack` does not decide whether the PR becomes auto-merge or merge queue entry.
GitHub does.

## Why the landing workflow exists

Real repos often produce a pile of already-open PRs before anyone has turned the
shape into a clean dependency chain.

In that situation, the graph is still useful. It tells you:

- what depends on what
- what to restack when lower branches move
- how to build one strict verified landing branch from the exact subset you want to ship

That is the move from “branch graph management” to “landing orchestration.”

## How this differs from Graphite and similar tools

The main difference is workflow shape, not command count:

- ordinary branches
- ordinary GitHub PRs
- explicit local metadata
- visible previews and repair loops
- no hosted control plane

That makes `stack` easier to adopt in repos where not everyone wants to depend
on the same end-to-end workflow tool.
