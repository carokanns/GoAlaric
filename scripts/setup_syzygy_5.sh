#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
target_dir="${1:-$repo_root/.tools/syzygy/3-4-5-complete}"
base_url="https://tablebase.lichess.ovh/tables/standard"
expected_count=290
expected_bytes=983957920

mkdir -p "$target_dir"

declare -A expected_sizes expected_hashes

while IFS=$'\t' read -r size table; do
  table="${table%$'\r'}"
  material="${table%.rtbw}"
  material="${material%.rtbz}"
  non_kings="${material//K/}"
  non_kings="${non_kings//v/}"
  if (( ${#non_kings} + 2 <= 5 )); then
    expected_sizes["$table"]="$size"
  fi
done < <(curl -fsSL --retry 3 "$base_url/bytes.tsv")

while read -r checksum table; do
  table="${table%$'\r'}"
  if [[ -n "${expected_sizes[$table]:-}" ]]; then
    expected_hashes["$table"]="$checksum"
  fi
done < <(curl -fsSL --retry 3 "$base_url/sha256")

total_bytes=0
for size in "${expected_sizes[@]}"; do
  (( total_bytes += size ))
done
if (( ${#expected_sizes[@]} != expected_count ||
      ${#expected_hashes[@]} != expected_count ||
      total_bytes != expected_bytes )); then
  echo "Unexpected complete 3-5-piece Syzygy manifest" >&2
  exit 1
fi

mapfile -t tables < <(printf '%s\n' "${!expected_sizes[@]}" | sort)
for table in "${tables[@]}"; do
  expected_size="${expected_sizes[$table]}"
  expected_hash="${expected_hashes[$table]}"
  target="$target_dir/$table"
  if [[ -s "$target" ]] &&
     [[ "$(stat -c %s "$target")" == "$expected_size" ]] &&
     echo "$expected_hash  $target" | sha256sum --quiet --check -; then
    continue
  fi
  partial="$target.partial"
  if [[ "$table" == *.rtbw ]]; then
    source_dir="3-4-5-wdl"
  else
    source_dir="3-4-5-dtz"
  fi
  curl -fsSL --retry 3 --continue-at - -o "$partial" "$base_url/$source_dir/$table"
  if [[ "$(stat -c %s "$partial")" != "$expected_size" ]]; then
    echo "Wrong size for $table" >&2
    exit 1
  fi
  echo "$expected_hash  $partial" | sha256sum --check -
  mv "$partial" "$target"
done

echo "Complete 3-5-piece Syzygy tablebase installed: $target_dir"
du -sh "$target_dir"
