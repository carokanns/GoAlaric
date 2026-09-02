# Framtida arbete: effektivare evaluering och sökning

Senast dokumenterad: 2026-08-27.

## Utgångsläge

Bedömningen efter genomgång och parameterkampanjer är att ytterligare
finjustering av de befintliga evalueringsparametrarna sannolikt har begränsad
potential. Flera termer samverkar och är korrelerade, medan hittills testade
enskilda parameterändringar i regel inte har visat någon säker förbättring.

De separata kungstryckstermerna ska tills vidare behållas. De mäter olika men
delvis överlappande signaler:

- möjliga säkra schackdrag;
- antal och styrka hos angripare i motståndarens kungzon;
- torn på öppna eller halvöppna linjer nära motståndarkungen.

Det finns inget konstaterat fel i att dessa signaler används samtidigt. Om de
senare undersöks ska deras individuella bidrag först instrumenteras i stället
för att nya vikter provas på måfå.

## Mest lovande nästa evalspår

Prioritera effektivisering av den befintliga evalueringen framför fler
evaltermer eller parametrar. Två huvudspår är aktuella:

1. Minska kostnaden för de beräkningar som alltid behöver utföras.
2. Undvik dyra evaldelar när ett säkert värdeintervall visar att de inte kan
   påverka sökningens förhållande till alfa och beta.

Det andra spåret motsvarar en försiktig form av lazy evaluation. Det får inte
införas förrän verkliga mätningar visar vilka delar som är dyra och hur stora
säkra marginaler som behövs.

## Instrumentering före beteendeförändring

Första experimentet ska endast mäta och får inte påverka vald score, drag eller
sökträd. Samla åtminstone följande per evaldel:

- antal anrop;
- sammanlagd och genomsnittlig exekveringstid;
- bidragets minimi-, maximi- och typiska absoluta storlek;
- hur ofta en billig partiell evaluering redan ligger tydligt utanför
  alfa-beta-fönstret;
- hur stor återstående positiv respektive negativ evalpåverkan de ännu inte
  beräknade delarna faktiskt får;
- hur ofta en hypotetisk tidig avbrytning hade gett samma slutsats som den
  fullständiga evalueringen.

Mät på ett representativt ställningsmaterial och under verklig iterativ
fördjupning. Enbart isolerade evalanrop är inte tillräckliga eftersom
alfa-beta-fönster, sökdjup och nodtyp påverkar nyttan.

Instrumenteringen bör kunna kompileras bort eller aktiveras uttryckligen så
att den inte belastar vanliga motorbyggen.

## Möjlig implementation efter mätning

Om mätningen visar potential kan evalueringen delas i en billig och en dyr
del. Efter den billiga delen beräknas ett konservativt intervall för vad den
fullständiga scoren kan bli. Tidig retur är tillåten endast när hela intervallet
ligger på samma säkra sida om relevant sökgräns.

Viktiga krav:

- marginalerna ska härledas ur mätdata och innehålla säkerhetsmarginal;
- mattvärden, terminala ställningar och exakta tabellbasresultat får inte
  approximeras;
- PV-noder och taktiskt känsliga noder kan behöva full evaluering;
- en avkortad score får inte lagras eller återanvändas som om den vore en exakt
  statisk evaluering;
- transpositionstabellens befintliga statiska eval ska återanvändas när dess
  identitet och giltighet är säker;
- inkrementell material- eller PST-beräkning ska behandlas som ett separat,
  semantikbevarande optimeringsförsök.

## Verifieringsgrind för evaleffektivisering

Varje kandidat ska först jämföras med baseline på samma commitnära testdata.
Minimikrav:

1. `go test ./...` och projektets fullständiga lokala testkedja passerar.
2. Perft är oförändrad.
3. Ett exakt kontrolläge där lazy evaluation är avstängd ger oförändrade
   evalvärden.
4. Fasta sökningar granskas för bestmove, score, noder, djup och NPS.
5. Mätningen upprepas flera gånger; en ren hastighetsändring bör ge en tydlig
   och stabil förbättring, inte bara normal tidsvariation.
6. Om sökträdet eller valda drag ändras ska kandidaten betraktas som en
   spelstyrkeändring och avgöras i en separat, parad motorkampanj.

En snabbare evaluering är inte automatiskt en bättre motor om osäkra
avkortningar förändrar kritiska sökbeslut.

## Framtida söklogik

Söklogik bedöms ha större möjlig förbättringspotential än fler evalvikter, men
ingen ny generell heuristik ska införas utan en konkret hypotes. Nästa idé bör
helst komma från profilering eller diagnostik av verkliga sökträd, exempelvis:

- en dyr nodtyp eller återkommande onödig omsökning;
- bristande dragordning som kan mätas;
- en pruningregel som missar eller behåller en tydligt identifierbar klass av
  drag;
- ett djup- eller tidsstyrningsproblem som syns reproducerbart.

Tidigare förkastade försök ska inte återinföras oförändrade. Det gäller bland
annat dynamiskt LMP-djup, null-move-verifiering i materialfattiga ställningar,
single-reply-extension och den prövade singular-extensionen.

## Rekommenderad återstartspunkt

När utvecklingen återupptas:

1. verifiera aktuell baseline och läs experimentloggen;
2. profilera evalkostnaden utan att ändra motorbeteendet;
3. välj högst en tydligt dyr evaldel eller en konservativ lazy-eval-gräns;
4. implementera ändringen isolerat på utvecklingsgrenen;
5. kör korrekthets- och prestandagrindarna;
6. starta matchkampanj endast om ändringen påverkar sökträdet eller om en
   faktisk styrkeeffekt behöver avgöras.

Syftet är att undvika fler parameterexperiment utan tydlig hypotes och i
stället söka en mätbar minskning av arbete per nod eller en konkret förbättring
av söklogiken.
