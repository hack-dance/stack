# Sandbox Fixtures

This folder exists to support real GitHub sandbox PR scenarios inside the
private `hack-dance/stack` repo.

The fixture scripts create a small set of deterministic branches and PRs:

## Scenarios

### Clean Stack

- `sandbox-clean-base` -> `main`
- `sandbox-clean-child` -> `sandbox-clean-base`
- `sandbox-clean-grandchild` -> `sandbox-clean-child`

Purpose:

- baseline stacked PR flow
- `stack submit`
- `stack queue` on the bottom PR
- clean `stack sync` after merges
- multi-level parent advancement checks

### Same-File Overlap

- `sandbox-overlap-base` -> `main`
- `sandbox-overlap-child` -> `sandbox-overlap-base`

Purpose:

- stacked PRs that touch the same file
- diff clarity and PR retargeting checks
- noisy but still-valid stacked submit/sync behavior

### Conflict Stack

- `sandbox-conflict-base` -> `main`
- `sandbox-conflict-child` -> `sandbox-conflict-base`
- `sandbox-trunk-drift` -> `main`

Purpose:

- same-file overlap across parent and child
- trunk drift that can later be merged to `main`
- manual and automated restack conflict testing

The shared conflict target is:

- `_test_data/sandbox/conflict/shared.txt`

### Queue Ready

- `sandbox-queue-ready` -> `main`

Purpose:

- isolated bottom-of-stack PR for `stack queue`
- queue and auto-merge checks without stack-related noise

## Scripts

From the repo root:

```bash
scripts/sandbox/seed-fixtures.sh
scripts/sandbox/report-fixtures.sh
scripts/sandbox/advance-conflict.sh
scripts/sandbox/cleanup-fixtures.sh
```

## Notes

- The scripts are designed to be idempotent.
- They use normal git branches and normal GitHub PRs.
- They force-push only the sandbox fixture branches listed above.
- Cleanup closes open PRs when possible and deletes the known fixture branches.
