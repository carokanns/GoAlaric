# GoAlaric optimizer – version 1.2.0

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

Det fristående `coordinate-multires`-kommandot startar inga riktiga matcher.
Fastchess används av `optimize` först när kampanjen uttryckligen har
`mode: real`.

## Autonom kedja med matchrunner

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

`mode: real` använder nu samma orchestration mot den befintliga kedjan
`testmonitor → Fastchess → GoAlaric`. En minimal real-kampanj kan till exempel
ha följande mål- och runtimeblock:

```json
{
  "goals": {
    "max_games": 4,
    "max_evaluations": 3,
    "optimizer": {"parameters": ["mobility_weight"]},
    "adaptive": {"min_blocks": 1, "max_blocks": 1},
    "real": {
      "testmonitor_command": ["/path/to/testmonitor"],
      "fastchess": "/path/to/fastchess",
      "opening_book": "/path/to/openings.epd",
      "tc": "0.2+0.01",
      "hash_mb": 16,
      "threads": 1,
      "workdir": "/path/to/GoAlaric"
    }
  }
}
```

Baselineens första sökresultat är en matchlös referenspunkt; riktiga matcher
startar först när kandidaten skiljer sig från det aktuella ankaret. Varje
realblock valideras av testmonitor innan det skrivs atomiskt till SQLite.
Valda parametrar måste ha sökmetadata (`min`, `max`, `step` och `min_step`) i
Python-registret; Go-registrets namn och standardvärden är oförändrade.
Minimalkedjan är verifierad med återstart, separat baseline-/kandidatfil,
total matchbudget, dubbelräkningskontroll och processaudit. Den fulla
v1.1-verifieringen körs från den installerade terminal-entrypointen och finns
som reproducerbart test i `tests/test_phase15.py`. Den täcker en
flerparametrig sökning till terminal checkpoint, avbrott/återstart i sökning
och bekräftelse, read-only-dashboard, statusövervakning, slutrapport samt
kontroll av unika spel och inga kvarvarande processer.

## Bekräftelsefas efter sökningen

När koordinatsökningen når en terminal checkpoint startar en aktiverad
bekräftelsefas automatiskt. Den sparas i egna SQLite-tabeller
(`confirmations`, `confirmation_blocks` och `confirmation_games`) och påverkar
inte `optimizer_state`, ankaret eller nästa sökkandidat. Slutkandidaten jämförs
alltid med kampanjens ursprungliga baseline.

Bekräftelsen använder ett nytt seed och ett separat öppningskatalogträd. Alla
öppningspar för det fasta spelantalet deklareras i förväg, körs utan adaptiv
gallring och kan återupptas med samma kommando:

```json
{
  "goals": {
    "confirmation": {
      "enabled": true,
      "games": 800,
      "seed": 20260830,
      "confidence": 0.95
    }
  }
}
```

Utfallet är `confirmed` när intervallets nedre gräns är över 50 procent,
`rejected` när den övre gränsen är under 50 procent, annars `inconclusive`.
Endast `confirmed` får ge en kandidatstatus som rekommenderad. Vid övriga
utfall görs ingen rekommendation och ingen kod eller parameterfil promoveras
automatiskt.

För falska verifieringskampanjer kan `confirmation.fake_result` anges som
W-D-L, till exempel `{"wins": 10, "draws": 0, "losses": 10}`. Det verkliga
flödet använder samma `goals.real`-konfiguration som den autonoma
`testmonitor → Fastchess → GoAlaric`-kedjan. Under körning visar
`status --watch` och dashboardens read-only API bekräftelsens status och
intervall; kampanjens vanliga matchbudget räknar inte bekräftelsepartierna.

När bekräftelsen är färdig skrivs `recommended-parameters.json` i kampanjens
datakatalog och registreras som ett separat SQLite-artifact endast vid
`confirmed`. Vid `rejected` eller `inconclusive` lämnas rekommendationsfält och
rekommendationsfil tomma. Ingen automatisk promotion sker.

## Slutverifiering för v1.1

Körbara JSON-exempel finns i
[`examples/phase15-v1.1-campaign.json`](examples/phase15-v1.1-campaign.json) och
[`examples/phase15-v1.1-registry.json`](examples/phase15-v1.1-registry.json).
Den fullständiga terminalrunbooken finns i
[`docs/phase15-v1.1-runbook.md`](docs/phase15-v1.1-runbook.md).

## v1.1.1 / explorativ sökning

Den vanliga sökningen är fortsatt strikt och flyttar endast ankaret efter ett
statistiskt `accept`. För nattliga sökningar kan kampanjen uttryckligen slå på
ett separat explorativt läge:

```json
{
  "goals": {
    "optimizer": {
      "parameters": ["activity_bias"],
      "exploratory": {
        "enabled": true,
        "min_score": 51.0
      }
    }
  }
}
```

När kandidatens maximala adaptiva sökbudget är förbrukad används punktresultatet
endast i detta läge: `score > min_score` blir `accept_exploratory` och flyttar
ankaret, annars blir det `reject_exploratory`. Resultatet märks med
`exploratory: true` och är inte statistiskt bekräftat. Standardläget är strikt.

Den avslutande fasta bekräftelsen påverkas inte och behåller sitt strikta
95-procentsintervall. Ett explorativt sökbeslut kan därför aldrig ensamt bli en
rekommendation eller promotion.

## v1.1.2 / livebekräftelse och korrekt slutrapport

När sökningen har nått sin terminala checkpoint växlar dashboarden och
`status --watch` till `confirming`. Kandidatlistan behålls som sökhistorik,
men bekräftelsen blir dashboardens huvuddel och uppdateras från färdiga
`confirmation_blocks` i SQLite efter varje öppningspar. Den visar kandidatens
parameterhash och differenser mot baseline, öppningspar, partier, W-D-L, score,
Elo, 95-procentsintervall samt förfluten och beräknad återstående tid.

`confirming` betyder att koordinatsökningen är klar men den fasta
bekräftelsen fortfarande körs. Kampanjen blir `completed` först när
bekräftelsen har avslutats som `confirmed`, `rejected` eller `inconclusive`.
Dashboarden är fortsatt helt skrivskyddad och läser bara SQLite, även för den
arkiverade v1.1.1-kampanjen.

## v1.2 / profiler för matchtid

v1.2 kan ge sökning och bekräftelse var sin namngiven tidsprofil. Profilen
består av namn och faktisk `tc`; dess hash sparas i checkpoint, trial- och
confirmation-resultat. Saknas `real.profiles` används fortfarande `real.tc`
med profilnamnet `default`, så äldre kampanjer behåller samma tidskontroll.

```json
{
  "goals": {
    "real": {
      "testmonitor_command": ["/path/to/testmonitor"],
      "fastchess": "/path/to/fastchess",
      "opening_book": "/path/to/openings.epd",
      "tc": "0.2+0.01",
      "profiles": {
        "long-search": {"tc": "1+0.02"},
        "long-confirmation": {"tc": "2+0.02"}
      }
    },
    "optimizer": {
      "parameters": ["mobility_weight"],
      "profile": "long-search"
    },
    "confirmation": {
      "enabled": true,
      "games": 100,
      "seed": 20260930,
      "confidence": 0.95,
      "profile": "long-confirmation"
    }
  }
}
```

`optimizer optimize campaign.json` skickar den upplösta profilens `tc` till
testmonitor och därifrån vidare till Fastchess. Dashboard, `status` och rapport
visar profilnamn, profilhash och faktisk tidskontroll för sök- och
bekräftelsefasen. En återstart med en annan profil avvisas av SQLite. Detta
delmål innehåller ingen nodbudget och ändrar inte sökalgoritmen.

Det reproducerbara underlaget finns i
[`tests/test_phase18.py`](tests/test_phase18.py). Det täcker fake-runnerns
profil- och återstartsflöde samt två små riktiga körningar av samma kandidat,
först med `0.2+0.01` och sedan med `1+0.02`, där `monitor-config.json` och
blockresultatet verifieras.

Slutrapporten skiljer nu mellan `final_anchor` och `highest_local_trial`:
`final_anchor` kommer alltid från optimizer-checkpointen och är kandidaten som
bekräftas mot ursprunglig baseline. `highest_local_trial` är endast den högsta
lokala matchscoren i sökhistoriken och är aldrig en slutrekommendation.
Rapportens `parameter_differences` avser den faktiskt bekräftade kandidaten.

Rapporten redovisar `search_games`, `confirmation_games` och `total_games`,
liksom separata sluttider för sökning och bekräftelse. Standardrapporten är
kompakt och utelämnar tusentals block-id:n. Använd `--detail` endast när
blockdetaljer behövs:

```bash
optimizer report <campaign-id> --data-dir <campaigns> --format json
optimizer report <campaign-id> --data-dir <campaigns> --format json --detail
```

Den arkiverade v1.1.1-databasen täcks av
`tests/test_phase17.py`; testet öppnar den read-only och kontrollerar
slutankare, bekräftelseutfall, partier, rapportkompakthet och att databasen
inte ändras. Den praktiska release- och driftbeskrivningen finns i
[`docs/phase17-v1.1.2-runbook.md`](docs/phase17-v1.1.2-runbook.md).

## v1.2.0 / profiler, LMR och slutverifiering

v1.2.0 är den färdiga releasen av matchprofilerna och den första körbara
search-parametern. Sökning och fast bekräftelse kan använda separata namngivna
profiler, medan kampanjer utan `real.profiles` fortsätter att använda
`real.tc`. Profilnamn, faktisk tidskontroll och profilhash valideras vid
återstart och sparas i SQLite-resultaten.

Registret `search-lmr-v1` exponerar den körbara parametern
`lmr_divisor_x100`. Motorns standardvärde är 225 och sökintervallet i den
verifierade pilotkampanjen var 175–275 med steg 25. Det äldre
`eval-pilot-v1`-registret förblir byte- och hashkompatibelt.

Den kompletta LMR-piloten körde 392 partier. Sökningen slutade med ankaret
`lmr_divisor_x100=175`; den fasta bekräftelsen mot baseline 225 gav
60–81–59, 50,25 % och ett 95-procentsintervall på 44,9049–55,5951 %.
Utfallet var `inconclusive`, så ingen kandidat rekommenderas och ingen
automatisk promotion sker.

Slutverifieringen omfattade också återstartbar bekräftelse, unikhetskontroll
för block och partier, kompakt JSON/HTML-rapportering och en exakt
`--max-results 1`-regression för att säkerställa att en bounded invocation
inte startar confirmation utanför sin kvot. Underlaget finns i
[`docs/phase21-v1.2-lmr-long-pilot-completion.md`](docs/phase21-v1.2-lmr-long-pilot-completion.md).

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
för syntetiska verifieringskörvägar; `mode: real` använder den riktiga
testmonitor-/Fastchess-kedjan.

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
