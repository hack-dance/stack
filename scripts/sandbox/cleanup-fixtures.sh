#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "${SCRIPT_DIR}/lib.sh"

stack_sandbox_require_tools
REPO_ROOT="$(stack_sandbox_repo_root)"
cd "${REPO_ROOT}"
stack_sandbox_fetch

for branch in "${STACK_SANDBOX_BRANCHES[@]}"; do
  number="$(stack_sandbox_pr_number "${branch}")"
  if [[ -n "${number}" ]]; then
    state="$(stack_sandbox_pr_state "${number}")"
    if [[ "${state}" == "OPEN" ]]; then
      echo "Closing PR #${number} for ${branch}"
      gh pr close --repo "${STACK_SANDBOX_REPO}" "${number}" --comment "Cleaning up sandbox fixture PR." >/dev/null
    fi
  fi

  if git show-ref --verify --quiet "refs/remotes/${STACK_SANDBOX_REMOTE}/${branch}"; then
    echo "Deleting remote branch ${branch}"
    git push "${STACK_SANDBOX_REMOTE}" --delete "${branch}" >/dev/null
  fi

  if git show-ref --verify --quiet "refs/heads/${branch}"; then
    echo "Deleting local branch ${branch}"
    git branch -D "${branch}" >/dev/null
  fi
done

git switch "${STACK_SANDBOX_TRUNK}" >/dev/null
echo "Sandbox fixture cleanup complete."
