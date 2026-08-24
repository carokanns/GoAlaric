#!/usr/bin/env bash
# Compare the fixed-depth tree size with historical LMP and depth-4 LMP.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
source_epd="${1:?usage: run_lmp_depth_probe.sh <source-epd> [positions] [depth] [output-dir]}"
positions="${2:-200}"
depth="${3:-12}"
output_dir="${4:-$repo_root/artifacts/bench/lmp-depth-probe}"

source_epd="$(realpath "$source_epd")"
mkdir -p "$output_dir"
output_dir="$(realpath "$output_dir")"

probe_tmp="$(mktemp -d)"
trap 'rm -rf "$probe_tmp"' EXIT

eligible="$(awk '{p=$1; gsub(/[1-8\/]/,"",p); if(length(p)>=18)n++} END{print n+0}' "$source_epd")"
if (( eligible < positions )); then
  echo "only $eligible positions contain at least 18 pieces; need $positions" >&2
  exit 1
fi
stride=$((eligible / positions))
awk -v stride="$stride" -v wanted="$positions" '
  {p=$1; gsub(/[1-8\/]/,"",p)}
  length(p)>=18 {seen++; if(seen%stride==0 && written<wanted){print; written++}}
' "$source_epd" >"$probe_tmp/positions.epd"

go build -o "$probe_tmp/goalaric" "$repo_root"
go build -o "$probe_tmp/testmonitor" "$repo_root/cmd/testmonitor"

baseline="$repo_root/optimizer/registries/search-lmp-depth-v1-default.json"
candidate="$repo_root/optimizer/registries/search-lmp-depth-v1-iteration12.json"

"$probe_tmp/testmonitor" bench --engine "$probe_tmp/goalaric" \
  --parameter-file "$baseline" --epd "$probe_tmp/positions.epd" \
  --depth "$depth" --repetitions 1 --output "$output_dir/baseline.json"
"$probe_tmp/testmonitor" bench --engine "$probe_tmp/goalaric" \
  --parameter-file "$candidate" --epd "$probe_tmp/positions.epd" \
  --depth "$depth" --repetitions 1 --output "$output_dir/iteration12.json"

jq -n --argjson depth "$depth" --slurpfile b "$output_dir/baseline.json" --slurpfile c "$output_dir/iteration12.json" '
  [range(0; ($b[0].cases|length)) as $i |
    {baseline:$b[0].cases[$i].median_nodes,
     candidate:$c[0].cases[$i].median_nodes,
     baseline_move:$b[0].cases[$i].samples[0].bestmove,
     candidate_move:$c[0].cases[$i].samples[0].bestmove}] as $cases |
  ($cases|map(.baseline)|add) as $baseline |
  ($cases|map(.candidate)|add) as $candidate |
  {positions:($cases|length), depth:$depth,
   baseline_nodes:$baseline, candidate_nodes:$candidate,
   net_node_change:($candidate-$baseline),
   net_percent:(($candidate-$baseline)*100/$baseline),
   fewer:($cases|map(select(.candidate<.baseline))|length),
   more:($cases|map(select(.candidate>.baseline))|length),
   equal:($cases|map(select(.candidate==.baseline))|length),
   changed_bestmove:($cases|map(select(.candidate_move!=.baseline_move))|length)}
' | tee "$output_dir/summary.json"
