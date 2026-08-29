# Bayesiansk ask/tell och SQLite-återstart

Detta delmål kopplar den ändliga, brusmedvetna BoTorch-sökningen till
optimerarens SQLite-checkpoint och fake-runner. Inga motorprocesser används.

## Fake-konfiguration

Det nya läget väljs uttryckligen; befintliga kampanjer fortsätter med
`coordinate-multires-v1`.

```json
{
  "goals": {
    "max_evaluations": 12,
    "optimizer": {
      "algorithm": "finite-noise-aware-bo-v1",
      "parameters": ["lmr_divisor_x100", "lmp_move_multiplier"],
      "initial_points": 5,
      "pairs_per_evaluation": 16
    },
    "fake_match": {
      "optimum": {
        "lmr_divisor_x100": 200,
        "lmp_move_multiplier": 4
      }
    }
  }
}
```

Kampanjen körs och återupptas med det vanliga kommandot:

```bash
optimizer optimize campaign.json --data-dir campaigns --max-results 1
```

`--max-results 1` returnerar först efter att ett komplett fake-resultat,
observationen och den efterföljande optimizer-checkpointen har skrivits.

## Atomisk modell

`bayesian_proposals` är den beständiga `ask`-kön. Ett avbrott efter `ask`
återanvänder exakt samma väntande förslag. `bayesian_observations` innehåller
parpoäng, score, skattad varians och fullständigt runnerresultat.

En `tell` skriver följande i samma SQLite-transaktion:

1. observationen,
2. förslagets terminala status,
3. optimizer-checkpointens nya revision och hash,
4. terminal kampanjstatus när budgeten är slut.

Modellidentiteten omfattar algoritm, seed, registerhash, sökrymd,
initialdesign, parbudget, evalueringsbudget samt Torch- och BoTorch-version.
En ändrad identitet avvisas vid återstart.

## Verifierad gräns

Fake-flödet är verifierat som en sammanhängande körning och som upprepade
invokeringar med ett resultat åt gången. Förslagsföljd, parutfall,
observationer, slutcheckpoint och budget är identiska. Schema 5 migreras
idempotent till schema 6 utan att äldre kampanjdata ändras.

Real-runner, adaptiva fidelity-beslut, confirmation och dashboardens
Bayes-vy tillhör efterföljande delmål. Det här delmålet startar inte
Fastchess, testmonitor eller GoAlaric.
