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
