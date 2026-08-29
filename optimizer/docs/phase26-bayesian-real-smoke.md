# Minimal verklig Bayesian smoke-pilot

Senast verifierad: 2026-08-29.

## Låsta komponenter

Piloten byggdes från commit `0ca92f8` (`Connect Bayesian search to fixed pair matches`).

| Komponent | SHA-256 |
|---|---|
| GoAlaric | `b61d0895f954840dcde6daa8810ff1c274a06dc11a069a2f16484dc86979d205` |
| testmonitor | `5a0a2c48def88c3870c66a76dd94d4640f482ca41cd5e26a1d7ce1b2a5980cc8` |
| Fastchess | `c140bfbed76c8ac08d0401f607ad87060f8fc538915a75709029088437407701` |
| `8moves_v3.pgn` | `5835239f88cc2c7511b177c32392a69f3ede21819cf0616f80a7f907cd21d17e` |

Kampanj-ID var `bayesian-real-smoke-20260829`. Register var
`search-hpo-v1`; den optimerade delrymden bestod av
`lmr_divisor_x100` och `lmp_move_multiplier`.

## Upplägg och resultat

- profil: `node-smoke`, faktiskt `20000 nodes/move`;
- två kandidater;
- två kompletta öppningspar per kandidat;
- exakt åtta verkliga partier;
- ursprunglig baseline: `(225, 4)`, inlagd som exakt 50 %-prior utan
  baseline-självmatch;
- återstart mellan kandidat ett och kandidat två;
- en terminal återstart efteråt för idempotenskontroll.

| Kandidat | W–D–L | Parpoäng | Score |
|---|---:|---:|---:|
| `(175, 3)` | 1–3–0 | `[1.5, 1.0]` | 62,5 % |
| `(175, 5)` | 0–2–2 | `[0.5, 0.5]` | 25,0 % |

Volymen är endast en transportkontroll. Resultaten får inte användas som
styrkebevis, rekommendation eller promotion.

## Integritetskontroll

Read-only-kontroll av SQLite och matchartefakter visade:

- två unika förslag och två observationer;
- fyra unika matchblock och åtta unika spelplatser;
- alla block färdiga på första försöket;
- inga `running`-block och frigjord owner-token;
- checkpoint revision 8, `phase=completed`, `result_count=2` och
  `consumed_games=8`;
- samtliga fyra `monitor-config.json` innehöll `nodes=20000`, två partier
  och ingen tidskontroll;
- baseline- och kandidatfilerna hade olika parameter-SHA i varje block;
- terminal återstart behöll revision 8, fyra block, åtta partier och två
  observationer;
- inga testmonitor-, Fastchess- eller GoAlaricprocesser blev kvar.

Det reproducerbara regressionstestet är
`Phase26MinimalRealBayesianTest`. Det bygger färska binärer, kör samma
tvåstegsflöde i en temporär katalog och kontrollerar profiltransport,
budget, unikhet och terminal idempotens.

## Slutkandidat och bekräftelse

Bayes-checkpointen sparar den högst skattade kandidaten inklusive fullständig
parameterfil, hash och punktresultat. Exakt baseline ingår som en 50 %-prior
och vinner ett lika resultat. Parametrar som inte ingår i den valda delrymden
hämtas från kampanjens verkliga baseline, inte från registerstandarderna.

Dashboard och slutrapport läser denna checkpoint som
`bayesian_checkpoint_candidate`. När fast bekräftelse är aktiverad jämförs
den kandidaten med ursprunglig baseline genom den befintliga strikta
bekräftelsekedjan. En begränsad invocation som precis förbrukat sitt sista
sökresultat startar inte bekräftelsen; nästa invocation kan köra exakt det
antal bekräftelseblock som återstår av dess kvot.

`Phase26BayesianCompletionTest` verifierar med fake-runner att:

- sista sökresultatet och slutkandidaten är checkpointade före retur;
- bekräftelsen skjuts upp över sökgränsen och återupptas ett öppningspar i
  taget utan dubbelräkning;
- dashboard växlar till `confirming` och visar rätt Bayes-kandidat;
- kompakt slutrapport delar upp sök-, bekräftelse- och totalpartier;
- `inconclusive` inte skapar någon rekommendation eller promotion;
- terminal återstart är idempotent.

## Grind

Den minimala verkliga kedjan och dess slutbehandling är godkända. Nästa
delmål är den längre stress-/budgetverifieringen (F), som inte startas här.
