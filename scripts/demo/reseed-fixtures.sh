#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../sandbox/lib.sh
source "${SCRIPT_DIR}/../sandbox/lib.sh"

stack_sandbox_require_tools
gh auth status -h github.com >/dev/null

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

gh repo clone "${STACK_SANDBOX_REPO}" "${tmpdir}/repo" -- --quiet >/dev/null
git -C "${tmpdir}/repo" config user.email stack@example.com
git -C "${tmpdir}/repo" config user.name "Stack Demo"
(
  cd "${tmpdir}/repo"
  scripts/sandbox/seed-fixtures.sh >/dev/null
)
