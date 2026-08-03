# Flerstegsplan för en starkare GoAlaric

## Sammanfattning

Förbättra spelstyrkan genom små, isolerade experiment i denna ordning:

1. Reproducerbara prestanda- och motormatcher.
2. Riskfria optimeringar i sökloopen.
3. Bättre dragordning och LMR.
4. Försiktigare trimning av pruning.
5. Datadriven evaluering och därefter incremental eval.

Varje experiment behålls endast om korrektheten är oförändrad och A/B-matcherna visar förbättring.

## Implementationsetapper

### 1. Mätbar baslinje

- Lägg till ett fastdjupsbenchmark över befintliga EPD-ställningar som rapporterar median för noder, tid, NPS, djup, score och bestmove över sju körningar.
- Lägg till ett Fastchess-baserat A/B-skript för basmotor mot kandidatmotor med parade öppningar och omvända färger.
- Kör båda motorerna med `Threads=1`, `Hash=128`, `Ponder=false` och samma tidskontroll.
- Använd minst 100 unika öppningar per 200-partierskörning, slumpa ur den
  installerade boken och spela varje öppning med omvända färger.
- Skriv nya resultat under en ignorerad `artifacts/bench/`-katalog; ändra inte historiska `movetime_results.txt` eller `perft_results.txt`.
- Dokumentera aktuell commit som baslinje innan första motorändringen.

### 2. Säkra sökoptimeringar

- Beräkna egenskaper som `givesCheck`, tactical, castling och pawn-push en gång per drag och återanvänd dem i extension, pruning och reduction.
- Undvik dubbla SEE/`NoSacrifice`-beräkningar mellan draggeneratorn och sökloopen genom att låta generatorn bära med sig klassificeringen.
- Ersätt modulo i nodpollningen med bitmask för intervallet 1024 och bevara gränspassering när SEE räknar flera noder.
- Profilera på nytt; acceptera endast semantikbevarande ändringar som ger minst 3 % bättre median-NPS och identiska fixed-depth-resultat.

### 3. Dragordning och LMR

Genomför och testa varje punkt separat:

- Gör history-värden djupberoende: bonus respektive malus baseras på `depth²`, begränsas till 400 och normaliseras till draggeneratorns 11-bitars score.
- Uppdatera alla tidigare sökta quiet-drag negativt vid ett quiet beta-cutoff och det vinnande draget positivt.
- Ersätt den grova LMR-regeln med en tabell baserad på djup och dragnummer. Reducera bara sena quiet-drag; minska reduktionen för PV, schack, killers och stark history.
- Använd först reducerad null-window-sökning, därefter full-depth null-window och slutligen full-window endast när resultaten kräver omsökning.
- Lägg till tester för history-mättnad, dragordning och att taktiska/schackgivande drag aldrig får otillåten reduktion.

### 4. Pruning

- Gör null-move-reduktionen djupberoende och behåll skydden för schack, mattscore och bondefattiga slutspel.
- Separera reverse futility, late-move och delta pruning i namngivna funktioner med dokumenterade marginaler.
- Lägg till tester för zugzwangliknande slutspel, schacknoder, promotions och mattsekvenser.
- Ändra endast en pruningregel per A/B-kandidat så att en regression kan spåras och reverteras isolerat.

### 5. Evaluering

- Reparera och förenkla den befintliga build-tag-baserade tunern så att den använder aktuella typer och kan köras deterministiskt.
- Skapa tränings- och valideringsdata från självspelade partier, med separata positioner per parti för att undvika dataläckage.
- Trimma de befintliga 78 parametrarna mot resultatdata innan nya evaltermer införs.
- Lägg endast till en evalfamilj åt gången, prioriterat kungssäkerhet, bondestruktur och pjäsaktivitet.
- När sök och eval är stabila: lagra material- och PST-deltan inkrementellt i board/undo-state, medan dynamiska termer fortsatt beräknas vid evaluering.

## Gränssnitt

- Motorns befintliga UCI-kommandon och options behålls.
- Nya skript får gränssnitten:
  - `run_search_bench.sh <engine> [repetitions]`
  - `run_match.sh <baseline> <candidate> <openings> [games]`
- Sök- och evalparametrar hålls som namngivna interna konstanter; inga nya UCI-options införs under denna roadmap.
- Fastchess används som separat testverktyg och blir inte ett Go-beroende.

## Test- och acceptansplan

För varje kandidat:

- `go test ./...`
- Perft depth 5 över samtliga sju testställningar med exakt förväntade nodtal.
- UCI-smoke för `uci`, `isready`, `position`, `go depth`, `go movetime`, `stop`, `ucinewgame` och `quit`.
- Fastdjupsbenchmark för illegala drag, tom `bestmove`, instabil score och prestandaförändring.
- 400 parade Fastchess-partier från 200 öppningar som screening; kandidater
  under 47 % poäng förkastas.
- Godkända kandidater går vidare till SPRT med `elo0=0`, `elo1=5`, `alpha=0.04`, `beta=0.20`, LLR-gränser kring −1,57/+3,00, högst 10 000 partier och tidskontroll `20+0.2`.
- Endast SPRT-godkända styrkeändringar införs. Inkonklusiva kandidater vid maxgränsen förkastas eller omarbetas.

## Antaganden

- Spelstyrka prioriteras framför enbart högre NPS.
- Fastchess installeras separat; det finns inte installerat i nuvarande miljö.
- Motormatcher körs enkeltrådat eftersom nuvarande `Threads`-option inte implementerar verklig parallell sökning.
- SMP, NNUE, öppningsbok och tablebases ligger utanför denna roadmap.
- Användarens orelaterade och ospårade filer, inklusive `AGENTS.md`, lämnas orörda.
