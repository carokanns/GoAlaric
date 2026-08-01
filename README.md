# GoAlaric
Alaric in Golang  
Trying to restore an old chesspogram in Go

## Testing
- `go test ./...`
- `./scripts/run_all_tests.sh` (runs unit tests + perft + movetime)
- `./scripts/run_search_bench.sh <engine>` (repeated fixed-depth benchmark)
- `./scripts/setup_match_book.sh` (installs the pinned 34,700-opening CC0 book)
- `./scripts/run_match.sh <baseline> <candidate>` (monitored Fastchess match)
- `./scripts/run_sprt_match.sh <baseline> <candidate>` (decisive SPRT match)

Long match status is persisted under `artifacts/matches/` and can be read with
`go run ./cmd/testmonitor status`. See `optimeringar/testmonitor.md` for the
complete workflow.

## Logging
- UCI option `LogFile=true` writes a summary row per search to `search.log`.
