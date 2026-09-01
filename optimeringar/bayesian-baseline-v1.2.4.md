# GoAlaric v1.2.4 – promoverad sökbaseline

Den fyrdimensionella Bayes-sökningen `search-hpo-4d-20260829` jämförde
96 kandidater och följdes av en fast bekräftelse mot den ursprungliga
baselinen. Bekräftelsen använde tidskontrollen `8+0.05`, parade öppningar,
Syzygy 3–5 pjäser och 6 000 partier.

Den bekräftade kandidaten är:

| Parameter | Gammal baseline | Ny baseline |
|---|---:|---:|
| `lmr_divisor_x100` | 225 | 225 |
| `lmp_move_multiplier` | 4 | 3 |
| `aspiration_initial_margin_cp` | 15 | 10 |
| `aspiration_min_depth` | 6 | 5 |

Kandidathash:
`aaafddd630c4cdefc5b44972f1b6680df01e2ad18ed397d63f8b9bfc7edabaa8`.

Bekräftelsen gav 1 452 vinster, 3 248 remier och 1 300 förluster. Score var
51,2667 procent med 95-procentsintervall 50,4104–52,1229 procent. Det
motsvarar cirka +8,8 Elo med 95-procentsintervall +2,9 till +14,8 Elo.
Utfallet var `confirmed`.

Den manuella promotionen ändrar motorns inbyggda defaults och de incheckade
standardregistren. Den innebär ingen ändring av sökalgoritmernas kod och
kampanjresultatet ska tolkas för parameterkombinationen som helhet.
