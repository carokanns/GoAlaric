#!/usr/bin/env bash
# Run the persistent Go benchmark monitor.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
engine="${1:?usage: run_search_bench.sh <engine> [repetitions] [depth] [output]}"
repetitions="${2:-7}"
depth="${3:-8}"
output="${4:-$repo_root/artifacts/bench/$(date '+%Y%m%d-%H%M%S').json}"

cd "$repo_root"
go run ./cmd/testmonitor bench \
  --engine "$engine" \
  --epd scripts/movetime_epd \
  --repetitions "$repetitions" \
  --depth "$depth" \
  --output "$output"
