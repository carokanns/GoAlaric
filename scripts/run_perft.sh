#!/usr/bin/env bash
# Kör GoAlaric-perft för varje rad i en EPD-fil och loggar noder, tid och NPS.
# Användning: ./scripts/run_perft.sh <djup> <epd-fil> [utfil]

set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 [depth] <epd-file> [output-file]" >&2
  echo "Default depth is 5 when omitted." >&2
  exit 1
fi

if [[ $# -eq 1 ]]; then
  depth="5"
  epd_file="$1"
  output_file="perft_results.txt"
else
  depth="$1"
  epd_file="$2"
  output_file="${3:-perft_results.txt}"
fi

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
bin_dir="$repo_root/bin"
engine_bin="$bin_dir/goalaric_perft"

mkdir -p "$bin_dir"

# Bygg binären en gång så vi slipper starta via go run för varje rad.
GO111MODULE=on go build -o "$engine_bin" "$repo_root"

if [[ -s "$output_file" ]]; then
  printf "\n" >>"$output_file"
fi
printf "==== %s ====  Depth: %s\n" "$(date '+%Y-%m-%d %H:%M:%S')" "$depth" >>"$output_file"
printf "%-4s %20s %10s %14s %20s %5s\n" "row" "nodes" "ms" "nps" "expected" "stat" >>"$output_file"

fens=()
expecteds=()
current_index=-1

while IFS= read -r line || [[ -n "$line" ]]; do
  line=${line%$'\r'}
  # Hoppa över tomma rader och kommentarer.
  [[ -z "${line// }" ]] && continue
  [[ "${line:0:1}" == "#" ]] && continue

  # Plocka första fyra fälten (FEN).
  set -- $line
  if [[ $# -ge 4 && "$1" == */* && ( "$2" == "w" || "$2" == "b" ) ]]; then
    current_index=$((current_index + 1))
    fens+=("$1 $2 $3 $4")
    expecteds+=("")
    continue
  fi

  if [[ $# -ge 2 && "$current_index" -ge 0 && "$1" =~ ^[0-9]+$ ]]; then
    if [[ "$1" == "$depth" ]]; then
      count="${2//,/}"
      count="${count//$'\r'/}"
      expecteds[$current_index]="$count"
    fi
  fi
done <"$epd_file"

nodes_sum=0
ms_sum=0
row_count=0

for i in "${!fens[@]}"; do
  fen="${fens[$i]}"
  expected="${expecteds[$i]}"

  # Starta motor för denna rad.
  coproc ENGINE { "$engine_bin"; }
  fd_in=${ENGINE[0]}
  fd_out=${ENGINE[1]}

  # UCI init
  printf "uci\n" >&"$fd_out"
  while read -r init_line <&"$fd_in"; do
    [[ $init_line == uciok ]] && break
  done
  printf "setoption name LogFile value true\n" >&"$fd_out"
  printf "isready\n" >&"$fd_out"
  while read -r init_line <&"$fd_in"; do
    [[ $init_line == readyok ]] && break
  done

  # Mätt körning
  printf "ucinewgame\n" >&"$fd_out"
  printf "position fen %s\n" "$fen" >&"$fd_out"
  printf "perft %s\n" "$depth" >&"$fd_out"

  start_ns=$(date +%s%N)
  nodes=0
  while read -r out_line <&"$fd_in"; do
    if [[ $out_line == Total:* ]]; then
      nodes=${out_line#Total:	}
    fi
    if [[ $out_line == Time:* ]]; then
      break
    fi
  done
  end_ns=$(date +%s%N)

  duration_ns=$((end_ns - start_ns))
  duration_ms=$((duration_ns / 1000000))
  nodes="${nodes//,/}"
  nodes="${nodes//$'\r'/}"
  nps=$(awk -v n="$nodes" -v ns="$duration_ns" 'BEGIN { if (ns == 0) { print 0; exit }; printf "%.0f", n / (ns / 1e9) }')

  status="-"
  expected_display="-"
  if [[ -n "$expected" ]]; then
    expected_display="$expected"
    if [[ "$nodes" == "$expected" ]]; then
      status="ok"
    else
      status="fel"
    fi
  fi

  row_count=$((row_count + 1))
  printf "%-4s %20s %10d %14s %20s %5s\n" "$row_count" "$nodes" "$duration_ms" "$nps" "$expected_display" "$status" >>"$output_file"

  printf "quit\n" >&"$fd_out"
  wait "$ENGINE_PID" 2>/dev/null || true
  eval "exec ${fd_in}>&-"
  eval "exec ${fd_out}>&-"

  nodes_sum=$((nodes_sum + nodes))
  ms_sum=$((ms_sum + duration_ms))
done

# Lägg till en avslutande rad med genomsnittliga värden och total NPS.
if [ "$row_count" -gt 0 ]; then
  avg_nodes=$((nodes_sum / row_count))
  avg_ms=$((ms_sum / row_count))
  total_nps=$(awk -v n="$nodes_sum" -v ms="$ms_sum" 'BEGIN { if (ms == 0) { print 0; exit }; printf "%.0f", n / (ms / 1000) }')
  printf "%-4s %20s %10s %14s %20s %5s\n" "avg" "$avg_nodes" "$avg_ms" "$total_nps" "-" "-" >>"$output_file"
fi

echo "Resultat sparat i $output_file"
