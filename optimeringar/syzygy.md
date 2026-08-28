# Valfri Syzygy-probering

GoAlaric kan använda lokala Syzygy-tabeller genom den inbyggda,
versionslåsta Fathom-kärnan. Funktionen är helt valfri: utan tabeller eller
med `CGO_ENABLED=0` används den vanliga sökningen och den inbyggda
materialremilogiken.

## UCI

```text
setoption name SyzygyPath value /sökväg/till/tabeller
setoption name SyzygyProbeDepth value 1
```

Standardvägen är `.tools/syzygy/3-4` relativt motorns arbetskatalog. En tom
`SyzygyPath` stänger av probering. En felaktig sökväg ger ett
`info string`-fel och lämnar motorn i avstängt, spelbart läge.

GoAlaric räknar antalet pjäser före varje probe. Om ställningen har fler
pjäser än den största inlästa tabellen anropas inte Fathom. Med uppsättningen
för tre och fyra pjäser nedan probas alltså aldrig ställningar med fem eller
fler pjäser.

Vid roten används DTZ med halvdragsklockan för ett 50-dragsmedvetet dragval.
I sökträdet används trådsäker WDL-probering efter en miss i
transpositionstabellen. WDL-probering görs konservativt endast när
halvdragsklockan är noll; roten kan använda andra värden. `tbhits` redovisas
i UCI:s info-rader.

Ställningar med rockadrätt probas inte. En passant konverteras till Fathoms
koordinatsystem och täcks av den deterministiska testsviten.

## Separata testlägen

Hämta en komplett uppsättning för tre och fyra pjäser (WDL och DTZ):

```bash
bash scripts/setup_small_syzygy.sh
```

Tabellerna hamnar i `.tools/syzygy/3-4`, tar ungefär 4,15 MiB och är
ignorerade av Git. Aktivera dem med:

```text
setoption name SyzygyPath value /absolut/sökväg/till/.tools/syzygy/3-4
```

Hämta den kompletta uppsättningen för tre till fem pjäser:

```bash
bash scripts/setup_syzygy_5.sh
```

De 145 WDL- och 145 DTZ-filerna hamnar i
`.tools/syzygy/3-4-5-complete`, verifieras mot Lichess SHA-256-manifest och
tar 983 957 920 byte, ungefär 939 MiB. WDL-delen räcker för Fastchess
tabellavdömning; GoAlarics rotprobering använder även DTZ-delen.

Installera de små, checksummekontrollerade testtabellerna:

```bash
bash scripts/setup_material_draw_oracle.sh
```

Kör både Syzygy-sviten och den obligatoriska cgo-fria sviten:

```bash
bash scripts/run_syzygy_tests.sh .tools/syzygy/3-4-5
```

Syzygy-sviten kontrollerar vinst, remi, förlust, cursed win vid 99
halvdrag, en passant, probe-depth, rockadrätt, UCI-konfiguration, DTZ-drag
och `tbhits`. Den cgo-fria sviten bygger en separat motor och kör dessutom
alla 16 inbyggda materialremifallen mot den.

Fathom-källan ligger i `syzygy/fathom/`, är låst till commit
`c9c6fef0dddc05d2e242c183acf5833149ab676d` och behåller MIT-licensen.
