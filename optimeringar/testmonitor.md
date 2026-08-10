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

Fastchess dömer vinst när båda motorerna är överens om minst 500 centipawns
fördel för samma sida under tre på varandra följande bedömningar. Det motsvarar
ungefär värdet av ett torn. Remiavdömningen börjar efter drag 40 och kräver åtta
bedömningar inom ±10 centipawns; drag 200 avslutar återstående partier som remi.

Om den lokala tabellmappen `.tools/syzygy/3-4` finns, skickar testmonitor
automatiskt samma `SyzygyPath` till både baseline och kandidat i screening,
SPRT och depth pre-scan. Den faktiska sökvägen sparas i `monitor-config.json`.
För en enskild körning kan det stängas av med `--syzygy-path off`, eller peka
ut en annan tabellmapp med `--syzygy-path <sökväg>`.

## Fristående depth pre-scan

Pre-scanningen mäter sista kompletta UCI-`depth` före varje `bestmove`. Den
körs utan LLM och påverkar inte kandidatens matchresultat. Baseline kalibreras
först i självspel och profilen cachas med motor-, Fastchess-, öppnings-,
maskin- och testkonfiguration:

```bash
go run ./cmd/testmonitor prescan \
  --engine artifacts/baseline/goalaric-36e10b7 \
  --role baseline --minimum-depth 8 \
  --games 40 --tc 20+0.2 --concurrency 8
```

Körningen startar frikopplat precis som en screening. `status`, `follow` och
`stop` kan användas med dess `--run-dir`. `depth-profile.json` innehåller
sample count, medel-, median-, p25- och p90-djup, selektivt djup, noder och
NPS. Beslutet är `depth_adequate` eller `increase_time_control`.

Kandidatkampanjen har tre lägen:

- `--prescan full --minimum-depth N`: använd cachad baseline och mät kandidaten mot baseline.
- `--prescan baseline --minimum-depth N`: kontrollera bara cachad baselineprofil.
- `--prescan skip --prescan-skip-reason "..."`: hoppa över depth-gaten.

Vid otillräckligt median-djup provas som standard
`20+0.2,30+0.3,45+0.45,60+0.6`. Första tidskontrollen som klarar gaten används
av både screening och eventuell SPRT. Om ingen klarar kravet avslutas
kampanjen med `depth_insufficient` utan att starta screening.

## Automatisk slututvärdering

Efter att den deterministiska pipelinen har skapat kandidatens
`experiment.json` kan screeningen startas med automatisk slututvärdering:

```bash
go run ./cmd/testmonitor start \
  --baseline <baseline> --candidate <candidate> \
  --candidate-id <id> --auto-evaluate
```

Ingen modell körs medan matchen pågår. Efter en maskinellt godkänd screening
skrivs ett kompakt event under `artifacts/llm-inbox/`, och installerad
`codex exec` körs en gång med read-only-sandbox, ett fast JSON-schema och
eventet som enda beslutsunderlag. Modellen får endast välja `sprt` eller
`no_sprt`; den föreslår ingen kodändring eller promotion. PGN och råloggar
skickas inte till modellen.

Efter en godkänd screening kan ett validerat `sprt`-beslut automatiskt starta
en ny körning med samma tidskontroll som screeningen och högst 10 000 partier. `alpha=0.04` och `beta=0.20`
ger LLR-gränser kring −1,57/+3,00 så svaga kandidater kan avslutas tidigare
utan att sänka den positiva acceptansgränsen. Go-koden kontrollerar först
att samtliga hårda teststeg är godkända och att binärernas SHA-256 fortfarande
matchar experimentet. Om modellen inte startar SPRT sätts experimentet till
`awaiting_decision` för manuell utvärdering.

Efter SPRT körs ingen modell alls. Experimentet sätts till `awaiting_decision`
så att resultat, eventuell baseline-promotion och nästa förbättring utvärderas
i den synliga användarsessionen. Stoppade eller misslyckade matcher behandlas
på samma manuella sätt och får aldrig startas om automatiskt.

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
`monitor-config.json`, PGN, `pgn-audit.json`, Fastchess-logg och full
programutskrift. Fastchess skriver dessutom sin egen `config.json`. PGN-granskningen
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
