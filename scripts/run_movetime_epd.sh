#!/usr/bin/env bash
# Kör "go movetime <ms>" för varje FEN-rad i en EPD-fil.
# Utskrift: bestmove, nodes och score (cp eller mate) för varje rad.

set -euo pipefail

if [[ $# -lt 1 ]]; then
  movetime="5000"
  epd_file="scripts/movetime_epd"
  output_file="movetime_results.txt"
  threshold="60"
else
  if [[ $# -eq 1 ]]; then
    movetime="5000"
    epd_file="$1"
    output_file="movetime_results.txt"
    threshold="60"
  else
    movetime="$1"
    epd_file="$2"
    output_file="${3:-movetime_results.txt}"
    threshold="${4:-60}"
  fi
fi

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
bin_dir="$repo_root/bin"
engine_bin="$bin_dir/goalaric_engine"

mkdir -p "$bin_dir"

# Bygg motorbinären (inte perft-bygget) för UCI-sök.
GO111MODULE=on go build -o "$engine_bin" "$repo_root/GoAlaric.go"

printf "%-12s %12s %12s %12s %3s\n" "move" "nodes" "score" "expected" "Δ" >"$output_file"

while IFS= read -r line || [[ -n "$line" ]]; do
  # Hoppa över tomma rader och kommentarer.
  [[ -z "${line// }" ]] && continue
  [[ "${line:0:1}" == "#" ]] && continue

  # Plocka första fyra fälten (FEN).
  set -- $line
  if [[ $# -lt 4 ]]; then
    continue
  fi
  fen="$1 $2 $3 $4"

  # Förväntat c1-värde i EPD-raden (om angivet).
  expected=$(echo "$line" | awk '{for(i=1;i<=NF;i++) if($i=="c1"){print $(i+1); exit}}')

  # Start motorprocess för denna rad.
  coproc ENG { "$engine_bin"; }
  fd_in=${ENG[0]}
  fd_out=${ENG[1]}

  # Init UCI
  printf "uci\n" >&"$fd_out"
  while read -r l <&"$fd_in"; do
    [[ $l == uciok ]] && break
  done
  printf "setoption name LogFile value true\n" >&"$fd_out"
  printf "isready\n" >&"$fd_out"
  while read -r l <&"$fd_in"; do
    [[ $l == readyok ]] && break
  done

  # Kör sök
  printf "ucinewgame\n" >&"$fd_out"
  printf "position fen %s\n" "$fen" >&"$fd_out"
  printf "go movetime %s\n" "$movetime" >&"$fd_out"

  bestmove="none"
  info_line=""
  while read -r l <&"$fd_in"; do
    case "$l" in
      info*score*)
        info_line="$l"
        ;;
      bestmove*)
        bestmove=$(echo "$l" | awk '{print $2}')
        break
        ;;
    esac
  done

  # Stäng processen.
  printf "quit\n" >&"$fd_out"
  wait "$ENG_PID" 2>/dev/null || true
  eval "exec ${fd_in}>&-"
  eval "exec ${fd_out}>&-"

  nodes=$(echo "$info_line" | awk '{for(i=1;i<=NF;i++) if($i=="nodes"){print $(i+1); exit}}')
  score=$(echo "$info_line" | awk '{
    for(i=1;i<=NF;i++){
      if($i=="mate"){print "mate " $(i+1); exit}
      if($i=="cp"){print $(i+1); exit}
    }
  }')

  nodes=${nodes:-0}
  score=${score:-}

  # Beräkna avvikelse om både score och expected är cp-tal.
  mark=""
  if [[ -z "${expected:-}" ]]; then
    mark="?"
  elif [[ -n "$score" && "$score" != mate* ]]; then
    diff=$(awk -v a="$score" -v b="$expected" 'BEGIN {d=a-b; if(d<0)d=-d; print d}')
    # Om diff > threshold sätt mark till X.
    mark=$(awk -v d="$diff" -v t="$threshold" 'BEGIN { if(d>t) print "X" }')
  fi

  printf "%-12s %12s %12s %12s %3s\n" "$bestmove" "$nodes" "$score" "${expected:-}" "$mark" >>"$output_file"
done <"$epd_file"

echo "Resultat sparat i $output_file"
