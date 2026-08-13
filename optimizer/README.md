# GoAlaric optimizer – fas 6

Fas 6 lägger en säker, sekventiell scheduler ovanpå den lokala
Python-/SQLite-kärnan. Den använder endast Python-standardbiblioteket och
skriver aldrig till Go-motorn, baseline eller dashboarden. Verkliga matcher,
Fastchess och optimeringsalgoritmer är fortfarande avsiktligt avstängda.

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

Fas 6 kan köras med en falsk testmonitor som får blockidentitet och
`result.json`-sökväg via argument. Monitorn ska skriva resultatet först när
hela blocket är färdigt:

```bash
PYTHONPATH=optimizer/src python3 -m goalaric_optimizer run <campaign-id> \
  --fake --monitor-command 'python3 fake_testmonitor.py' \
  --blocks 3 --pairs-per-block 1
```

Schedulern kör högst ett block åt gången och startar monitorn i en egen Unix-
processgrupp. `pause` och `stop` avslutar hela gruppen; det pågående blocket
blir `interrupted` och räknas inte. Nästa `resume` eller nya `run` spelar om
samma blockidentitet. En död monitor återställs på samma sätt. SQLite-WAL är
fortsatt enda sanningskällan.

Databasen använder WAL, foreign keys, centrala statusövergångar, unika
parameter- och blockidentiteter samt append-only events. Checkpoint och färdigt
matchblock skrivs i samma SQLite-transaktion. Statuskommandon öppnar databasen
read-only och skriver inga events.
