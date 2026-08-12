# Specifikation: automatisk parameteroptimerare för GoAlaric

## 1. Syfte

Bygg ett fristående optimeringssystem som långsamt och reproducerbart söker
bättre evaluerings- och sökparametrar för GoAlaric.

Systemet ska kunna köras i flera dygn, stoppas när som helst och fortsätta
från senaste säkra checkpoint. Det får aldrig automatiskt ändra baseline,
`current.json` eller källkod.

Målet är inte ett matematiskt globalt optimum. Målet är att hitta
parameteruppsättningar som ger statistiskt bättre spelstyrka i en definierad
testmiljö och som kan reproduceras och verifieras senare.

## 2. Övergripande arkitektur

```text
Python-optimerare
  ├── parameterregister
  ├── förslagsalgoritmer
  ├── SQLite-databas och checkpoints
  ├── jobbkö och stopp/återstart
  └── terminalstatus / lokal read-only dashboard
              │
              ▼
Go testmonitor
  ├── bygger eller startar kandidat
  ├── kör sanitets- och deterministiska tester
  ├── kör återstartbara matchblock
  ├── samlar W-D-L, score, Elo och metadata
  └── returnerar kompakt resultatpaket
              │
              ▼
GoAlaric
  └── läser en versionsmärkt, validerad parameterfil
```

Ansvarsgränsen ska vara tydlig:

- Python bestämmer nästa parameterförslag och analyserar resultat.
- GoAlaric läser och använder parametrar i motorn.
- `cmd/testmonitor` verifierar kandidater och kör matcher.
- Ingen LLM behövs under optimeringen.
- LLM kan senare läsa kompakta resultat och föreslå beslut, men ingår inte i
  själva optimeringsloopen.

## 3. Grundprinciper

- En kandidat behandlas åt gången.
- Baseline är fast under hela optimeringskampanjen.
- Varje parameteruppsättning får ett kanoniskt JSON-format och en hash.
- Samma parameteruppsättning får aldrig testas två gånger av misstag.
- Alla motorinställningar, binäridentiteter, seeds och öppningsdata sparas.
- Eval- och sökparametrar optimeras i separata kampanjer.
- Styrka är huvudmålet. Remifrekvens, NPS, nodantal och djup rapporteras
  separat och blandas inte in i styrkemålet utan uttrycklig viktning.
- Kandidater kan förkastas automatiskt, men promotion kräver mänskligt
  godkännande.

## 4. Parameterregister

Alla justerbara parametrar ska beskrivas i ett versionshanterat JSON-register,
exempelvis `optimizer/registries/eval-pilot-v1.json`. JSON används i första
versionen för att undvika externa parserberoenden och för att förenkla
kanonisk hashning.

Varje parameter ska ha:

- stabilt namn,
- grupp, exempelvis `eval_pawn`, `eval_king_safety` eller `search_lmr`,
- typ: heltal, flyttal, boolesk eller kategorisk,
- minimi- och maximivärde,
- tillåten steglängd eller diskreta värden,
- standardvärde från baseline,
- kort beskrivning,
- beroenden och begränsningar,
- klass: `eval`, `search`, `correctness` eller `mixed`,
- om ändringen kräver ombyggnad eller endast ny parameterfil.

Exempel:

```json
{
  "version": 1,
  "parameters": [
    {
      "name": "lmr_base",
      "group": "search_lmr",
      "type": "integer",
      "min": 1,
      "max": 8,
      "step": 1,
      "default": 4,
      "change_class": "search",
      "runtime": "parameter_file"
    }
  ]
}
```

Det gamla anonyma parameterarrayformatet i GoAlaric ska inte vara det
officiella gränssnittet för optimeraren. Parametrarna ska få stabila namn och
konverteras centralt till motorns interna representation.

## 5. Parameteröverföring till motorn

Föredraget gränssnitt för Fastchess och `testmonitor` är en UCI-option:

```text
setoption name ParameterFile value /absolut/sökväg/trial-000123.json
```

Ett senare kommandoradsargument kan anropa samma centrala laddare:

```bash
goalaric --parameters trial-000123.json
```

Själva parameterfilen är det versionsmärkta och hashbara gränssnittet; UCI och
kommandoraden är endast två sätt att välja filen.

Regler:

- Parameterfilen läses före första sökningen.
- Parametrar valideras mot registerversionen.
- Ogiltiga, saknade eller okända parametrar ska ge ett tydligt fel.
- Parametrar ändras aldrig under ett parti.
- En match använder samma parameterfil i alla sina partier.
- Parameterfilen ska kunna återges exakt från SQLite och artefaktkatalogen.
- Baseline använder också en explicit, materialiserad standardparameterfil.

## 6. Binär- och parameteridentitet

En motorinstansidentitet ska bestå av minst:

```text
engine_sha256
parameter_file_sha256
parameter_register_version
```

För källspårbarhet sparas även:

```text
git_commit
git_branch
build_vcs_revision
```

Testsystemet ska inte anta att en binär kommer från Git-katalogen där den
råkar ligga. Commit ska hämtas från binärens inbäddade VCS-information när den
finns; SHA-256 är alltid den säkra identiteten.

Två försök får inte förväxlas om de använder samma binär men olika
parameterfiler. Testmonitors vanliga spärr mot identiska binärer ska behållas;
samma binär får endast möta sig själv i ett uttryckligt optimizerläge där
parameteridentiteterna är giltiga och olika.

## 7. Optimeringsmetoder

Olika parameterfamiljer kräver olika metoder.

| Situation | Metod |
|---|---|
| En parameter | Intervall- eller stegvis sökning |
| 2–10 samverkande parametrar | CMA-ES eller Bayesiansk optimering |
| Många numeriska sökparametrar | SPSA |
| Många evalvikter | Texel-liknande tuning |
| Booleska/kategoriska val | Separata A/B-försök |

Första versionen ska optimera en logisk grupp åt gången. Alla eval- och
sökparametrar ska inte släppas fria samtidigt, eftersom resultatet då blir
svårt att tolka och lätt överanpassas till testmiljön.

### 7.1 Evalparametrar

Evalparametrar kan först gallras billigt mot ett stort positionsmaterial från
riktiga partier:

1. Samla positioner, FEN och slutresultat.
2. Dela materialet deterministiskt i träning, validering och slutprov.
3. Beräkna evalvärden för positionerna.
4. Optimera parametrar med Texel-liknande logistisk förlust.
5. Kontrollera resultatet på orörda positioner.
6. Bygg en riktig kandidat och verifiera den med matcher.

Det statiska resultatet är endast en gallring. Kandidat 023 visade att bättre
positionsförlust inte automatiskt betyder bättre spelstyrka.

### 7.2 Sökparametrar

LMR, LMP, null move, aspiration, history, killer och liknande ska huvudsakligen
optimeras med riktiga matcher. Effekten beror på hela sökträdet och kan inte
värderas tillförlitligt från statiska positioner.

SPSA är lämpligt som första metod:

1. Skapa två närliggande uppsättningar `theta+` och `theta-`.
2. Spela dem med samma parade öppningsblock.
3. Beräkna resultatskillnaden.
4. Uppdatera parametrarna i riktning mot den observerade förbättringen.
5. Spara optimizer-state efter varje iteration.
6. Testa regelbundet den bästa uppsättningen mot den fasta baselinen.

## 8. Datamaterial och läckageskydd

Optimeringen ska använda tre separata datamängder:

- träningsdata för att föreslå parametrar,
- kontroll-/valideringsdata för återkommande bedömning,
- ett orört slutprov för promotionskandidater.

Öppningsgrupper ska delas deterministiskt. Samma öppningspar, inklusive
färgvänt par, får inte hamna i olika partitioner.

Varje match ska ha:

- fast eller explicit sparad seed,
- versionsidentifierad öppningsbok,
- tidskontroll,
- hash/concurrency/threads,
- pondering-inställning,
- Syzygy-inställning,
- contempt och remiregler,
- baseline- och kandidatidentitet.

När närliggande parameteruppsättningar jämförs bör de använda samma
öppningsblock. Vid slutlig bekräftelse ska minst en ny seed användas.

## 9. Successiv testkedja

Varje parameterförslag passerar successivt dyrare nivåer.

### Nivå 1: sanitetskontroll

- parameterfilen valideras,
- motorn startar,
- `uci` och `isready` fungerar,
- kort perft körs,
- motorn kraschar inte.

### Nivå 2: deterministisk kontroll

- fixed-depth bestmove, score och noder registreras,
- ändringar är tillåtna för eval- och sökklasser,
- anomalier rapporteras,
- implementation-klassen kan kräva exakt ekvivalens.

### Nivå 3: snabb gallring

40–100 parade partier med låg kostnad. Uppenbart svaga kandidater stoppas.

### Nivå 4: ordinarie screening

400 parade partier, normalt `10+0.1`, med fast seed och samma öppningsurval för
alla jämförda kandidater.

### Nivå 5: bekräftelse

Ny seed och helst längre tidskontroll för kandidater som ser lovande ut.

### Nivå 6: SPRT

SPRT körs endast för kandidater som passerat screening och övriga hårda tester.
SPRT ska använda den fastställda kampanjpolicyn och separat resultatpaket.

## 10. Återstartbara matchblock

En lång match ska delas i deterministiska block, exempelvis 10 eller 20
öppningspar. SQLite lagrar blockstatus och resultat.

Efter varje avslutat block sparas data atomiskt:

- W-D-L,
- score,
- aktuell Elo-estimering,
- öppningsblockets seed och identitet,
- eventuella fel,
- lokala rapportlänkar,
- checkpoint för optimeraren.

Vid avbrott:

- avslutade block lämnas orörda,
- ett ofullständigt block markeras `interrupted` och spelas om från början,
- inga färdiga partier räknas dubbelt,
- nästa körning återupptar första saknade block.

Fastchess behöver alltså inte fortsätta mitt i en gammal process. Detta gör att
ett strömavbrott normalt högst förlorar ett pågående block.

## 11. SQLite och beständighet

SQLite är optimerarens enda sanningskälla. WAL-läge ska användas.

Minsta tabeller:

```text
campaigns
parameter_sets
trials
match_blocks
games
optimizer_state
artifacts
events
```

### `campaigns`

Kampanjens namn, baseline, registerversion, mål, status och tidsstämplar.

### `parameter_sets`

Kanonisk parameter-JSON, parameterhash, grupp och skapad tidpunkt.

### `trials`

Ett optimeringsförsök med parameterhash, status, algoritm, seed och resultat.

### `match_blocks`

Blocknummer, öppningslista, status, PID, W-D-L, score och rapportvägar.

### `games`

Kompakt spelresultat och identiteter. Full PGN sparas som lokal artefakt och
behöver inte läsas av optimeraren vid varje beslut.

### `optimizer_state`

Serialiserat tillstånd för SPSA/CMA-ES, nästa iteration, bästa kandidat och
slumptalsgeneratorns state.

### `events`

Append-only-historik för start, checkpoint, avbrott, fel, återstart och
slutstatus.

Alla skrivningar som påverkar återstart ska ske i transaktioner. En checkpoint
får inte rapporteras som sparad innan SQLite-transaktionen är committad.

## 12. Status, stopp och återstart

Optimeraren ska kunna köras utan LLM och utan dashboard.

Minimikommandon:

```bash
optimizer start campaign.json
optimizer status
optimizer status --watch
optimizer best
optimizer trials --last 20
optimizer show <trial-id>
optimizer pause
optimizer resume
optimizer stop
```

`status --watch` ska endast läsa databasen och får inte skriva statusposter.

Status bör visa:

```text
Campaign: eval-pawn-weights-001
State: running
Runtime: 2d 04h 17m
Trials: 87 completed, 1 running, 12 pending
Games: 18 640
Current trial: 88, block 7/20, games 126/400
Current W-D-L: 35-61-30, score 52.0%
Best confirmed: trial 61, estimated +12 Elo
Last checkpoint: 14:37:18
```

Ctrl+C, SIGTERM och en stoppfil ska ge en kontrollerad avstängning:

1. skapa inga nya jobb,
2. låt pågående block avslutas om möjligt,
3. stoppa motorprocesser kontrollerat,
4. spara resultat och optimizer-state,
5. markera försöket `interrupted`,
6. avsluta.

Statusar:

```text
pending, running, completed, failed, interrupted, paused, rejected
```

Efter omstart ska avslutade jobb lämnas orörda, samma parameterhashar inte
skapas igen och optimeringsalgoritmen fortsätta från senaste checkpoint.

## 13. Lokal dashboard

Efter att terminalflödet är stabilt kan en liten skrivskyddad dashboard byggas:

```bash
optimizer dashboard --listen 127.0.0.1 --port 8080
```

Den ska endast läsa SQLite och visa:

- kampanjstatus,
- aktuellt försök och matchblock,
- W-D-L och score,
- bästa parameteruppsättningar,
- Elo med osäkerhetsintervall,
- historisk utveckling,
- processortid,
- fel och stopp,
- parameterdifferenser mot baseline.

Ingen åtkomst från andra maskiner behövs. Fjärråtkomst sker vid behov via
RustDesk.

## 14. Promotion och säkerhet

Optimeraren får aldrig:

- ändra baseline,
- ändra `current.json`,
- skriva över källkod,
- automatiskt pusha eller promovera,
- starta SPRT utan screening-gate.

Den får skapa ett promotionsförslag med:

- bästa parameteruppsättning,
- full matchhistorik,
- Elo-estimat och osäkerhetsintervall,
- resultat från flera seeds,
- resultat vid längre tidskontroll,
- remi-, nod- och NPS-statistik,
- parameterfilens hash,
- motorbinärens SHA-256 och commit,
- reproducerbart kommando.

Promotion sker först efter separat mänskligt godkännande.

## 15. Första implementeringsomgången

Första versionen ska vara liten och lokal:

1. Pythonprogram med CLI.
2. JSON-register för en enda evalgrupp.
3. Parameterfil som GoAlaric kan läsa och validera.
4. SQLite med WAL och atomiska checkpoints.
5. Deterministisk jobbkö med återstartbara matchblock.
6. Anrop till befintlig `cmd/testmonitor`.
7. `status`, `status --watch`, `pause`, `resume` och `stop`.
8. Deterministisk koordinatsökning och Texel-liknande gallring för en liten
   evalgrupp.
9. 40–100 partiers gallring och därefter 400-partiers screening.
10. Promotionsrapport utan automatisk promotion.

Ingen webbserver, distribuerad körning eller SPSA för alla sökparametrar behövs
i första versionen.

## 16. Testplan för optimeringssystemet

### Enhetstester

- parameterregister och typvalidering,
- min/max och beroenden,
- kanonisk JSON och parameterhash,
- deterministisk seed- och öppningsblockgenerering,
- SQLite-transaktioner,
- checkpoint och återstart,
- duplicate prevention,
- SPSA/Texel-state,
- statusrendering.

### Integrationstester

- falsk motor,
- falsk Fastchess,
- avbrott mitt i ett block,
- omstart efter avbrott,
- upprepad blockhantering,
- identiska parameterhashar,
- parameterfil med felaktig version,
- saknad eller modifierad binär,
- matchresultat som återupptas utan dubbelräkning.

### Acceptanstest

En testkampanj ska kunna:

1. startas,
2. köras tills flera block är färdiga,
3. stoppas med Ctrl+C,
4. startas igen,
5. fortsätta på rätt block,
6. visa samma tidigare resultat,
7. slutföras med ett reproducerbart promotionsförslag.

## 17. Öppen designfråga

Innan full implementation måste parameterregistret fastställas. Börja med en
liten, välförstådd grupp, exempelvis 5–10 evalparametrar. Lägg inte samtidigt
till passed-pawn-logik, remidetektorer, LMR, LMP och historyparametrar.

När flödet har bevisats stabilt kan nästa kampanjgrupp vara en liten SPSA-grupp
för sökningen.
