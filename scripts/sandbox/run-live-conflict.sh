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
drift_branch="sandbox-trunk-drift"
base_branch="sandbox-conflict-base"
child_branch="sandbox-conflict-child"

echo "Building stack CLI"
stack_sandbox_go build -o "${binary}" ./cmd/stack

echo "Cloning sandbox repo into ${clone_dir}"
gh repo clone "${STACK_SANDBOX_REPO}" "${clone_dir}" -- --quiet

git -C "${clone_dir}" config user.email stack@example.com
git -C "${clone_dir}" config user.name "Stack Sandbox"
git -C "${clone_dir}" fetch origin "${base_branch}" "${child_branch}" >/dev/null
git -C "${clone_dir}" branch -f "${base_branch}" "origin/${base_branch}" >/dev/null
git -C "${clone_dir}" branch -f "${child_branch}" "origin/${child_branch}" >/dev/null

(
  cd "${clone_dir}"
  "${binary}" init --trunk "${STACK_SANDBOX_TRUNK}" --remote origin
  "${binary}" track "${base_branch}" --parent "${STACK_SANDBOX_TRUNK}"
  "${binary}" track "${child_branch}" --parent "${base_branch}"
)

echo "Merging ${drift_branch} to create real trunk drift"
drift_number="$(stack_sandbox_pr_number "${drift_branch}")"
if [[ -z "${drift_number}" ]]; then
  echo "Missing open PR for ${drift_branch}; reseed fixtures first." >&2
  exit 1
fi
gh pr merge --repo "${STACK_SANDBOX_REPO}" "${drift_number}" --merge --delete-branch=false >/dev/null

(
  cd "${clone_dir}"
  git fetch origin "${STACK_SANDBOX_TRUNK}" >/dev/null
  git reset --hard "origin/${STACK_SANDBOX_TRUNK}" >/dev/null

  set +e
  "${binary}" restack "${base_branch}" --yes
  restack_status=$?
  set -e
  if [[ ${restack_status} -eq 0 ]]; then
    echo "Expected ${base_branch} restack to conflict after trunk drift, but it succeeded." >&2
    exit 1
  fi

  if [[ ! -f ".git/stack/op.json" ]]; then
    echo "Expected worktree operation journal at .git/stack/op.json" >&2
    exit 1
  fi

  cat > "_test_data/sandbox/conflict/shared.txt" <<'EOF'
conflict scenario
owner: sandbox-conflict-base
resolution: resolved base after trunk drift
EOF
  git add "_test_data/sandbox/conflict/shared.txt"
  "${binary}" continue

  cat > "_test_data/sandbox/conflict/shared.txt" <<'EOF'
conflict scenario
owner: sandbox-conflict-child
resolution: resolved child after trunk drift
EOF
  git add "_test_data/sandbox/conflict/shared.txt"
  "${binary}" continue

  if [[ -f ".git/stack/op.json" ]]; then
    echo "Operation journal still present after finishing restack." >&2
    exit 1
  fi
)

echo
echo "Conflict scenario restack metadata after live resolution:"
(
  cd "${clone_dir}"
  "${binary}" status --json
)

echo
echo "Reseeding sandbox fixtures so future runs stay deterministic"
"${SCRIPT_DIR}/seed-fixtures.sh" >/dev/null

echo "Conflict live check complete."
