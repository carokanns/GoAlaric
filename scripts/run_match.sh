#!/usr/bin/env bash
# Start a monitored Fastchess match in the background.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
baseline="${1:?usage: run_match.sh <baseline> <candidate> [openings] [games]}"
candidate="${2:?usage: run_match.sh <baseline> <candidate> [openings] [games]}"
openings="${3:-}"
games="${4:-400}"

cd "$repo_root"
args=(
  start
  --fastchess .tools/fastchess/bin/fastchess
  --baseline "$baseline"
  --candidate "$candidate"
  --games "$games"
)
if [[ -n "$openings" ]]; then
  args+=(--openings "$openings")
fi
go run ./cmd/testmonitor "${args[@]}"
