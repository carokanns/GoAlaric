# Experimentlogg

Den här filen fylls på av `testmonitor record-decision`. Maskinella detaljer
ligger under `artifacts/experiments/`, som är lokala och ignorerade av Git.

## candidate-003-quiet-see

- Status: `rejected`
- Rekommendation: `reject`
- Nästa ändring: Create candidate 4 with depth-dependent history bonuses and maluses.
- Hypotes: Depth-scaled history updates improve quiet-move ordering and search efficiency without changing chess correctness.
- Orsak: All correctness gates passed, but median fixed-depth NPS changed by -0.46%. Fixed-depth behavior changed broadly and movetime selected a different bestmove in 3 of 14 positions, so the expected speed benefit is absent and screening is not justified.
