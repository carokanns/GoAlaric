# Benchmark baselines

## Commands
- `go test ./search -run=^$ -bench BenchmarkSearchID -benchtime=1x -benchmem -cpuprofile search.cpu -memprofile search.mem`
- `go test ./eval -run=^$ -bench . -benchtime=1x -benchmem -cpuprofile eval.cpu -memprofile eval.mem`

## Results (benchtime=1x)
- `BenchmarkSearchIDDepth2`: 12.9ms/op, 112,080 B/op, 233 allocs/op.
- `BenchmarkSearchIDDepth3`: 14.2ms/op, 462,968 B/op, 1,091 allocs/op.
- `BenchmarkCompEvalStartPos`: 21.4µs/op, 0 B/op, 0 allocs/op.
- `BenchmarkCompEvalTactical`: 12.4µs/op, 0 B/op, 0 allocs/op.
- `BenchmarkHashEvalVariedPositions`: 42.3µs/op, 0 B/op, 0 allocs/op.

## Profiling highlights
- CPU (search): `transTable.Store` and `context.(*cancelCtx).propagateCancel` each accounted for 25% of sampled CPU time during the search benchmarks; `benchmarkSearchID` itself summed to 75% inclusive.
- Memory (search): `transTable.Alloc` dominated allocations (~96.9% of 132MB) during search benchmark setup.
- CPU (eval): CPU samples were dominated by runtime unlock/sweep activity in the short run (10ms sampled).
- Memory (eval): `BenchmarkHashEvalVariedPositions` (27.2%) and `benchmarkCompEval` (20.0%) drove most allocation volume in eval benchmarks, largely from profiling/gzip overhead rather than evaluation itself.

## Notes
- A full `go test ./...` run currently hangs (aborted manually) because of long-running search tests; benchmark runs above use `-run=^$` to avoid that interference.
