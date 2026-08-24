# Dynamisk LMP på återstående djup 4

Verifierad 2026-08-24 innan någon matchkampanj startades.

## Variant

- baseline: `lmp_depth4_min_iteration=0`, historiskt LMP med maxdjup 3
- kandidat: `lmp_depth4_min_iteration=12`
- oförändrad `lmp_move_multiplier=4`
- kandidaten tillåter LMP på återstående djup 4 från iterativt sökdjup 12
- återstående djup 5 och högre är fortsatt skyddade

Vid djup 4 börjar gallringen först när 16 drag redan har sökts. Alla tidigare
skydd för PV-noder, farliga drag och mattvärden finns kvar.

## Ställningar och metod

`fullGP.epd` användes som diagnostisk ställningssamling, inte som öppningsbok.
Av 25 230 ställningar med minst 18 pjäser valdes deterministiskt var 126:e
ställning tills 200 ställningar hade erhållits. Båda varianterna kördes med
samma binär, Hash 128 MB, en tråd, fast djup 12 och en körning per ställning.

Testet kan reproduceras med:

```bash
scripts/run_lmp_depth_probe.sh /home/peter/Projekt/GoAlaric/fullGP.epd
```

## Resultat

| Mått | Baseline | Kandidat |
|---|---:|---:|
| Totala noder | 66 254 716 | 63 765 179 |
| Nettoskillnad |  | −2 489 537 (−3,76 %) |

- färre noder i 147 av 200 ställningar
- fler noder i 53 av 200 ställningar
- medianförändring per ställning: −2,25 %
- samma bästa drag i 192 av 200 ställningar
- samma slutscore i 130 av 200 ställningar

Som kontroll kördes de första 20 ställningarna på djup 11. Nodantal, score och
bästa drag var identiska i samtliga 20 fall. Kandidaten påverkar alltså inte
sökningen innan aktiveringsdjupet.

Nodminskningen visar att ändringen har en mätbar men måttlig effekt på trädet.
Testet mäter inte spelstyrka; det avgörs först av en parad matchkampanj med
`8moves_v3.pgn`.
