# Experimentlogg

Den här filen fylls på av `testmonitor record-decision`. Maskinella detaljer
ligger under `artifacts/experiments/`, som är lokala och ignorerade av Git.

## candidate-003-quiet-see

- Status: `rejected`
- Rekommendation: `reject`
- Nästa ändring: Create candidate 4 with depth-dependent history bonuses and maluses.
- Hypotes: Depth-scaled history updates improve quiet-move ordering and search efficiency without changing chess correctness.
- Orsak: All correctness gates passed, but median fixed-depth NPS changed by -0.46%. Fixed-depth behavior changed broadly and movetime selected a different bestmove in 3 of 14 positions, so the expected speed benefit is absent and screening is not justified.

## candidate-004-depth-history

- Status: `awaiting_approval`
- Rekommendation: `screening`
- Nästa ändring: Run the 400-game paired screening at 20+0.2 before considering SPRT.
- Hypotes: Depth-dependent history updates may improve move ordering and playing strength despite a small fixed-depth NPS decrease.
- Orsak: All correctness gates passed. The candidate changes search behavior as intended, with a -0.70% median NPS change and mixed fixed-depth deltas. Because this is a strength-oriented history change, a 400-game screening is justified; SPRT remains separately approval-gated.

## candidate-004-depth-history

- Status: `promoted`
- Rekommendation: `promote`
- Nästa ändring: Use integrated commit 6c787b7 as the baseline for the next candidate.
- Hypotes: Depth-dependent history updates improve move ordering and playing strength despite a small fixed-depth NPS decrease.
- Orsak: All correctness gates passed. Screening passed with 103 wins, 216 draws and 81 losses (52.8%). SPRT then accepted H1 after 3079 games with 699 wins, 1789 draws and 591 losses (51.8%); LLR 2.95 exceeded the upper bound 2.94. The previous invalid_unpaired_openings result was caused only by the obsolete strict pairing policy: one opening was incomplete because games ran in parallel. Explicit approval was granted and the candidate was integrated as commit 6c787b7.

## candidate-005-lmr

- Status: `awaiting_approval`
- Rekommendation: `screening`
- Nästa ändring: Run the 400-game paired screening at 20+0.2; let automatic evaluation decide whether to start SPRT.
- Hypotes: Depth- and move-number-based LMR may improve playing strength by spending search effort on more relevant moves despite lower fixed-depth NPS.
- Orsak: All correctness gates passed. The candidate intentionally changes search behavior, with a -2.51% median fixed-depth NPS delta and changed bestmove in 3 of 14 positions. LMR is a strength-oriented change, so a paired 400-game screening is required before accepting or rejecting it.

## candidate-005-lmr

- Status: `promoted`
- Rekommendation: `promote`
- Nästa ändring: Use integrated commit a9647fa as the baseline for the next candidate.
- Hypotes: Depth- and move-number-based LMR improves playing strength despite intentionally changed fixed-depth search behavior.
- Orsak: All correctness gates passed. Screening scored 53.6%. SPRT accepted H1 after 2203 games with 534 wins, 1241 draws and 428 losses (52.4%); LLR 2.97 exceeded the upper bound 2.94. Fixed-depth changes are expected because this candidate was explicitly not semantic-preserving. Explicit human approval was granted and the candidate was integrated as commit a9647fa.

## candidate-006-lmr-research

- Status: `awaiting_approval`
- Rekommendation: `screening`
- Nästa ändring: Run the 400-game paired screening at 20+0.2 before considering SPRT.
- Hypotes: The completed LMR re-search ladder improves playing strength by avoiding unnecessary full-window searches despite lower fixed-depth NPS.
- Orsak: All hard correctness gates passed. The candidate intentionally changes search behavior, with a -2.28% median NPS delta and four changed bestmoves in 14 fixed-depth positions. A strength match is required to determine whether the more selective re-search sequence is beneficial.

## candidate-019-syzygy

- Status: `promoted`
- Rekommendation: `promote`
- Nästa ändring: Inför en försiktig mittspelsvärdering av bondestorm mot motståndarens rockerade kung.
- Hypotes: Valfri Syzygy-probering ger exakta fåpjäsbeslut när tabeller finns utan att försämra normalt spel eller det inbyggda remiskyddet när tabeller saknas.
- Orsak: Alla korrekthetstester, den deterministiska Syzygy-sviten och den cgo-fria reservvägen godkändes. `IsDeadMaterial` körs före Syzygy i sökträdet och fungerar utan externa tabeller. Screeningen med Syzygy avstängt gav förväntat neutrala 49,6 procent över 400 partier. Den automatiskt startade SPRT-körningen stoppades vid 0 partier eftersom den inte kunde mäta Syzygy-funktionen.

## candidate-020-shelter-average

- Status: `promoted`
- Rekommendation: `promote`
- Nästa ändring: Inför en försiktig mittspelsvärdering av bondestorm mot motståndarens rockerade kung.
- Hypotes: Återställd medelvärdesberäkning för möjlig rockad ger avsedd kungssäkerhetsvärdering utan att försämra spelstyrkan.
- Orsak: Kandidaten korrigerar en regression där den möjliga rockadens bondeskydd i praktiken dividerades med 200 i stället för att medelvärdesbildas. Alla tester godkändes och screeningen slutade 109 vinster, 183 remier och 108 förluster (50,1 procent). Ändringen promoveras som korrekthetsbaseline trots neutral Elo.
