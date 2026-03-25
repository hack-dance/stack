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

git switch --detach "${STACK_SANDBOX_REMOTE}/${STACK_SANDBOX_TRUNK}" >/dev/null
git switch -C "sandbox-trunk-drift" >/dev/null
mkdir -p "_test_data/sandbox/conflict"
cat > "_test_data/sandbox/conflict/shared.txt" <<'EOF'
conflict scenario
owner: sandbox-trunk-drift
resolution: advanced trunk drift version
EOF
git add "_test_data/sandbox/conflict/shared.txt"

if git diff --cached --quiet; then
  echo "No new trunk-drift changes to commit." >&2
else
  git commit -m "testdata: advance sandbox trunk drift" >/dev/null
fi

stack_sandbox_push_branch "sandbox-trunk-drift"
stack_sandbox_upsert_pr \
  "sandbox-trunk-drift" \
  "${STACK_SANDBOX_TRUNK}" \
  "sandbox: trunk drift" \
  "$(stack_sandbox_branch_body sandbox-trunk-drift)"

echo "Updated sandbox-trunk-drift."
echo "Merge that PR into ${STACK_SANDBOX_TRUNK} when you want to trigger a real restack conflict."
