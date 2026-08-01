# Testmonitor

`cmd/testmonitor` kör fasta sökbenchmark och övervakar Fastchess-matcher utan
att en agent eller terminal behöver vara ansluten under hela körningen.

## Fastdjupsbenchmark

```bash
./scripts/run_search_bench.sh bin/goalaric 7 8
```

Rapporten skrivs som JSON under `artifacts/bench/`. Varje position körs i en
ny motorprocess med `Hash=128`, `Threads=1` och `Ponder=false`.

## A/B-match

```bash
./scripts/setup_match_book.sh
./scripts/run_match.sh \
  artifacts/baseline/goalaric-933f163 \
  artifacts/candidate/goalaric-ischeck-cache
```

Kommandot startar screeningen i bakgrunden. Standardvärdena är 400 partier
från 200 öppningar, `30+0.3`, åtta samtidiga enkeltrådade partier och parade
färger. Den
CC0-licensierade standardboken innehåller 34 700 åttadragsöppningar från
`official-stockfish/books`. En ny slumpseed skapas för varje match och sparas
i `config.json`; Fastchess väljer öppningarna slumpmässigt och `-repeat` spelar
varje vald öppning med omvända färger. Fastchess blandar först hela boken och
kapar sedan den blandade listan till antalet matchrundor, så en öppning
återanvänds inte inom körningen.

Monitoreringen vägrar starta om boken har färre än 100 öppningar eller om
antalet begärda öppningspar är större än boken.

Läs status utan att störa matchen:

```bash
go run ./cmd/testmonitor status
go run ./cmd/testmonitor wait
```

Kontrollera hela konfigurationen utan att starta en match:

```bash
go run ./cmd/testmonitor validate \
  --baseline artifacts/baseline/goalaric-933f163 \
  --candidate artifacts/candidate/goalaric-ischeck-cache
```

Varje körning får en egen katalog under `artifacts/matches/` med `status.json`,
PGN, `pgn-audit.json`, Fastchess-logg och full programutskrift. PGN-granskningen
räknar faktiska bokdrag och unika öppningssekvenser, kontrollerar att varje
öppning används exakt två gånger och markerar identiska partier och identiska
färgväxlade par. Resultatet bäddas även in i
`status.json`. Kandidaten klarar den första screeningen vid minst 47 procent.
Om screeningen godkänns startas den beslutande, maximalt 10 000 partier långa,
SPRT-körningen separat:

```bash
./scripts/run_sprt_match.sh \
  artifacts/baseline/goalaric-933f163 \
  artifacts/candidate/goalaric-ischeck-cache
```

En valfri PGN kan också granskas separat:

```bash
go run ./cmd/testmonitor audit-pgn --pgn artifacts/matches/<körning>/games.pgn
```

Start av den beslutande matchen görs endast efter uttryckligt klartecken.
