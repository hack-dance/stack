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
base_branch="sandbox-clean-base"
child_branch="sandbox-clean-child"
grandchild_branch="sandbox-clean-grandchild"

echo "Building stack CLI"
stack_sandbox_go build -o "${binary}" ./cmd/stack

echo "Cloning sandbox repo into ${clone_dir}"
gh repo clone "${STACK_SANDBOX_REPO}" "${clone_dir}" -- --quiet

git -C "${clone_dir}" config user.email stack@example.com
git -C "${clone_dir}" config user.name "Stack Sandbox"
git -C "${clone_dir}" fetch origin \
  "${base_branch}" \
  "${child_branch}" \
  "${grandchild_branch}" >/dev/null
git -C "${clone_dir}" branch -f "${base_branch}" "origin/${base_branch}" >/dev/null
git -C "${clone_dir}" branch -f "${child_branch}" "origin/${child_branch}" >/dev/null
git -C "${clone_dir}" branch -f "${grandchild_branch}" "origin/${grandchild_branch}" >/dev/null

state_file="${clone_dir}/.git/stack/state.json"

merge_pr_by_head() {
  local branch="$1"
  local number
  number="$(stack_sandbox_pr_number "${branch}")"
  if [[ -z "${number}" ]]; then
    echo "Missing open PR for ${branch}; reseed fixtures first." >&2
    exit 1
  fi
  gh pr merge --repo "${STACK_SANDBOX_REPO}" "${number}" --merge --delete-branch=false >/dev/null
}

expect_pr_base() {
  local branch="$1"
  local expected_base="$2"
  local actual_base
  actual_base="$(
    gh pr list \
      --repo "${STACK_SANDBOX_REPO}" \
      --head "${branch}" \
      --state open \
      --json baseRefName \
      --jq '.[0].baseRefName // empty'
  )"
  if [[ "${actual_base}" != "${expected_base}" ]]; then
    echo "Expected PR base for ${branch} to be ${expected_base}, got ${actual_base}" >&2
    exit 1
  fi
}

expect_state_parent() {
  local branch="$1"
  local expected_parent="$2"
  python3 - "$state_file" "$branch" "$expected_parent" <<'PY'
import json
import sys

state_path, branch, expected_parent = sys.argv[1:]
with open(state_path, "r", encoding="utf-8") as handle:
    state = json.load(handle)
actual_parent = state["branches"][branch]["parentBranch"]
if actual_parent != expected_parent:
    raise SystemExit(f"expected {branch} parent {expected_parent}, got {actual_parent}")
PY
}

set_grandchild_anchor_to_main_head() {
  python3 - "$state_file" "$grandchild_branch" <<'PY'
import json
import subprocess
import sys

state_path, branch = sys.argv[1:]
main_head = subprocess.check_output(
    ["git", "rev-parse", "main"], text=True
).strip()
with open(state_path, "r", encoding="utf-8") as handle:
    state = json.load(handle)
state["branches"][branch]["restack"]["lastParentHeadOid"] = main_head
with open(state_path, "w", encoding="utf-8") as handle:
    json.dump(state, handle, indent=2)
    handle.write("\n")
PY
}

(
  cd "${clone_dir}"
  "${binary}" init --trunk "${STACK_SANDBOX_TRUNK}" --remote origin
  "${binary}" track "${base_branch}" --parent "${STACK_SANDBOX_TRUNK}"
  "${binary}" track "${child_branch}" --parent "${base_branch}"
  "${binary}" track "${grandchild_branch}" --parent "${child_branch}"
  "${binary}" submit --all --yes --no-restack
)

echo "Merging ${base_branch} to trigger clean merged-parent advancement"
merge_pr_by_head "${base_branch}"

(
  cd "${clone_dir}"
  sync_output="$("${binary}" sync --apply)"
  printf "%s\n" "${sync_output}"
)

expect_state_parent "${child_branch}" "${STACK_SANDBOX_TRUNK}"
expect_pr_base "${child_branch}" "${STACK_SANDBOX_TRUNK}"
expect_state_parent "${grandchild_branch}" "${child_branch}"

echo "Corrupting grandchild anchor to force manual review after the child merge"
(
  cd "${clone_dir}"
  set_grandchild_anchor_to_main_head
)

echo "Merging ${child_branch} to trigger ambiguous merged-parent handling"
merge_pr_by_head "${child_branch}"

ambiguous_output="$(
  cd "${clone_dir}" && "${binary}" sync --apply
)"
printf "%s\n" "${ambiguous_output}"

if [[ "${ambiguous_output}" != *"manual review before reparenting"* ]]; then
  echo "Expected sync output to report manual review for ${grandchild_branch}" >&2
  exit 1
fi

expect_state_parent "${grandchild_branch}" "${child_branch}"
expect_pr_base "${grandchild_branch}" "${child_branch}"

echo
echo "Sync scenario status after live verification:"
(
  cd "${clone_dir}"
  "${binary}" status --json
)

echo
echo "Reseeding sandbox fixtures so future runs stay deterministic"
"${SCRIPT_DIR}/seed-fixtures.sh" >/dev/null

echo "Sync live check complete."
