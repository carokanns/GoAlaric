#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
target_dir="${1:-$repo_root/.tools/syzygy/small-3}"
base_url="http://tablebase.sesse.net/syzygy/3-4-5"

mkdir -p "$target_dir"

# Complete useful three-piece set. KvK needs no table file.
tables=(
  "99bf9b05295781611cdd7c5c3d51bf85 KBvK.rtbw"
  "88d5f823e67448b279bb045977a80a39 KBvK.rtbz"
  "b6781a75ffe2ab41507f91151869a418 KNvK.rtbw"
  "42893523156bbc5d8c3c7207a7710ad7 KNvK.rtbz"
  "46f6ef491bd26696d7b20281e7c5b721 KPvK.rtbw"
  "54460894c15f087cfd16670bf1513755 KPvK.rtbz"
  "f06221548404795b6b33469e247b4560 KQvK.rtbw"
  "ac866466e16eb19a4f8c796f8e1abd2b KQvK.rtbz"
  "89a27823bfa03d0b0f25728c4f0fb571 KRvK.rtbw"
  "9cb0795fa43904a3e91bf749971964b8 KRvK.rtbz"
)

for entry in "${tables[@]}"; do
  read -r expected table <<<"$entry"
  target="$target_dir/$table"
  if [[ -s "$target" ]] && echo "$expected  $target" | md5sum --quiet --check -; then
    continue
  fi
  partial="$target.partial"
  curl -fsSL --retry 3 -o "$partial" "$base_url/$table"
  echo "$expected  $partial" | md5sum --check -
  mv "$partial" "$target"
done

echo "Small Syzygy tablebase installed: $target_dir"
du -sh "$target_dir"
