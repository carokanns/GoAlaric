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

Kommandot startar screeningen i bakgrunden och visar delresultat löpande i
terminalen. Standardvärdena är 400 partier
från 200 öppningar, `20+0.2`, åtta samtidiga enkeltrådade partier och parade
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

Visa de senaste periodiska delresultaten:

```bash
go run ./cmd/testmonitor progress
go run ./cmd/testmonitor progress \
  --run-dir artifacts/matches/<körning> --tail 5
```

Följ en redan startad match direkt i terminalen:

```bash
go run ./cmd/testmonitor follow \
  --run-dir artifacts/matches/<körning>
```

`run_match.sh` och `run_sprt_match.sh` använder följningsläget automatiskt.
Tryck `Ctrl+C` för att lämna visningen; den frikopplade matchprocessen fortsätter
i bakgrunden. Använd `testmonitor stop` endast när själva matchen ska avbrytas.

Screening skriver automatiskt ett snapshot efter vart tionde färdigt parti.
SPRT skriver efter vart femtionde parti och inkluderar aktuell LLR samt nedre
och övre beslutsgräns. Båda skriver alltid ett sista snapshot vid avslut eller
avbrott. Dessutom skrivs aktuell ställning varje minut, även om färre än nästa
antal partier har hunnit bli färdiga. Tidsintervallet kan ändras med
`--progress-interval`, och partiintervallet med `--progress-games`, när
`testmonitor start` anropas.

Historiken sparas maskinläsbart i `progress.jsonl`; senaste snapshot finns i
`progress.json`. En läsbar `[progress]`-rad skrivs också till `match.out` och
`monitor.log`.

Screening kräver att samtliga öppningar har spelats med båda färgerna. SPRT
utvärderas däremot av Fastchess efter varje färdigt parti och får därför ha
exakt en ensam slutöppning när en beslutsgräns stoppar matchen. Fler ensamma
eller överanvända öppningsgrupper underkänner fortfarande körningen.

Avbryt den senaste pågående screening- eller SPRT-matchen:

```bash
go run ./cmd/testmonitor stop
```

För att välja en bestämd körning:

```bash
go run ./cmd/testmonitor stop \
  --run-dir artifacts/matches/<körning>
```

Monitorprocessen skickar först en mjuk stoppsignal till Fastchess och samtliga
motorprocesser och använder en hård stoppsignal efter tio sekunder om de inte
avslutas. Färdiga partier, PGN och loggar bevaras. `status.json` får status
`stopped` och beslutet `stopped_by_user`; avbrottet räknas inte som en
SPRT-acceptans eller som ett motorfel.

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
