#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
tables="${1:-${GOALARIC_SYZYGY_PATH:-$repo_root/.tools/syzygy/3-4-5}}"
report_dir="$repo_root/artifacts/syzygy"

if [[ ! -d "$tables" ]]; then
  echo "Syzygy tables not found: $tables" >&2
  exit 2
fi
tables="$(cd "$tables" && pwd)"

mkdir -p "$report_dir"

echo "Running deterministic Syzygy probes against $tables"
GOALARIC_SYZYGY_PATH="$tables" go test -count=1 -run 'Syzygy|Tablebase' ./syzygy ./search ./uci \
  | tee "$report_dir/syzygy-test.log"

echo "Building optional cgo/Fathom engine"
go build -trimpath -o "$report_dir/goalaric-syzygy" ./GoAlaric.go

echo "Building and testing mandatory no-cgo fallback"
CGO_ENABLED=0 go build -trimpath -o "$report_dir/goalaric-no-syzygy" ./GoAlaric.go
CGO_ENABLED=0 go test -count=1 ./board ./syzygy/... ./search ./uci \
  | tee "$report_dir/no-syzygy-test.log"
bash "$repo_root/scripts/run_material_draw_tests.sh" \
  "$report_dir/goalaric-no-syzygy" \
  "$repo_root/scripts/material_draw_cases.json" \
  6 \
  "$report_dir/no-syzygy-material-draw.json"

echo "Syzygy reports saved in $report_dir"
