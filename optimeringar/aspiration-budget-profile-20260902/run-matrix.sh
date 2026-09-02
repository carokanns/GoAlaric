#!/usr/bin/env bash
set -euo pipefail

repo=/home/peter/Projekt/GoAlaric
experiment=aspiration-budget-profile-20260902
definition="$repo/optimeringar/$experiment"
artifact="$repo/artifacts/experiments/$experiment"
engine="$artifact/bin/goalaric"
monitor="$artifact/bin/testmonitor"
fastchess="$repo/.tools/fastchess/bin/fastchess"
openings="$repo/.tools/books/8moves_v3.pgn"
syzygy="$repo/.tools/syzygy/3-4-5-complete"
baseline="$definition/parameters/margin-10.json"

if [[ ${1:-} != --execute ]]; then
  echo "Planerad men inte startad: 15 celler, 128 partier/cell, 1920 partier totalt."
  echo "Nodbudgetar: 25000 100000 400000; marginaler: 5 10 15 20 30."
  echo "Sex samtidiga partier inom varje cell."
  echo "Start kräver uttryckligen: $0 --execute"
  exit 0
fi

for required in "$engine" "$monitor" "$fastchess" "$openings" "$syzygy" "$baseline"; do
  [[ -e $required ]] || { echo "Saknas: $required" >&2; exit 1; }
done

for nodes in 25000 100000 400000; do
  for margin in 5 10 15 20 30; do
    padded=$(printf '%02d' "$margin")
    candidate="$definition/parameters/margin-$padded.json"
    run_dir="$artifact/runs/nodes-$nodes-margin-$padded"
    if [[ -f $run_dir/status.json ]] && grep -q '"state": "completed"' "$run_dir/status.json"; then
      echo "Hoppar över färdig cell: nodes=$nodes margin=$margin"
      continue
    fi
    "$monitor" start \
      --baseline "$engine" \
      --candidate "$engine" \
      --baseline-parameter-file "$baseline" \
      --candidate-parameter-file "$candidate" \
      --optimizer-mode \
      --allow-identical-binaries \
      --fastchess "$fastchess" \
      --openings "$openings" \
      --nodes "$nodes" \
      --games 128 \
      --concurrency 6 \
      --hash 128 \
      --threads 1 \
      --seed 20261005 \
      --draw-movenumber 60 \
      --syzygy-path "$syzygy" \
      --depth-profile \
      --aspiration-profile \
      --profile-role candidate \
      --progress-games 8 \
      --progress-interval 30s \
      --run-dir "$run_dir" \
      --repo-root "$repo"
    "$monitor" wait --run-dir "$run_dir" --interval 5s
  done
done

python3 "$definition/summarize.py"
