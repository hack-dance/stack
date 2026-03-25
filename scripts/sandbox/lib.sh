#!/usr/bin/env bash

set -euo pipefail

STACK_SANDBOX_REMOTE="${STACK_SANDBOX_REMOTE:-origin}"
STACK_SANDBOX_TRUNK="${STACK_SANDBOX_TRUNK:-main}"
STACK_SANDBOX_REPO="${STACK_SANDBOX_REPO:-hack-dance/stack}"

STACK_SANDBOX_BRANCHES=(
  "sandbox-clean-base"
  "sandbox-clean-child"
  "sandbox-clean-grandchild"
  "sandbox-overlap-base"
  "sandbox-overlap-child"
  "sandbox-conflict-base"
  "sandbox-conflict-child"
  "sandbox-trunk-drift"
  "sandbox-queue-ready"
)

stack_sandbox_repo_root() {
  git rev-parse --show-toplevel
}

stack_sandbox_require_tools() {
  command -v git >/dev/null 2>&1 || {
    echo "git is required" >&2
    exit 1
  }
  command -v gh >/dev/null 2>&1 || {
    echo "gh is required" >&2
    exit 1
  }
}

stack_sandbox_go() {
  if command -v go >/dev/null 2>&1; then
    go "$@"
    return
  fi
  if command -v mise >/dev/null 2>&1; then
    mise exec -- go "$@"
    return
  fi

  echo "go or mise is required" >&2
  exit 1
}

stack_sandbox_ensure_clean_worktree() {
  git diff --quiet
  git diff --cached --quiet
}

stack_sandbox_current_branch() {
  git branch --show-current
}

stack_sandbox_restore_branch() {
  local branch="$1"
  if [[ -n "${branch}" ]] && git show-ref --verify --quiet "refs/heads/${branch}"; then
    git switch "${branch}" >/dev/null
  fi
}

stack_sandbox_fetch() {
  git fetch --prune "${STACK_SANDBOX_REMOTE}"
}

stack_sandbox_remote_head() {
  local branch="$1"
  git show-ref --hash --verify "refs/remotes/${STACK_SANDBOX_REMOTE}/${branch}" 2>/dev/null || true
}

stack_sandbox_push_branch() {
  local branch="$1"
  local remote_head
  remote_head="$(stack_sandbox_remote_head "${branch}")"
  if [[ -n "${remote_head}" ]]; then
    git push \
      "--force-with-lease=refs/heads/${branch}:${remote_head}" \
      "${STACK_SANDBOX_REMOTE}" \
      "${branch}:refs/heads/${branch}"
  else
    git push --set-upstream "${STACK_SANDBOX_REMOTE}" "${branch}:refs/heads/${branch}"
  fi
}

stack_sandbox_pr_number() {
  local branch="$1"
  gh pr list \
    --repo "${STACK_SANDBOX_REPO}" \
    --head "${branch}" \
    --state open \
    --json number \
    --jq '.[0].number // empty'
}

stack_sandbox_reopenable_pr_number() {
  local branch="$1"
  gh pr list \
    --repo "${STACK_SANDBOX_REPO}" \
    --head "${branch}" \
    --state closed \
    --json number \
    --jq '.[0].number // empty'
}

stack_sandbox_merged_pr_number() {
  local branch="$1"
  gh pr list \
    --repo "${STACK_SANDBOX_REPO}" \
    --head "${branch}" \
    --state merged \
    --json number \
    --jq '.[0].number // empty'
}

stack_sandbox_pr_state() {
  local number="$1"
  gh pr view \
    --repo "${STACK_SANDBOX_REPO}" \
    "${number}" \
    --json state \
    --jq '.state'
}

stack_sandbox_upsert_pr() {
  local branch="$1"
  local pr_base="$2"
  local title="$3"
  local body="$4"

  local number
  number="$(stack_sandbox_pr_number "${branch}")"
  if [[ -z "${number}" ]]; then
    number="$(stack_sandbox_reopenable_pr_number "${branch}")"
  fi

  if [[ -z "${number}" ]]; then
    gh pr create \
      --repo "${STACK_SANDBOX_REPO}" \
      --base "${pr_base}" \
      --head "${branch}" \
      --title "${title}" \
      --body "${body}" >/dev/null
    return
  fi

  local state
  state="$(stack_sandbox_pr_state "${number}")"
  if [[ "${state}" == "CLOSED" ]]; then
    gh pr reopen --repo "${STACK_SANDBOX_REPO}" "${number}" >/dev/null
  fi

  gh pr edit \
    --repo "${STACK_SANDBOX_REPO}" \
    "${number}" \
    --base "${pr_base}" \
    --title "${title}" \
    --body "${body}" >/dev/null
}

stack_sandbox_known_branch() {
  local branch="$1"
  local known
  for known in "${STACK_SANDBOX_BRANCHES[@]}"; do
    if [[ "${known}" == "${branch}" ]]; then
      return 0
    fi
  done
  return 1
}

stack_sandbox_branch_git_base() {
  case "$1" in
    sandbox-clean-base) echo "${STACK_SANDBOX_REMOTE}/${STACK_SANDBOX_TRUNK}" ;;
    sandbox-clean-child) echo "sandbox-clean-base" ;;
    sandbox-clean-grandchild) echo "sandbox-clean-child" ;;
    sandbox-overlap-base) echo "${STACK_SANDBOX_REMOTE}/${STACK_SANDBOX_TRUNK}" ;;
    sandbox-overlap-child) echo "sandbox-overlap-base" ;;
    sandbox-conflict-base) echo "${STACK_SANDBOX_REMOTE}/${STACK_SANDBOX_TRUNK}" ;;
    sandbox-conflict-child) echo "sandbox-conflict-base" ;;
    sandbox-trunk-drift) echo "${STACK_SANDBOX_REMOTE}/${STACK_SANDBOX_TRUNK}" ;;
    sandbox-queue-ready) echo "${STACK_SANDBOX_REMOTE}/${STACK_SANDBOX_TRUNK}" ;;
    *) echo "unknown sandbox branch: $1" >&2; exit 1 ;;
  esac
}

stack_sandbox_branch_pr_base() {
  case "$1" in
    sandbox-clean-base|sandbox-overlap-base|sandbox-conflict-base|sandbox-trunk-drift|sandbox-queue-ready)
      echo "${STACK_SANDBOX_TRUNK}"
      ;;
    sandbox-clean-child) echo "sandbox-clean-base" ;;
    sandbox-clean-grandchild) echo "sandbox-clean-child" ;;
    sandbox-overlap-child) echo "sandbox-overlap-base" ;;
    sandbox-conflict-child) echo "sandbox-conflict-base" ;;
    *) echo "unknown sandbox branch: $1" >&2; exit 1 ;;
  esac
}

stack_sandbox_branch_title() {
  case "$1" in
    sandbox-clean-base) echo "sandbox: clean base" ;;
    sandbox-clean-child) echo "sandbox: clean child" ;;
    sandbox-clean-grandchild) echo "sandbox: clean grandchild" ;;
    sandbox-overlap-base) echo "sandbox: overlap base" ;;
    sandbox-overlap-child) echo "sandbox: overlap child" ;;
    sandbox-conflict-base) echo "sandbox: conflict base" ;;
    sandbox-conflict-child) echo "sandbox: conflict child" ;;
    sandbox-trunk-drift) echo "sandbox: trunk drift" ;;
    sandbox-queue-ready) echo "sandbox: queue ready" ;;
    *) echo "unknown sandbox branch: $1" >&2; exit 1 ;;
  esac
}

stack_sandbox_branch_commit_message() {
  case "$1" in
    sandbox-clean-base) echo "testdata: seed sandbox clean base" ;;
    sandbox-clean-child) echo "testdata: seed sandbox clean child" ;;
    sandbox-clean-grandchild) echo "testdata: seed sandbox clean grandchild" ;;
    sandbox-overlap-base) echo "testdata: seed sandbox overlap base" ;;
    sandbox-overlap-child) echo "testdata: seed sandbox overlap child" ;;
    sandbox-conflict-base) echo "testdata: seed sandbox conflict base" ;;
    sandbox-conflict-child) echo "testdata: seed sandbox conflict child" ;;
    sandbox-trunk-drift) echo "testdata: seed sandbox trunk drift" ;;
    sandbox-queue-ready) echo "testdata: seed sandbox queue ready" ;;
    *) echo "unknown sandbox branch: $1" >&2; exit 1 ;;
  esac
}

stack_sandbox_branch_body() {
  case "$1" in
    sandbox-clean-base)
      cat <<'EOF'
Baseline clean parent PR for stacked-flow testing.

Scenario:
- clean stack
- bottom branch for queue and sync checks
EOF
      ;;
    sandbox-clean-child)
      cat <<'EOF'
Second clean stacked PR targeting `sandbox-clean-base`.

Scenario:
- clean stack
- child branch for submit, sync, and parent advancement checks
EOF
      ;;
    sandbox-clean-grandchild)
      cat <<'EOF'
Third clean stacked PR targeting `sandbox-clean-child`.

Scenario:
- clean stack
- grandchild branch for multi-level stacked PR checks
EOF
      ;;
    sandbox-overlap-base)
      cat <<'EOF'
Parent PR for same-file overlap testing.

Scenario:
- same-file overlap
- base branch edits section A of one shared file
EOF
      ;;
    sandbox-overlap-child)
      cat <<'EOF'
Child PR for same-file overlap testing.

Scenario:
- same-file overlap
- child branch edits section B of the same shared file
EOF
      ;;
    sandbox-conflict-base)
      cat <<'EOF'
Parent PR for trunk-drift conflict testing.

Scenario:
- trunk drift / conflict
- base branch edits the shared conflict file
EOF
      ;;
    sandbox-conflict-child)
      cat <<'EOF'
Child PR for trunk-drift conflict testing.

Scenario:
- trunk drift / conflict
- child branch further edits the same conflict block
EOF
      ;;
    sandbox-trunk-drift)
      cat <<'EOF'
Standalone PR used to create trunk drift against the conflict stack.

Scenario:
- trunk drift / conflict
- merge this before restacking the conflict stack to trigger real rebase conflicts
EOF
      ;;
    sandbox-queue-ready)
      cat <<'EOF'
Standalone bottom-of-stack PR for queue and auto-merge testing.

Scenario:
- queue-ready
- isolated PR targeting main
EOF
      ;;
    *)
      echo "unknown sandbox branch: $1" >&2
      exit 1
      ;;
  esac
}

stack_sandbox_render_branch() {
  local branch="$1"

  mkdir -p "_test_data/sandbox"
  case "${branch}" in
    sandbox-clean-base)
      mkdir -p "_test_data/sandbox/clean"
      cat > "_test_data/sandbox/clean/base.txt" <<'EOF'
clean stack: base branch
purpose: baseline parent PR
EOF
      ;;
    sandbox-clean-child)
      mkdir -p "_test_data/sandbox/clean"
      cat > "_test_data/sandbox/clean/child.txt" <<'EOF'
clean stack: child branch
purpose: child PR on top of sandbox-clean-base
EOF
      ;;
    sandbox-clean-grandchild)
      mkdir -p "_test_data/sandbox/clean"
      cat > "_test_data/sandbox/clean/grandchild.txt" <<'EOF'
clean stack: grandchild branch
purpose: grandchild PR on top of sandbox-clean-child
EOF
      ;;
    sandbox-overlap-base)
      mkdir -p "_test_data/sandbox/overlap"
      cat > "_test_data/sandbox/overlap/shared.md" <<'EOF'
# Same-File Overlap

## Section A

Parent branch owns this section and introduces the file.

## Section B

This section will be edited by the child branch.
EOF
      ;;
    sandbox-overlap-child)
      mkdir -p "_test_data/sandbox/overlap"
      cat > "_test_data/sandbox/overlap/shared.md" <<'EOF'
# Same-File Overlap

## Section A

Parent branch owns this section and introduces the file.

## Section B

Child branch rewrites this section to create same-file stacked diffs without a guaranteed merge conflict.
EOF
      ;;
    sandbox-conflict-base)
      mkdir -p "_test_data/sandbox/conflict"
      cat > "_test_data/sandbox/conflict/shared.txt" <<'EOF'
conflict scenario
owner: sandbox-conflict-base
resolution: parent branch version
EOF
      ;;
    sandbox-conflict-child)
      mkdir -p "_test_data/sandbox/conflict"
      cat > "_test_data/sandbox/conflict/shared.txt" <<'EOF'
conflict scenario
owner: sandbox-conflict-child
resolution: child branch version
EOF
      ;;
    sandbox-trunk-drift)
      mkdir -p "_test_data/sandbox/conflict"
      cat > "_test_data/sandbox/conflict/shared.txt" <<'EOF'
conflict scenario
owner: sandbox-trunk-drift
resolution: trunk drift version
EOF
      ;;
    sandbox-queue-ready)
      mkdir -p "_test_data/sandbox/queue"
      cat > "_test_data/sandbox/queue/ready.txt" <<'EOF'
queue-ready fixture
purpose: isolated bottom PR for merge queue and auto-merge checks
EOF
      ;;
    *)
      echo "unknown sandbox branch: ${branch}" >&2
      exit 1
      ;;
  esac
}

stack_sandbox_seed_branch() {
  local branch="$1"
  local git_base
  local pr_base
  local title
  local body
  local commit_message

  git_base="$(stack_sandbox_branch_git_base "${branch}")"
  pr_base="$(stack_sandbox_branch_pr_base "${branch}")"
  title="$(stack_sandbox_branch_title "${branch}")"
  body="$(stack_sandbox_branch_body "${branch}")"
  commit_message="$(stack_sandbox_branch_commit_message "${branch}")"

  git switch --detach "${git_base}" >/dev/null
  git switch -C "${branch}" >/dev/null
  stack_sandbox_render_branch "${branch}"
  git add "_test_data/sandbox"
  if git diff --cached --quiet; then
    echo "No fixture changes for ${branch}" >&2
  else
    git commit -m "${commit_message}" >/dev/null
  fi
  stack_sandbox_push_branch "${branch}"
  stack_sandbox_upsert_pr "${branch}" "${pr_base}" "${title}" "${body}"
}

stack_sandbox_report_branch() {
  local branch="$1"
  local number
  number="$(stack_sandbox_pr_number "${branch}")"

  if [[ -n "${number}" ]]; then
    gh pr view \
      --repo "${STACK_SANDBOX_REPO}" \
      "${number}" \
      --json number,title,headRefName,baseRefName,state,isDraft,url
  else
    number="$(stack_sandbox_reopenable_pr_number "${branch}")"
    if [[ -n "${number}" ]]; then
      gh pr view \
        --repo "${STACK_SANDBOX_REPO}" \
        "${number}" \
        --json number,title,headRefName,baseRefName,state,isDraft,url
    else
      number="$(stack_sandbox_merged_pr_number "${branch}")"
      if [[ -n "${number}" ]]; then
        gh pr view \
          --repo "${STACK_SANDBOX_REPO}" \
          "${number}" \
          --json number,title,headRefName,baseRefName,state,isDraft,url
      else
        printf '{"headRefName":"%s","state":"MISSING","url":"","number":0,"title":"","baseRefName":""}\n' "${branch}"
      fi
    fi
  fi
}
