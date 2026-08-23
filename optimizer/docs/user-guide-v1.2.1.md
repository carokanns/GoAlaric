k# GoAlaric optimizer v1.2.1 – svensk användarhandbok

Den här handboken beskriver den praktiska vägen från en kampanjfil till en
återstartbar riktig optimeringskampanj. Exemplen använder
`/home/peter/Projekt/GoAlaric-optimizer` som optimizer-repo och
`/home/peter/Projekt/GoAlaric` som motor-repo. Byt sökvägarna om dina binärer
ligger någon annanstans.

Handboken gäller JSON-kampanjer och v1.2.1. Den startar inte någon kampanj
själv och ändrar inte SQLite-databaser eller matchartefakter.

## Fusklapp

Kör från `/home/peter/Projekt/GoAlaric-optimizer` efter att den virtuella
miljön har aktiverats:

```bash
source optimizer/.venv/bin/activate
optimizer init /sökväg/campaign.json --data-dir /sökväg/campaigns
optimizer optimize /sökväg/campaign.json --data-dir /sökväg/campaigns
optimizer status <campaign-id> --data-dir /sökväg/campaigns
optimizer status <campaign-id> --data-dir /sökväg/campaigns --watch --interval 2
optimizer trials <campaign-id> --last 20 --data-dir /sökväg/campaigns
optimizer dashboard <campaign-id> --data-dir /sökväg/campaigns --listen 127.0.0.1:8787
optimizer report <campaign-id> --data-dir /sökväg/campaigns --format json --output report.json
```

För en kontrollerad återstart används samma `campaign.json` och samma
`--data-dir`. `optimizer optimize` initierar kampanjen om det behövs och
återupptar annars den sparade SQLite-checkpointen.

## 1. Översikt

Optimeraren provar parameteruppsättningar genom kedjan:

```text
optimizer optimize
        │
        ├─ flerupplöst koordinatsökning skapar nästa kandidat
        ├─ adaptiv gallring jämför kandidaten med aktuellt ankare
        └─ fast bekräftelse jämför slutkandidaten med ursprunglig baseline
```

Sökfasen är en deterministisk koordinatsökning. Den börjar vid baseline,
provar `+step` och därefter `-step` för varje vald parameter i registerordning.
Ett förbättrande varv börjar om med samma upplösning. Ett helt resultatlöst
varv halverar stegen, ned till respektive `min_step`. Alla övergångar och
resultat skrivs till SQLite atomiskt.

Adaptiv gallring är matchbudgeten för en kandidat. Efter varje färdigt
öppningspar uppdateras W–D–L, score, Elo och intervall. Svaga kandidater kan
stoppas tidigt; övriga får fortsätta tills `max_blocks` eller ett statistiskt
beslut nås.

Den fasta bekräftelsen startar först när sökningen är terminal. Den använder
ett separat seed, ett fast antal kompletta öppningspar och ingen adaptiv
gallring. Den jämför alltid slutkandidaten med den ursprungliga baseline, inte
med det senaste ankaret under sökningen. Bekräftelsen matas inte tillbaka till
koordinatsökningen.

Begreppen betyder:

- **Baseline** är den ursprungliga parameteruppsättningen. Den sparas när
  kampanjen initieras och ändras inte av sökningen.
- **Ankare** är sökningens aktuella bästa uppsättning. Det börjar som baseline
  och flyttas endast av ett accepterat resultat.
- **Kandidat** är den parameteruppsättning som testas i ett trial.
- **Slutkandidat** är ankaret från optimizer-checkpointen när sökningen
  avslutas. Det är denna som bekräftas.
- **Rekommendation** skapas endast när den fasta bekräftelsen blir
  `confirmed`. Den är ett manuellt beslutsunderlag; v1.2.1 gör ingen automatisk
  promotion.

I strikt läge flyttar endast ett statistiskt `accept` ankaret. Ett
`uncertain`-resultat är inte ett godkännande. Med
`goals.optimizer.exploratory.enabled: true` kan en maximal men osäker kandidat
få `accept_exploratory` när punktresultatet överstiger `min_score`. Det får
flytta ankaret i sökfasen, men är uttryckligen explorativt och räcker aldrig
för en rekommendation. Den avslutande bekräftelsen behåller sitt strikta
95-procentskrav.

## 2. Förutsättningar

### Pythonmiljö och installation

Paketet kräver Python 3.11 eller senare och har inga externa runtime-
dependencies. Från optimizer-repots rot:

```bash
cd /home/peter/Projekt/GoAlaric-optimizer
python3 -m venv optimizer/.venv
source optimizer/.venv/bin/activate
python -m pip install -e optimizer
optimizer --help
```

Vid utveckling eller felsökning kan samma källkod köras utan installation av
entrypointen:

```bash
PYTHONPATH=optimizer/src optimizer/.venv/bin/python -m goalaric_optimizer --help
```

Aktivera `.venv` när du utvecklar eller testar Pythonkoden. Den aktiverade
miljön gör att `python`, `pip` och `optimizer` kommer från projektet.

### Binärer och öppningsfil

En riktig kampanj behöver:

- en körbar GoAlaric-binär,
- en körbar `testmonitor`-binär,
- Fastchess,
- en EPD- eller öppningsfil som testmonitor kan läsa,
- ett optimizerregister med parametergränser,
- en baseline-parameterfil eller registerdefaultvärden.

Kontrollera alla sökvägar innan `init`:

```bash
ENGINE=/home/peter/Projekt/GoAlaric-optimizer/artifacts/v1.2/node-budget-pilot/bin/goalaric
TESTMONITOR=/home/peter/Projekt/GoAlaric-optimizer/artifacts/v1.2/node-budget-pilot/bin/testmonitor
FASTCHESS=/home/peter/Projekt/GoAlaric/.tools/fastchess/bin/fastchess
OPENINGS=/home/peter/Projekt/GoAlaric/fullGP.epd
REGISTRY=/home/peter/Projekt/GoAlaric-optimizer/artifacts/v1.2/lmr-long-pilot/registry.json
DEFAULTS=/home/peter/Projekt/GoAlaric-optimizer/optimizer/registries/search-lmr-v1-default.json

test -x "$ENGINE"
test -x "$TESTMONITOR"
test -x "$FASTCHESS"
test -f "$OPENINGS"
test -f "$REGISTRY"
test -f "$DEFAULTS"
sha256sum "$ENGINE" "$TESTMONITOR" "$FASTCHESS" "$OPENINGS" "$REGISTRY" "$DEFAULTS"
```

I en riktig kampanjfil är `baseline.engine_id` sökvägen till GoAlaric-
binären. `goals.real.testmonitor_command` är en lista eller en shell-liknande
sträng; en lista är tydligast. `fastchess`, `opening_book` och `workdir` ligger
under `goals.real`.

Relativa runtime-sökvägar till motor, Fastchess, öppningsfil och `workdir`
tolkas relativt kampanjfilens katalog. Använd absoluta sökvägar i långvariga
kampanjer för att undvika att en senare körning startas från fel katalog.
Register- och baseline-sökvägar är också säkrast absoluta.

### Register och parameterfil

Ett register innehåller parameterordning och defaultvärden. För
koordinatsökning behöver den valda parametern dessutom `min`, `max`, `step`
och `min_step`. Alla parametrar som motorn kräver måste fortfarande finnas i
parameterfilen; `goals.optimizer.parameters` väljer bara vilka som ska sökas.

Viktiga register i repot:

- `optimizer/registries/eval-pilot-v1-default.json` innehåller de åtta
  eval-defaultvärdena.
- `optimizer/registries/search-lmr-v1-default.json` innehåller LMR-defaulten
  `lmr_divisor_x100: 225`.
- `optimizer/registries/search-lmr-v1.json` innehåller LMR-parametern med
  sökintervall 175–275, steg 25 och `min_step: 25`.

`eval-pilot-v1-default.json` är ett defaultregister utan sökmetadata. Det är
alltså inte tillräckligt som koordinatregister för de åtta evalparametrarna.
Skapa ett separat kampanjregister genom att behålla alla åtta parametrar och
lägga till gränser för de parametrar som ska sökas. Ändra inte registret som
redan används av en pågående eller arkiverad kampanj.

En parameterfil är ett JSON-objekt med `schema_version`, `registry` och en
lista av `{ "name", "value" }`. `init` skapar automatiskt
`baseline-parameters.json` från registret om `baseline.parameter_file` inte
anges. En uttrycklig baselinefil måste matcha registret exakt.

## 3. Snabbstart: en liten riktig kampanj

Det här är en komplett smoke-kampanj för en enda LMR-parameter. Den använder
de binärer och filer som finns i pilotartefakterna ovan. Den har två
sökevalueringar: en matchlös baseline-referens och en kandidat med ett
öppningspar. Bekräftelsen har fyra partier och är avsiktligt för liten för
styrke- eller promotionsbeslut.

Skapa en tom arbetskatalog och spara JSON-blocket som
`/tmp/goalaric-optimizer-smoke/campaign.json`:

```bash
mkdir -p /tmp/goalaric-optimizer-smoke
```

```json
{
  "schema_version": 1,
  "campaign_id": "lmr-smoke-v1-2-1",
  "name": "GoAlaric LMR smoke campaign",
  "mode": "real",
  "registry": "/home/peter/Projekt/GoAlaric-optimizer/artifacts/v1.2/lmr-long-pilot/registry.json",
  "baseline": {
    "engine_id": "/home/peter/Projekt/GoAlaric-optimizer/artifacts/v1.2/node-budget-pilot/bin/goalaric",
    "parameter_file": "/home/peter/Projekt/GoAlaric-optimizer/optimizer/registries/search-lmr-v1-default.json"
  },
  "master_seed": 20261110,
  "partitions": {
    "optimization": {
      "name": "optimization"
    }
  },
  "goals": {
    "max_games": 2,
    "max_evaluations": 2,
    "max_passes": 1,
    "optimizer": {
      "parameters": ["lmr_divisor_x100"],
      "profile": "smoke-search",
      "exploratory": false
    },
    "adaptive": {
      "min_blocks": 1,
      "max_blocks": 1,
      "weak_upper_score": 45.0,
      "target_score": 50.0
    },
    "real": {
      "testmonitor_command": [
        "/home/peter/Projekt/GoAlaric-optimizer/artifacts/v1.2/node-budget-pilot/bin/testmonitor"
      ],
      "fastchess": "/home/peter/Projekt/GoAlaric/.tools/fastchess/bin/fastchess",
      "opening_book": "/home/peter/Projekt/GoAlaric/fullGP.epd",
      "tc": "0.2+0.01",
      "profiles": {
        "smoke-search": {
          "tc": "0.2+0.01"
        },
        "smoke-confirmation": {
          "tc": "1+0.02"
        }
      },
      "hash_mb": 64,
      "threads": 1,
      "syzygy_path": "off",
      "workdir": "/home/peter/Projekt/GoAlaric"
    },
    "confirmation": {
      "enabled": true,
      "games": 4,
      "seed": 20261111,
      "confidence": 0.95,
      "profile": "smoke-confirmation"
    }
  }
}
```

Kör först initiering och läs resultatet:

```bash
CAMPAIGN=/tmp/goalaric-optimizer-smoke/campaign.json
DATA_DIR=/tmp/goalaric-optimizer-smoke/campaigns

optimizer init "$CAMPAIGN" --data-dir "$DATA_DIR"
optimizer status lmr-smoke-v1-2-1 --data-dir "$DATA_DIR"
```

Kör baseline-checkpointen med en resultatkvot på ett. Baseline är en
matchlös referenspunkt; detta steg ska inte starta Fastchess:

```bash
optimizer optimize "$CAMPAIGN" --data-dir "$DATA_DIR" --max-results 1
optimizer trials lmr-smoke-v1-2-1 --last 5 --data-dir "$DATA_DIR"
```

Kör därefter exakt en kandidat. När den sista sökevalueringen samtidigt gör
sökningen terminal väntar bekräftelsen till nästa invocation, eftersom
`--max-results 1` gäller den aktuella invocationen:

```bash
optimizer optimize "$CAMPAIGN" --data-dir "$DATA_DIR" --max-results 1
optimizer status lmr-smoke-v1-2-1 --data-dir "$DATA_DIR"
```

Fortsätt utan kvot. Då startar och slutförs den fasta bekräftelsen:

```bash
optimizer optimize "$CAMPAIGN" --data-dir "$DATA_DIR"
optimizer status lmr-smoke-v1-2-1 --data-dir "$DATA_DIR"
optimizer trials lmr-smoke-v1-2-1 --last 20 --data-dir "$DATA_DIR"
```

När status är `completed` kan rapporterna skrivas. Standardrapporten är
kompakt; detaljrapporten innehåller block-ID:n och blockposter:

```bash
optimizer report lmr-smoke-v1-2-1 \
  --data-dir "$DATA_DIR" --format json \
  --output "$DATA_DIR/lmr-smoke-v1-2-1/final-report.json"
optimizer report lmr-smoke-v1-2-1 \
  --data-dir "$DATA_DIR" --format html \
  --output "$DATA_DIR/lmr-smoke-v1-2-1/final-report.html"
optimizer report lmr-smoke-v1-2-1 \
  --data-dir "$DATA_DIR" --format json --detail \
  --output "$DATA_DIR/lmr-smoke-v1-2-1/final-report-detail.json"
```

Detta test visar transport, checkpoint, återstart och rapportering. Fyra
bekräftelsepartier är inte ett promotionsunderlag.

## 4. Fullständiga kampanjmallar

Båda mallarna nedan är kompletta `mode: real`-filer. De använder
`search-lmr-v1`, men samma profilmekanik gäller för evalregistret och andra
register. Anpassa `campaign_id`, seed och budget före en riktig nattkörning.

### A. Tidsprofiler med `tc`

Varje namngiven profil innehåller exakt ett `tc`-fält. Sökningen kör längre
än den gamla smoke-profilen och bekräftelsen har en separat, ännu längre
tidskontroll.

```json
{
  "schema_version": 1,
  "campaign_id": "lmr-time-v1-2-1",
  "name": "GoAlaric LMR time-profile campaign",
  "mode": "real",
  "registry": "/home/peter/Projekt/GoAlaric-optimizer/artifacts/v1.2/lmr-long-pilot/registry.json",
  "baseline": {
    "engine_id": "/home/peter/Projekt/GoAlaric-optimizer/artifacts/v1.2/node-budget-pilot/bin/goalaric",
    "parameter_file": "/home/peter/Projekt/GoAlaric-optimizer/optimizer/registries/search-lmr-v1-default.json"
  },
  "master_seed": 20261120,
  "partitions": {
    "optimization": {
      "name": "optimization"
    }
  },
  "goals": {
    "max_games": 256,
    "max_evaluations": 40,
    "max_passes": 20,
    "optimizer": {
      "parameters": ["lmr_divisor_x100"],
      "profile": "long-search",
      "exploratory": {
        "enabled": true,
        "min_score": 51.0
      }
    },
    "adaptive": {
      "min_blocks": 4,
      "max_blocks": 16,
      "weak_upper_score": 45.0,
      "target_score": 50.0
    },
    "real": {
      "testmonitor_command": [
        "/home/peter/Projekt/GoAlaric-optimizer/artifacts/v1.2/node-budget-pilot/bin/testmonitor"
      ],
      "fastchess": "/home/peter/Projekt/GoAlaric/.tools/fastchess/bin/fastchess",
      "opening_book": "/home/peter/Projekt/GoAlaric/fullGP.epd",
      "tc": "0.2+0.01",
      "profiles": {
        "long-search": {
          "tc": "1+0.02"
        },
        "long-confirmation": {
          "tc": "2+0.02"
        }
      },
      "hash_mb": 64,
      "threads": 1,
      "syzygy_path": "off",
      "workdir": "/home/peter/Projekt/GoAlaric"
    },
    "confirmation": {
      "enabled": true,
      "games": 200,
      "seed": 20261121,
      "confidence": 0.95,
      "profile": "long-confirmation"
    }
  }
}
```

`real.tc` finns kvar som bakåtkompatibelt fallbackvärde. Eftersom
`optimizer.profile` och `confirmation.profile` är angivna används ändå
`long-search` respektive `long-confirmation`.

### B. Fasta nodprofiler med `nodes`

En nodeprofil betyder fast nodbudget per drag. `nodes: 100000` skickas som
`--nodes 100000` till testmonitor och vidare som `-each nodes=100000` till
Fastchess. Sökning och bekräftelse kan ha olika budget.

```json
{
  "schema_version": 1,
  "campaign_id": "lmr-node-v1-2-1",
  "name": "GoAlaric LMR node-profile campaign",
  "mode": "real",
  "registry": "/home/peter/Projekt/GoAlaric-optimizer/artifacts/v1.2/lmr-long-pilot/registry.json",
  "baseline": {
    "engine_id": "/home/peter/Projekt/GoAlaric-optimizer/artifacts/v1.2/node-budget-pilot/bin/goalaric",
    "parameter_file": "/home/peter/Projekt/GoAlaric-optimizer/optimizer/registries/search-lmr-v1-default.json"
  },
  "master_seed": 20261130,
  "partitions": {
    "optimization": {
      "name": "optimization"
    }
  },
  "goals": {
    "max_games": 256,
    "max_evaluations": 40,
    "max_passes": 20,
    "optimizer": {
      "parameters": ["lmr_divisor_x100"],
      "profile": "node-search",
      "exploratory": {
        "enabled": true,
        "min_score": 51.0
      }
    },
    "adaptive": {
      "min_blocks": 4,
      "max_blocks": 16,
      "weak_upper_score": 45.0,
      "target_score": 50.0
    },
    "real": {
      "testmonitor_command": [
        "/home/peter/Projekt/GoAlaric-optimizer/artifacts/v1.2/node-budget-pilot/bin/testmonitor"
      ],
      "fastchess": "/home/peter/Projekt/GoAlaric/.tools/fastchess/bin/fastchess",
      "opening_book": "/home/peter/Projekt/GoAlaric/fullGP.epd",
      "tc": "0.2+0.01",
      "profiles": {
        "node-search": {
          "nodes": 100000
        },
        "node-confirmation": {
          "nodes": 250000
        }
      },
      "hash_mb": 64,
      "threads": 1,
      "syzygy_path": "off",
      "workdir": "/home/peter/Projekt/GoAlaric"
    },
    "confirmation": {
      "enabled": true,
      "games": 200,
      "seed": 20261201,
      "confidence": 0.95,
      "profile": "node-confirmation"
    }
  }
}
```

Detta är ogiltigt och ska avvisas:

```json
{
  "name": "bad-profile",
  "tc": "1+0.02",
  "nodes": 100000
}
```

En profil måste ha exakt ett av `tc` och `nodes`. `nodes` måste vara ett
positivt heltal. En ändring av namn, läge, faktisk tidskontroll eller
nodbudget efter att kampanjen har startat bryter profilidentiteten och
avvisas vid återstart.

## 5. Parameteroptimering

### Eval-pilot-v1

`eval-pilot-v1` innehåller åtta parameterplatser och används för den äldre
evalfamiljen. Defaultfilen är:

```text
/home/peter/Projekt/GoAlaric-optimizer/optimizer/registries/eval-pilot-v1-default.json
```

Den filen anger värden, men inga koordinatgränser. För riktig optimering ska
ett separat kampanjregister innehålla alla åtta namn och sökmetadata för de
valda parametrarna. Exempel: sök endast `mobility_weight` runt baseline 18
och lämna övriga sju på sina defaultvärden:

```json
{
  "schema_version": 1,
  "registry": "eval-pilot-v1",
  "parameters": [
    {
      "name": "mobility_weight",
      "value": 18,
      "min": 12,
      "max": 24,
      "step": 2,
      "min_step": 1
    },
    {
      "name": "mobility_shift",
      "value": 9
    },
    {
      "name": "activity_bias",
      "value": 5
    },
    {
      "name": "activity_shift",
      "value": 1
    },
    {
      "name": "activity_knight_weight",
      "value": 1
    },
    {
      "name": "activity_bishop_weight",
      "value": 3
    },
    {
      "name": "activity_rook_weight",
      "value": 5
    },
    {
      "name": "activity_queen_weight",
      "value": 2
    }
  ]
}
```

Spara registret separat från gamla kampanjer och ange det i kampanjfilens
`registry`. Ange defaultfilen `eval-pilot-v1-default.json` som
`baseline.parameter_file` om den ska användas som explicit baseline.

### Search-lmr-v1

`search-lmr-v1` innehåller endast `lmr_divisor_x100`. Standardvärdet är 225.
Det verifierade sökregistret anger 175–275 med steg 25 och `min_step` 25.
Lägre divisor ger i motorn aggressivare LMR-reduktion; högre divisor ger
försiktigare reduktion. Sökparametrar behöver därför tillräckligt sökdjup
eller en fast nodbudget för att effekten faktiskt ska aktiveras.

### Välja parametrar och steg

`goals.optimizer.parameters` är en lista med register-namn. Välj få parametrar
i taget när effekten är okänd; en kandidat ändrar normalt en koordinat från det
aktuella ankaret medan övriga värden hålls fasta.

- `min` och `max` är tillåtna motorvärden.
- `step` är första och nuvarande grova provsteg.
- `min_step` är minsta upplösning efter halverade, resultatlösa varv.
- Baselinevärdet måste ligga inom `min`–`max`.
- `step` och `min_step` måste vara positiva och `min_step <= step`.

För varje parameter provas först `anchor + step`, sedan `anchor - step`, med
klippning mot intervallet. Ett klart `accept` eller `accept_exploratory` gör
resultatet valbart när riktningarna sammanställs. Ett helt förbättrande varv
startar om från första valda parameter; ett resultatlöst varv halverar steg
som ännu kan förfinas. SQLite sparar nästa riktning och checkpoint, så en
återstart provar inte samma parameterhash som ett nytt trial.

En grov budgetuppskattning för sökningen är:

```text
search_games <= max(0, max_evaluations - 1) × adaptive.max_blocks × 2
```

Varje adaptivt block är i den verkliga kedjan ett öppningspar, alltså två
partier. `max_evaluations` bevakar sökningens totala `result_count`; den
matchlösa baseline-referensen räknas som det första resultatet. Därför finns
normalt `max_evaluations - 1` kandidatplatser. Tidig `reject_early` kan ge
färre partier. Den fasta bekräftelsens `games` räknas separat och ska planeras
som ett större, oberoende test.

## 6. Körkommandon

Alla kommandon nedan använder samma `--data-dir` för en kampanj. Om flaggan
utelämnas använder CLI:n normalt `optimizer/campaigns` från projektets rot.

### Initiering

```bash
optimizer init campaign.json --data-dir /tmp/goalaric-campaigns
```

`init` validerar kampanjfil och register, skapar kampanjkatalogen, skriver
`baseline-parameters.json` om den saknas och initierar SQLite med WAL.
Utskriften innehåller bland annat campaign-id, databas, config-hash,
baseline-hash och registeridentitet. Att köra `init` igen är tillåtet om
konfiguration och baselineartefakt fortfarande matchar.

### Starta eller återuppta optimering

```bash
optimizer optimize campaign.json --data-dir /tmp/goalaric-campaigns
```

Kommandot startar eller återupptar den autonoma sökningen. I `mode: real`
kopplas kandidatens valda profil till testmonitor och Fastchess. När sökningen
är terminal och `goals.confirmation.enabled` är sant går nästa normala steg
vidare till confirmation.

### Begränsa en invocation

```bash
optimizer optimize campaign.json \
  --data-dir /tmp/goalaric-campaigns --max-results 1
```

`--max-results` är arbetskvoten för just den invocationen. `0` betyder tills
sökningen eller kampanjbudgeten är terminal. Den räknar sökresultat som
faktiskt har matats in i koordinatsökningen; resultatet och nästa checkpoint
skrivs innan kommandot returnerar. Om det sista sökresultatet förbrukar en
bounded kvot startar confirmation inte i samma invocation. Nästa invocation
kan fortsätta därifrån utan dubbelräkning.

`--max-games N` och `--max-evaluations N` kan tillfälligt överstyra kampanjens
budget för den aktuella körningen. Använd dem för smoke- eller felsökningstest,
inte för att oavsiktligt ändra den dokumenterade kampanjplanen.

### Status och watch

```bash
optimizer status <campaign-id> --data-dir /tmp/goalaric-campaigns
optimizer status <campaign-id> --data-dir /tmp/goalaric-campaigns \
  --watch --interval 2
optimizer status <campaign-id> --data-dir /tmp/goalaric-campaigns \
  --watch --interval 2 --iterations 30
```

Utan `--watch` skrivs en JSON-snapshot en gång. `--watch` skriver snapshots
med valt intervall; `--iterations 0` betyder tills användaren avbryter.
`--iterations` är praktiskt i ett automatiserat smoke-test.

### Paus, återuppta och stoppa

```bash
optimizer pause <campaign-id> --data-dir /tmp/goalaric-campaigns
optimizer resume <campaign-id> --data-dir /tmp/goalaric-campaigns
optimizer stop <campaign-id> --data-dir /tmp/goalaric-campaigns
```

`pause` avslutar aktiva matchblock kontrollerat och lämnar kampanjen pausad.
`resume` fortsätter från SQLite. `stop` markerar kampanjen som avbruten och
avslutar aktiva sökblock; använd `resume` för att fortsätta. v1.2.1 har inget
separat CLI-kommando för att pausa ett aktivt confirmation-block. Låt därför
helst pågående confirmation-block slutföras innan kontrollen avbryts. Kör inte
två optimizerprocesser för samma kampanj.

### Trials och enskilda resultat

```bash
optimizer trials <campaign-id> --last 20 --data-dir /tmp/goalaric-campaigns
optimizer best <campaign-id> --data-dir /tmp/goalaric-campaigns
optimizer show <campaign-id> trial-000001 --data-dir /tmp/goalaric-campaigns
```

`trials` visar senaste trialposter med status, parameterhash, statistik och
profil. `best` läser senaste färdiga trial; det är inte samma sak som
slutankaret. `show` kräver både campaign-id och exakt trial-id.

### Dashboard

Starta den lokala, skrivskyddade dashboarden i en separat terminal:

```bash
optimizer dashboard <campaign-id> \
  --data-dir /tmp/goalaric-campaigns \
  --listen 127.0.0.1:8787 --refresh-ms 1000
```

Öppna sedan:

```text
http://127.0.0.1:8787/
```

Dashboarden läser SQLite read-only med query-only-läge. Den har inga
kontrollknappar och påverkar inte körningen. `--refresh-ms` är millisekunder.
API-snapshoten kan vid behov läsas med:

```bash
curl http://127.0.0.1:8787/api/dashboard
```

### Slutrapport

Rapport är ett alias för `dashboard-report` och kräver att kampanjen är
färdig, inklusive confirmation om den är aktiverad:

```bash
optimizer report <campaign-id> --data-dir /tmp/goalaric-campaigns \
  --format json --output /tmp/goalaric-campaigns/report.json
optimizer report <campaign-id> --data-dir /tmp/goalaric-campaigns \
  --format html --output /tmp/goalaric-campaigns/report.html
optimizer report <campaign-id> --data-dir /tmp/goalaric-campaigns \
  --format json --detail --output /tmp/goalaric-campaigns/report-detail.json
```

`--format` är `html` eller `json`. Utan `--output` skrivs innehållet till
stdout. Standardrapporten är kompakt. `--detail` lägger till block-ID:n och
blockposter och bör användas när blocknivå behövs för revision.

### Rena Ctrl-C-förfaranden

- `Ctrl-C` i `status --watch` avslutar endast observatören. CLI:n fångar detta
  utan traceback.
- `Ctrl-C` i dashboardterminalen avslutar dashboardservern; optimizer och
  aktiva matcher fortsätter oberoende.
- För att avbryta en pågående optimizerkörning, använd i första hand `pause`
  eller `stop` från en annan terminal under sökfasen. Då markeras ett aktivt
  sökblock korrekt och processgruppen avslutas.
- Undvik `Ctrl-C` mitt i en riktig confirmation. Den aktuella CLI:n har ingen
  separat confirmation-pause/stop; låt helst blocket slutföras. Om processen
  ändå måste avbrytas, kontrollera med `pgrep` att testmonitor/Fastchess och
  GoAlaric verkligen har lämnat innan samma kampanj återupptas. Nästa
  `optimize` kan därefter återställa ett övergivet confirmation-block från
  SQLite. Ändra inte databasen manuellt.
- Om Pythonprocessen dör oväntat ska samma `optimizer optimize`-kommando
  återupptas. Vid uppstart återställs övergivna jobb och nästa körning använder
  SQLite-checkpointen. Kontrollera alltid status och processer före och efter.

## 7. Övervakning

### Dashboardens huvudstatus

- `running` betyder att sökfasen eller ett matchblock fortfarande pågår.
- `confirming` betyder att sökningen är färdig men confirmation ännu inte är
  klar. Bekräftelsen visas då som dashboardens huvudsektion.
- `completed` betyder att kampanjen är färdig. Med confirmation kräver det
  att confirmation har fått `confirmed`, `rejected` eller `inconclusive`.

Dashboarden visar kandidatlistan som sökhistorik och visar samtidigt den
aktiva bekräftelsens kandidat, hash, parameterdiff, öppningspar, partier,
W–D–L, score, Elo, intervall, profil, starttid, förfluten tid och beräknad
återstående tid.

### W–D–L, score, Elo och intervall

W–D–L är kandidatens vinster, remier och förluster mot den referens som
respektive fas använder. Score är procentuell matchpoäng:

```text
score = (vinster + 0,5 × remier) / partier × 100
```

Elo är ett estimat med ett kontinuitetskorrigerat intervall. Scoreintervallet
är det viktiga beslutsunderlaget i den adaptiva fasen och confirmation. Ett
intervall som omfattar 50 procent är inte ett statistiskt bevis på förbättring.

### Förbrukade partier

I status och dashboard visas kampanjens förbrukade partier. I slutrapporten
delas de uttryckligen upp i:

- `search_games`: partier i kandidat- och adaptiv sökning,
- `confirmation_games`: partier i den fasta bekräftelsen,
- `total_games`: summan av de två.

Baseline-referensen är normalt matchlös. Ett avbrutet eller ofullständigt
block ska inte räknas som färdigt resultat.

### Slutankare och högsta lokala trial

`final_anchor` kommer från optimizer-checkpointens `anchor_parameters`. Det är
den enda parameteruppsättning som ska skickas till confirmation. `highest_local_trial`
är endast den högsta lokala matchscoren i historiken mot ett tidigare ankare.
Den kan vara imponerande men är inte automatiskt slutparameter, rekommendation
eller bekräftad förbättring.

### Profil och processaudit

Status, dashboard och rapport visar profilnamn, profilhash och faktisk
tidskontroll eller nodbudget, till exempel `node-search · 100000 nodes/move`.
För en riktig blockkörning är `monitor-config.json` den primära kontrollen av
vad testmonitor fick. Fastchess-kommandot ska för tidsprofil ha `-each tc=...`
och för nodeprofil `-each nodes=...`, aldrig båda.

Kontrollera aktiva processer från en separat terminal:

```bash
pgrep -af 'optimizer|testmonitor|fastchess|goalaric' || true
```

Granska träffarna manuellt eftersom `pgrep` även kan hitta den egna
kommandoraden. Efter en avslutad eller pausad körning ska inga oavsiktliga
optimizer-, testmonitor-, Fastchess- eller GoAlaricprocesser finnas kvar.

## 8. Tolkning av resultat

### Sökbeslut

- `accept`: strikt adaptivt beslut; hela scoreintervallets nedre gräns ligger
  över målet, normalt 50 procent. Kandidaten kan flytta ankaret.
- `accept_exploratory`: punktresultatet överstiger
  `goals.optimizer.exploratory.min_score` efter maximal sökbudget. Kandidaten
  kan flytta ankaret, men beslutet är explorativt och inte statistiskt
  bekräftat.
- `reject`: strikt adaptivt beslut där intervallets övre gräns ligger under
  målet. Ankaret flyttas inte.
- `reject_early`: kandidaten stoppades tidigt eftersom övre intervallet låg
  under `weak_upper_score`. Ankaret flyttas inte.
- `reject_exploratory`: explorativ punktgräns nåddes inte. Ankaret flyttas
  inte.
- `uncertain`: intervallet omfattar målet efter maximal strikt sökbudget.
  Ankaret flyttas inte. I explorativt läge omklassificeras ett sådant
  maxresultat uttryckligen till `accept_exploratory` eller
  `reject_exploratory` enligt punktgränsen.

Ett `accept*`-beslut är auktoritativt i koordinatsökningen när beslutet finns
i resultatraden. Äldre syntetiska resultat som saknar `decision` kan använda
`candidate_objective` som fallback. Ett högt punktresultat ensamt är däremot
inte ett bevis: slumpen kan ge 61 procent i ett litet block och ändå ge ett
intervall som omfattar 50 procent eller vänder i ett större test.

### Bekräftelseutfall

- `confirmed`: det fasta 95-procentsintervallets nedre gräns är över 50
  procent. Endast detta utfall får skapa en rekommenderad parameterfil.
- `rejected`: intervallets övre gräns är under 50 procent. Baseline förblir
  det säkra manuella utgångsvärdet.
- `inconclusive`: intervallet omfattar 50 procent. Ingen kandidat
  rekommenderas.

Ingen av dessa utfall promoverar automatiskt en parameter till motorn. Även
`confirmed` är ett manuellt granskningsunderlag.

## 9. Säker återstart

1. Använd exakt samma kampanjfil. Ändra inte `campaign_id`, `master_seed`,
   registry, baseline eller profilblock mellan invocations.
2. Använd exakt samma `--data-dir`; där ligger `campaign.db`, baselinefil,
   checkpoints och kampanjens artifacts.
3. Återuppta med samma kommando:

   ```bash
   optimizer optimize /sökväg/campaign.json --data-dir /sökväg/campaigns
   ```

4. Kontrollera först ägare och aktiva processer:

   ```bash
   optimizer status <campaign-id> --data-dir /sökväg/campaigns
   pgrep -af 'optimizer|testmonitor|fastchess|goalaric' || true
   ```

5. Om kampanjen är pausad eller avbruten, använd `resume` eller låt
   `optimize` återuppta enligt den dokumenterade vägen.

Profilens namn, hash, läge och faktiska `tc`/`nodes` sparas i checkpoint och
resultat. En återstart med ändrad profil, ändrad nodbudget eller blandat
profilformat avvisas medvetet i stället för att blanda resultat från olika
beräkningsbudgetar. Samma skydd gäller ändrat register och ändrad baseline-
parameterartefakt.

SQLite-databasen och kampanjens owner-token är sanningskällan för ägarskap.
Starta inte en andra optimizerprocess parallellt. En OS-lock hindrar samtidiga
kontrolloperationer, och en ny optimizer återställer övergivna jobb först när
den själv tar över kampanjen.

### Säker säkerhetskopia

Ta en SQLite-backup till en separat plats före lång körning. Ändra inte
originalet och återställ inte backupen över en aktiv databas. Python-standard-
bibliotekets online-backup kan användas:

```bash
optimizer/.venv/bin/python - <<'PY'
import sqlite3

source = sqlite3.connect("/sökväg/campaigns/<campaign-id>/campaign.db")
target = sqlite3.connect("/tmp/<campaign-id>-campaign-backup.db")
with target:
    source.backup(target)
target.close()
source.close()
PY
```

## 10. Praktiska rekommendationer

- Börja alltid med en smoke-kampanj med en parameter, en kandidat och små
  adaptiva block.
- Kör några bounded invocations med `--max-results 1` och kontrollera
  checkpoint, profil och återstart innan en obevakad kampanj.
- Använd tidsprofil när du vill styra verklig betänketid. Använd nodeprofil
  när kandidaterna ska jämföras med ungefär samma sökarbete per drag. Fast
  nodbudget är inte samma sak som exakt lika lång väggklocktid.
- Använd längre sökprofil än evalprofil för sökparametrar som LMR, LMP, null
  move, history, aspiration och liknande. Parametern måste faktiskt påverka
  sökningen på den valda profilen.
- Använd en separat och betydligt större confirmation med nytt seed och nya
  öppningspar. Låt confirmation vara oberoende av sökbesluten.
- Dimensionera `max_games` efter värsta fall:
  `max_evaluations × max_blocks × 2` för sökningen, och planera
  `confirmation.games` separat.
- Låt dashboard och `status --watch` vara observatörer. De skriver inte till
  SQLite och ska inte användas som kontrollgränssnitt.
- Säkerhetskopiera SQLite med online-backup till en separat fil. Rör aldrig
  originaldatabasen manuellt under en körning.
- Ingen punktpoäng, explorativt accept eller liten pilot räcker ensam för
  promotion.

## 11. Felsökning

### Kommandot ger tom output

Kontrollera först att rätt venv och källkod används:

```bash
source /home/peter/Projekt/GoAlaric-optimizer/optimizer/.venv/bin/activate
cd /home/peter/Projekt/GoAlaric-optimizer
optimizer --help
```

Kör sedan `status` utan `--watch`. CLI:n skriver JSON till stdout; fel skrivs
som `goalaric_optimizer: ...` till stderr. För en observerad körning kan du
använda `--iterations 1` för att skilja ett snabbt avslut från ett långvarigt
watch-läge.

### Fel sökväg

Kör `test -x` för binärer och `test -f` för register, parameterfil och EPD.
Använd absoluta sökvägar i kampanjfilen. `goals.real.engine` kommer från
`baseline.engine_id`; det finns inget separat `goals.real.engine`-fält.

### Saknad EPD eller för liten öppningsfil

Kontrollera:

```bash
test -f /home/peter/Projekt/GoAlaric/fullGP.epd
wc -l /home/peter/Projekt/GoAlaric/fullGP.epd
```

Kontrollera också att `opening_book` pekar på samma fil vid återstart och att
den räcker för kampanjens block och confirmation. Ändra inte öppningsfilen
mitt i en kampanj; dess hash ingår i matchunderlaget.

### Fastchess startar inte

Kontrollera först `status`, `trials` och den aktuella trialens `run_dir`:

```bash
optimizer status <campaign-id> --data-dir /sökväg/campaigns
optimizer trials <campaign-id> --last 5 --data-dir /sökväg/campaigns
optimizer show <campaign-id> <trial-id> --data-dir /sökväg/campaigns
```

I den aktuella blockkatalogen kan du granska `monitor-config.json`,
`status.json` och `block-report.json`. De visar mottagna profilvärden,
kommandorad, motor-/parameteridentiteter och testresultat. Kontrollera också
att Fastchess är körbar och att `workdir` finns.

### ParameterFile avvisas

Parameterfilen måste ha samma `schema_version` och `registry` som registret,
alla registerparametrar exakt en gång och endast fälten `name` och `value` per
parameterpost. Ett eval-register med åtta parametrar kräver alltså åtta
parameterposter även om endast en väljs i `optimizer.parameters`.

Kontrollera att GoAlaric-binären är byggd för samma register och att baseline
och kandidat har samma registeridentitet. Ändra inte gamla parameterfiler för
att passa en ny kampanj; skapa en separat register- eller parameterartefakt.

### Profilen matchar inte checkpointen

Kontrollera `optimizer.profile` och `confirmation.profile` mot
`goals.real.profiles`. Varje profil måste ha exakt ett av `tc` och `nodes`.
För nodeprofil ska `nodes` vara positivt heltal. Profilnamn, hash, läge och
budget måste vara samma som vid första körningen. Om de avsiktligt ska ändras,
starta en ny kampanj med nytt `campaign_id` och ny `--data-dir`; blanda inte
resultaten.

### Dashboarden visar gammalt tillstånd

Kontrollera att dashboarden och `status` använder samma campaign-id och
`--data-dir` som optimizerprocessen:

```bash
optimizer status <campaign-id> --data-dir /sökväg/campaigns
curl http://127.0.0.1:8787/api/dashboard
```

Dashboarden pollar med `--refresh-ms` och API:t använder `cache: no-store`.
Starta inte en andra dashboard på en annan datakatalog av misstag. Status och
trials från SQLite är den primära diagnosen; dashboarden är read-only.

### Processen avslutas direkt

Kör samma kommando igen i förgrunden och läs stderr. Kontrollera därefter:

```bash
optimizer status <campaign-id> --data-dir /sökväg/campaigns
optimizer trials <campaign-id> --last 10 --data-dir /sökväg/campaigns
pgrep -af 'optimizer|testmonitor|fastchess|goalaric' || true
```

Vanliga orsaker är fel JSON, saknat register, saknad binär/EPD, en profil som
inte kan lösas, en ändrad checkpoint eller att en annan optimizer redan äger
kampanjen. För verkliga block finns diagnosfiler i `run_dir`; läs dem innan du
försöker återuppta. Starta ingen ny kampanj på samma data-dir för att kringgå
ett fel.

## 12. Avslutande checklista

### Före start

- [ ] `optimizer --help` körs från rätt `.venv`.
- [ ] GoAlaric, testmonitor och Fastchess är körbara och hashade.
- [ ] Öppningsfil, register och baseline-parameterfil finns.
- [ ] Registerordning, parameterintervall och `optimizer.parameters` är
      avsiktliga.
- [ ] Varje profil innehåller exakt ett av `tc` eller `nodes`.
- [ ] Sök- och bekräftelseprofil är namngivna och tillräckligt långa.
- [ ] Nytt master-seed och separat confirmation-seed är valda.
- [ ] `max_games`, `max_evaluations`, `max_blocks` och confirmation.games är
      förenliga.
- [ ] SQLite är säkerhetskopierad till separat plats.
- [ ] Ingen annan optimizer äger campaign-id:t.

### Under körning

- [ ] `status --watch` och dashboard pekar på rätt data-dir.
- [ ] Status växlar från `running` till `confirming` först efter terminal sökfas.
- [ ] Profilnamn, faktisk `tc`/`nodes` och profilhash är rätt.
- [ ] Sökningens kandidatlista sparas som historik.
- [ ] Confirmation visar slutankarets parametrar och diff mot baseline.
- [ ] W–D–L, score, Elo, intervall och consumed games uppdateras.
- [ ] Vid paus används `pause`, och vid avsiktligt stopp används `stop`.
- [ ] Efter återstart används samma kampanjfil och data-dir.
- [ ] Inga dubbla processer eller oväntade parallella kampanjer finns.

### Efter avslutad kampanj

- [ ] Status är `completed` och confirmation har ett slututfall.
- [ ] Rapporten visar `final_anchor`, inte bara `highest_local_trial`.
- [ ] `search_games`, `confirmation_games` och `total_games` stämmer.
- [ ] Kandidatens hash, parametrar och parameterdiff är rätt.
- [ ] Standardrapporten är kompakt; detaljrapport används endast vid behov.
- [ ] Inga optimizer-, testmonitor-, Fastchess- eller GoAlaricprocesser finns
      kvar.
- [ ] SQLite, rapporter och relevanta blockartefakter är arkiverade.

### Före manuell parameterändring eller promotion

- [ ] Utfallet är `confirmed`, inte bara `accept` eller
      `accept_exploratory`.
- [ ] 95-procentsintervallets nedre gräns ligger över 50 procent.
- [ ] Confirmation använde rätt slutkandidat mot ursprunglig baseline.
- [ ] Profil, motorbinär, parameterhash, seed och öppningshash är dokumenterade.
- [ ] `rejected` eller `inconclusive` lämnar baseline som rekommendation.
- [ ] Ingen automatisk promotion har skett; ändringen granskas och genomförs
      manuellt.
