# GoAlaric optimizer v1.2.1 – releaseförberedelse

Senast verifierad: 2026-08-18.

## Releaseomfattning

v1.2.1 innehåller två korrigeringar ovanpå v1.2.0:

1. Matchprofiler kan använda exakt ett av `tc` eller `nodes`. Nodeprofiler
   betyder fast nodbudget per drag och transporteras som `--nodes N` till
   testmonitor och `-each nodes=N` till Fastchess. Profilens namn, läge,
   gräns och hash sparas i SQLite, visas i status/dashboard/rapport och
   valideras vid återstart. Äldre tidsprofilrader och kampanjer behåller sitt
   kompatibla `real.tc`-beteende.
2. När koordinatsökningens budget tar slut efter baseline och den första
   riktningen appliceras ett redan accepterat resultat på ankaret. Ingen
   motsatt evaluering startas. Avslag och strikt osäkra resultat lämnar
   ankaret oförändrat. Reconciliation och upprepade återstarter är
   deterministiska och idempotenta.

## Verifierade piloter

LMR-piloten med tidsprofiler finns i
[`phase21-v1.2-lmr-long-pilot-completion.md`](phase21-v1.2-lmr-long-pilot-completion.md):
392 partier, slutankare `lmr_divisor_x100=175` mot baseline 225 och
bekräftelsen 60–81–59, 50,25 %, `inconclusive`. Ingen kandidat rekommenderades.

Den lilla riktiga nodeprofilpiloten finns som read-only-underlag i
`artifacts/v1.2/node-budget-pilot/node-budget-confirmation-final-report.json`.
Den använde `node-search=100000 nodes/move` i sökningen och
`node-confirmation=250000 nodes/move` i bekräftelsen. Slutankaret blev
`lmr_divisor_x100=200` mot baseline 225. Bekräftelsen blev 1–2–1, 50 %,
`inconclusive`; ingen rekommendation och ingen automatisk promotion skapades.
Pilotens sök- och bekräftelsefas omfattade fyra partier vardera.

## Säkra verifieringskommandon

Kör från repots rot. Dessa kommandon startar inga optimizer-kampanjer,
Fastchess- eller testmonitorprocesser:

```bash
optimizer/.venv/bin/python -m unittest discover -s optimizer/tests -p 'test*.py'
go test ./...
go vet ./...
optimizer/.venv/bin/python -m compileall -q optimizer/src optimizer/tests
git diff --check
```

Nodeprofilernas fake-/SQLite-/transport- och PGN-tester finns i
[`../tests/test_phase22.py`](../tests/test_phase22.py). Budgetgränsens
SQLite-/fake-runner-regression finns i
[`../tests/test_phase20.py`](../tests/test_phase20.py).

## Releasepolicy

Bekräftelseresultat matas inte tillbaka till koordinatsökningen. Endast
`confirmed` kan ge en manuell rekommendation. `rejected` och `inconclusive`
ger ingen rekommendation, och v1.2.1 gör ingen automatisk promotion.

Denna arbetsomgång ändrar inte kampanjdatabaser eller artefaktrapporter och
skapar ingen commit, tagg eller push.
