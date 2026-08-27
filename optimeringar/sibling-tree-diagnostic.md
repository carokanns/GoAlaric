# Opt-in sibling-tree diagnostic

Diagnostiken mäter hur ofta ett normalt sökträd innehåller ett drag som är
klart bättre än sina syskondrag. Den ändrar inte motorns söklogik, förlänger
inte sökningen och används inte i vanliga UCI-sökningar.

## Metod

`capture` kör fasta djupsökningar på valda EPD-ställningar och sparar ett
begränsat, deterministiskt urval av noder som nått den normala sökningens
dragloop. För varje sparad nod lagras ställningen, djupet, fönstret, PV/icke-PV,
schackstatus, hård gallring, TT-drag och den normala nodräknaren.

`analyze` läser JSONL-spåret. För varje sparad ställning genereras alla legala
drag. Varje drag söks sedan från en ren sökstatus med fullt alfa-beta-fönster
och samma återstående djup. Rapporten sorterar syskondragen och beräknar
skillnaden mellan bästa och näst bästa score i centipawn. Mates rapporteras
separat och får ingen konstgjord centipawngap.

Sökningen återställs från FEN vid offlineanalysen. EPD:ns första fyra fält
används; kommentarer och facitfält som `c0`/`c1` ignoreras. FEN innehåller inte
den fullständiga repetitionshistoriken, så repetitionkänsliga grenar ska
tolkas som diagnostiska indikeringar och inte som ett separat korrekthetsfacit.

## Körning

Kör från GoAlaric-roten. Standard är Syzygy avstängt för att göra mätningen
oberoende av lokalt installerade tabeller.

```bash
go run ./cmd/siblingdiag capture \
  --epd fullGP.epd \
  --depth 8 \
  --min-depth 5 \
  --limit 200 \
  --max-positions 100 \
  --sample-modulo 97 \
  --sample-seed 20260826 \
  --output /tmp/goalaric-sibling-trace.jsonl \
  --syzygy off

go run ./cmd/siblingdiag analyze \
  --input /tmp/goalaric-sibling-trace.jsonl \
  --output /tmp/goalaric-sibling-analysis.json \
  --syzygy off
```

`--max-positions` gäller per indata-ställning. Minska `--limit` eller höj
`--sample-modulo` för en snabbare första mätning. Analysens trösklar är 25,
50, 75, 100 och 200 cp. De är rapportgränser, inte beslut om att införa en
singular extension.

Ett alternativ med installerade tabeller är exempelvis:

```bash
go run ./cmd/siblingdiag capture --epd fullGP.epd --depth 8 \
  --limit 20 --syzygy .tools/syzygy/3-4
```

## Tolkning

Ett stort gap visar att den bästa sökningen i den aktuella motorn tydligt
skiljer ut ett drag från sina syskon vid det analyserade djupet. Det bevisar
inte i sig att en extension är korrekt eller spelstyrkehöjande. Före en
eventuell ändring bör man kontrollera gapets stabilitet vid djupare sökning,
om draget är ett TT-drag, om ställningen är taktisk och hur ofta beteendet
förekommer. Därefter krävs separata zugzwang-/korrekthetstester och match mot
baseline.
