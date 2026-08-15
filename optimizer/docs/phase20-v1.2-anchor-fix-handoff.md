# v1.2 koordinatankare – återupptagningsanteckning

Senast uppdaterad: 2026-08-15.

Detta dokument beskriver det ocommittade arbetet med styrfelet i
lmr-long-pilot-v1-2.

## Arbetsläge

- Gren: tooling/parameter-optimizer
- HEAD: bf3021d
- Grenen är i synk med origin.
- Ingen commit, push, tagg eller versionsändring har gjorts.
- Arbetskopian är avsiktligt smutsig med ändringarna nedan.
- Ingen GoAlaric-, testmonitor- eller Fastchessprocess körs. Ingen dashboard
  startades av denna fix.

Aktuell git status --short:

 M optimizer/src/goalaric_optimizer/coordinate.py
 M optimizer/src/goalaric_optimizer/optimization.py
 M optimizer/tests/test_confirmation.py
?? optimizer/docs/phase20-v1.2-anchor-fix-handoff.md
?? optimizer/tests/test_phase20.py

## Rotorsak och korrigering

Pilotdatabasen är:

artifacts/v1.2/lmr-long-pilot/campaigns/lmr-long-pilot-v1-2/campaign.db

Den har inte ändrats. Read-only-kontrollen visade checkpoint-revision 69,
result_count=3, ankare lmr_divisor_x100=225, kandidat 250 som
reject_exploratory/loss och kandidat 200 som accept_exploratory/win.

Flödet var:

1. adaptive-lagret sparade kandidatens matchresultat.
2. coordinate-lagret klassificerade punktresultatet över 51 procent som
   accept_exploratory och win.
3. _step() skrev resultatet atomiskt till SQLite och returnerade
   produced_result=True.
4. run(max_results=1) räknade resultatet och returnerade efter den atomiska
   checkpointen.
5. _select_coordinate() kördes först vid nästa anrop.

SQLite innehöll därför det accepterade resultatet i coordinate_results, men
anchor_parameters var fortfarande 225. Det var ett kvot-/tillståndsproblem,
inte ett fel i adaptive-beslutet.

Den första korrigeringen löste kvotgränsen med _settle_after_result(), men
code review visade ytterligare två problem:

- reconcile_checkpoint() kunde själv nå phase=completed utan att köra den
  terminala kampanjstatusövergången som fanns i run().
- _classify() prioriterade candidate_objective framför ett explicit adaptivt
  decision.

Den nya _complete_campaign_if_finished() används från normal run(),
reconcile_checkpoint() och stop(). Därmed blir en sökning som avslutas under
reconciliation också campaign.status=completed; nästa invocation kan sedan
öppna confirmation och visa confirming. Bounded invocation behåller sin kvot
och startar inte confirmation när den själv precis konsumerat sista
sökresultatet.

Ett giltigt beslut prioriteras nu framför objective metadata: accept-varianter
klassificeras som win, reject-varianter som loss och uncertain som uncertain.
Explorativ policy behåller den avsiktliga omskrivningen av ett maximalt
punktresultat till accept_exploratory eller reject_exploratory. Ett reused
resultat som hör till ett annat ankare förblir uncertain för att inte återanvända
ett beslut från fel jämförelse.

FakeAdaptiveEvaluator modellerar ett deterministiskt objective, inte ett
statistiskt adaptivt beslut. Den lämnar därför decision och uncertain utanför
både det returnerade syntetiska resultatet och trialens checkpoint, så äldre
objective-baserade fakeflöden fortsätter använda objective-fallbacken även vid
reused-resultat.

## Kodändringar

### optimizer/src/goalaric_optimizer/coordinate.py

- _settle_after_result() körs efter ett nytt resultat när kvoten är nådd.
- Den får bara utföra deterministisk bookkeeping: gränshopp, återanvändning,
  koordinatval och passomstart.
- Den får aldrig utvärdera en ny kandidat eller använda extra matcher.
- accept och accept_exploratory flyttar ankaret.
- reject, reject_early, reject_exploratory och strikt uncertain gör det inte.
- _next_step_produces_evaluation() identifierar om nästa steg kräver en ny
  evaluator-körning.
- reconcile_checkpoint() reparerar väntande deterministiska steg utan
  evaluatoranrop och är idempotent.
- _complete_campaign_if_finished() centraliserar terminalstatusen för normal
  körning, reconciliation och explicit stop.
- decision har företräde framför candidate_objective; objective används endast
  när beslut saknas.

### optimizer/src/goalaric_optimizer/optimization.py

- AutonomousOptimizer.run() kör reconcile_checkpoint() vid återstart.
- En bounded invocation som precis förbrukat sökbudgeten startar inte
  bekräftelsen i samma anrop; bekräftelsen fortsätter vid nästa anrop.

### Tester

optimizer/tests/test_phase20.py innehåller sju tester:

- fake-runner genom AutonomousOptimizer, max-results=1 och
  accept_exploratory, med nytt ankare före retur;
- alla accept-/reject-beslut;
- pending selection som repareras utan evaluatoranrop och är idempotent;
- avbrott efter sista atomiska kandidatresultatet och terminal reconciliation;
- bounded sista sökresultat som skjuter confirmation till nästa invocation;
- motsägande decision/objective, objective-fallback och reused-skydd.

optimizer/tests/test_confirmation.py förväntar nu kandidat p=2 och diff 2 i
livebekräftelsen, eftersom det nya korrekta ankaret används.

Dashboardens running/idle-presentation är inte ändrad.

## Historisk återhämtning

Originaldatabasen öppnades read-only och kopierades med SQLite backup() till
en temporär fil i /tmp. En evaluator som kastar fel vid varje anrop användes.

På kopian blev resultatet:

- före: revision 69, ankare 225, result_count=3;
- efter reconcile_checkpoint(): ankare 200, result_count=3, revision 72;
- inga nya matcher eller evaluatoranrop;
- andra återhämtningen gav samma state och checkpoint-hash;
- de 128 sparade partierna påverkades inte.

Slutsats: kampanjen kan återupptas säkert med den nya koden. Normal
optimizer optimize reparerar först checkpointen och fortsätter sedan med
nästa ej utvärderade kandidat. Originalet återupptogs inte.

## Verifiering

- fas 16 + fas 18 fake + fas 19 + fas 20: 15 passed
- fake-/SQLite-/dashboardsvit: 49 tester, 0 fel, 0 errors
- go test ./...: PASS
- go vet ./...: PASS
- compileall: PASS
- git diff --check: PASS

Real-processklasserna kördes inte i den slutliga sviten eftersom uppgiften
förbjuder Fastchess-, testmonitor- och motorstarter:

- ConfirmationMinimalRealTest
- Phase8RealIntegrationTest
- Phase9RealIntegrationTest
- Phase13MinimalRealOptimizationTest
- Phase18MinimalRealProfileTest

En tidig bred verifieringskörning råkade inkludera real-klasser på temporära
testkampanjer. De påverkade inte pilotdatabasen; efteråt fanns inga
realprocesser kvar. Alla slutliga körningar exkluderade klasserna ovan.

## Återuppta senare

Kör först:

    cd /home/peter/Projekt/GoAlaric-optimizer
    source optimizer/.venv/bin/activate
    git status --short
    git diff --check
    git diff -- optimizer/src/goalaric_optimizer/coordinate.py optimizer/src/goalaric_optimizer/optimization.py optimizer/tests/test_confirmation.py optimizer/tests/test_phase20.py

Säker verifiering:

    optimizer/.venv/bin/python -m unittest -q optimizer.tests.test_phase16 optimizer.tests.test_phase18.Phase18FakeProfileTest optimizer.tests.test_phase19 optimizer.tests.test_phase20
    go test ./...
    go vet ./...
    optimizer/.venv/bin/python -m compileall -q optimizer/src optimizer/tests
    git diff --check

Starta inte optimizer optimize mot pilotkampanjen, Fastchess, testmonitor,
commit, push eller tagg innan ändringarna är granskade och uttryckligen
godkända.

Nästa rimliga steg är att granska diffen och därefter committa fixen separat.
