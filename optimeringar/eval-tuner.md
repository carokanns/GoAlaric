# Deterministisk eval-tuner

`cmd/evaltuner` bygger separata tränings- och valideringsdata från avslutade
Fastchess-partier och optimerar en liten parameterfamilj utan LLM-anrop.

Den första parameterfamiljen omfattar endast MG-/EG-straff för:

- isolerade bönder
- svaga bönder enligt befintlig `isWeak`
- dubbelbönder

Fribönder ingår inte. Deras värdering kräver fler samverkande egenskaper och
ska inte blandas in i den första parameterkontrollen.

## Dataset

Exempel:

```bash
go run ./cmd/evaltuner dataset \
  --pgn artifacts/matches/run-1/games.pgn \
  --pgn artifacts/matches/run-2/games.pgn \
  --seed 42 \
  --validation-percent 20 \
  --group-plies 16 \
  --minimum-ply 20 \
  --stride 8 \
  --max-per-game 10 \
  --max-games 2000 \
  --output-dir artifacts/evaltuner/pawn-structure-v1
```

Splitten görs med hash av startställningen och de första 16 halvdragen. Två
partier med samma öppning, inklusive ett färgvänt öppningspar, hamnar därför
alltid i samma partition. Identiska FEN-ställningar sparas bara en gång.

`manifest.json` innehåller konfiguration, antal partier/grupper/ställningar och
SHA-256 för både käll-PGN och genererade JSONL-filer. Datasetet innehåller ingen
tidsstämpel, så samma källor och argument ger identiskt innehåll.

## Tuning

```bash
go run ./cmd/evaltuner tune \
  --train artifacts/evaltuner/pawn-structure-v1/train.jsonl \
  --validation artifacts/evaltuner/pawn-structure-v1/validation.jsonl \
  --steps 4,2,1 \
  --passes 3 \
  --output artifacts/evaltuner/pawn-structure-v1/tune-result.json
```

Verktyget kalibrerar först den logistiska skalan mot oförändrad baseline-eval
och använder sedan deterministisk koordinatsökning. Endast träningsfelet får
styra parameterurvalet. Valideringsfelet rapporteras före och efter men påverkar
inte sökningen.

Ett bättre valideringsfel är bara ett underlag för en ny motorkandidat. Tunern
ändrar inte baseline och startar inga matcher.

## Kandidat 023

Två separata dataset kördes med korsvalidering. Den valda försiktiga
parametrisering är:

```json
{
  "isolated_mg": 15,
  "isolated_eg": 22,
  "weak_mg": 0,
  "weak_eg": 3,
  "doubled_mg": 0,
  "doubled_eg": 0
}
```

Den förbättrade valideringsförlusten är liten. Parametrarna ska därför
behandlas som en hypotes för kandidat 023 och avgöras av screening, inte som en
ny baseline.
