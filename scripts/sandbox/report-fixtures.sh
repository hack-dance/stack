#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "${SCRIPT_DIR}/lib.sh"

stack_sandbox_require_tools
REPO_ROOT="$(stack_sandbox_repo_root)"
cd "${REPO_ROOT}"

for branch in "${STACK_SANDBOX_BRANCHES[@]}"; do
  stack_sandbox_report_branch "${branch}"
done
