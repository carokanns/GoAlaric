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

Screening kräver att samtliga öppningar har spelats med båda färgerna. SPRT kör
parallella partier och kan därför avslutas med ett eller flera ofullständiga
öppningspar. PGN-auditen rapporterar dem, men de underkänner inte Fastchess
SPRT-beslut. Ett manuellt stopp avbryter matchen omedelbart.

## Automatisk slututvärdering

Efter att den deterministiska pipelinen har skapat kandidatens
`experiment.json` kan screeningen startas med automatisk slututvärdering:

```bash
go run ./cmd/testmonitor start \
  --baseline <baseline> --candidate <candidate> \
  --candidate-id <id> --auto-evaluate
```

Ingen modell körs medan matchen pågår. När matchstatusen är terminal skrivs ett
kompakt event under `artifacts/llm-inbox/`, och installerad `codex exec` körs
en gång med read-only-sandbox, ett fast JSON-schema och eventet som enda
beslutsunderlag. PGN och råloggar skickas inte till modellen.

Efter en godkänd screening kan ett validerat `sprt`-beslut automatiskt starta
en ny `30+0.3`-körning med högst 10 000 partier. Go-koden kontrollerar först
att samtliga hårda teststeg är godkända och att binärernas SHA-256 fortfarande
matchar experimentet. Övriga screeningbeslut skapar
`approval-package.json` och inväntar godkännande.

Efter SPRT skapas alltid ett godkännandepaket med baseline-rekommendation och
nästa förbättring. Promotion och kodändringar sker aldrig automatiskt. Stoppade
eller misslyckade matcher får inte promoveras eller startas om automatiskt.

Varje event har en maskinell kvittens som förhindrar dubbla modell-anrop och
dubbla SPRT-starter. Ett misslyckat Codex-anrop försöks inte igen automatiskt:

```bash
go run ./cmd/testmonitor retry-evaluation \
  --run-dir artifacts/matches/<körning>
```

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
Utan `--auto-evaluate` startas den beslutande, maximalt 10 000 partier långa,
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

Manuell körning kräver klartecken. Med `--auto-evaluate` får modellen välja
SPRT, men den deterministiska monitorn startar matchen och tillämpar alla
hårda skyddsregler.
