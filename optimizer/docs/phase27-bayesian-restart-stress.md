# Bayesian restart- och budgetstress

Senast verifierad: 2026-08-29.

## Omfattning

Delmål F verifierar den autonoma Bayes-kedjan efter den minimala verkliga
smoken. Testet använder en snabb SQLite-backed matchrunner; inga riktiga
motor-, testmonitor- eller Fastchessprocesser startas. Den verkliga
profiltransporten är separat verifierad i fas 26.

Den reproducerbara körningen är
`Phase27BayesianRestartStressTest.test_fifty_process_deaths_and_confirmation_restarts_are_exact`.
Allt körs mot en enda temporär SQLite-databas med:

- fyra parametrar och en sökrymd på 625 kombinationer;
- 24 Bayes-kandidater;
- sex kompletta öppningspar per kandidat;
- exakt 288 sökpartier;
- 50 avsiktliga `SIGKILL` efter att ett matchblock atomiskt tagits i anspråk;
- en separat fast bekräftelse på 20 öppningspar och 40 partier;
- en återstart per bekräftelsepar samt terminala idempotensåterstarter;
- dashboard och `status --watch` under den aktiva sökningen.

## Resultat

Grinden passerade med följande kontroller:

- 24 unika förslag, parameterhashar och observationer;
- 144 unika, kompletta sökblock och 288 unika spelplatser;
- summan av blockförsök var exakt 144 lyckade försök plus 50 dödade försök;
- minst 50 `abandoned_job_recovered`-händelser sparades;
- checkpointen innehöll `result_count=24`, `consumed_games=288` och
  `phase=completed`;
- bekräftelsen innehöll 20 unika block och 40 unika partier;
- slututfallet var `inconclusive`, utan rekommendation eller automatisk
  promotion;
- dashboarden var skrivskyddad, visade `bayesian_checkpoint_candidate` och
  summerade 288 sökpartier, 40 bekräftelsepartier och 328 totalpartier;
- upprepade terminala återstarter ändrade inga förslag, observationer, block,
  partier eller försök;
- dashboardpollning efter avslut ändrade inte databasfilens mtime;
- inga körande sök- eller bekräftelseblock blev kvar.

## Fel som stressen upptäckte

Den första körningen stoppade på ett legitimt parresultat vars exakta score
var periodisk i decimalform. Fixed-pair-lagret returnerade den adaptiva
rapportens avrundade procentscore, medan Bayes-lagret verifierade den mot
parpoängen med strikt tolerans.

Korrigeringen beräknar därför observationens score direkt från de sparade
parpoängen. Det korta regressionstestet
`Phase25BayesianFixedPairTransportTest.test_fractional_fixed_pair_score_is_transported_without_rounding`
låser fallet `pair_points=[2.0, 1.5, 0.0]`, alltså exakt `7/12`.

## Grind

Delmål F är godkänt när detta test, hela Python-sviten, Go-testerna,
`go vet`, `compileall`, `git diff --check` och processaudit är gröna. Detta
underlag innebär ingen parameterrekommendation och ingen merge till `master`.
