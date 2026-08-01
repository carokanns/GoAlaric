#!/usr/bin/env bash
# Install the pinned CC0 opening book used by monitored A/B matches.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
book_commit="65815ccdbc7727cd4f6aee252ba8f67fb740e92f"
archive_sha256="7e1e9dd118b4bb97d8a8b5b8a790c86e21f8509d59a27d2883767d94477be02e"
book_dir="$repo_root/.tools/books"
book_file="$book_dir/8moves_v3.pgn"

if [[ -s "$book_file" ]] && [[ "$(rg -c '^\[Event ' "$book_file")" == "34700" ]]; then
  echo "Opening book already installed: $book_file"
  exit 0
fi

download_dir="$(mktemp -d)"
trap 'rm -rf "$download_dir"' EXIT
archive="$download_dir/8moves_v3.pgn.zip"
base_url="https://raw.githubusercontent.com/official-stockfish/books/$book_commit"

curl --fail --location --silent --show-error \
  "$base_url/8moves_v3.pgn.zip" --output "$archive"
actual_sha256="$(sha256sum "$archive" | awk '{print $1}')"
if [[ "$actual_sha256" != "$archive_sha256" ]]; then
  echo "Opening book checksum mismatch" >&2
  exit 1
fi

mkdir -p "$book_dir"
unzip -oq "$archive" -d "$book_dir"
curl --fail --location --silent --show-error \
  "$base_url/LICENSE" --output "$book_dir/LICENSE.official-stockfish-books"

opening_count="$(rg -c '^\[Event ' "$book_file")"
if [[ "$opening_count" != "34700" ]]; then
  echo "Expected 34700 openings, found $opening_count" >&2
  exit 1
fi

echo "Installed $opening_count openings: $book_file"
