#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../sandbox/lib.sh
source "${SCRIPT_DIR}/../sandbox/lib.sh"

stack_sandbox_require_tools
gh auth status -h github.com >/dev/null

repo_root="$(stack_sandbox_repo_root)"
demo_root="${repo_root}/.vhs/demo/status"
binary_dir="${repo_root}/.vhs/bin"
clone_dir="${demo_root}/repo"

rm -rf "${demo_root}"
mkdir -p "${binary_dir}"

if [[ -n "${STACK_DEMO_GOOS:-}" && -n "${STACK_DEMO_GOARCH:-}" ]]; then
  GOOS="${STACK_DEMO_GOOS}" GOARCH="${STACK_DEMO_GOARCH}" CGO_ENABLED=0 \
    stack_sandbox_go build -o "${binary_dir}/stack" ./cmd/stack >/dev/null
else
  stack_sandbox_go build -o "${binary_dir}/stack" ./cmd/stack >/dev/null
fi

gh repo clone "${STACK_SANDBOX_REPO}" "${clone_dir}" -- --quiet >/dev/null

git -C "${clone_dir}" config user.email stack@example.com
git -C "${clone_dir}" config user.name "Stack Demo"
git -C "${clone_dir}" fetch origin \
  sandbox-clean-base \
  sandbox-clean-child \
  sandbox-clean-grandchild >/dev/null
git -C "${clone_dir}" branch -f sandbox-clean-base origin/sandbox-clean-base >/dev/null
git -C "${clone_dir}" branch -f sandbox-clean-child origin/sandbox-clean-child >/dev/null
git -C "${clone_dir}" branch -f sandbox-clean-grandchild origin/sandbox-clean-grandchild >/dev/null
git -C "${clone_dir}" switch sandbox-clean-grandchild >/dev/null
cp "${binary_dir}/stack" "${clone_dir}/stack-demo"
chmod +x "${clone_dir}/stack-demo"

echo "status demo ready"
