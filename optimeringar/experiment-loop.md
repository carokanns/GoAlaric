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
Det kompakta modellunderlaget innehåller alltid denna flagga. `semantic_ok` är
bara ett korrekthetskrav när `semantic_preserving` är `true`; annars är ändrade
noder, scores och bestmoves diagnostik och förväntade effekter av kandidaten.

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

Efter en godkänd screening får den automatiska modellen endast välja `sprt`
eller `no_sprt`. Ett validerat `sprt`-beslut startar matchen automatiskt;
`no_sprt` lämnar experimentet i `awaiting_decision`. Efter SPRT körs ingen
modell: resultatet lämnas alltid för utvärdering i den synliga
användarsessionen.

## Frikopplad kandidatkampanj från Codex CLI

Efter att en kandidatändring är implementerad, testad lokalt och committad kan
Codex starta hela återstående kedjan med:

```bash
scripts/start_candidate_campaign.sh \
  --candidate-id <id> \
  --candidate-worktree <absolut-worktree> \
  --baseline <baseline-binär> \
  --hypothesis "<hypotes>" \
  --change "<ändring>" \
  --semantic-preserving=false \
  --prescan full --minimum-depth 8
```

Kommandot validerar en ren kandidatworktree, bygger en fristående
`testmonitor` och startar en transient `systemd --user`-service. Servicen bygger
kandidaten, kör den deterministiska pipelinen och genomför vald depth pre-scan
innan screening. En cachad baselineprofil återanvänds. Vid för lågt
median-djup provas nästa konfigurerade tidskontroll; samma tidskontroll används
sedan av screening och SPRT. Läget `baseline` hoppar över kandidatprofilen när
ändringen inte påverkar djupet nämnvärt, och `skip` används för ändringar som
inte är djupberoende. Därefter läser servicen endast lokala statusfiler var tionde
sekund. Det kostar inga modelltokens. Den befintliga kompakta engångsmodellen
väljer endast `sprt` eller `no_sprt` efter godkänd screening.

Vid `no_sprt`, avslutad SPRT, hårt testfel eller annat terminalt fel skriver
servicen `artifacts/automation/active-campaign.json` med status
`awaiting_decision`, `tests_failed` eller `failed` och avslutas automatiskt.
Användaren kan därefter återuppta Codex CLI i projektmappen och begära manuell
utvärdering. Visa aktuell status med:

```bash
scripts/campaign_status.sh
```
