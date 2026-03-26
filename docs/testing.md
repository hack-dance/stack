# Testing

`stack` now has three verification layers.

## 1. Unit and fixture coverage

Run the normal suite:

```bash
mise exec -- go test ./...
mise exec -- go build ./...
```

This covers:

- shared `git-common-dir` vs worktree `git-dir` state paths
- stack validation rules, including cycles and duplicate PR linkage
- git ancestor checks and explicit `--force-with-lease` behavior
- command-level flows for:
  - `stack adopt pr`
  - `stack compose`
  - `stack verify add`
  - `stack verify list`
  - `stack supersede`
  - `stack closeout`
  - `stack submit`
  - `stack sync --apply`
  - `stack queue`

Those command tests use real temporary git repositories and a fake `gh` binary.

## 2. Local smoke checks

For a quick local alpha check:

```bash
tmpdir=$(mktemp -d)
repo="$tmpdir/repo"
mkdir -p "$repo"
git -C "$repo" init -b main
git -C "$repo" config user.email stack@example.com
git -C "$repo" config user.name "Stack Test"
printf "hello\n" > "$repo/README.md"
git -C "$repo" add README.md
git -C "$repo" commit -m init
mise exec -- go build -o "$tmpdir/stack" ./cmd/stack
(
  cd "$repo" && \
  "$tmpdir/stack" init --trunk main --remote origin && \
  "$tmpdir/stack" create feature/a && \
  "$tmpdir/stack" status --json
)
```

## 3. Real GitHub sandbox checks

These are opt-in and non-destructive by default.

### Pin the GitHub identity

If multiple `gh` accounts exist on the machine, do not rely on active-account
state alone for live sandbox runs. Pin `GH_TOKEN` to the intended account:

```bash
gh auth switch -u roodboi
TOKEN="$(gh auth token)"
GH_TOKEN="$TOKEN" scripts/sandbox/seed-fixtures.sh
```

That avoids account drift during long-running scripts and makes auth failures
easier to interpret.

### Seed real sandbox PR fixtures

Create or refresh the deterministic sandbox PR set in `hack-dance/stack`:

```bash
GH_TOKEN="$TOKEN" scripts/sandbox/seed-fixtures.sh
GH_TOKEN="$TOKEN" scripts/sandbox/report-fixtures.sh
```

Advance only the trunk-drift branch when you want to trigger a real rebase
conflict later:

```bash
GH_TOKEN="$TOKEN" scripts/sandbox/advance-conflict.sh
```

Clean everything back up:

```bash
GH_TOKEN="$TOKEN" scripts/sandbox/cleanup-fixtures.sh
```

Verify the live sandbox repo:

```bash
STACK_RUN_GITHUB_SANDBOX=1 mise exec -- go test ./internal/github -run TestSandboxRepoView -v
```

Optionally verify that the CLI can read a real existing PR:

```bash
STACK_GITHUB_SANDBOX_PR_NUMBER=<pr-number> \
mise exec -- go test ./internal/github -run TestSandboxViewExistingPR -v
```

Run the end-to-end scenarios in a temp clone:

```bash
GH_TOKEN="$TOKEN" scripts/sandbox/run-live-queue.sh
GH_TOKEN="$TOKEN" scripts/sandbox/run-live-sync.sh
GH_TOKEN="$TOKEN" scripts/sandbox/run-live-conflict.sh
```

These scripts intentionally mutate the live sandbox repo, then reseed the
consumed fixtures so the next run starts from a known state.

- `run-live-queue.sh`
  - verifies `stack submit` can refresh a real PR
  - verifies `stack queue` can hand off a real PR through `gh pr merge --auto --merge --match-head-commit`
  - a final GitHub `mergeStateStatus` of `BLOCKED` can still count as a successful handoff when branch protection or queue policy takes over after the auto-merge request
- `run-live-sync.sh`
  - verifies `stack sync --apply` reparents a clean child after its parent PR is merged
  - verifies a clean two-hop advancement from parent to child to grandchild
  - verifies the child PR base is retargeted on GitHub
  - verifies ambiguous merged-parent cases stop for manual review instead of mutating child metadata or PR base
- `run-live-conflict.sh`
  - verifies a real parent restack conflict after trunk drift
  - verifies `stack continue` can finish a resolved rebase without an interactive editor
  - verifies a second child conflict and recovery cycle after the parent moves
  - verifies the sandbox can be reseeded after those destructive checks

Optional environment:

- `STACK_GITHUB_SANDBOX_REPO_ROOT`
  - overrides the repo root used for the sandbox tests

## Landing-workflow verification

When you change landing-orchestration behavior, do not stop at the generic
queue/restack loop. Make sure the local command suite covers:

- `compose`
- `verify`
- `supersede`
- `closeout`
- landing-aware `queue`

If the change touches GitHub mutation paths for landing PR creation, edit, or
closeout, pair the local suite with a pinned-token sandbox run.

## Current boundary

The normal suite now exercises most risky local behaviors and GitHub command
wiring, but it still does not fully prove:

- real `gh pr create/edit/close/merge` behavior against GitHub in every account configuration
- merge queue branch protection behavior
- every end-to-end landing-orchestration scenario against the live sandbox repo

Those still need deliberate sandbox runs before calling the feature set fully
proven.

## Strongest current loop

For work that changes queue, restack, submit, landing orchestration, or
crash-recovery behavior, use this loop:

```bash
mise exec -- go test ./...
mise exec -- go build ./...
gh auth switch -u roodboi
TOKEN="$(gh auth token)"
GH_TOKEN="$TOKEN" scripts/sandbox/seed-fixtures.sh
GH_TOKEN="$TOKEN" scripts/sandbox/run-live-queue.sh
GH_TOKEN="$TOKEN" scripts/sandbox/run-live-sync.sh
GH_TOKEN="$TOKEN" scripts/sandbox/run-live-conflict.sh
GH_TOKEN="$TOKEN" scripts/sandbox/report-fixtures.sh
```

That combination gives:

- local unit and fixture coverage
- real `gh` mutation coverage against GitHub
- a real queue or auto-merge handoff
- a real interrupted restack with journaled recovery and resume
- deterministic fixture reseeding for the next pass
