#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

if ! command -v gh >/dev/null 2>&1; then
  echo "gh is required to render demos" >&2
  exit 1
fi

render_cmd='
set -euo pipefail
cd "'"${REPO_ROOT}"'"
vhs docs/demo/status.tape
vhs docs/demo/queue.tape
'

"${REPO_ROOT}/scripts/demo/setup-status-demo.sh" >/dev/null
"${REPO_ROOT}/scripts/demo/setup-queue-demo.sh" >/dev/null

if command -v vhs >/dev/null 2>&1 && command -v ffmpeg >/dev/null 2>&1 && command -v ttyd >/dev/null 2>&1; then
  bash -lc "${render_cmd}"
else
  if ! command -v nix-shell >/dev/null 2>&1; then
    echo "rendering demos requires either local vhs/ffmpeg/ttyd or nix-shell" >&2
    exit 1
  fi
  nix-shell -p vhs ffmpeg ttyd git gh --run "${render_cmd}"
fi

"${REPO_ROOT}/scripts/demo/reseed-fixtures.sh" >/dev/null
