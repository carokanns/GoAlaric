# Utvecklingsplan: flertrådad sökning i GoAlaric

Senast utarbetad: 2026-09-03.

## Syfte och beslut

Målet är att införa verkligt stöd för UCI-optionen `Threads` utan att försämra
enkärnig korrekthet, stabilitet eller spelstyrka. Första implementationen ska
använda **Lazy SMP** med en fast pool av långlivade workers. Young Brothers
Wait Concept (YBWC) och andra split-point-algoritmer sparas som möjliga senare
förbättringar.

Arbetet delas i två oberoende huvudändringar:

1. en trådsäker, Crafty-inspirerad transpositionstabell;
2. worker-isolering och Lazy SMP.

De får inte införas i samma första commit. Varje del ska kunna testas, mätas,
förkastas eller återställas separat. `master` och v1.2.4 är referens tills hela
flertrådsarbetet är godkänt och manuellt mergat.

## Utgångsläge

Referens vid planens tillkomst:

- version: v1.2.4;
- commit: `69f8d7c`;
- inbyggd sökbaseline: LMR 225, LMP-multiplikator 3,
  aspirationsmarginal 10 cp och aspirationsstartdjup 5;
- `Threads` visas redan som UCI-option med intervallet 1–16;
- `Engine.Threads` har standardvärdet 1;
- `slEntries`, generatorscratch och vissa andra arrayer har plats för 16
  workers;
- hjälptrådarna i `searchGo()` är bortkommenterade och `Threads > 1` startar
  därför ännu ingen parallell sökning;
- den kvarvarande `splitPoint`/`rootSP`-koden är en ofullständig äldre stomme
  och ska inte betraktas som färdig YBWC;
- transpositionstabellen, `SG.History`, `Best`, `current`, `limit` och delar av
  rotlogiken innehåller muterbar global state som inte får användas samtidigt
  av flera workers.

Syzygy-räknaren är atomär och Fathoms vanliga WDL-probe har trådsäker
inramning. Trådsäkerheten ska ändå verifieras under verklig parallell sökning;
root-probe och byte av tabellväg ska endast ske utanför aktiv sökning.

## Varför Lazy SMP väljs först

Alfa–beta är starkt ordningsberoende. Första draget i en nod sätter normalt
gränsen som gör senare drag billiga. Att skapa en goroutine vid varje nod eller
omedelbart dela alla syskondrag skulle därför orsaka:

- mycket redundant sökning innan alfa har stabiliserats;
- goroutine-, kanal- och synkroniseringskostnad i den hetaste koden;
- allokeringar och onödigt arbete för Go:s garbage collector;
- svåra cutoff-, PV-, stopp- och livscykelproblem.

Lazy SMP använder i stället ett litet fast antal workers. Varje worker gör en
full iterativ sökning från samma rot. Workers hjälper varandra indirekt genom
den gemensamma transpositionstabellen. Modellen accepterar en viss mängd
dubbelarbete för att slippa synkronisering vid varje nod.

YBWC kan ge mer kontrollerad parallellism genom att söka det äldsta barnet
först och därefter dela yngre syskon. Det är ett möjligt steg först om Lazy SMP
visar otillräcklig skalning och profilering visar att dubbelarbetet är den
dominerande orsaken.

## Målarkitektur

### Worker-lokal state

Varje worker ska äga minst:

- en egen kopia av rotställningen och hela reversibla sökhistoriken;
- `Local`, sökstack, PV och senast färdigställda iteration;
- killers, history och annan dragordningsstate;
- pawn- och eval-cache;
- generatorscratch;
- lokal nodräknare, selektivt djup och tabellbasträffar;
- worker-ID och eventuell deterministisk diversifieringspolicy.

Worker-lokala datastrukturer ska förallokeras. Sökningens normala nodväg ska
inte skapa goroutines, channels, closures eller heapallokeringar.

### Gemensam state

Endast följande ska delas under sökning:

- transpositionstabellen;
- atomär stoppsignal och sökgeneration;
- skrivskyddade sökparametrar och UCI-gränser;
- en central tidsstyrning som ägs av huvudworkern;
- Syzygy-tabellerna;
- publicerade, atomära workersammanfattningar för noder och status.

Ändringar av `Hash`, `Threads`, `ParameterFile`, Contempt eller SyzygyPath ska
fortsatt vara förbjudna medan en sökning är aktiv.

### Huvudworker

Worker 0 ska ensam:

- styra tid och hård stopptid;
- skriva UCI-`info`;
- besluta när hela sökningen ska stoppas;
- invänta alla helpers;
- skriva exakt ett `bestmove`.

Helpers får aldrig skriva direkt till UCI.

## Delmål A: reproducerbar enkärnig referens

Skapa en ren `development`-gren från den då aktuella, godkända `master`.
Innan kod ändras ska följande sparas som referens:

- `go test ./...`;
- `go test -race ./...`;
- `go vet ./...`;
- projektets perft-resultat;
- fasta `go depth`- och `go nodes`-sökningar med bestmove, score, PV, noder och
  djup;
- minst fem upprepningar av relevant sökbenchmark och NPS;
- maskin, Go-version, commit, Hash, Threads och testställningar.

Acceptansgrind:

- referensen är grön och reproducerbar;
- inga andra kampanjer belastar maskinen under prestandamätningen;
- `Threads=1` är uttrycklig referens för alla senare steg.

## Delmål B: atomär Crafty-liknande TT

### Föreslagen representation

Craftys lockless-modell lagrar en packad datadel och en kontrollordsdel:

```text
data  = pack(move, score, depth, bound, generation)
guard = fullHashKey XOR data
```

Vid probe läses båda orden atomärt. Posten accepteras endast om:

```text
guard XOR data == sökt fullHashKey
```

En blandning av data från två samtidiga skrivningar ska därmed bli en miss i
stället för en falsk träff. Förlorade eller tillfälligt ogiltiga TT-poster är
acceptabla; data från fel ställning är inte acceptabelt.

GoAlarics dragkodning använder 21 bitar. Följande preliminära layout ryms i ett
64-bitars dataord och ska låsas först efter gränstester:

| Fält | Preliminärt antal bitar |
|---|---:|
| move | 21 |
| score | 16 |
| depth | 8 |
| bound | 2 |
| generation | 9 |
| reserv | 8 |

Posten kan då bestå av två `atomic.Uint64` och fortfarande vara 16 byte. Alla
accesser till de två orden måste använda `sync/atomic`; vanliga `uint64` är
inte tillåtna i parallell sökning.

### Semantik som ska bevaras

- fyra kandidater per nuvarande probeområde;
- nuvarande mate-score-konvertering relativt ply;
- exakt/lower/upper-bound;
- djup- och generationsbaserad ersättning;
- bevarande av gammalt drag när en lämplig ny post saknar drag;
- `hashfull`/`Used` med dokumenterad och trådsäker approximation;
- säkert generationsvarv och `Clear` endast när ingen sökning körs.

Om generationsuppdatering vid en probe behålls ska den ske med en atomär,
validerad uppdatering. Den får inte återinstallera en post som en annan worker
redan har ersatt. Ett enklare alternativ är att endast åldersmarkera vid
store, men det är en söksemantisk ändring och måste då utvärderas separat.

### TT-tester

- packa och packa upp samtliga gränsvärden;
- alla dragtyper inklusive rockad, en passant och promotion;
- positiva och negativa normal-, mate- och tabellbasscore;
- exact/lower/upper och alla relevanta djup;
- generationsvarv;
- nuvarande ersättningsfall;
- simulerade blandningar av `data` och `guard` från olika poster måste ge
  miss;
- många samtidiga probes/stores mot samma och olika buckets;
- `go test -race` ska vara helt rent;
- tabellen får aldrig returnera ett drag som inte kan valideras för den
  aktuella nyckeln.

### TT-prestandagrind

Kör först enbart med `Threads=1`. Fasta nodsökningar ska ge samma bestmove,
score och söksemantik som referensen. NPS mäts i minst fem alternerande
före/efter-körningar.

Mål: högst cirka 2 procent medianförlust. En stabil förlust över 5 procent är
releaseblockerande och kräver profilering eller annan representation. Ingen
matchkampanj behövs om trädet och resultaten är exakt oförändrade.

Committa och pusha TT-ändringen separat först när grinden är grön.

## Delmål C: isolera all worker-state

Refaktorera sökningen medan endast worker 0 fortfarande körs:

1. skapa en tydlig `Worker`-struktur;
2. flytta `Best`, PV, current, nodräknare och lokala limits dit de hör;
3. flytta `SG.History` till worker-lokal state;
4. koppla befintliga lokala killer-, pawn-, eval- och generatorstrukturer till
   workern;
5. skilj central tidsstyrning från worker-lokala räknare;
6. ta bort eller isolera oanvänd `splitPoint`-state så att den inte av misstag
   påverkar Lazy SMP;
7. bevara testhookar utan global samtidsskrivning.

Brädkopieringen ska granskas fält för fält. Repetitionshistorik, root state,
en passant, rockadrätt och 50-dragsräknare måste vara kompletta och oberoende
för varje worker.

Acceptansgrind:

- endast en worker kör;
- fasta sökningar matchar referensen;
- perft är oförändrad;
- stopp och UCI-resultat är oförändrade;
- inga extra goroutines lämnas kvar;
- hela race-sviten är grön.

Committa refaktoreringen separat.

## Delmål D: persistent worker-pool och Lazy SMP med två workers

### Livscykel

- poolen skapas eller ändrar storlek endast när motorn är idle;
- huvudworkern använder den befintliga sökgoroutinen;
- `Threads-1` helpers är långlivade goroutines som väntar på arbete;
- ett `go` publicerar en immutable sökbeskrivning och väcker helpers;
- alla workers får egna rotkopior innan sökningen börjar;
- huvudworkern sätter stoppsignalen och väntar alltid in helpers;
- `stop`, `quit`, nytt `position` och `ucinewgame` får inte lämna workers från
  föregående sökning aktiva.

Channels eller condition-liknande signalering får användas mellan sökningar,
men aldrig per nod.

### Första Lazy SMP-beteendet

Första versionen ska vara konservativ:

- alla workers kör samma iterativa fördjupning från roten;
- naturlig schemaläggning och den delade TT:n får först skapa skillnaderna;
- huvudworkerns senaste kompletta iteration avgör `bestmove`;
- helpers fungerar initialt som TT-hjälpare;
- ingen root-move-fördelning, split point eller omröstning införs samtidigt.

Efter att korrektheten är visad kan deterministisk diversifiering provas, till
exempel en liten worker-ID-baserad variation av aspirationsfönstret eller
startdjupet. Varje sådan policy är en separat sökändring och måste mätas.

### Noder och tid

- `go movetime` och vanlig tidsstyrning använder samma väggklocka för alla
  workers;
- `go nodes N` avser motorns sammanlagda publicerade nodantal;
- worker-lokala räknare ska undvika en atomär operation i varje nod;
- eventuell nodöverskjutning från blockvis publicering ska vara begränsad,
  testad och dokumenterad;
- UCI-`nodes`, `nps`, `seldepth` och `tbhits` ska avse hela poolen;
- `go depth N` ska stoppa när huvudworkern har färdigställt det begärda
  djupet; helperdjup rapporteras inte som huvudresultat.

## Delmål E: korrekthets- och livscykeltester

Automatisera minst följande för `Threads=1,2,4` och där det är rimligt även 8
och 16:

- start, naturligt slut och exakt ett `bestmove`;
- omedelbart `stop` och upprepade stoppsignaler;
- `go infinite` följt av `stop`;
- `go movetime`, `go nodes` och `go depth`;
- mycket korta tidsgränser;
- matt, patt, repetition, 50-dragsregel och Contempt;
- Syzygy WDL under parallell sökning och root-probe utanför den;
- `ucinewgame` och ny ställning mellan täta sökningar;
- avvisade optionändringar under aktiv sökning;
- upprepad ändring av `Threads` när motorn är idle;
- inga illegala drag, panics, deadlocks eller goroutine-läckor;
- hundratals start/stopp-cykler;
- samtidiga TT-stresstester;
- hela `go test -race ./...`.

Särskilda assertions ska kontrollera att en gammal worker aldrig kan skriva PV,
resultat eller TT-generation till en ny sökning.

## Delmål F: resultatval mellan workers

När tvåworkersökningen är stabil kan resultatval utvecklas separat.

Rekommenderad ordning:

1. huvudworkerns resultat, som i första versionen;
2. välj en helper endast om den har en djupare komplett iteration och ett
   användbart, icke-avbrutet resultat;
3. överväg därefter Stockfish-liknande röstning per rotmove, viktad med score
   och resultatkvalitet;
4. mate- och tabellbasresultat måste ha egna, entydiga prioritetsregler;
5. en oavslutad iteration får aldrig slå ut en komplett iteration.

Varje steg ska vara en separat kandidat. Ett snyggare röstsystem är inte
automatiskt starkare och måste verifieras i match.

## Delmål G: skalnings- och styrkemätning

### Teknisk skalning

Mät på samma fasta EPD-material med 1, 2, 4, 8 och 16 workers:

- sammanlagda noder och NPS;
- median- och percentildjup;
- tid till ett bestämt komplett djup;
- TT-träffar och hashfull;
- andel gemensamt kontra duplicerat arbete om det kan instrumenteras billigt;
- allokeringar och GC-pauser;
- CPU-utnyttjande och minnesbandbredd;
- stopplatens och nodgränsens överskjutning.

Lazy SMP behöver inte ge linjär NPS- eller nodskalning. Viktigare är kortare
tid till relevant djup och bättre drag vid samma väggtid.

### Matchverifiering

Kör i denna ordning:

1. ett litet UCI/Fastchess-smoketest;
2. 1 thread mot 2 threads vid samma väggtid och samma totala Hash;
3. 1 mot 4 threads;
4. vid behov 2 mot 4 för att mäta avtagande utbyte;
5. en större parad bekräftelse först efter positiv riktning.

Detta är en avsiktlig hårdvarufördel och behöver inte isoleras som en vanlig
parameterförändring. Rapportera både faktisk Elo-skillnad och kostnaden i
kärnor. Ponder ska vara avstängt och båda sidor ska använda samma Syzygy- och
öppningsunderlag.

Ingen flertrådad baseline promoveras automatiskt.

## Garbage collection och minneslayout

GC är inte ett argument mot parallell alfa–beta i sig. Kravet är att den heta
sökvägen är nästan allokeringsfri:

- TT ska vara en sammanhängande slice utan Go-pekare i posterna;
- workers och stackar förallokeras;
- inga jobbobjekt skapas per nod;
- channels används endast för poolens livscykel;
- stora worker-lokala räknare kan behöva cache-line-padding för att undvika
  false sharing;
- inga generella ändringar av `GOGC` eller `GOMAXPROCS` görs innan profiler
  visar ett verkligt behov.

`Threads=N` anger antal sökworkers. Go-runtime får normalt själv schemalägga
dem på tillgängliga kärnor.

## Riskregister

| Risk | Motåtgärd |
|---|---|
| Falsk TT-träff efter samtidiga skrivningar | full 64-bitars XOR-validering och atomära ord |
| TT blir en flaskhals | inga globala lås; profilera cachemissar och contention |
| Global history får datarace | worker-lokal history i första versionen |
| Gammal worker överlever `stop` | sök-ID, gemensam stoppsignal och obligatorisk join/wait |
| Flera `bestmove` | endast huvudworkern får skriva UCI |
| Fel total nodgräns | blockvis atomär publicering med testad maxöverskjutning |
| Board-state delas av misstag | fältvis kopieringsaudit och repetitions-/undo-tester |
| Syzygy-lås begränsar skalning | separat profilering med och utan tabellbaspositioner |
| GC eller allokeringar ökar | alloc-profiler och förallokerade workers |
| False sharing mellan workers | separera/padda ofta skrivna räknare |
| Fler trådar ger svagare sökning | stegvis 2/4/8-test och match innan promotion |
| Gammal YBWC-stomme blandas in | håll split-point-koden inaktiv eller avlägsna den separat |

## Stopp- och beslutsgrindar

Arbetet stoppas och omprövas om något av följande inträffar:

- `Threads=1` kan inte hållas sökekvivalent med referensen;
- race-detektorn hittar ett reproducerbart race;
- TT:n kan returnera hybriddata som giltig;
- stopplatens eller goroutine-läckor inte kan begränsas;
- TT-ombyggnaden ger mer än cirka 5 procent stabil enkärnig
  prestandaförlust;
- två workers ger tydlig och reproducerbar spelstyrkeförlust;
- implementationen kräver lås eller allokering vid praktiskt taget varje nod.

Om Crafty-modellen inte fungerar tillräckligt bra i Go är första reservplanen
en kompakt atomär enords-TT med kortare nyckel. Stripade bucket-lås används
endast som korrekthetsreferens eller diagnostik, inte som självklar slutdesign.

## Definition av färdig första flertrådsversion

Första versionen är färdig när:

- `Threads=1` är korrekt och utan mätbar regression;
- `Threads=2` och `Threads=4` gör verklig parallell sökning;
- all sökstate har tydligt ägarskap;
- TT, stopp och publicerade räknare är trådsäkra;
- samtliga correctness-, race-, perft- och UCI-tester passerar;
- avbrott lämnar inga workers eller processer kvar;
- teknisk skalning är dokumenterad;
- minst en parad match visar den praktiska effekten av fler kärnor;
- användardokumentationen beskriver `Threads`, Hash och total nodbudget;
- merge till `master` sker manuellt efter granskning.

En sådan ändring bör betraktas som en ny funktionsversion, sannolikt v1.3.0,
inte som en liten parameterrelease.

## Möjlig senare YBWC-fas

YBWC övervägs endast efter en färdig Lazy SMP-baseline. En framtida prototyp
ska då:

- använda samma fasta worker-pool;
- söka första barnet seriellt;
- skapa split points endast över ett minimidjup och med flera kvarvarande drag;
- återanvända förallokerade split-point-poster;
- dela alfa och cutoff med atomära operationer eller mycket små, lokala lås;
- avbryta yngre syskon omedelbart vid cutoff;
- falla tillbaka till vanlig sökning när inga workers är lediga;
- jämföras direkt med Lazy SMP vid samma antal workers och väggtid.

Ingen goroutine eller kanal får skapas per söknod.

## Källor att återläsa före implementation

- Craftys XOR-baserade lockless TT:
  <https://github.com/MartinMSPedersen/Crafty-Chess/blob/master/source/hash.c>
- Craftys tvåordsrepresentation:
  <https://github.com/MartinMSPedersen/Crafty-Chess/blob/master/source/chess.h>
- Stockfish worker-pool och resultatval:
  <https://github.com/official-stockfish/Stockfish/blob/master/src/thread.cpp>
- Stockfish iterativa workersökning:
  <https://github.com/official-stockfish/Stockfish/blob/master/src/search.cpp>
- Stockfish gemensamma TT och dokumenterade samtidighetsval:
  <https://github.com/official-stockfish/Stockfish/blob/master/src/tt.h>
- Ethereals worker-lokala state och Lazy SMP:
  <https://github.com/AndyGrant/Ethereal/blob/master/src/thread.c>
- Översikt över Lazy SMP:
  <https://www.chessprogramming.org/Lazy_SMP>
- Young Brothers Wait Concept:
  <https://www.chessprogramming.org/Young_Brothers_Wait_Concept>
- Go `sync/atomic`:
  <https://pkg.go.dev/sync/atomic>

## Rekommenderad återstart

När arbetet återupptas:

1. läs detta dokument och kontrollera att `master` fortfarande är den avsedda
   baselinen;
2. skapa eller återställ en ren `development`-gren;
3. genomför delmål A;
4. implementera endast delmål B;
5. granska och mät TT:n innan någon helper startas;
6. fortsätt därefter ett delmål och en separat commit i taget;
7. börja med två workers och lämna YBWC till sist.
