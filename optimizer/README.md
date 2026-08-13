# GoAlaric optimizer – fas 5

Fas 5 innehåller den lokala Python-/SQLite-kärnan. Den använder endast
Python-standardbiblioteket och skriver aldrig till Go-motorn, baseline eller
dashboarden.

Installera paketet lokalt eller använd `PYTHONPATH` från repots rot:

```bash
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer init campaign.json
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer status <campaign-id>
```

En kampanjfil använder `schema_version: 1`, ett JSON-register, ett fast
`master_seed`, baselineidentitet och öppningspartitioner. `mode: fake` är den
enda körvägen i fas 5; den ändrar bara kampanjens SQLite-status och startar
inga matcher eller schedulerjobb.

Exempel på kontrollflöde:

```bash
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer init campaign.json
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer run <campaign-id>
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer pause <campaign-id>
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer resume <campaign-id>
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer status <campaign-id> --watch
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer stop <campaign-id>
```

Databasen använder WAL, foreign keys, centrala statusövergångar, unika
parameter- och blockidentiteter samt append-only events. Checkpoint och färdigt
matchblock skrivs i samma SQLite-transaktion. Statuskommandon öppnar databasen
read-only och skriver inga events.
