# GoAlaric optimizer v1.2 – matchprofiler

## Syfte

Det första v1.2-delmålet verifierar namngivna tidskontroller utan nodbudget
och utan ändring av koordinatsökningen. Sökning och fast bekräftelse kan välja
olika profiler.

## Konfiguration

Profiler ligger under `goals.real.profiles`. Varje profil måste ha en unik
namnnyckel och ett icke-tomt `tc`-värde. `goals.optimizer.profile` väljer
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
        "long-confirmation": {"tc": "2+0.02"}
      }
    },
    "optimizer": {"profile": "long-search"},
    "confirmation": {
      "enabled": true,
      "games": 100,
      "seed": 20260930,
      "confidence": 0.95,
      "profile": "long-confirmation"
    }
  }
}
```

## Identitet och återstart

En upplöst profil sparas som namn, hash och `tc` i optimizer-checkpointen,
trials, matchblockens result-json och confirmation-tabellen. Vid återstart
måste samma profilidentitet användas. En ändrad profil avvisas i stället för
att blanda tidskontroller i samma kampanj.

## Livekontroll

```bash
source optimizer/.venv/bin/activate
optimizer optimize campaign.json --data-dir artifacts/campaigns
optimizer status <campaign-id> --data-dir artifacts/campaigns --watch --interval 1
optimizer dashboard <campaign-id> --data-dir artifacts/campaigns \
  --listen 127.0.0.1:8787 --refresh-ms 500
```

Status, dashboard och rapport visar profilens namn och faktiska `tc`. Real
runnerns `monitor-config.json` är den primära artefakten för att kontrollera
vilken tidskontroll testmonitor tog emot; testmonitor använder den sedan i
Fastchess-kommandot.

## Verifiering

Kör fake-testet och det lilla riktiga testet separat:

```bash
PYTHONPATH=optimizer/src optimizer/.venv/bin/python \
  -m unittest optimizer.tests.test_phase18.Phase18FakeProfileTest
PYTHONPATH=optimizer/src optimizer/.venv/bin/python \
  -m unittest optimizer.tests.test_phase18.Phase18MinimalRealProfileTest
```

Det riktiga testet använder samma kandidat i två isolerade kampanjer, först
med `0.2+0.01` och därefter med `1+0.02`. Det kontrollerar profilhash, tc i
SQLite-resultatet, `monitor-config.json`, komplett tvåpartiersblock och att
inga schedulerprocesser finns kvar. Profiltestet använder en befintlig
eval-parameter enbart som plumbingtest; någon LMR- eller annan sökparameter
läggs inte till i detta delmål.
