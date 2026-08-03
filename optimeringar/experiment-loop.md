# Tokenminimal experimentloop

`cmd/testmonitor pipeline` kör hela den deterministiska testkedjan utan LLM.
Resultat och fullständiga loggar sparas under
`artifacts/experiments/<candidate-id>/`. Endast `decision_input.json` behöver
lämnas till modellen.

## Körning

```bash
go run ./cmd/testmonitor pipeline \
  --baseline artifacts/baseline/goalaric-933f163 \
  --candidate artifacts/candidate/goalaric-new \
  --candidate-id candidate-new
```

Pipelineordningen är `go test`, perft, UCI-smoke, fixed-depth benchmark och
movetime. Ett korrekthetsfel stoppar senare steg. Fastdjupsresultat måste vara
identiska som standard; använd `--semantic-preserving=false` för en kandidat
som uttryckligen ändrar söksemantiken.

Screening kan startas explicit:

```bash
go run ./cmd/testmonitor pipeline \
  --baseline <baseline> --candidate <candidate> \
  --candidate-id <id> --screening
```

SPRT körs inte av den deterministiska pipelinen. En manuellt startad screening
med `--auto-evaluate` kan däremot följas av SPRT utan ett mellanliggande
mänskligt godkännande när modellbeslutet är `sprt` och alla hårda grindar
passerar.
En pågående SPRT kan alltid avbrytas med `go run ./cmd/testmonitor stop` eller
med `--run-dir` för att välja en viss körning. Partiresultat och loggar fram
till avbrottet bevaras. SPRT kör parallellt och stoppas omedelbart; pågående
öppningspar behöver alltså inte färdigspelas.

Pågående matcher rapporterar delresultat via `testmonitor progress`. Skripten
`run_match.sh` och `run_sprt_match.sh` visar dem automatiskt i terminalen;
`testmonitor follow` kan ansluta till en redan startad match.
Standardintervallet är 10 partier för screening och 50 för SPRT; SPRT-raden
innehåller även LLR och beslutsgränser. En extra statusrad skrivs varje minut.

## LLM-underlag och beslut

```bash
go run ./cmd/testmonitor snapshot --candidate-id candidate-new
go run ./cmd/testmonitor record-decision \
  --candidate-id candidate-new --decision decision.json
```

Modellen ska returnera JSON med `candidate_id`, `recommendation`,
`next_change`, `hypothesis`, `required_tests` och `reason`. Tillåtna
rekommendationer är `reject`, `continue`, `propose_change`, `screening`,
`sprt` och `promote`. Även `promote` blir `awaiting_approval`; baseline
ändras aldrig automatiskt.

`decision_input.json` innehåller sammanfattade mätvärden, fel, ändrade
positioner och länkar till lokala artefakter. Råloggar och PGN inkluderas inte
i modellunderlaget.

Automatiska beslut använder samma kompakta data och samma beslutsfält. Efter
screening startas endast ett validerat SPRT-beslut automatiskt. Efter SPRT
skrivs `approval-package.json` med rekommendation om baseline och nästa
förbättring; paketet inväntar alltid användarens godkännande.
