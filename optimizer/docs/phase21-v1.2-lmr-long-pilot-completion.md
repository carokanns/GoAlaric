# v1.2 LMR long pilot – slutverifiering

Senast verifierad: 2026-08-15.

## Kampanj

Den befintliga kampanjen `lmr-long-pilot-v1-2` återupptogs med det ordinarie
`optimizer optimize`-kommandot. Ingen ny kampanj eller databas initierades.

- commit: `1e31b40` (`Fix coordinate anchor reconciliation`)
- status: `completed`
- sökfas: 96 kompletta block, 192 partier
- bekräftelsefas: 100 kompletta öppningspar, 200 partier
- total: 392 partier
- slutankare från optimizer-checkpoint: `lmr_divisor_x100=175`
- ursprunglig baseline: `lmr_divisor_x100=225`
- bekräftelseprofil: `long-confirmation`, faktisk tc `2+0.02`
- bekräftelsens kandidathash: `31a5051ab7289c2dc260021998022f994791cf098e69235c4f55d4bf6ced8d04`
- baseline-hash: `fdfd15e61148fea75b6210ec07b84151ef2c654205e2479a2e441be06edbd824`

Bekräftelseresultatet är:

| W–D–L | Score | 95 %-intervall | Utfall |
|---:|---:|---:|---|
| 60–81–59 | 50,25 % | 44,90494158–55,59505842 % | `inconclusive` |

`automatic_promotion` är `false` och ingen kandidat rekommenderas. Baseline
förblir därför det säkra manuella utgångsvärdet.

Sökfasen avslutades `2026-08-15T18:53:41.448059Z` och bekräftelsen avslutades
`2026-08-15T19:38:07.779822Z`.

## Databasintegritet

Read-only-kontroller mot `campaign.db` visade:

- 100/100 bekräftelseblock har status `completed`.
- 100 unika block-ID:n, 100 unika blockindex och 100 unika materialiserade
  öppningshashar.
- Alla bekräftelseblock hade första försöket (`attempt=1`).
- 200 unika bekräftelsepartier och 200 unika `(block_id, game_index)`-platser.
- W–D–L i block, partier och confirmation-raden är 60–81–59.
- Sökfasen har 96 unika block och 192 unika partier.
- Checkpointens ankare och bekräftelsens kandidatdokument är båda 175; ingen
  högsta lokal trial används som slutankare.
- Kampanjens owner-token är frigjord och inga körande block finns kvar.

De kompakta rapporterna finns här:

- `artifacts/v1.2/lmr-long-pilot/pilot-final-report-v1.2.json`
- `artifacts/v1.2/lmr-long-pilot/pilot-final-report-v1.2.html`

Standardrapporten innehåller inte per-block-ID:n.

## Kvotutredning

Den tidigare körningen med `--max-results 1` hade ett verkligt styrfel före
ankarkorrigeringen. När den sista sökevalueringen förbrukade invocationens
kvot fortsatte den gamla integrationsvägen direkt till confirmation. Den
räknade då ut `remaining=0`; i `ConfirmationCampaign.run()` betyder noll
obegränsad körning. Ett tvåblockstest kunde därför slutföra båda blocken,
alltså fyra partier, trots en kvot på ett sökresultat.

Den aktuella koden spärrar confirmation när den bounded invocationen precis
har förbrukat sin sökkvot. Nästa invocation startar confirmation med kvarvarande
kvot som blockgräns. `ConfirmationCampaign.run(max_blocks=1)` returnerar efter
ett färdigt block och kan återupptas idempotent.

Detta är verifierat med fake-/SQLite-regressionen
`Phase20CoordinateQuotaTest.test_bounded_final_search_result_defers_confirmation_until_next_invocation`:

1. invocation 1 med gräns 1 gör en sökevaluering.
2. invocation 2 med gräns 1 gör den sista sökevalueringen och startar inte
   confirmation.
3. invocation 3 med gräns 1 registrerar exakt ett confirmation-block och två
   partier.
4. en obegränsad återstart slutför återstående confirmation utan dubbletter.

Ingen ytterligare riktig match kördes för kvotutredningen och pilotdatabasen
ändrades inte manuellt.

## Verifiering

- fake-/SQLite-/dashboardsvit utan realintegrationsklasser: **49 tester OK**
- phase20 efter den exakta kvotregressionen: **7 tester OK**
- confirmation fake outcomes: **3 tester OK**
- `go test ./...`: **PASS**
- `go vet ./...`: **PASS**
- `python -m compileall -q optimizer/src optimizer/tests`: **PASS**
- `git diff --check`: **PASS**
- processaudit efter körning: inga optimizer-, testmonitor-, Fastchess- eller
  GoAlaricprocesser kvar

Pilotdatabasen får inte återupptas igen; den är terminal och verifierad.
