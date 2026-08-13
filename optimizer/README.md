# GoAlaric optimizer – version 1 (fas 11)

## Version 1 / fas 11

Fas 11 är slutverifierad med en full pilotkampanj genom Python → testmonitor →
Fastchess → GoAlaric, med adaptiv gallring, flera pause/resume- och
stop/resume-cykler, read-only-dashboard och längre process-/databasstressaudit.

Det spårbara underlaget finns här:

- [fas 11-runbook](docs/phase11-v1-runbook.md)
- [verifieringsunderlag](docs/phase11-v1-verification.json)
- [rekommenderad parameterfil](docs/phase11-v1-recommended-parameters.json)
- [full pilotorkestrering](tools/run_full_pilot.py)

Version 1 gör ingen automatisk promotion. `mobility_weight: 19` är endast en
manuell rekommendation från 3–5–0 över åtta partier och kräver ett separat,
betydligt större bekräftelsetest mot baseline.

## Version 1.1 / deterministisk flerupplöst koordinatsökning

Den syntetiska v1.1-sökningen finns som `coordinate-multires`. Den börjar från
baseline och använder registrets `min`, `max`, `step` och `min_step`. För varje
valt parameter testas `+step` och `-step` i registerordning. En förbättring
startar om varvet med samma upplösning; ett helt resultatlöst varv halverar
stegen, ned till `min_step`. Parameterhashar och nästa sökposition sparas
atomiskt i SQLite. Samma parameteruppsättning återanvänds aldrig som ett nytt
trial.

Exempel mot en syntetisk målfunktion:

```bash
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer coordinate-multires <campaign-id> \
  --registry /path/to/multires-registry.json \
  --fake-optimum '{"a":6,"b":10}' \
  --parameters a,b
```

Denna fas startar inga riktiga matcher. Fastchess kopplas in först efter att
konvergens, halvering, dubblettfrihet och fullständig checkpoint-återstart är
verifierade.

## Nästa steg / autonom kedja med falsk matchrunner

Den första end-to-end-kopplingen finns som `optimize`. Kommandot initierar
eller återupptar kampanjen, låter `coordinate-multires` skapa nästa
parameteruppsättning, materialiserar den som en parameterfil och skickar den
genom samma `AdaptiveCampaign`-kontrollflöde som den verkliga matchkedjan.
Den falska runnern jämför deterministiskt kandidaten med sökningens aktuella
ankare och skriver block, W-D-L, spel och checkpoint till SQLite.

En kampanj kan till exempel innehålla:

```json
{
  "goals": {
    "max_games": 1000,
    "optimizer": {"parameters": ["a", "b"], "max_passes": 20},
    "adaptive": {"min_blocks": 1, "max_blocks": 2},
    "fake_match": {"optimum": {"a": 6, "b": 10}}
  }
}
```

Kör eller återuppta sedan samma kampanj med ett enda kommando:

```bash
source optimizer/.venv/bin/activate
optimizer optimize campaign.json --data-dir optimizer/campaigns
```

`--max-results 2` begränsar endast den aktuella körningen och lämnar en
återstartbar checkpoint. `max_games` är kampanjens totala fake-matchbudget;
när den tar slut avslutas sökningen idempotent med `stop_reason`.
Kandidatfilerna sparas under kampanjens `candidates/`-katalog och registreras
som SQLite-artifacts. Den nuvarande implementationen accepterar fortfarande
kampanjformatets JSON; YAML-adaptern är inte en del av detta delmål.

Detta delmål startar inga riktiga matcher. `mode: real` avvisas uttryckligen
tills hela flödet är verifierat med den falska runnern och Fastchess därefter
kan kopplas in utan att ändra sök- eller checkpointlogiken.

Fas 6 lägger en säker, sekventiell scheduler ovanpå den lokala
Python-/SQLite-kärnan. Den använder endast Python-standardbiblioteket och
skriver aldrig till Go-motorn, baseline eller dashboarden.

Installera paketet lokalt eller använd `PYTHONPATH` från repots rot:

```bash
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer init campaign.json
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer status <campaign-id>
```

En kampanjfil använder `schema_version: 1`, ett JSON-register, ett fast
`master_seed`, baselineidentitet och öppningspartitioner. `mode: fake` används
för verifieringskörvägarna; de ändrar bara kampanjens SQLite-status och startar
inga riktiga matcher.

Exempel på kontrollflöde:

```bash
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer init campaign.json
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer run <campaign-id>
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer pause <campaign-id>
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer resume <campaign-id>
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer status <campaign-id> --watch
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer stop <campaign-id>
```

Fas 6 kan köras med en falsk testmonitor som får blockidentitet och
`result.json`-sökväg via argument. Monitorn ska skriva resultatet först när
hela blocket är färdigt:

```bash
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer run <campaign-id> \
  --fake --monitor-command 'python3 fake_testmonitor.py' \
  --blocks 3 --pairs-per-block 1
```

Schedulern kör högst ett block åt gången och startar monitorn i en egen Unix-
processgrupp. `pause` och `stop` avslutar hela gruppen; det pågående blocket
blir `interrupted` och räknas inte. Nästa `resume` eller nya `run` spelar om
samma blockidentitet. En död monitor återställs på samma sätt. SQLite-WAL är
fortsatt enda sanningskällan.

Fas 7 lägger till deterministisk koordinatsökning. Den börjar med
baselineparametrarna, provar `+step` och `-step` inom varje registers min/max,
och väljer endast ett tydligt bättre resultat. Förlust och osäkerhet lämnar
ankaret oförändrat. Varje nytt resultat skriver W-D-L, score, osäkerhet,
parameterhash och RNG-checkpoint atomiskt till SQLite. Ett avbrott kan därför
fortsätta med samma nästa koordinat utan att prova en redan färdig
parameterhash igen.

Den syntetiska Fas 7-körvägen kräver ett register med exempelvis `min`, `max`
och `step` per parameter:

```bash
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer coordinate <campaign-id> \
  --registry registry.json \
  --fake-optimum '{"a":3,"b":1}' \
  --max-results 1
```

Upprepa kommandot för att simulera avbrott och återstart. Utan `--max-results`
kör den syntetiska sökningen till konvergens. Ingen riktig motor eller match
startas av koordinatsökningskommandot.

Fas 8 lägger till den första verkliga end-to-end-körvägen. `real-run` skapar
ett enda idempotent trial/block, materialiserar blocket med testmonitors
deterministiska öppningslogik och låter schedulern starta den riktiga Go-
`testmonitor` med Fastchess. Samma motorbinär kan användas med baseline- och
candidate-parameterfiler när `--optimizer-mode` används internt.

Testmonitors `block-report.json` och `status.json` valideras innan resultatet
skrivs atomiskt till SQLite. W-D-L, score, öppningsidentitet, färgväxling och
motor-/parameteridentiteter måste stämma. Ett avbrutet block blir
`interrupted`, får nytt försök med samma öppningshash och kan inte räknas
igen efter completion.

Körvägen är avsiktligt begränsad till ett block och en parameterändring ett
steg från baseline. Ingen längre optimeringskampanj startas här. Den äldre
`tune`-koden ligger fortsatt utanför systemet.

Fas 9 lägger till adaptiv gallring med en fast, deterministisk blockbudget.
Varje komplett öppningspar uppdaterar W-D-L, score, ett kontinuitetskorrigerat
Elo-estimat och ett 95-procentigt score-/Elo-intervall. En kandidat vars övre
scoreintervall ligger under `weak_upper_score` stoppas tidigt. Lovande och
osäkra kandidater fortsätter till `max_blocks`.

Beslut, statistik, färdiga block och nästa blockindex sparas i SQLite. Ett
avbrott mellan block återupptar samma kandidat och samma nästa block; oanvänd
matchbudget stängs som `rejected` när ett slutbeslut är fattat. Den adaptiva
slutrapporten kan användas direkt som evaluatorresultat i Fas 7:s
`AdaptiveCoordinateEvaluator`, så koordinatsökningen kan skapa nästa kandidat
utan att förlora beslutunderlaget.

För en liten verklig kampanj används `adaptive-real` med samma motor-,
parameter- och öppningsargument som `real-run`, samt exempelvis:

```bash
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer adaptive-real <campaign-id> \
  --data-dir optimizer/campaigns \
  --registry optimizer/registries/eval-pilot-v1-default.json \
  --testmonitor-command ./artifacts/tools/testmonitor \
  --fastchess .tools/fastchess/bin/fastchess \
  --baseline ./artifacts/baseline/goalaric-<baseline> \
  --candidate ./artifacts/baseline/goalaric-<baseline> \
  --candidate-parameter-file /path/to/candidate-parameters.json \
  --opening-book /path/to/openings.epd \
  --min-blocks 1 --max-blocks 2
```

Fas 9 kör ingen längre kampanj och den verkliga kontrollen är avsiktligt
begränsad till en mycket liten kandidatbudget.

Fas 10 lägger till en lokal, skrivskyddad dashboard. Den öppnar kampanjens
SQLite-fil med `mode=ro` och `query_only=ON`, binder endast till
`127.0.0.1` och pollas automatiskt med HTTP GET. Dashboarden visar kampanjens
status, aktuella trial och block, W-D-L, score, Elo och 95-procentiga intervall,
kandidatkö, förbrukade partier, checkpoint/fel samt bästa parameteruppsättning
med skillnader mot baseline. Den har inga kontrollknappar och påverkar inte
`pause`, `resume` eller `stop`.

Starta dashboarden medan schedulern kör:

```bash
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer dashboard <campaign-id> \
  --data-dir optimizer/campaigns --listen 127.0.0.1:8787
```

Stoppa dashboardprocessen med `Ctrl-C` eller `SIGTERM`; schedulern fortsätter
oberoende. En avslutad kampanj kan exporteras utan databasändring:

```bash
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer dashboard-report <campaign-id> \
  --data-dir optimizer/campaigns --format html --output campaign-report.html
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer report <campaign-id> \
  --data-dir optimizer/campaigns --format json --output campaign-report.json
```

Databasen använder WAL, foreign keys, centrala statusövergångar, unika
parameter- och blockidentiteter samt append-only events. Checkpoint och färdigt
matchblock skrivs i samma SQLite-transaktion. Statuskommandon öppnar databasen
read-only och skriver inga events.
