#!/usr/bin/env bash
# Kör en gemensam testkedja: go test + perft + movetime.
# Användning: ./scripts/run_all_tests.sh [perft_depth] [perft_epd] [movetime_ms] [movetime_epd]

set -euo pipefail

perft_depth="${1:-4}"
perft_epd="${2:-scripts/perft_tests.txt}"
movetime_ms="${3:-2000}"
movetime_epd="${4:-scripts/movetime_epd}"

repo_root="$(cd "$(dirname "$0")/.." && pwd)"

echo "==> go test ./..."
GO111MODULE=on go test "$repo_root"/...

echo "==> perft: depth=$perft_depth epd=$perft_epd"
"$repo_root/scripts/run_perft.sh" "$perft_depth" "$repo_root/$perft_epd" "$repo_root/perft_results.txt"

echo "==> movetime: ms=$movetime_ms epd=$movetime_epd"
"$repo_root/scripts/run_movetime_epd.sh" "$movetime_ms" "$repo_root/$movetime_epd" "$repo_root/movetime_results.txt"

echo "All tests completed."
