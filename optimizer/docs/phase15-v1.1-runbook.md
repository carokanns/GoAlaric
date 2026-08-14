# GoAlaric optimizer v1.1 – slutverifiering

Detta är runbooken för den reproducerbara slutverifieringen av v1.1. Den
autonoma körningen startas från en vanlig terminal med Python-entrypointen;
ingen LLM behövs under kampanjen. Huvudverifieringen använder den snabba falska
runnern. Den befintliga minimala verkliga verifieringen kör samma flöde genom
testmonitor, Fastchess och GoAlaric.

## Installation

Från repots rot:

```bash
source optimizer/.venv/bin/activate
optimizer --help
```

Miljön kan också användas utan aktivering:

```bash
optimizer/.venv/bin/optimizer --help
```

Kontrollera att paketet rapporterar v1.1 efter release:

```bash
optimizer/.venv/bin/python -c 'import goalaric_optimizer; print(goalaric_optimizer.__version__)'
```

## Exempel från terminal

Exemplet använder JSON och en syntetisk målfunktion. Den verkliga kampanjfilen
kan ersätta `mode: fake` när `goals.real` pekar på testmonitor, Fastchess,
öppningsfil och motorbinär.

```bash
export DATA_DIR="$PWD/artifacts/phase15/campaigns"
export CAMPAIGN="$PWD/optimizer/examples/phase15-v1.1-campaign.json"
export CAMPAIGN_ID=phase15-v1-1-example

optimizer init "$CAMPAIGN" --data-dir "$DATA_DIR"
optimizer status "$CAMPAIGN_ID" --data-dir "$DATA_DIR"
```

Starta dashboarden i en separat terminal:

```bash
optimizer dashboard "$CAMPAIGN_ID" --data-dir "$DATA_DIR" \
  --listen 127.0.0.1:8787 --refresh-ms 500
```

Starta därefter den autonoma körningen:

```bash
optimizer optimize "$CAMPAIGN" --data-dir "$DATA_DIR"
```

`optimize` fortsätter själv genom flerupplöst koordinatsökning och startar den
fasta bekräftelsen automatiskt efter sökningens terminala SQLite-checkpoint.

## Observation, stopp och återstart

Status kan följas från en annan terminal medan körningen pågår:

```bash
optimizer status "$CAMPAIGN_ID" --data-dir "$DATA_DIR" --watch --interval 1
curl http://127.0.0.1:8787/api/dashboard
```

För ett deterministiskt planerat avbrott under sökningen begränsas en
invokering. Samma kommando återstartar från SQLite:

```bash
optimizer optimize "$CAMPAIGN" --data-dir "$DATA_DIR" --max-results 1
optimizer optimize "$CAMPAIGN" --data-dir "$DATA_DIR" --max-results 1
```

När bekräftelsen har startat kan samma procedur användas igen. En begränsad
invokering lämnar redan färdiga bekräftelseblock orörda och nästa invokering
fortsätter med nästa block:

```bash
optimizer status "$CAMPAIGN_ID" --data-dir "$DATA_DIR"
optimizer optimize "$CAMPAIGN" --data-dir "$DATA_DIR" --max-results 1
optimizer optimize "$CAMPAIGN" --data-dir "$DATA_DIR"
```

Vid ett manuellt säkert stopp används `stop`, följt av `resume` och samma
autonoma kommando:

```bash
optimizer stop "$CAMPAIGN_ID" --data-dir "$DATA_DIR"
optimizer resume "$CAMPAIGN_ID" --data-dir "$DATA_DIR"
optimizer optimize "$CAMPAIGN" --data-dir "$DATA_DIR"
```

Ett avbrutet block räknas inte som spelat. SQLite är sanningskällan för
checkpoint, blockstatus och spelidentiteter.

## Slutrapport och rekommendation

När status visar `confirmation.status=completed` ska rapporten skrivas och
kontrolleras:

```bash
optimizer report "$CAMPAIGN_ID" --data-dir "$DATA_DIR" \
  --format json --output "$DATA_DIR/$CAMPAIGN_ID/final-report.json"
optimizer report "$CAMPAIGN_ID" --data-dir "$DATA_DIR" \
  --format html --output "$DATA_DIR/$CAMPAIGN_ID/final-report.html"
cat "$DATA_DIR/$CAMPAIGN_ID/final-report.json"
test ! -e "$DATA_DIR/$CAMPAIGN_ID/recommended-parameters.json" || \
  cat "$DATA_DIR/$CAMPAIGN_ID/recommended-parameters.json"
```

Bekräftelsen jämför alltid slutkandidaten med ursprunglig baseline. Utfallet
`confirmed` får rekommendera kandidaten. Vid `rejected` eller `inconclusive`
ska rapportens `recommendation` vara tomt och filen
`recommended-parameters.json` saknas.
Fältet `automatic_promotion` ska alltid vara `false`.

## Reproducerbar verifiering

Kör hela terminaldrivna verifieringen från repots rot:

```bash
PYTHONPATH=optimizer/src optimizer/.venv/bin/python \
  -m unittest optimizer.tests.test_phase15
```

Kör även bekräftelsens alla tre statistiska utfall och minimala verkliga
Fastchess-kedja:

```bash
PYTHONPATH=optimizer/src optimizer/.venv/bin/python \
  -m unittest optimizer.tests.test_confirmation
```

För slutlig kontroll av den lokala arbetsmiljön:

```bash
go test ./...
go vet ./...
ps -eo pid=,stat=,command= | rg 'testmonitor|fastchess|goalaric|optimizer dashboard' || true
```

Den sista kontrollen ska inte visa kvarvarande kampanjprocesser efter
verifieringen. Kampanjdatabas, rapporter, loggar och matchfiler under
`artifacts/` är verifieringsunderlag och ska normalt inte committas.
