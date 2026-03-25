#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "${SCRIPT_DIR}/lib.sh"

stack_sandbox_require_tools
REPO_ROOT="$(stack_sandbox_repo_root)"
cd "${REPO_ROOT}"
stack_sandbox_ensure_clean_worktree
stack_sandbox_fetch
gh auth status -h github.com >/dev/null

ORIGINAL_BRANCH="$(stack_sandbox_current_branch)"
cleanup() {
  stack_sandbox_restore_branch "${ORIGINAL_BRANCH}"
}
trap cleanup EXIT

for branch in "${STACK_SANDBOX_BRANCHES[@]}"; do
  echo "Seeding ${branch}"
  stack_sandbox_seed_branch "${branch}"
done

echo
echo "Sandbox fixtures are ready:"
"${SCRIPT_DIR}/report-fixtures.sh"
