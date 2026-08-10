# Deterministiska materialremitester

Testsviten har två separata delar:

1. `generate_material_draw_cases.py` använder lokala Syzygy-tabeller som facit
   och genererar reproducerbara ställningar med ett fast slumpfrö.
2. `run_material_draw_tests.sh` kör GoAlaric på fast sökdjup och kräver att
   motorns bestmove tillhör mängden Syzygy-korrekta drag.

Falltypen `force_material_draw` kräver att den sämre sidan väljer en
forcerad övergång till matematiskt dött material. `avoid_material_draw`
kräver att den vinnande sidan undviker drag som kastar bort vinsten genom
samma övergång.

Installera det lokala oraklet och små 3–4-pjästabeller:

```bash
bash scripts/setup_material_draw_oracle.sh
```

Återskapa fallfilen deterministiskt:

```bash
PYTHONPATH=.tools/python python3 scripts/generate_material_draw_cases.py \
  --syzygy-path .tools/syzygy/3-4-5 \
  --output scripts/material_draw_cases.json
```

Kör regressionstestet:

```bash
bash scripts/run_material_draw_tests.sh
```

Fathom och tabellbasfiler ligger under `.tools/` och versionshanteras inte.
Den genererade JSON-filen versionshanteras, vilket gör den ordinarie
regressionen helt lokal och oberoende av nätverk och Syzygy-installation.

Sviten innehåller 16 fall: åtta där den sämre sidan måste forcera remi och
åtta där den bättre sidan måste undvika en materialremifälla. Testprogrammet
startar en ny motorprocess för varje fall, söker på fast djup 6, skriver en
kompakt JSON-rapport och returnerar felkod 1 vid schackligt testfel respektive
2 vid infrastrukturfel.

Med kandidat 018 klaras alla 16 fall. Baseline `66e8d11` väljer rätt drag i
vinstfällorna men missar den exakta remisemantiken i samtliga åtta
forcerade fall: den rapporterar cirka +22–24 cp i stället för contempt-remi
`-5`.

## Separat motorintegration

Motorintegration med Syzygy blir en egen kandidat, eftersom den ändrar
sökresultat och därför inte ska blandas in i materialremidetektorn. Den bör:

- använda en versionslåst, inbyggd Fathom-kärna med cgo och en fungerande
  stub när cgo inte är tillgängligt;
- exponera UCI-valen `SyzygyPath` och `SyzygyProbeDepth`;
- aldrig proba en ställning med rockadrätt och lämna vanliga sökresultat
  orörda om tabell saknas eller proben misslyckas;
- använda WDL i trädet och DTZ/50-dragsmedvetet dragval endast i roten;
- rapportera antal lyckade prober via `tbhits`;
- ha en egen deterministisk testsvit mot de lokala 3–4-pjästabellerna,
  inklusive vinst, remi, förlust, 50-dragsfall, en passant, ogiltig sökväg
  och avstängd Syzygy.

Fathoms CLI som byggs av installationsskriptet är ett referensverktyg. Den
framtida motorkopplingen ska byggas trådsäkert och får alltså inte använda
CLI-byggens `TB_NO_THREADS`.
