# GoAlaric optimizer v1.2 – matchprofiler

## Syfte

Sökning och fast bekräftelse kan välja olika namngivna profiler utan ändring av
koordinatsökningen. En profil har exakt ett läge: tid (`tc`) eller fast
nodbudget per drag (`nodes`).

## Konfiguration

Profiler ligger under `goals.real.profiles`. Varje profil måste ha en unik
namnnyckel och exakt ett icke-tomt `tc`-värde eller positivt `nodes`-värde.
`goals.optimizer.profile` väljer
sökprofilen och `goals.confirmation.profile` väljer bekräftelseprofilen.

Om `profiles` saknas används `goals.real.tc` som profilen `default`. Detta är
beteendemässigt bakåtkompatibelt med äldre kampanjfiler.

```json
{
  "goals": {
    "real": {
      "tc": "0.2+0.01",
      "profiles": {
        "long-search": {"tc": "1+0.02"},
        "node-search": {"nodes": 100000},
        "node-confirmation": {"nodes": 250000}
      }
    },
    "optimizer": {"profile": "node-search"},
    "confirmation": {
      "enabled": true,
      "games": 100,
      "seed": 20260930,
      "confidence": 0.95,
      "profile": "node-confirmation"
    }
  }
}
```

## Identitet och återstart

En upplöst profil sparas som namn, hash, läge och faktisk gräns i
optimizer-checkpointen, trials, matchblockens result-json och
confirmation-tabellen. Vid återstart måste samma profilidentitet användas. En
ändrad profil eller nodbudget avvisas i stället för att blanda körprofiler i
samma kampanj.

## Livekontroll

```bash
source optimizer/.venv/bin/activate
optimizer optimize campaign.json --data-dir artifacts/campaigns
optimizer status <campaign-id> --data-dir artifacts/campaigns --watch --interval 1
optimizer dashboard <campaign-id> --data-dir artifacts/campaigns \
  --listen 127.0.0.1:8787 --refresh-ms 500
```

Status, dashboard och rapport visar profilens namn och faktiska gräns. För
nodeprofiler visas exempelvis `node-search · 100000 nodes/move`. Real runnerns
`monitor-config.json` är den primära artefakten för att kontrollera att
testmonitor tog emot `nodes` utan `time_control`; Fastchess-kommandot använder
sedan `-each nodes=100000`. Tidsprofiler använder motsvarande `-each tc=...`.

## Verifiering

Kör fake-testet och det lilla riktiga testet separat:

```bash
PYTHONPATH=optimizer/src optimizer/.venv/bin/python \
  -m unittest optimizer.tests.test_phase18.Phase18FakeProfileTest
PYTHONPATH=optimizer/src optimizer/.venv/bin/python \
  -m unittest optimizer.tests.test_phase18.Phase18MinimalRealProfileTest
PYTHONPATH=optimizer/src optimizer/.venv/bin/python \
  -m unittest optimizer.tests.test_phase22
```

Det riktiga tidsprofiltestet använder samma kandidat i två isolerade kampanjer,
först med `0.2+0.01` och därefter med `1+0.02`. Nodeprofiltestet kör två riktiga
partier med `nodes=100000`, separata parameterfiler och samma motorbinär.
Tillsammans kontrollerar testen profilhash, SQLite-resultat,
`monitor-config.json`, blockrapport, PGN-noder/seldepth och att inga
schedulerprocesser finns kvar. Ingen nodbudget påverkar sökalgoritmens beslut.
