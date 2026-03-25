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

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

binary="${tmpdir}/stack"
clone_dir="${tmpdir}/repo"
branch="sandbox-queue-ready"

echo "Building stack CLI"
stack_sandbox_go build -o "${binary}" ./cmd/stack

echo "Cloning sandbox repo into ${clone_dir}"
gh repo clone "${STACK_SANDBOX_REPO}" "${clone_dir}" -- --quiet

git -C "${clone_dir}" config user.email stack@example.com
git -C "${clone_dir}" config user.name "Stack Sandbox"
git -C "${clone_dir}" fetch origin \
  "${STACK_SANDBOX_TRUNK}:${STACK_SANDBOX_TRUNK}" \
  "${branch}:${branch}" >/dev/null

(
  cd "${clone_dir}"
  "${binary}" init --trunk "${STACK_SANDBOX_TRUNK}" --remote origin
  "${binary}" track "${branch}" --parent "${STACK_SANDBOX_TRUNK}"
  "${binary}" submit "${branch}" --yes --no-restack
  "${binary}" queue "${branch}" --yes
)

number="$(stack_sandbox_pr_number "${branch}")"
if [[ -z "${number}" ]]; then
  number="$(stack_sandbox_merged_pr_number "${branch}")"
fi
if [[ -z "${number}" ]]; then
  echo "Could not resolve sandbox PR for ${branch} after queue handoff." >&2
  exit 1
fi

echo
echo "Queue scenario result:"
gh pr view \
  --repo "${STACK_SANDBOX_REPO}" \
  "${number}" \
  --json number,state,mergeStateStatus,headRefName,baseRefName,url

if [[ "$(stack_sandbox_pr_state "${number}")" == "MERGED" ]]; then
  echo
  echo "Reseeding ${branch} so the queue fixture stays reusable"
  "${SCRIPT_DIR}/seed-fixtures.sh" >/dev/null
else
  echo
  echo "Queue fixture remains open on GitHub; leaving it in place instead of reseeding."
fi

echo "Queue live check complete."
