#!/usr/bin/env bash
# Compare committed baseline and candidate search trees on fixed zugzwangs.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
baseline_ref="${1:-master}"
candidate_ref="${2:-HEAD}"
depth="${3:-14}"
output_dir="${4:-$repo_root/artifacts/bench/null-verification}"
epd="$repo_root/scripts/null_move_zugzwang.epd"

mkdir -p "$output_dir"
output_dir="$(realpath "$output_dir")"
probe_tmp="$(mktemp -d)"
trap 'rm -rf "$probe_tmp"' EXIT

mkdir -p "$probe_tmp/baseline-src" "$probe_tmp/candidate-src"
git -C "$repo_root" archive "$baseline_ref" | tar -x -C "$probe_tmp/baseline-src"
git -C "$repo_root" archive "$candidate_ref" | tar -x -C "$probe_tmp/candidate-src"

(
  cd "$probe_tmp/baseline-src"
  go build -o "$probe_tmp/baseline" .
)
(
  cd "$probe_tmp/candidate-src"
  go build -o "$probe_tmp/candidate" .
  go build -o "$probe_tmp/testmonitor" ./cmd/testmonitor
)

"$probe_tmp/testmonitor" bench \
  --engine "$probe_tmp/baseline" \
  --epd "$epd" \
  --depth "$depth" \
  --repetitions 1 \
  --output "$output_dir/baseline.json"

"$probe_tmp/testmonitor" bench \
  --engine "$probe_tmp/candidate" \
  --epd "$epd" \
  --depth "$depth" \
  --repetitions 1 \
  --output "$output_dir/candidate.json"

jq -n \
  --arg baseline_ref "$baseline_ref" \
  --arg candidate_ref "$candidate_ref" \
  --argjson depth "$depth" \
  --slurpfile baseline "$output_dir/baseline.json" \
  --slurpfile candidate "$output_dir/candidate.json" '
  [range(0; ($baseline[0].cases | length)) as $i |
    {
      fen: $baseline[0].cases[$i].fen,
      baseline_nodes: $baseline[0].cases[$i].median_nodes,
      candidate_nodes: $candidate[0].cases[$i].median_nodes,
      baseline_move: $baseline[0].cases[$i].samples[0].bestmove,
      candidate_move: $candidate[0].cases[$i].samples[0].bestmove,
      baseline_score: $baseline[0].cases[$i].samples[0].score,
      candidate_score: $candidate[0].cases[$i].samples[0].score
    }
  ] as $cases |
  ($cases | map(.baseline_nodes) | add) as $baseline_nodes |
  ($cases | map(.candidate_nodes) | add) as $candidate_nodes |
  {
    baseline_ref: $baseline_ref,
    candidate_ref: $candidate_ref,
    depth: $depth,
    positions: ($cases | length),
    baseline_nodes: $baseline_nodes,
    candidate_nodes: $candidate_nodes,
    node_change: ($candidate_nodes - $baseline_nodes),
    node_change_percent: (($candidate_nodes - $baseline_nodes) * 100 / $baseline_nodes),
    changed_bestmove: ($cases | map(select(.baseline_move != .candidate_move)) | length),
    changed_score: ($cases | map(select(.baseline_score != .candidate_score)) | length),
    cases: $cases
  }
' | tee "$output_dir/summary.json"
