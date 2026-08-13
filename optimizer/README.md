# GoAlaric optimizer – fas 9

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

Fas 9 kör ingen längre kampanj och skapar ingen dashboard; den verkliga
kontrollen är avsiktligt begränsad till en mycket liten kandidatbudget.

Databasen använder WAL, foreign keys, centrala statusövergångar, unika
parameter- och blockidentiteter samt append-only events. Checkpoint och färdigt
matchblock skrivs i samma SQLite-transaktion. Statuskommandon öppnar databasen
read-only och skriver inga events.
