# Framtida språkbyte för GoAlaric

Senast uppdaterad: 2026-09-04.

## Status

Detta dokument beskriver ett möjligt framtida byte från Go till ett språk med
mer direkt kontroll över minne, kodgenerering, SIMD och flertrådning. Det är
en översiktlig utvecklingsplan, inte ett beslut att påbörja en omskrivning nu.

Nuvarande Go-version är den verifierade motorn och ska bevaras som baseline.
En ny implementation får inte ersätta den förrän den har visat korrekthet,
stabilitet och en tillräckligt stor uppmätt fördel.

NNUE behandlas i `optimeringar/nnue-utvecklingsplan.md` och ligger längre fram
i tiden. Om ett språkbyte görs bör den nya enkeltrådade motorn först bli
färdig och verifierad. Flertrådad sökning kommer därefter och NNUE senare som
ett separat projekt.

## Rekommenderad språkordning

### 1. Rust

Rust är förstahandsvalet för en eventuell ny GoAlaric-version. Det ger:

- prestandapotential nära C och C++;
- ingen garbage collector;
- tydlig kontroll över minneslayout och allokeringar;
- säkrare ägarskap för bräde, sökstack och hjälptabeller;
- starkt typstöd för att skilja färg, pjäs, ruta, drag och bitboard;
- kompilatorkontrollerad delning mellan söktrådar;
- atomics och möjlighet till lågnivåoptimering;
- CPU-specifika intrinsics för bland annat POPCNT, BMI och AVX;
- bättre långsiktig säkerhet än C och normalt C++.

Rusts ägarskapsmodell kan göra den första porteringen långsammare, särskilt
för rekursiv sökning med mycket muterbart tillstånd. Samma modell kan däremot
förhindra en stor klass av livstidsfel och races innan programmet körs. Det är
särskilt värdefullt när Lazy SMP senare införs.

### 2. C++

C++ är andrahandsvalet och kan vara förstaval om absolut topprestanda,
SIMD/NNUE eller närhet till etablerade schackmotorverktyg väger tyngre än
språksäkerhet.

Fördelar:

- mycket moget stöd för intrinsics, SIMD, LTO och PGO;
- stor erfarenhetsbas från moderna schackmotorer;
- full kontroll över cachelinjer och datastrukturer;
- god kodgenerering i GCC och Clang.

Nackdelarna är större risk för minnesfel, data races och otydliga
ägandeförhållanden. Stockfishs GPL-kod får inte direkt kopieras till
MIT-licensierade GoAlaric utan ett medvetet licensbeslut.

### 3. C

C kan ge mycket hög prestanda och är bekant från Alaric. Det är ett rimligt
alternativ för att vidareutveckla gammal Alaric-kod eller skriva en mycket
liten fristående kärna.

För en full modern motor är C tredjehandsvalet. Lazy SMP, atomisk TT,
trådlokalt söktillstånd och framtida NNUE innebär omfattande manuell
livstids- och synkroniseringshantering. Risken för subtila fel blir större än
i Rust.

## Rust och GoAlarics bitboards

Rust passar väl för motorns omfattande bithantering. Ett bitboard motsvaras
direkt av `u64`, eventuellt omslutet av en egen typ:

```rust
#[derive(Clone, Copy, PartialEq, Eq)]
struct Bitboard(u64);
```

Språket har stabila operationer för:

- `AND`, `OR`, `XOR` och komplement;
- vänster- och högerskift;
- `count_ones` för populationsräkning;
- `trailing_zeros` och `leading_zeros`;
- rotationer och byteordning;
- wrapping-, checked- och overflowing-aritmetik;
- `AtomicU64` för samtidiga datastrukturer.

Magic bitboards, de Bruijn-indexering, Zobrist-hashning, attackmasker och
förberäknade tabeller kan representeras direkt. Kompilatorn kan generera
POPCNT, TZCNT och andra maskininstruktioner när målprocessorn stöder dem.

Två praktiska regler gäller:

1. Prestanda ska alltid mätas i ett optimerat releasebygge.
2. `unsafe` och okontrollerad indexering ska endast införas i små profilerade
   hotspots efter att den säkra implementationen har blivit korrekt.

En port av `bit`, `square`, `material`, attacktabeller och perft är därför en
bra första prototyp. Den delen är både naturlig i Rust och lätt att jämföra
med Go-versionen.

## Alternativ A: fullständig omskrivning

En fullständig omskrivning ger en enhetlig motor utan språkgränser i den heta
sökvägen. Det är den renaste långsiktiga lösningen, men också den dyraste och
mest riskfyllda.

### Etapp A: frys referensen

- Tagga eller notera exakt Go-commit och binärhash.
- Spara perftfacit, UCI-regressioner och fixed-depth-resultat.
- Spara representativa movetime- och nodmätningar.
- Dokumentera standardparametrar, Syzygy-inställningar och UCI-options.
- Ändra inte Go-baseline under jämförelsen.

### Etapp B: skapa en minimal Rust-motor

- Skapa projektet på en separat utvecklingsgren eller i en separat katalog.
- Inför tydliga typer för sida, ruta, pjäs, drag och bitboard.
- Porta hashnycklar och deterministiska initialiseringstabeller.
- Läs FEN och skriv tillbaka en verifierbar positionsrepresentation.
- Använd inga externa beroenden i den heta kärnan utan tydlig nytta.

### Etapp C: bräde och draggenerering

- Porta make/undo och null move.
- Hantera rockad, en passant och promotion.
- Porta pseudolegala och legala drag.
- Kör samma perftpositioner på varje djup.
- Lägg differentialtester som jämför Go och Rust automatiskt.

Ingen sökning eller evaluering ska portas vidare innan perft är helt
identiskt.

### Etapp D: enkeltrådad sökning

- Porta sökstack, repetition, 50-dragsregel och contempt.
- Porta transpositionstabell och score-normalisering för mattvärden.
- Porta quiescence, move ordering, pruning, reductions och extensions en del
  i taget.
- Jämför bästa drag, score, PV och noder vid fasta djup.
- Tillåt dokumenterade avvikelser endast när orsaken är känd.

Sökalgoritmen ska först vara funktionellt likvärdig. Nya sökidéer får inte
blandas in under porteringen.

### Etapp E: evaluering, Syzygy och UCI

- Porta den klassiska evalueringen och dess standardparametrar.
- Verifiera fasta evalpositioner mellan Go och Rust.
- Inför Syzygy med samma probningsregler.
- Implementera `uci`, `isready`, `ucinewgame`, `position`, `go`, `stop`,
  `quit` och övriga options.
- Säkerställ exakt ett `bestmove` även vid stop och avbrott.

### Etapp F: prestandabeslut

Mät Go och Rust på samma dator, compiler flags, hashstorlek, trådantal,
positioner och sökgräns:

- noder per sekund;
- tid till fast djup;
- uppnått djup vid fast tid;
- allokeringar och peakminne;
- TT-träffar;
- variation mellan upprepade körningar.

En full port bör inte fortsätta enbart för en liten teoretisk fördel. Som
preliminär beslutsgrind är en stabil förbättring på ungefär 25–30 procent i
relevanta sökmätningar ett starkt argument. En mindre vinst kan ändå vara
värdefull om Rust samtidigt gör den kommande flertrådningen betydligt säkrare.

### Etapp G: flertrådad sökning

Först när den enkeltrådade Rust-motorn är komplett och godkänd införs Lazy
SMP:

- långlivade arbetstrådar;
- arbetarlokalt bräde, sökstack och history;
- delad atomisk transpositionstabell;
- gemensam atomisk stoppsignal;
- worker 0 ansvarar för tid och UCI-utskrift;
- total nodbudget över samtliga trådar;
- race-, livscykel- och skalningstester.

Den sparade flertrådningsplanen i
`optimeringar/flertradad-sokning-plan.md` kan återanvändas, men anpassas till
Rusts ägarskap och atomics.

### Etapp H: matchverifiering och ersättning

- Kör först kort smoke-match mot Go-baseline.
- Kör en längre parad kampanj med samma tid och hårdvara.
- Kontrollera styrka både med en och flera trådar.
- Behåll Go-versionen reproducerbar även efter en lyckad Rust-release.
- Gör ingen automatisk promotion.

## Alternativ B: blanda språk

Språk kan kombineras framgångsrikt om gränsen ligger utanför den heta
sökloopen. Ju oftare gränsen korsas, desto större blir kostnaden och
komplexiteten.

### Python för verktyg och Go eller Rust för motorn

Detta är den mest naturliga och redan använda uppdelningen:

```text
Python: optimering, kampanjer, analys och framtida NNUE-träning
Go/Rust: UCI-motor, sökning och evaluering
SQLite/filer/processer: reproducerbar kommunikation
```

Python behöver aldrig anropas från en söknod. Verktyg och motor kan utvecklas
och testas oberoende.

### Go som UCI-skal och Rust som hel sökkärna

En övergångsarkitektur kan låta Go fortsätta hantera UCI och konfiguration,
medan Rust äger hela brädet och sökningen:

```text
Go: position och sökgräns
          |
          | ett grovt ABI-anrop per sökning
          v
Rust: bräde, draggenerering, TT, eval och full sökning
          |
          v
Go: bestmove, PV och statistik
```

Detta kan underlätta en stegvis migration. Gränssnittet bör vara ett litet,
versionsmärkt C-ABI med enkla heltal, bytebuffertar och tydligt ägande.

Go ska inte skicka Go-pekare som Rust behåller efter anropet. Rust ska inte
anropa tillbaka in i Go från varje nod. Fel, stoppsignal och livstid måste
definieras uttryckligt.

När Rust-kärnan blivit komplett bör man bedöma om Go-skalet fortfarande ger
något. Ett mycket tunt skal kan annars skapa bygg- och felsökningskostnader
utan praktisk nytta.

### Separat Rust-process

Under porteringen kan Go och Rust köras som separata UCI-processer eller
testverktyg. Det är utmärkt för differentialtester och kräver inget FFI.

Det är däremot inte en slutlig motorarkitektur om Go-processen endast
vidarebefordrar varje UCI-kommando till Rust-processen. Då är Rust-programmet
i praktiken redan motorn.

### En liten Rust- eller C++-funktion i Go

Detta bör undvikas i den heta vägen. Följande är olämpliga FFI-gränser:

- ett anrop per söknod;
- ett anrop per genererat drag;
- ett anrop per TT-probe;
- ett anrop per klassisk evaluering;
- ett anrop per NNUE-ackumulatoruppdatering.

`cgo` eller ett C-ABI medför anropskostnad, komplicerad stack- och
trådhantering samt svårare profilering. Miljontals små anrop kan äta upp hela
vinsten från den snabbare funktionen.

En liten främmande komponent kan vara rimlig när varje anrop gör mycket
arbete, exempelvis:

- laddning eller konvertering av en stor modell;
- offlineanalys;
- ett fullständigt sökuppdrag;
- en grov batchoperation.

Om endast några SIMD-rutiner är flaskhalsar kan en liten Go-assemblerfil vara
en enklare lösning än att införa ett helt andra systemspråk.

## Varför inte blanda Go-sökning och Rust-NNUE per eval

NNUE anropas mycket ofta. Ett separat Rust-anrop för varje lövnod riskerar att
bli dyrare än inferensvinsten och försvårar dessutom en ackumulator per
sökply. Om NNUE senare skrivs i Rust bör den ligga i samma Rust-kärna som
sökningen, eller anropas i så stora batcher att språkgränsens kostnad blir
försumbar. Det senare passar sämre med en traditionell alfa-beta-sökning.

## Gemensamma risker

Oavsett full omskrivning eller språkblandning måste följande hanteras:

- subtila avvikelser i make/undo och repetitionshistorik;
- ändrad heltalsaritmetik och overflowbeteende;
- arrayindex och skift med ogiltigt antal bitar;
- annan struct-layout och cachelokalitet;
- förändrad ordning mellan lika rankade drag;
- olika atomisk minnesordning;
- plattformsbyggen för Linux, Windows och eventuellt ARM;
- licenser för kod, bibliotek, nät och träningsdata;
- reproducerbara compiler- och buildinställningar.

Rusts säkra standardläge minskar flera risker men ersätter inte perft,
fixed-depth-tester, race-tester och matcher.

## Rekommenderad praktisk start

Om språkfrågan återupptas bör första försöket vara begränsat till en
Rust-prototyp, inte en full motor:

1. Skapa en ren utvecklingsgren.
2. Porta bitboards, rutor, material och attacktabeller.
3. Porta FEN, make/undo och legal draggenerering.
4. Nå full perftlikhet med Go.
5. Lägg till en enkel fixed-depth alfa-beta-sökning.
6. Mät samma ställningar i Go och Rust.
7. Gör en uttrycklig fortsätt/förkasta-bedömning.

Denna prototyp svarar på de viktigaste frågorna tidigt:

- Är Rust-koden begriplig och underhållbar för projektet?
- Kan brädet och sökstacken uttryckas utan onödig kopiering?
- Eliminerar kompilatorn gränskontroller i de heta looparna?
- Ger Rust en verklig prestandafördel på aktuell hårdvara?
- Är vinsten stor nog för att motivera resten av porteringen?

## Rekommenderat beslut i nuläget

- Behåll nuvarande GoAlaric som spelbar och reproducerbar motor.
- Genomför inte NNUE nu.
- Om ett språkbyte provas: välj Rust först.
- Börja med en liten enkeltrådad och mätbar prototyp.
- Implementera inte nya sökidéer samtidigt med porteringen.
- Inför flertrådad sökning först efter godkänd enkeltrådad likvärdighet.
- Använd språkblandning endast med grova, stabila gränssnitt.
- Välj C++ endast om Rust visar sig olämpligt eller om maximal SIMD/NNUE-
  integration senare blir det avgörande kravet.
- Välj C främst om arbetet i stället återgår till Alarics befintliga C-kod.
