#!/usr/bin/env bash
set -euo pipefail

FILE="${1:-filstruktur.json}"
ROOT_DIR="${ROOT_DIR:-$(pwd)}"
EXTRA_ALLOWED=${EXTRA_ALLOWED:-math} # lägg till fler med mellanslag, ex: "math time"

# Plocka upp alla icke-dolda mappar i roten (plus Root) som tillåtna.
mapfile -t DIRS < <(find "$ROOT_DIR" -maxdepth 1 -mindepth 1 -type d -printf '%f\n' | grep -v '^\.')

if [[ -n "$EXTRA_ALLOWED" ]]; then
  read -r -a EXTRA_ARR <<< "$EXTRA_ALLOWED"
else
  EXTRA_ARR=()
fi

ALLOWED_JSON=$(printf '%s\n' "${DIRS[@]}" "${EXTRA_ARR[@]}" "Root" | jq -R . | jq -s .)

jq -r --argjson allowed "$ALLOWED_JSON" '
  map(. as $row | {
    mapp: $row.mapp,
    gofil: $row."go-fil",
    funktionsnamn: $row.funktionsnamn,
    targets: (
      ($row.anropar // "")
      | split(",")
      | map(gsub("^\\s+|\\s+$"; ""))
      | map(select(test("\\.")))
      | map(split(".")[0])
      | map(select(. != $row.mapp))
      | unique
    )
  })
  | map(select(.mapp as $m | $allowed | index($m)))          # behåll bara egna mappar
  | map(.targets |= map(select(. as $t | $allowed | index($t))))  # behåll bara anrop mot egna mappar
  | map(select(.targets | length > 0))                        # släng tomma efter filtrering
  | unique_by({mapp, gofil, funktionsnamn})
  | group_by(.mapp)
  | .[]
  | "mapp: \(.[0].mapp)\n" +
    (
      group_by(.gofil)
      | map(
          "  " + .[0].gofil + ":\n" +
          (map("    " + .funktionsnamn + " -> " + (.targets | join(", "))) | join("\n"))
        )
      | join("\n")
    ) + "\n"
' "$FILE" > extern.txt
