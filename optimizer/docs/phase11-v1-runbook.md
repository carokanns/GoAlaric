# GoAlaric parameter optimizer v1 — fas 11 pilot

Kampanj-ID: phase11-full-eval-pilot.

Runbooken beskriver den fulla pilotkampanjen och är en del av v1-underlaget.
Pilotens stora SQLite-databas, matchloggar och dashboard-samplingar skapas under
artifacts/ och ska normalt inte checkas in.

## Förutsättningar

Pilotkörningen använder Python-standardbiblioteket, Go 1.22-motorn, Go-
testmonitor, lokal Fastchess, åtta parametrar i registret eval-pilot-v1, seed
20260813, hash 128 MB, en tråd och tidskontrollen 10+0.1. Baseline ska vara
fast under hela kampanjen. Ingen automatisk promotion är tillåten.

Exemplet nedan utgår från två lokala worktrees:

~~~bash
export REPO_ROOT=/path/to/GoAlaric-optimizer
export ENGINE_ROOT=/path/to/GoAlaric
export BUILD_DIR=/tmp/goalaric-v1-build
export DATA_DIR="$REPO_ROOT/artifacts/phase11/campaigns"
export CAMPAIGN_ID=phase11-full-eval-pilot
export PYTHONPATH="$REPO_ROOT/optimizer/src"
export FASTCHESS="$ENGINE_ROOT/.tools/fastchess/bin/fastchess"
export OPENING_BOOK="$ENGINE_ROOT/.tools/books/8moves_v3.pgn"
~~~

## Installation och build

~~~bash
cd "$REPO_ROOT"
mkdir -p "$BUILD_DIR"
go build -o "$BUILD_DIR/goalaric" .
go build -o "$BUILD_DIR/testmonitor" ./cmd/testmonitor
test -x "$BUILD_DIR/goalaric"
test -x "$BUILD_DIR/testmonitor"
test -x "$FASTCHESS"
test -s "$OPENING_BOOK"
~~~

## Start

Starta dashboarden i en separat terminal. Den är skrivskyddad och binder endast
till loopback:

~~~bash
cd "$REPO_ROOT"
PYTHONPATH="$REPO_ROOT/optimizer/src" python3 -m goalaric_optimizer dashboard "$CAMPAIGN_ID" --data-dir "$DATA_DIR" --listen 127.0.0.1:8787 --refresh-ms 500
~~~

Starta därefter pilotorkestreringen i en annan terminal:

~~~bash
cd "$REPO_ROOT"
PYTHONPATH="$REPO_ROOT/optimizer/src" python3 optimizer/tools/run_full_pilot.py --repo "$REPO_ROOT" --data-dir "$DATA_DIR" --campaign-id "$CAMPAIGN_ID" --registry "$REPO_ROOT/optimizer/registries/eval-pilot-v1-default.json" --testmonitor "$BUILD_DIR/testmonitor" --fastchess "$FASTCHESS" --engine "$BUILD_DIR/goalaric" --opening-book "$OPENING_BOOK" --tc 10+0.1 --hash-mb 128 --threads 1 --max-blocks 4 --weak-upper-score 40.0
~~~

Orkestreringen provar 15 lagliga ±1-grannar runt baseline och kör riktiga
matcher genom Python → testmonitor → Fastchess → GoAlaric. Varje kandidat får
adaptiv gallring efter kompletta tvåspelsblock, med högst fyra block.

## Status och dashboard

~~~bash
PYTHONPATH="$REPO_ROOT/optimizer/src" python3 -m goalaric_optimizer status "$CAMPAIGN_ID" --data-dir "$DATA_DIR"
curl http://127.0.0.1:8787/api/dashboard
~~~

Dashboarden får användas för observation under körningen men har inga
kontrollkommandon och ska inte ändra kampanjens SQLite-databas.

## Paus och återstart

För en planerad paus:

~~~bash
PYTHONPATH="$REPO_ROOT/optimizer/src" python3 -m goalaric_optimizer pause "$CAMPAIGN_ID" --data-dir "$DATA_DIR"
PYTHONPATH="$REPO_ROOT/optimizer/src" python3 -m goalaric_optimizer resume "$CAMPAIGN_ID" --data-dir "$DATA_DIR"
~~~

Återstarten fortsätter från SQLite-checkpointen. Ett avbrutet block räknas inte
som färdigt och samma blockidentitet används vid nästa försök.

## Säkert stopp och återstart

~~~bash
PYTHONPATH="$REPO_ROOT/optimizer/src" python3 -m goalaric_optimizer stop "$CAMPAIGN_ID" --data-dir "$DATA_DIR"
PYTHONPATH="$REPO_ROOT/optimizer/src" python3 -m goalaric_optimizer resume "$CAMPAIGN_ID" --data-dir "$DATA_DIR"
~~~

stop avslutar aktiva block och markerar kampanjen som avbruten. Efter resume ska
status och blockförsök kontrolleras innan körningen lämnas obevakad.

Pilotverifieringen omfattade pause/resume på kandidaterna 1 och 15 samt
stop/resume på kandidat 8. Det avbrutna blockets första försök räknades inte som
spel; försök 2 återanvände blockidentiteten.

## Slutrapport och stopp

~~~bash
PYTHONPATH="$REPO_ROOT/optimizer/src" python3 -m goalaric_optimizer report "$CAMPAIGN_ID" --data-dir "$DATA_DIR" --format json --output "$DATA_DIR/$CAMPAIGN_ID/final-report.json"
PYTHONPATH="$REPO_ROOT/optimizer/src" python3 -m goalaric_optimizer report "$CAMPAIGN_ID" --data-dir "$DATA_DIR" --format html --output "$DATA_DIR/$CAMPAIGN_ID/final-report.html"
~~~

Efter terminal status ska dashboardprocessen stoppas med Ctrl-C eller SIGTERM.
Kontrollera därefter att inga motor-, testmonitor- eller Fastchessprocesser finns
kvar. De genererade rapporterna och kampanjfilerna är verifieringsartefakter, inte
versionsfiler.

## Förväntad manuell granskning

Pilotens rekommendation finns i
phase11-v1-recommended-parameters.json. Den är endast en rekommendation:
mobility_weight: 19 stöds här av 3 vinster, 5 remier och 0 förluster över 8
partier. Det är för litet underlag för promotion. Verifieringsunderlaget finns i
phase11-v1-verification.json.

Ställ aldrig om baseline eller genomför promotion automatiskt på grundval av
detta pilotresultat. Ett separat, betydligt större bekräftelsetest mot baseline
krävs för mobility_weight: 19.
