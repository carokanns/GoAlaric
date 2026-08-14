# GoAlaric optimizer v1.1.1 – explorativ sökning

v1.1.1 skiljer sökfasens beslut från bekräftelsefasens statistiska beslut.
Någon lång nattkampanj ska inte startas innan denna verifiering är grön.

## Konfiguration

Explorativ sökning är avstängd som standard. Slå på den per kampanj:

```json
{
  "goals": {
    "optimizer": {
      "parameters": ["activity_bias"],
      "exploratory": {
        "enabled": true,
        "min_score": 51.0
      }
    }
  }
}
```

`min_score` är en punktresultatgräns i procent och måste ligga mellan 0 och
100. Gränsen är strikt: `score > min_score` accepterar explorativt. Ett
maximalt men osäkert kandidatblock med lägre eller lika score får
`reject_exploratory`.

## Beslutsgränser

- `accept`: strikt adaptivt konfidensbeslut; får flytta ankaret.
- `reject` eller `reject_early`: strikt avslag; flyttar inte ankaret.
- `uncertain` i strikt sökläge: flyttar inte ankaret.
- `accept_exploratory`: punktresultat över gränsen efter maximal sökbudget;
  flyttar ankaret men märks alltid `exploratory: true`.
- `reject_exploratory`: punktresultat på eller under gränsen; flyttar inte
  ankaret.

Explorativ policy sparas i SQLite-checkpointen. En återstart med annan policy
stoppas i stället för att blanda två beslutssystem i samma kampanj.

## Verifiering före nattkampanj

Kör från repots rot, utan LLM:

```bash
PYTHONPATH=optimizer/src optimizer/.venv/bin/python \
  -m unittest optimizer.tests.test_phase16
PYTHONPATH=optimizer/src optimizer/.venv/bin/python \
  -m unittest discover -s optimizer/tests -p 'test_*.py'
```

Kontrollera därefter:

```bash
optimizer/.venv/bin/python -c 'import goalaric_optimizer; print(goalaric_optimizer.__version__)'
git status --short
```

Den fasta bekräftelsen behåller sitt strikta 95-procentskrav. Explorativt
accepterade kandidater får inte kallas statistiskt bekräftade och får inte
promoveras automatiskt. Vid `rejected` eller `inconclusive` skapas ingen
rekommendationsfil.
