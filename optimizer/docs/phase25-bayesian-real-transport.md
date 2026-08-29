# Bayesiansk transport till verkliga matchkedjan

Detta delmål kopplar `finite-noise-aware-bo-v1` till den befintliga kedjan
testmonitor → Fastchess → GoAlaric. Själva delmålsverifieringen använder en
SQLite-backed fake block-runner; inga realprocesser krävs före smoke-piloten.

## Matchmodell

- Den ursprungliga baselinepunkten läggs in i modellen som exakt 50 % med
  mycket liten känd varians. Någon meningslös baseline-mot-baseline-match
  startas därför inte och den kostar inga partier.
- Varje föreslagen kandidat jämförs mot den ursprungliga baselinefilen.
- `pairs_per_evaluation` är ett fast antal kompletta öppningspar. Adaptiv
  tidig gallring används inte inne i denna evaluering.
- Varje öppningspar är ett separat SQLite-matchblock. Dess två resultat
  omvandlas till en pentanomial parpoäng i mängden 0, 0,5, 1, 1,5 eller 2.
- Den valda tids- eller nodprofilen följer kandidaten hela vägen till
  testmonitor och Fastchess.

Exempel:

```json
{
  "goals": {
    "max_games": 64,
    "max_evaluations": 4,
    "optimizer": {
      "algorithm": "finite-noise-aware-bo-v1",
      "parameters": ["lmr_divisor_x100", "lmp_move_multiplier"],
      "initial_points": 3,
      "pairs_per_evaluation": 8,
      "profile": "node-search"
    }
  }
}
```

`max_games` begränsar antalet hela kandidatevalueringar. En budget som inte
rymmer nästa fullständiga evaluering startar den inte.

## Återstart

Förslaget ligger kvar som `pending` tills alla dess par är färdiga och `tell`
har checkats in. Vid återstart:

1. en övergiven processgrupp avslutas,
2. ett eventuellt `running`-block återställs till `interrupted`,
3. samma trial och samma blockidentiteter återanvänds,
4. endast saknade par körs,
5. observation och optimizer-checkpoint skrivs atomiskt.

En terminal matchtrial vars Bayesian-observation ännu inte hann skrivas kan
återanvändas utan en ny match.

## Verifiering

Fake-transporttestet verifierar tre fasta öppningspar, pentanomialen
`[2, 1, 0]`, nodprofilen `100000 nodes/move`, avbrott efter första paret,
återstart, exakt sex partier, tre unika block och högst ett försök per block.

Automatisk slutbekräftelse och den minimala verkliga smoke-piloten tillhör
nästa delmål.
