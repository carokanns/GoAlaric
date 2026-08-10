#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
engine="${1:-$repo_root/bin/goalaric_material_draw}"
cases="${2:-$repo_root/scripts/material_draw_cases.json}"
depth="${3:-6}"
report="${4:-$repo_root/artifacts/material-draw/report.json}"

if [[ $# -eq 0 ]]; then
  mkdir -p "$(dirname "$engine")"
  GO111MODULE=on go build -o "$engine" "$repo_root/GoAlaric.go"
fi

GO111MODULE=on go run "$repo_root/cmd/materialdrawtest" \
  --engine "$engine" \
  --cases "$cases" \
  --depth "$depth" \
  --output "$report"

echo "Resultat sparat i $report"
