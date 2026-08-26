# Diagnostik av singulara drag i sökträdet

Detta är en opt-in-mätning. Den ändrar inte motorns sökbeslut och inför ingen
extension. Syftet är att mäta om ett stabilt mycket bättre drag förekommer
tillräckligt ofta och om den verkliga TT-informationen kan hitta fallen.

## Material och urval

`capture` läser färdiga Fastchess-PGN:er med UCI-drag. En ställning väljs
deterministiskt per parti efter sista `{book}`-draget och senast vid angiven
partiply. Ställningar med högst fyra pjäser tas inte med eftersom Syzygy normalt
hanterar dessa slutspel.

Varje källställning söks med motorns normala root-sökning och iterativa
fördjupning. Endast slutiterationen spåras, så TT-drag, lagrat TT-djup, bound
och score kommer från tidigare verklig sökning. Noder på återstående djup 6
och 8 tilldelas en deterministisk 64-bitars rang. Ett globalt bottom-k-urval
behåller de 200 lägsta rangerna per djup över hela materialet. Urvalet beror
därför inte på vilken nod som råkar besökas först.

```bash
go run ./cmd/siblingdiag capture \
  --pgn /home/peter/Projekt/GoAlaric-optimizer/artifacts/development/single-reply-extension-6000-20260826/run/games.pgn \
  --depth 10 \
  --target-depths 6,8 \
  --per-depth 200 \
  --limit 600 \
  --max-game-ply 120 \
  --sample-seed 20260826 \
  --syzygy off \
  --output /tmp/goalaric-sibling-trace-v2.json
```

EPD kan användas för små reproduktioner genom att ersätta `--pgn` med
`--epd FILE`, men acceptansmätningen ska använda den representativa PGN:n.

## Stabilitetsanalys

`analyze` söker varje legalt syskondrag separat med fullt fönster och ren
sökstatus. Sökningen görs först på nodens registrerade återstående djup och
sedan två ply djupare. Rapporten sparar alla dragscore samt antal legala drag,
antal pjäser och om bästa draget redan omfattas av befintlig check-, recapture-,
vinnande-slag- eller fribondeextension.

```bash
go run ./cmd/siblingdiag analyze \
  --input /tmp/goalaric-sibling-trace-v2.json \
  --output /tmp/goalaric-sibling-analysis-v2.json \
  --syzygy off
```

Ett kvalificerat fall måste vara utanför schack, ha minst fyra legala drag och
fler än fyra pjäser, sakna matt-score och inte redan få extension. Ett stabilt
singularfall kräver dessutom samma bästa drag på båda djupen och minst 50 cp
till det näst bästa draget på båda djupen.

TT-signalen i rapporten är medvetet definierad före en eventuell implementation:
en riktig lagrad TT-post med lower/exact-bound, ett drag, ett icke-mattscore och
lagrat djup minst `nodens djup - 3`. En sann positiv kräver att TT-draget också
är det stabila bästa draget. Rapporten visar träffandel, recall, falska positiva
och uppskattade stabila fall respektive TT-triggers per sökt miljon noder.

Frekvensens 95-procentsintervall är Wilsonintervallet. FEN återger inte hela
den interna sökvägens repetitionshistorik; resultatet är därför diagnostisk
evidens och inte ett fristående schackfacit.

## Beslutsgrind

Rapporten ger `eligible_for_experimental_extension` endast om alla krav nås:

- minst 3 procent av kvalificerade noder är stabila singularfall;
- TT-signalen hittar minst 70 procent av dem;
- högst två falska positiva förekommer per sann positiv.

Ett passerat resultat tillåter bara nästa experimentsteg. En extension ska
därefter klara fasta taktik- och zugzwangtester samt en fixed-depth-jämförelse
med högst cirka 10 procent median nodökning innan en separat match övervägs.

## Representativ mätning 2026-08-26

Mätningen kördes mot den färdiga 6000-partiersfilen
`artifacts/development/single-reply-extension-6000-20260826/run/games.pgn`
med 600 deterministiskt valda post-book-ställningar. Capture sökte 50 883 732
noder och såg 72 884 kandidater på återstående djup 6 samt 14 741 på djup 8.
De fulla lokala rapporterna skapades som:

- `/tmp/goalaric-sibling-trace-v2-final.json`
- `/tmp/goalaric-sibling-analysis-v2-final.json`

Resultatet blev 116 stabila singularfall bland 335 kvalificerade noder, alltså
34,63 procent med Wilson 95-procentsintervall 29,73–39,87 procent. För djup 6
var utfallet 62/174 (35,63 procent) och för djup 8 54/161 (33,54 procent).
TT-poster fanns i 383/400 stickprov.

Den fördefinierade TT-signalen hittade däremot bara 72/116 stabila fall:
62,07 procents recall mot kravet 70 procent. Den gav 144 falska positiva,
exakt två per sann positiv, och klarade därmed precis den separata
precisionströskeln. Den uppskattade frekvensen var cirka 499 stabila fall och
930 möjliga TT-triggers per sökt miljon noder.

Beslutsgrinden blev `do_not_implement_extension`. Frekvenskravet passerade,
men recallkravet gjorde det inte. Ingen singular extension, fixed-depth-
kandidat eller matchkampanj infördes som följd av mätningen.
