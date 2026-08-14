# GoAlaric optimizer v1.1.2 – livebekräftelse och slutrapport

v1.1.2 förtydligar övergången mellan den explorativa/strikta sökningen och den
fasta statistiska bekräftelsen. Den ändrar inte parameterpolicy eller gör någon
automatisk promotion.

## Installation

Från repots rot används projektets lokala Pythonmiljö:

```bash
source optimizer/.venv/bin/activate
optimizer --help
python -c 'import goalaric_optimizer; print(goalaric_optimizer.__version__)'
```

Aktivering är inte nödvändig för vanliga körningar:

```bash
optimizer/.venv/bin/optimizer --help
optimizer/.venv/bin/python -c 'import goalaric_optimizer; print(goalaric_optimizer.__version__)'
```

Versionen ska vara `1.1.2`.

## Start och livevisning

Starta eller återuppta en kampanj från en vanlig terminal:

```bash
optimizer optimize campaign.json --data-dir artifacts/campaigns
```

Följ samtidigt kampanjen i två andra terminaler:

```bash
optimizer status <campaign-id> --data-dir artifacts/campaigns --watch --interval 1
optimizer dashboard <campaign-id> --data-dir artifacts/campaigns \
  --listen 127.0.0.1:8787 --refresh-ms 500
```

Dashboarden på `http://127.0.0.1:8787/` är read-only. När de vanliga
sökkandidaterna är färdiga växlar den automatiskt från sökhistorik till en
huvudsektion för bekräftelsen. Där visas slutkandidatens hash, fullständiga
parameterdifferenser mot baseline, öppningspar, partier, W-D-L, score, Elo,
95-procentsintervall, starttid, förfluten tid och beräknad återstående tid.
Värdena kommer direkt från färdiga `confirmation_blocks` i SQLite efter varje
öppningspar; varken dashboarden eller `status` läser matchfiler.

## `confirming` och `completed`

Sökningens terminala checkpoint är inte kampanjens slut när bekräftelse är
aktiverad. I det läget är den publika kampanjstatusen `confirming`. Den fasta
bekräftelsen kan stoppas och återupptas med samma `optimize`-kommando utan att
redan färdiga öppningspar räknas igen:

```bash
optimizer optimize campaign.json --data-dir artifacts/campaigns --max-results 1
optimizer optimize campaign.json --data-dir artifacts/campaigns
```

För ett säkert manuellt stopp används fortfarande `stop`, följt av `resume`:

```bash
optimizer stop <campaign-id> --data-dir artifacts/campaigns
optimizer resume <campaign-id> --data-dir artifacts/campaigns
optimizer optimize campaign.json --data-dir artifacts/campaigns
```

Först när varje deklarerat bekräftelseblock är färdigt övergår kampanjen till
`completed`. Bekräftelsens utfall är då `confirmed`, `rejected` eller
`inconclusive`. Endast `confirmed` kan skapa en parameterfil för manuell
granskning; ingen promotion sker automatiskt.

## Slutrapport

Skriv den vanliga kompakta rapporten när kampanjen är `completed`:

```bash
optimizer report <campaign-id> --data-dir artifacts/campaigns \
  --format json --output artifacts/campaigns/<campaign-id>/final-report.json
optimizer report <campaign-id> --data-dir artifacts/campaigns \
  --format html --output artifacts/campaigns/<campaign-id>/final-report.html
```

Standardrapporten innehåller summeringar men inga blocklistor eller tusentals
block-id:n. För felsökning eller revision kan samma rapport begäras med
fullständig blockdetalj:

```bash
optimizer report <campaign-id> --data-dir artifacts/campaigns --format json --detail
```

Tolka rapporten så här:

- `final_anchor` kommer från optimizer-checkpointen. Detta är slutkandidaten
  som skickades till bekräftelsen.
- `highest_local_trial` är den bästa enskilda lokala matchscoren under
  sökningen. Den kan skilja sig från `final_anchor` och är ingen rekommendation.
- Bekräftelseobjektet innehåller kandidatens hash, fullständiga parametrar och
  diff mot ursprunglig baseline. Rapportens `parameter_differences` avser samma
  faktiskt bekräftade kandidat.
- `search_games`, `confirmation_games` och `total_games` skiljer sökbudget,
  fast bekräftelse och total förbrukning. `times` skiljer sökningens sluttid
  från bekräftelsens sluttid.

Vid `rejected` eller `inconclusive` ska bekräftelseutfallet synas i rapporten,
men `recommendation` och `recommended-parameters.json` ska saknas.

## Regressions- och releaseverifiering

Den arkiverade v1.1.1-kampanjen öppnas read-only av ett särskilt regressionstest:

```bash
PYTHONPATH=optimizer/src optimizer/.venv/bin/python \
  -m unittest optimizer.tests.test_phase17
```

Det kontrollerar slutankaret, `highest_local_trial`, bekräftelsens W-D-L,
speluppdelningen och att den kompakta rapporten saknar blockdetaljer utan att
den gamla databasen migreras eller ändras.

Kör därefter den fulla lokala verifieringen:

```bash
PYTHONPATH=optimizer/src optimizer/.venv/bin/python \
  -m unittest discover -s optimizer/tests -p 'test*.py'
go test ./...
go vet ./...
bash scripts/run_perft.sh 4 scripts/perft_tests.txt /tmp/goalaric-v112-perft.txt
bash scripts/run_movetime_epd.sh 2000 scripts/movetime_epd /tmp/goalaric-v112-movetime.txt
git diff --check
ps -eo pid=,ppid=,stat=,command= | rg '[t]estmonitor|[f]astchess|[g]oalaric|[o]ptimizer dashboard' || true
```

Processauditen ska vara tom efter verifieringen. Kampanjdatabaser, matchfiler,
loggar och rapporter i `artifacts/` är underlag och ska normalt inte ingå i
releasecommitten.
