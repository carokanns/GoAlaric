# Aspirationsmarginal vid olika sökbudgetar

Experimentet jämför `aspiration_initial_margin_cp` 5, 10, 15, 20 och 30 mot
GoAlaric v1.2.4-baselinevärdet 10 vid 25 000, 100 000 och 400 000 noder per
drag. `aspiration_min_depth` ligger kvar på 5 och alla andra parametrar är
motorns inbyggda v1.2.4-värden.

Varje cell använder samma 64 parade öppningar, alltså 128 partier. Matrisen
omfattar 15 celler och totalt 1 920 partier. Cellen 10 mot 10 är en
självspelskontroll och räknas inte som en kandidat.

Profileringen är opt-in genom UCI-optionen `AspirationProfile`. Den vanliga
motorn har optionen avstängd. Testmonitor sparar slutdjup, fail-low, fail-high,
antal omsökningar, fullfönstersökningar samt deras nodkostnad i
`depth-profile.json` och `aspiration-profile.json` för varje cell.

## Förberedelse

Binärerna byggs i:

```text
artifacts/experiments/aspiration-budget-profile-20260902/bin/
```

Kontrollera planen utan att starta matcher:

```bash
bash optimeringar/aspiration-budget-profile-20260902/run-matrix.sh
```

När körningen senare har godkänts startas hela matrisen uttryckligen med:

```bash
bash optimeringar/aspiration-budget-profile-20260902/run-matrix.sh --execute
```

Skriptet kör en cell i taget. Inom varje cell kör Fastchess sex partier
parallellt. Avbrott görs med testmonitors ordinarie `stop`-kommando mot den
aktuella cellens `run-dir`; en redan färdig cell hoppas över vid återstart.

Efter körningen:

```bash
python3 optimeringar/aspiration-budget-profile-20260902/summarize.py
```

Ingen parameter promoveras automatiskt. Resultatet används endast för att
avgöra om aspirationsmarginalens bästa område flyttar sig med sökbudgeten.
