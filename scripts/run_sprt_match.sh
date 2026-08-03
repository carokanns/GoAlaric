#!/usr/bin/env bash
# Start the decisive monitored SPRT match in the background.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
baseline="${1:?usage: run_sprt_match.sh <baseline> <candidate> [openings] [games]}"
candidate="${2:?usage: run_sprt_match.sh <baseline> <candidate> [openings] [games]}"
openings="${3:-}"
games="${4:-10000}"

cd "$repo_root"
args=(
  start
  --follow
  --fastchess .tools/fastchess/bin/fastchess
  --baseline "$baseline"
  --candidate "$candidate"
  --games "$games"
  --tc 20+0.2
  --sprt
)
if [[ -n "$openings" ]]; then
  args+=(--openings "$openings")
fi
go run ./cmd/testmonitor "${args[@]}"
