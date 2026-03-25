# Testing

`stack` now has three verification layers:

## 1. Unit and Fixture Coverage

Run the normal test suite:

```bash
mise exec -- go test ./...
mise exec -- go build ./...
```

This covers:

- shared `git-common-dir` vs worktree `git-dir` state paths
- stack validation rules, including cycles and duplicate PR linkage
- git ancestor checks and explicit `--force-with-lease` behavior
- command-level flows for:
  - `stack sync --apply`
  - `stack submit`
  - `stack queue`

Those command tests use real temporary git repositories and a fake `gh` binary.

## 2. Local Smoke Checks

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

## 3. Real GitHub Sandbox Checks

These are opt-in and non-destructive by default.

### Seed Real Sandbox PR Fixtures

Create or refresh the deterministic sandbox PR set in `hack-dance/stack`:

```bash
scripts/sandbox/seed-fixtures.sh
scripts/sandbox/report-fixtures.sh
```

Advance only the trunk-drift branch when you want to trigger a real rebase conflict later:

```bash
scripts/sandbox/advance-conflict.sh
```

Clean everything back up:

```bash
scripts/sandbox/cleanup-fixtures.sh
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

Run the destructive end-to-end scenarios in a temp clone:

```bash
scripts/sandbox/run-live-queue.sh
scripts/sandbox/run-live-conflict.sh
```

These scripts intentionally mutate the live sandbox repo, then reseed the consumed fixtures so the next run starts from a known state.

- `run-live-queue.sh`
  - verifies `stack submit` can refresh a real PR
  - verifies `stack queue` works against GitHub with `gh pr merge --auto --merge --match-head-commit`
  - verifies the queue fixture can be reseeded after the PR is merged
- `run-live-conflict.sh`
  - verifies a real parent restack conflict after trunk drift
  - verifies `stack continue` can finish a resolved rebase without an interactive editor
  - verifies a second child conflict and recovery cycle after the parent moves
  - verifies the sandbox can be reseeded after those destructive checks

Optional environment:

- `STACK_GITHUB_SANDBOX_REPO_ROOT`
  - overrides the repo root used for the sandbox tests

## Current Boundary

The normal suite now exercises most risky local behaviors and GitHub command wiring, but it still does not fully prove:

- real `gh pr create/edit/merge` behavior against GitHub
- merge queue branch protection behavior
- end-to-end submit/sync/queue flows against the live `hack-dance/stack` sandbox

Those still need deliberate sandbox runs before calling V1 feature complete.

## Strongest Current Loop

For work that changes queue, restack, submit, or crash-recovery behavior, use this loop:

```bash
mise exec -- go test ./...
mise exec -- go build ./...
scripts/sandbox/seed-fixtures.sh
scripts/sandbox/run-live-queue.sh
scripts/sandbox/run-live-conflict.sh
scripts/sandbox/report-fixtures.sh
```

That combination gives:

- local unit and fixture coverage
- real `gh` mutation coverage against GitHub
- a real queue or auto-merge handoff
- a real interrupted restack with journaled recovery and resume
- deterministic fixture reseeding for the next pass
