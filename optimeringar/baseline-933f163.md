# Baseline 933f163

Den frysta referensmotorn för den första verifierade optimeringsomgången är
byggd från commit `933f163` på grenen `optimering1`. Motorkällan hade inga
spårade ändringar när binären byggdes.

- Binär: `artifacts/baseline/goalaric-933f163`
- SHA-256: `cca29be2401d861a8981191d008551cca423ac15209d6fabbe658092030d67f6`
- Byggkommando: `go build -trimpath -o artifacts/baseline/goalaric-933f163 ./GoAlaric.go`
- Benchmark: depth 8, 14 positioner, sju repetitioner
- Median: 746 130 NPS och 37 ms
- Rapport: `artifacts/bench/baseline-933f163-depth8.json`

Binären och de fullständiga rapporterna ligger under `artifacts/` och är
avsiktligt ignorerade av Git. Denna fil är det beständiga receptet för att
återskapa och identifiera baslinjen.
