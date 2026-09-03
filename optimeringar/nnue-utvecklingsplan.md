# Framtida utvecklingsplan för NNUE i GoAlaric

Senast uppdaterad: 2026-09-03.

## Status och avgränsning

NNUE är en möjlig framtida utvecklingsriktning, men är inte aktuell att
implementera nu. Dokumentet sparar de viktigaste tekniska besluten och en
ordning för ett eventuellt senare arbete.

Ingen befintlig evaluering ska tas bort innan en NNUE-kandidat har visat sig
korrekt, tillräckligt snabb och starkare i reproducerbara matcher. Den
klassiska evalueringen ska finnas kvar som referens och reservväg under hela
utvecklingen.

## Varför NNUE är intressant

GoAlarics klassiska evaluering är handskriven och innehåller många användbara
schackkunskaper. Fortsatt trimning av enskilda parametrar har hittills gett
små eller osäkra förbättringar. Ett NNUE-nät skulle kunna lära sig kombinationer
av positionella egenskaper som är svåra att uttrycka som oberoende termer.

NNUE lämpar sig för alfa-beta-motorer eftersom nätets första lager kan
uppdateras inkrementellt när ett drag endast ändrar ett fåtal pjäser. Nätet
kan därför ge en rikare evaluering utan kostnaden för ett vanligt stort
neuralt nät vid varje lövnod.

## Förutsättningar i GoAlaric

Den nuvarande motorn har flera egenskaper som underlättar en integration:

- bitboards och direkt åtkomst till innehållet på varje ruta;
- centraliserad hantering av att sätta, ta bort och flytta pjäser;
- make/undo-stack för sökningen;
- en tydlig anropspunkt för statisk evaluering;
- lokalt söktillstånd som kan utökas med en ackumulator per arbetstråd.

Följande dragtyper måste hanteras uttryckligen i NNUE-uppdateringen:

- vanliga drag;
- slag;
- en passant;
- promotion;
- rockad;
- kungsdrag som ändrar nätets perspektiv eller feature-bucket;
- null move;
- undo av samtliga ovanstående.

## Den största risken: nät och träningsdata

Inferenskoden är inte den svåraste delen. Det verkliga projektet är att få
fram ett kompatibelt nät med tillräckligt bra och representativa träningsdata.

Ett nät bör tränas på miljontals varierade och deduplicerade ställningar. Data
från endast GoAlarics egna matcher riskerar att återge motorns befintliga
svagheter och en begränsad fördelning av öppningar och ställningstyper.

En lämplig första väg är därför:

1. Samla ställningar från varierade PGN-källor och GoAlarics självspel.
2. Ta endast ett begränsat antal ställningar per parti för att minska starkt
   beroende mellan närliggande positioner.
3. Deduplicera med positionshash och behåll hela partier åtskilda mellan
   träning och validering.
4. Etikettera ställningarna med en stark referensmotor vid en fast och
   reproducerbar nodbudget.
5. Spara både sökvärde och partiets slutresultat när detta är känt.
6. Träna med en kontrollerad blandning av evalmål och partimål.
7. Kvantisera nätet och verifiera att heltalsinferensen ger samma resultat i
   träningsverktyget och GoAlaric.

Träningsmaterialet bör filtrera eller särskilt märka:

- dubbletter och nästan identiska ställningar från samma parti;
- mattvärden och extrema taktiskt instabila positioner;
- korrupta eller olagliga positioner;
- bokställningar som är kraftigt överrepresenterade;
- enkla tabellslutspel som Syzygy redan löser;
- ställningar där etiketten inte är stabil vid en något större sökbudget.

Datasetet ska innehålla öppning, mittspel och slutspel samt både jämna och
obalanserade ställningar. Testdata ska delas per helt parti, inte genom att
slumpa enskilda positioner, för att undvika läckage mellan träning och test.

## Alternativ för det första nätet

### Befintligt nät

Ett befintligt nät ger den kortaste vägen till ett inferenstest, men kräver
exakt samma features, arkitektur, kvantisering och filformat som nätet
förväntar sig. Ett Stockfish-nät är inte en allmän svart låda som kan kopplas
direkt till en annan feature-transformering.

Moderna Stockfish-nät och deras arkitektur förändras dessutom över tid. En
implementation ska därför låsa ett uttryckligt nätformat och en uttrycklig
arkitekturversion i stället för att försöka läsa godtyckliga `.nnue`-filer.

### Eget pilotnät

Ett mindre eget nät är enklare att förstå, testa och optimera i ren Go. Det är
den rekommenderade första produktionsoberoende prototypen. Syftet med det
första nätet är att bevisa hela kedjan, inte omedelbart maximal spelstyrka.

### Referensmotoretiketter

Ett eget nät kan först tränas för att approximera djupa evalvärden från en
stark referensmotor. Senare kan målfunktionen förbättras med självspel och
partiresultat. Det minskar kravet på att GoAlaric redan ska kunna skapa alla
bra etiketter själv.

## Licensgräns

GoAlaric är MIT-licensierat. Stockfishs programkod är GPLv3 och ska inte
kopieras eller direkt portas till GoAlaric utan ett medvetet beslut om
licenskonsekvenserna.

Officiella Stockfish-nät publiceras separat under CC0, men nätets licens löser
inte kodfrågan. Om ett sådant nät används måste den kompatibla inferensen
implementeras självständigt eller hämtas från en källa vars licens är
kompatibel med projektet. Licens och ursprung ska dokumenteras för både kod,
nät och träningsdata.

Referenser:

- <https://github.com/official-stockfish/networks>
- <https://github.com/official-stockfish/nnue-pytorch>
- <https://github.com/official-stockfish/Stockfish/tree/master/src/nnue>

## Föreslagen implementation

### Delmål A: lås specifikationen

- Välj en liten och dokumenterad feature-uppsättning och nätarkitektur.
- Definiera filformat, dimensionsstorlekar, heltalstyper och kvantisering.
- Definiera poängskalan i relation till GoAlarics centipawnvärden.
- Bestäm hur nätets identitet och SHA-256 ska rapporteras.
- Skapa en separat utvecklingsgren; `master` ska förbli användbar.

### Delmål B: träningskedja i liten skala

- Extrahera ett litet reproducerbart dataset.
- Etikettera det vid fast nodbudget.
- Träna ett litet pilotnät.
- Exportera ett versionsmärkt och hashat nät.
- Kontrollera inferensresultat mot fasta testvektorer.

Detta delmål ska bevisa:

```text
PGN -> positioner -> etiketter -> träning -> kvantisering
    -> nätfil -> referensinferens
```

### Delmål C: full omräkning i GoAlaric

- Lägg inferensen i ett separat `nnue`-paket.
- Läs nätfilen med strikt dimensions-, versions- och hashkontroll.
- Bygg alla aktiva features från det aktuella brädet vid varje evaluering.
- Inför UCI-alternativ motsvarande `UseNNUE` och `EvalFile`.
- Behåll klassisk eval som standard tills försöket är verifierat.

Full omräkning blir sannolikt för långsam för slutlig användning, men ger ett
enkelt korrekthetsfacit för nästa delmål.

### Delmål D: inkrementell ackumulator

- Lägg en NNUE-ackumulator per sökply och per sökarbetare.
- Uppdatera den från dragets exakta pjäsförändringar.
- Gör full refresh när kungsberoende features kräver det.
- Verifiera varje inkrementellt resultat mot full omräkning.
- Undvik beroendecykel mellan `board`, `eval` och `nnue`; brädet bör exponera
  neutrala dragdeltan i stället för att känna till nätimplementationen.

Ackumulatorn ska vara arbetarlokal från början så att den senare passar i
den planerade Lazy SMP-implementationen.

### Delmål E: sökintegration

- Välj NNUE eller klassisk eval vid den befintliga statiska evalpunkten.
- Bevara mattvärden, Syzygy-resultat och sidperspektiv.
- Kontrollera eval-cache och transpositionstabell vid byte av nät.
- Rensa relevanta cacher när `EvalFile` eller evaltyp ändras.
- Förbjud nätbyte under aktiv sökning.
- Rapportera aktiv evaltyp, nätfil och näthash via UCI-information.

Sökparametrar som bygger på evalvärden kan behöva trimmas om NNUE:s
fördelning skiljer sig från den klassiska evalueringen. Detta gäller bland
annat pruninggränser, aspiration windows och contempt. De ska inte ändras i
samma första kandidat utan mätas separat.

### Delmål F: korrekthet och prestanda

Obligatoriska tester:

- fasta inferenstestvektorer;
- full omräkning mot inkrementell uppdatering;
- långa slumpmässiga make/undo-sekvenser;
- slag, en passant, promotion och rockad;
- kungsdrag och feature-refresh;
- null move och undo;
- byte mellan klassisk eval och NNUE mellan sökningar;
- felaktig, trunkerad eller inkompatibel nätfil;
- determinism vid `Threads=1`;
- `go test ./...`, full testsuite, perft och movetime;
- race-test när flertrådad sökning senare införs.

Mät minst:

- nanosekunder per full och inkrementell evaluering;
- allokeringar per evaluering;
- noder per sekund;
- uppnått sökdjup vid samma tid;
- skillnad i nodantal vid fast djup;
- andel accumulator-refreshes.

Pure Go-inferens ska mätas innan eventuell SIMD- eller assembleroptimering.
Optimeringen ska göras först när profiler visar den verkliga flaskhalsen.

### Delmål G: styrkeverifiering

1. Fasta taktiska och positionella tester.
2. Kort smoke-match mot klassisk eval.
3. Reproducerbar screening med parade öppningar.
4. Längre oberoende bekräftelsematch.
5. Separat kontroll vid mer realistisk tidskontroll.

Ingen automatisk promotion ska ske. NNUE blir standard först efter tydlig
matchfördel, acceptabel hastighet och godkänd korrekthet. Vid oavgjort eller
osäkert resultat behålls klassisk eval som standard.

## Förhållande till flertrådad sökning

NNUE och Lazy SMP är var för sig stora förändringar och ska inte utvecklas i
samma kandidat. NNUE bör först verifieras med `Threads=1`.

Trots detta ska designen från början anta:

- en delad, oföränderlig nätmodell;
- en separat ackumulatorstack per sökarbetare;
- inga globala muterbara inferensbuffertar;
- säker nätlivstid under hela sökningen;
- summerbar statistik utan lås i den heta evalvägen.

När både NNUE och flertrådad sökning är godkända separat kan de kombineras och
race-, skalnings- och styrketestas på nytt.

## Lagring och reproducerbarhet

Följande ska följa varje tränat nät:

- arkitekturversion;
- nätets SHA-256;
- datasetversion och urvalsregler;
- referensmotor och exakt commit/binärhash;
- etiketteringsbudget och UCI-inställningar;
- träningskodens commit;
- seed, hyperparametrar och epoker;
- kvantiseringsparametrar;
- valideringsresultat;
- matchresultat mot klassisk eval;
- licens och datakällor.

Stora dataset och tillfälliga träningsartefakter ska normalt inte lagras i
Git. Manifest, små testvektorer, nätidentitet och reproduktionskommandon ska
däremot versionshanteras.

## Beslutsgrind innan arbetet återupptas

NNUE-projektet bör startas först när följande resurser finns:

- tid och diskutrymme för att skapa och etikettera ett större dataset;
- en vald och licensmässigt säker nätarkitektur;
- en plan för Pythonbaserad träning och Go-baserad inferens;
- en separat utvecklingsgren;
- en låst klassisk baseline;
- möjlighet att genomföra längre, parade bekräftelsematcher.

Det första konkreta arbetet ska vara en liten tränings- och
referensinferenspipeline. Ingen ändring av GoAlarics standardevaluering ska
göras innan den kedjan producerar ett läsbart, hashat nät med reproducerbara
testvektorer.
