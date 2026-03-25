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
base_branch="sandbox-clean-base"
child_branch="sandbox-clean-child"
grandchild_branch="sandbox-clean-grandchild"

echo "Building stack CLI"
stack_sandbox_go build -o "${binary}" ./cmd/stack

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

setup_clone() {
  local clone_dir="$1"

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

  (
    cd "${clone_dir}"
    "${binary}" init --trunk "${STACK_SANDBOX_TRUNK}" --remote origin
    "${binary}" track "${base_branch}" --parent "${STACK_SANDBOX_TRUNK}"
    "${binary}" track "${child_branch}" --parent "${base_branch}"
    "${binary}" track "${grandchild_branch}" --parent "${child_branch}"
    "${binary}" submit --all --yes --no-restack
  )
}

expect_state_parent() {
  local state_file="$1"
  local branch="$2"
  local expected_parent="$3"
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
  local state_file="$1"
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

clean_clone="${tmpdir}/clean-sync"
setup_clone "${clean_clone}"
clean_state_file="${clean_clone}/.git/stack/state.json"

echo "Merging ${base_branch} to trigger clean merged-parent advancement"
merge_pr_by_head "${base_branch}"

clean_output="$(
  cd "${clean_clone}" && "${binary}" sync --apply
)"
printf "%s\n" "${clean_output}"

expect_state_parent "${clean_state_file}" "${child_branch}" "${STACK_SANDBOX_TRUNK}"
expect_pr_base "${child_branch}" "${STACK_SANDBOX_TRUNK}"

echo "Merging ${child_branch} to verify clean two-hop advancement"
merge_pr_by_head "${child_branch}"

second_clean_output="$(
  cd "${clean_clone}" && "${binary}" sync --apply
)"
printf "%s\n" "${second_clean_output}"

if [[ "${second_clean_output}" == *"manual review before reparenting"* ]]; then
  echo "Expected clean grandchild advancement, got manual review output." >&2
  exit 1
fi

expect_state_parent "${clean_state_file}" "${grandchild_branch}" "${STACK_SANDBOX_TRUNK}"
expect_pr_base "${grandchild_branch}" "${STACK_SANDBOX_TRUNK}"

echo
echo "Reseeding sandbox fixtures before the ambiguous sync scenario"
"${SCRIPT_DIR}/seed-fixtures.sh" >/dev/null

ambiguous_clone="${tmpdir}/ambiguous-sync"
setup_clone "${ambiguous_clone}"
ambiguous_state_file="${ambiguous_clone}/.git/stack/state.json"

echo "Merging ${base_branch} again for ambiguous merged-parent handling"
merge_pr_by_head "${base_branch}"

ambiguous_clean_output="$(
  cd "${ambiguous_clone}" && "${binary}" sync --apply
)"
printf "%s\n" "${ambiguous_clean_output}"

expect_state_parent "${ambiguous_state_file}" "${child_branch}" "${STACK_SANDBOX_TRUNK}"
expect_pr_base "${child_branch}" "${STACK_SANDBOX_TRUNK}"

echo "Corrupting the grandchild anchor to force manual review"
(
  cd "${ambiguous_clone}"
  set_grandchild_anchor_to_main_head "${ambiguous_state_file}"
)

echo "Merging ${child_branch} to verify ambiguous merged-parent handling"
merge_pr_by_head "${child_branch}"

ambiguous_output="$(
  cd "${ambiguous_clone}" && "${binary}" sync --apply
)"
printf "%s\n" "${ambiguous_output}"

if [[ "${ambiguous_output}" != *"manual review before reparenting"* ]]; then
  echo "Expected sync output to report manual review for ${grandchild_branch}" >&2
  exit 1
fi

expect_state_parent "${ambiguous_state_file}" "${grandchild_branch}" "${child_branch}"
expect_pr_base "${grandchild_branch}" "${child_branch}"

echo
echo "Ambiguous sync scenario status after live verification:"
(
  cd "${ambiguous_clone}"
  "${binary}" status --json
)

echo
echo "Reseeding sandbox fixtures so future runs stay deterministic"
"${SCRIPT_DIR}/seed-fixtures.sh" >/dev/null

echo "Sync live check complete."
