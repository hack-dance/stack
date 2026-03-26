# Landing workflow follow-ups

This is a historical design note from before the landing-orchestration work was
implemented. Many of the follow-ups in this document now exist in the CLI.

For the current operator-facing workflow, use
[landing-workflow.md](landing-workflow.md).

This document captures the next layer of work that became obvious while using
`stack` to turn a pile of existing PRs into one verified landing PR in
`TeamSidewinder/event-agent`.

The current tool is already useful for:

- adopting existing branches into an explicit parent graph
- keeping parent/child intent legible
- restacking after lower branches move
- syncing PR base drift back to GitHub

What it does not yet handle well is the operator workflow after the graph is
known:

- grouping already-open PRs into one strict landing batch
- excluding later follow-up commits from that landing batch
- marking original PRs as superseded by the landing PR
- carrying verification evidence alongside the stack
- telling the operator which tickets and original PRs are safe to close after
  merge and deploy

Those are not side concerns. For a team that produces many runner PRs in
parallel, they are the difference between a stack tool and a landing workflow.

## What we learned from a real workflow

The `event-agent` discovery batch looked like this:

- original PRs: `#353`, `#354`, `#363`, `#364`
- intended outcome: one combined landing PR with the verified set
- complication: the working composed branch later picked up an extra follow-up
  commit that we did not want to include in the first merge

`stack` was still useful:

- it helped make the intended grouping explicit
- it helped us reason about parent order and adoption

But the final landing workflow was manual:

1. identify the exact commit that represented the strict verified scope
2. cut a fresh landing branch at that commit
3. push it manually
4. open a combined PR manually
5. comment on the original PRs manually
6. manually decide which Linear tickets were safe to close after deploy

That is the gap this document is about.

## Recommendation

Keep `stack` and keep using it.

Do not turn it into a hosted system or a merge queue reimplementation.

Do extend it from:

- branch graph management

to:

- branch graph management plus landing orchestration

The shape should stay explicit, local-first, and Git/GitHub-native.

## Priority follow-ups

### 1. First-class landing branch composition

The tool should help create a strict landing branch from a selected portion of
the stack.

Example operator need:

```bash
stack compose discovery-core \
  --branches hack-agent/lnhack-66-... \
  --branches hack-agent/lnhack-74-... \
  --branches hack-agent/lnhack-68-... \
  --branches hack-agent/lnhack-69-...
```

Or, when the stack already exists:

```bash
stack compose discovery-core --from <bottom-branch> --to <top-branch>
```

Expected behavior:

- create a new ordinary branch, for example `stack/discovery-core`
- base it on trunk
- replay only the selected stack commits in order
- exclude later unrelated or follow-up commits unless explicitly requested
- show the exact commit set before mutating anything

This is the biggest missing piece from the real workflow.

### 2. Superseded PR support

Once a composed landing PR exists, the original PRs should not remain ambiguous.

Example:

```bash
stack supersede --landing stack/discovery-core --prs 353,354,363,364
```

Expected behavior:

- add a comment to each original PR saying it is superseded by the landing PR
- optionally add a local metadata link from original PRs to the landing PR
- optionally close the originals after the landing PR merges
- refuse to guess if more than one open PR appears to own the same branch

This should be explicit and reversible.

### 3. Verification metadata

Right now verification lives in the PR body or in chat.

That is too fragile for a stack tool that is supposed to support landing order
and closeout decisions.

Add a lightweight verification record per branch or landing branch.

Example:

```bash
stack verify add stack/discovery-core \
  --note "AA General Festival Discovery Deeplink" \
  --run-id b2f34b20-... \
  --score 100 \
  --passed
```

Expected stored fields:

- branch or landing branch
- check type: sim, unit, integration, manual, deploy, smoke
- identifier: run id, check URL, commit SHA
- pass/fail
- optional score
- optional note
- timestamp

This should feed status and closeout views.

### 4. Closeout planning

After merge, the operator needs one command that answers:

- which original PRs should now be closed as superseded
- which tickets are safe to close immediately
- which tickets remain pending post-deploy checks
- what post-deploy checks are still outstanding

Example:

```bash
stack closeout stack/discovery-core
```

Expected output:

- landing PR
- superseded PRs
- tickets safe to close now
- tickets blocked on deploy verification
- required follow-up checks

This should be read-only by default and optionally able to post comments or
write local notes.

### 5. Better status for operators

`stack status` should grow from “stack health” into “what should I do next”.

Useful additions:

- landing branch detection
- original PR to landing PR relationships
- verification summary
- unresolved post-deploy work
- clear ready/blocking reasons

Example questions it should answer directly:

- which branch is the real merge target?
- which original PRs are now traceability-only?
- which stack item is blocked, and on what?
- is this safe to queue?

### 6. Better adoption ergonomics for existing PR piles

The adoption fix for stale branches was necessary, but the operator experience
is still too manual for a large pile of existing PRs.

Improvements worth adding:

- `stack adopt pr 353 --parent main`
- `stack adopt pr 354 --parent pr/353`
- optional helpers to fetch the PR head locally if missing
- clearer warnings when a branch has drift that suggests “compose instead of
  direct stack submit”

The tool should not infer the whole dependency graph. It should make the
adoption path faster and safer once the operator already knows the shape.

## Commands worth adding

Suggested command surface:

- `stack compose`
- `stack supersede`
- `stack verify add`
- `stack verify list`
- `stack closeout`
- `stack adopt pr`

Commands that probably should not exist yet:

- automatic ticket closure
- automatic deployment polling
- merge queue orchestration beyond normal `stack queue` handoff

Those belong later, if at all.

## Recommended v1 order

1. `stack compose`
2. `stack supersede`
3. `stack verify add` and `stack verify list`
4. `stack closeout`
5. `stack status` improvements
6. `stack adopt pr`

This order matches the highest-friction steps from the real discovery landing.

## Design constraints

Keep the following properties:

- ordinary local branches
- ordinary GitHub PRs
- explicit operator choices
- deterministic repair flows
- no hidden hosted state
- no attempt to replace GitHub merge queue

The tool should help an operator say:

- “these are the changes I want to land together”
- “these are the original PRs this replaces”
- “this is the evidence that the landing branch is good”
- “these are the tickets safe to close after deploy”

without requiring a separate spreadsheet or a memory-heavy manual ritual.
