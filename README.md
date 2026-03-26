# stack

`stack` is a CLI for stacked pull requests and landing orchestration on GitHub.

It helps you do two related jobs without inventing a hosted control plane:

- keep an explicit parent graph for ordinary stacked PRs
- turn a verified set of existing PRs into one explicit landing branch and landing PR

![Status demo](docs/demo/status.gif)

![Queue demo](docs/demo/queue.gif)

## What stacked PRs are

A stacked PR flow splits one larger change into a chain:

- branch A targets `main`
- branch B builds on A and its PR targets A
- branch C builds on B and its PR targets B

That gives reviewers smaller PRs. It also makes landing order explicit. The
cost is that branch heads and PR bases need to move together when something
lower in the stack changes or merges.

## What `stack` does

`stack` keeps that workflow explicit and repairable:

- create or track branches inside a stack
- restack branches when parents move
- submit one normal GitHub PR per tracked branch
- compose one landing branch from a selected verified subset
- attach verification and ticket metadata to the landing branch
- mark original PRs as superseded and close them out after landing
- hand the real merge target to GitHub auto-merge or merge queue

The branches stay ordinary Git branches. The PRs stay ordinary GitHub PRs. If
you stop using `stack`, the repo still looks like a normal repo.

## Two landing paths

Use the basic stacked-PR flow when each branch should land in order as its own
PR.

Use the landing workflow when the graph is useful for organization and repair,
but the real merge target should be one combined landing PR. That is the right
path when you already have a pile of open PRs, want to verify a strict subset,
and need the originals to become traceability-only.

## Quick start

Clean stack:

```bash
stack init --trunk main --remote origin
stack create feature/base
stack create feature/child
stack submit --all
stack queue feature/base
```

Landing workflow from an existing PR pile:

```bash
stack init --trunk main --remote origin
stack adopt pr 353 --parent main
stack adopt pr 354 --parent pr/353
stack compose discovery-core --from pr/353 --to pr/354 --ticket LNHACK-66 --open-pr
stack verify add stack/discovery-core --type sim --run-id run-123 --passed
stack supersede --landing stack/discovery-core --prs 353,354 --close-after-merge
stack queue stack/discovery-core
stack closeout stack/discovery-core --apply
```

For the full daily workflow, start with [docs/usage.md](docs/usage.md).

For the real operator workflow around one combined landing PR, start with
[docs/landing-workflow.md](docs/landing-workflow.md).

## Merge queue

`stack` does not reimplement merge queue. It only hands off the chosen PR
through `gh pr merge --auto`.

Sometimes that PR is the bottom tracked branch. Sometimes it is the landing PR.
GitHub decides whether the handoff becomes auto-merge or queue entry.

The repo must have auto-merge enabled before `stack queue` can work.

## Starting from existing PRs

You do not need to start with a clean stack on day one.

If you already have a larger PR pile, `stack` can help you:

- adopt the existing heads into an explicit graph
- repair parent order and base drift
- compose one strict landing branch from a verified subset
- keep the original PRs for traceability instead of queueing them directly

Use [docs/adopting-existing-prs.md](docs/adopting-existing-prs.md) to make the
graph explicit, then [docs/landing-workflow.md](docs/landing-workflow.md) to
turn that graph into one landing PR.

## How it differs from Graphite and similar tools

`stack` stays deliberately simple:

- ordinary branches
- ordinary GitHub PRs
- explicit local metadata
- previews and repair loops instead of hidden automation

That makes it a good fit for teams that want stacked PRs and landing batches on
GitHub without committing the repo to a hosted workflow layer.

## Install

```bash
brew tap hack-dance/homebrew-tap
brew install hack-dance/tap/stack
```

More install and source-build options live in [docs/install.md](docs/install.md).

## Documentation

- [docs/README.md](docs/README.md) for the full docs index
- [docs/usage.md](docs/usage.md) for the standard stacked-PR loop
- [docs/landing-workflow.md](docs/landing-workflow.md) for the landing-orchestration path
- [docs/adopting-existing-prs.md](docs/adopting-existing-prs.md) for making an existing PR graph explicit
- [docs/how-it-works.md](docs/how-it-works.md) for the model and workflow
- [docs/troubleshooting.md](docs/troubleshooting.md) for failure modes and repair paths
- [docs/cli/stack.md](docs/cli/stack.md) for generated command reference

Contributor docs live in [docs/testing.md](docs/testing.md) and
[docs/releasing.md](docs/releasing.md). The bundled agent skill lives at
[skills/stack-cli/SKILL.md](skills/stack-cli/SKILL.md).
