# Experiment: återanvänd `IsCheck` per drag

## Kandidat

Kandidaten beräknar om ett drag ger schack högst en gång i respektive
sökiteration och återanvänder resultatet i pruning, extensions och LMR.
Fixed-depth-sökningen är semantiskt oförändrad.

## Resultat före match

- Baseline: commit `933f163`, SHA-256 dokumenterad i `baseline-933f163.md`.
- Baseline depth-8 median: 746 130 NPS; upprepad körning 792 000 NPS.
- Kandidat depth-8 median: 918 720 NPS.
- Förbättring: 16,0–23,1 procent beroende på baseline-körning.
- Samtliga 14 fixed-depth-positioner gav identiska noder, score och bestmove.
- `go test ./...`, perft depth 5, movetime och Fastchess UCI-compliance passerade.

## Kasserad screening

En första 200-partierskörning gav 59 vinster, 101 remier och 40 förluster,
54,7 procent och en uppskattning på +33,11 ± 33,98 Elo. Resultatet används
inte som acceptansbevis eftersom sviten bara innehöll 20 öppningar och därför
återanvändes för ofta.

## Ny matchkonfiguration

- Bok: `8moves_v3.pgn` från `official-stockfish/books`, CC0.
- 34 700 unika åttadragssekvenser; monitoreringen kräver minst 100.
- Fastchess blandar hela boken med en sparad slumpseed, väljer utan
  återläggning och spelar varje öppning två gånger med omvända färger.
- Standardtid: `30+0.3`, `Threads=1`, `Hash=128`, `Ponder=false`.
- Ny screening: 400 partier från 200 slumpade, färgväxlade öppningar.
- Godkänd screening följs av separat SPRT 0/5 Elo, högst 10 000 partier.
- Ingen ny match startas innan användaren uttryckligen ger klartecken.

## Giltig screening

Körningen `artifacts/matches/20260801-181323` avslutades normalt efter 400
partier med tidskontroll `30+0.3`:

- Kandidatens resultat: 93 vinster, 218 remier och 89 förluster, 50,5 procent.
- Fastchess: +3,47 ±23,00 Elo och LOS 61,66 procent.
- 200 unika öppningar användes exakt två gånger vardera; samtliga öppningslinjer
  var 16 halvdrag långa.
- 398 unika fullständiga partiförlopp; två färgväxlade par blev identiska.
- Inga illegala drag, krascher, timeoutfel eller andra motorfel rapporterades.

Screeningen passerar 47-procentsgränsen men styrkeskillnaden är statistiskt
osäker. Kandidaten måste därför fortfarande godkännas av den beslutande
SPRT-körningen innan ändringen får behållas.

Den beslutande SPRT-körningen startades men avbröts på användarens begäran
efter 624 färdiga partier: 106 vinster, 137 förluster och 381 remier,
47,5 procent och LLR −1,24. Den var därmed inte H1-godkänd. Kandidaten är
förkastad och ändringen har tagits bort från `search/search.go`.
