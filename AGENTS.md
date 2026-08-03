# AGENTS.md

Syftet med den här filen är att hjälpa kodagenter att förbättra GoAlaric utan att tappa motorstyrka, korrekthet eller mätbarhet.

## Projektmål

GoAlaric är en UCI-schackmotor i Go. Viktigast är:

1. Korrekt schacklogik: legala drag, rockad, en passant, schack, matt, patt och perft ska stämma.
2. Stabil UCI: motorn ska fungera i GUI och svara korrekt på `uci`, `isready`, `position`, `go`, `stop`, `quit` och projektets extra kommandon.
3. Mätbar spelstyrka och prestanda: förbättringar ska kunna visas med tester, perft, movetime-resultat eller profiler.

## Vad förbättring betyder

Att förbättra GoAlaric betyder i första hand något av detta:

1. Snabbare program med samma resultat. Exempel: färre allokeringar, snabbare draggenerering, effektivare hashning eller billigare interna kontroller utan ändrad schacklogik.
2. Smartare sökalgoritm som ger djupare eller bättre sökning på samma tid. Exempel: bättre dragordning, pruning, extensions, transpositionstabell, tidsstyrning eller quiescence search.
3. Bättre evaluering av ställningen. Exempel: bättre pjäsaktivitet, kungssäkerhet, bondestruktur, slutspel, mobilitet eller parametertrimning.

Sök och evaluering påverkar varandra. Det viktiga slutmålet är inte bara en snyggare implementation, utan ett bättre schackprogram: korrekt, stabilt, snabbare när det ska vara snabbare och starkare när spelstyrkan mäts.

## Arbetsregler för agenter

- Läs befintlig kod innan du ändrar. Följ lokala mönster i paket som `board`, `gen`, `search`, `eval`, `uci`, `move`, `material`, `bit`, `hash` och `square`.
- Gör små, begripliga ändringar. Undvik stora omskrivningar om de inte direkt behövs för uppgiften.
- Ändra inte testdata, resultatfiler eller gamla profiler för att få en ändring att se bättre ut.
- Bevara användarens orelaterade ändringar. Revertera aldrig kod som du inte själv har ändrat om inte användaren uttryckligen ber om det.
- Prioritera korrekthet före hastighet. En snabbare motor som spelar illegala drag eller bryter perft är en regression.
- När du optimerar: mät före och efter. Skriv ner kommando, djup/tid, testfil och relevanta resultat.
- Håll kommentarer korta och praktiska. Kommentera komplicerad schack- eller söklogik, inte uppenbara tilldelningar.

## Vanliga kommandon

Kör alltid minst unit-tester efter kodändringar:

```bash
go test ./...
```

Kör hela lokala testkedjan när ändringen kan påverka draggenerering, sök, eval, UCI eller tidsstyrning:

```bash
./scripts/run_all_tests.sh
```

Perft separat:

```bash
./scripts/run_perft.sh 4 scripts/perft_tests.txt perft_results.txt
```

Movetime-regression separat:

```bash
./scripts/run_movetime_epd.sh 2000 scripts/movetime_epd movetime_results.txt
```

Bygg motorn:

```bash
go build -o bin/goalaric ./GoAlaric.go
```

## Frikopplad kandidatkampanj

När användaren uttryckligen har godkänt en färdig, committad och pushad
kandidat startas den återstående testkedjan med
`scripts/start_candidate_campaign.sh`. Vänta på kvittensen
`Campaign service started` innan användaren avslutar Codex CLI. Starta aldrig
en andra kampanj parallellt.

Den transienta `systemd --user`-servicen bygger kandidaten, kör den
deterministiska pipelinen, startar screening och följer eventuell SPRT utan
återkommande modell-anrop. När användaren återupptar Codex CLI, kör
`scripts/campaign_status.sh` och utvärdera terminalt resultat manuellt. Baseline
och nästa kodändring kräver fortsatt uttryckligt godkännande.

## Korrekthetskrav

- Ändringar i `board`, `gen`, `move`, `material`, `square` eller `bit` ska normalt verifieras med `go test ./...` och perft.
- Ändringar i `search`, `eval`, `hash`, `parms` eller tidsstyrning ska normalt verifieras med `go test ./...` och minst en movetime-körning.
- Ändringar i `uci` ska testas med unit-tester och, om möjligt, genom att starta motorn och skicka enkla UCI-kommandon.
- Om en test inte kan köras, säg tydligt vilken test som saknas och varför.

## Prestanda och optimering

När du förbättrar spelstyrka eller hastighet:

- Utgå från dokumenterade idéer i `optimeringar/` när de är relevanta.
- Jämför mot tidigare resultat i `movetime_results.txt`, `perft_results.txt` eller nya lokala körningar.
- Var försiktig med global state i söket. Kontrollera att `ucinewgame`, ny position och ny sökning inte ärver fel state.
- Profiler och genererade resultat får användas som beslutsunderlag, men ska inte blandas ihop med källkod.

## Kodstil

- Projektet använder Go-modulen `goalaric` och Go 1.22.2.
- Kör `gofmt` på Go-filer du ändrar.
- Behåll paketgränserna tydliga:
  - `board`: ställning och spelstate.
  - `gen`: draggenerering och dragordning.
  - `search`: sök, perft, transpositioner och tidsstyrning.
  - `eval`: utvärdering, attacker och bonde-/pjästabeller.
  - `uci`: textprotokoll mot GUI.
- Undvik nya externa beroenden om de inte är tydligt motiverade.

## Saker att vara extra vaksam på

- Schackregler: rockadrätt, en passant, promotions, pinned pieces och schackdetektion.
- Tidsstyrning: `go movetime`, infinite search, `stop` och bestmove-flöde.
- Transpositionstabell och eval-cache: felaktig återanvändning kan ge subtila styrkefel.
- Resultatfiler: `perft_results.txt` och `movetime_results.txt` är mätloggar, inte facit som ska handredigeras för kodändringar.

## När du är klar

Rapportera kort:

- Vad som ändrades.
- Vilka tester eller mätningar som kördes.
- Eventuella kvarvarande risker eller nästa rimliga steg.
