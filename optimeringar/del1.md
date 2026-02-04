# Optimeringar – Del 1

## Föreslagna optimeringar (spelande delen, inte perft)
1. Fixa eval‑hashen så cache faktiskt skrivs tillbaka.
2. Root‑ordning: undvik full `Qs` per drag, använd lättviktssortering.
3. Minska `eval.IsCheck`‑anrop i sökloopen (kalla bara när draget inte redan är “dangerous”).
4. Begränsa SEE/`NoSacrifice`‑kostnad (mer selektivt, undvik för quiets).
5. Optimera `incNode`/poll (bitmask i stället för `%`, färre GUI‑uppdateringar).
6. Större steg: incremental eval och incremental attack/pin‑info.

## Summering + status på optimeringar

### Genomförd (på `optimering1`)
- **Eval‑hash‑fix**: cache skrivs tillbaka i `eval/eval.go`.
- **Ändring i `run_movetime_epd.sh`**: skriver nu `avg`‑rad med `n/s=...` automatiskt.
- **Städning av `.gitignore`** (dubbletter bort).
- `optimering1` är pushad.

### Testad men reverterad
- **Root‑ordning/Qs‑optimering** (lättviktssortering i root). Du valde att backa den.

### Testad tillfälligt men borttagen
- Räknare för `ischeck_calls` / `ischeck_skipped` togs bort igen.

### Ej genomförda ännu
- Minska `eval.IsCheck`‑anrop i sökloopen (den enkla varianten).
- Begränsa SEE/`NoSacrifice`.
- `incNode`/poll‑optimering.
- Incremental eval / incremental attack‑info.
