# Genomförandeplan: automatisk parameteroptimerare för GoAlaric

## 1. Mål för genomförandet

Genomförandet ska stegvis leverera ett lokalt system som kan:

1. köra samma GoAlaric-binär med olika, verifierade parameteruppsättningar,
2. föreslå och köa optimeringsförsök deterministiskt,
3. köra parade matcher i återstartbara block,
4. spara allt nödvändigt till SQLite,
5. pausas, stoppas och fortsätta utan dubbelräkning,
6. visa status utan att störa körningen,
7. skapa ett reproducerbart promotionsförslag,
8. aldrig automatiskt ändra baseline eller källkod.

Planen är uppdelad i små leveranser. Varje fas har en acceptansgrind som måste
vara godkänd innan nästa fas påbörjas.

## 2. Korrigerade designbeslut

Följande beslut gör den första versionen enklare och säkrare än en alltför bred
första implementation.

### 2.1 Egen ren utvecklingsgren

Arbetet ska göras i en separat worktree och gren baserad på den aktuella
baselinecommitten. Huvudarbetskopians orelaterade eller ocommittade filer får
inte följa med.

Optimeringssystemet är ett verktygsprojekt. En senare motorändring med nya
parametervärden ska fortfarande bli en separat kandidat.

### 2.2 JSON i första versionen

Parameterregister, parameterfiler och kampanjkonfiguration använder JSON i
version 1. Det undviker ett externt YAML-beroende och ger ett entydigt
kanoniskt format för hashning.

YAML kan läggas till senare som ett användarvänligt indataformat, men ska då
normaliseras till samma kanoniska JSON innan identitet beräknas.

### 2.3 Ett gemensamt motor-API för parameterinläsning

GoAlaric ska ha en central parameterladdare. För matchintegration exponeras
den primärt som UCI-option:

```text
setoption name ParameterFile value /absolut/sökväg/trial-000123.json
```

Fastchess kan då ge baseline och kandidat varsin parameterfil trots att de
använder samma motorbinär.

Ett kommandoradsargument kan senare anropa samma laddare:

```bash
goalaric --parameters trial-000123.json
```

Regler:

- filen får sättas innan första sökningen,
- `isready` ska inte svara `readyok` om filen är ogiltig,
- parametrarna är låsta medan en sökning eller match pågår,
- ny parameterfil kräver en verklig rensning av TT-, eval- och pawn-cache,
- varje motorprocess använder exakt en parameteruppsättning under ett parti.

### 2.4 Baseline har också en parameteridentitet

Även baseline ska ha en explicit parameterfil, genererad från baselines
standardvärden. Därmed jämförs alltid två fullständiga motorinstanser:

```text
engine SHA-256 + parameter SHA-256 + registerversion
```

En saknad parameterfil får inte betyda olika saker i olika delar av systemet.
Om standardvärden tillåts ska de materialiseras som en kanonisk fil före
matchstart.

### 2.5 Standardspärren för identiska binärer behålls

Nuvarande `testmonitor` ska fortsätta avvisa identiska binärer i vanliga
kandidatmatcher. Ett separat optimizerläge får tillåta samma binär endast när:

- båda parameterfilerna finns och har validerats,
- deras SHA-256 skiljer sig,
- registerversionen är samma,
- full motorinstansidentitet sparas i matchstatus.

Detta får inte implementeras som en generell `allow identical`-genväg.

### 2.6 Återstart sker mellan block, inte mitt i Fastchess-processen

Fastchess behöver inte kunna återuppta en avbruten process. Optimeraren delar
i stället varje försök i fristående block med ett bestämt antal öppningspar.

Varje block har egen:

- öppningslista eller deterministisk öppningsdelmängd,
- seed,
- run-katalog,
- status och PGN,
- resultatpost i SQLite.

Ett färdigt block räknas en gång. Ett ofullständigt block spelas om från
början. I version 1 sparas inte enskilda partier från ett ofullständigt block.

### 2.7 En enda matchkörning åt gången

SQLite får innehålla många köade trials, men version 1 kör endast ett
matchblock åt gången. Detta följer projektets en-kandidat-i-taget-policy,
undviker resursobalans och gör resultaten jämförbara.

### 2.8 SPRT är inte första optimeringsmåttet

SPSA- och koordinatiterationer använder fasta, parade matchblock. SPRT används
först när en samlad parameteruppsättning ska bekräftas mot baseline.

Man får inte summera Fastchess-LLR från fristående block utan en uttryckligt
verifierad statistisk aggregator. Version 1 kör därför slutlig SPRT som en
vanlig sammanhängande `testmonitor`-match. Om den avbryts bevaras delresultatet,
men den påstås inte kunna fortsätta exakt från samma LLR.

### 2.9 Dashboard skjuts upp

Terminalkommandon, checkpoint och återstart färdigställs före dashboarden.
Dashboarden är en separat, skrivskyddad process och ingår inte i version 1:s
acceptanskrav.

## 3. Föreslagen katalogstruktur

```text
optimizer/
├── SPEC.md
├── IMPLEMENTATION-PLAN.md
├── README.md
├── pyproject.toml
├── parameters.schema.json
├── registries/
│   └── eval-pilot-v1.json
├── campaigns/
│   └── example-eval-pilot.json
├── src/
│   └── goalaric_optimizer/
│       ├── __init__.py
│       ├── cli.py
│       ├── canonical.py
│       ├── registry.py
│       ├── database.py
│       ├── scheduler.py
│       ├── runner.py
│       ├── status.py
│       ├── statistics.py
│       └── algorithms/
│           ├── coordinate.py
│           ├── texel.py
│           └── spsa.py
└── tests/
    ├── fixtures/
    ├── test_registry.py
    ├── test_database.py
    ├── test_resume.py
    └── test_runner.py
```

Python 3:s standardbibliotek används i första versionen:

- `argparse`,
- `sqlite3`,
- `json`,
- `hashlib`,
- `subprocess`,
- `signal`,
- `statistics`,
- `http.server` först i dashboardfasen.

Externa optimeringsbibliotek införs endast om den egna pilotversionen visar
att de behövs.

## 4. Fas 0: isolera utvecklingsmiljön

### Leverans

- Skapa en separat worktree från aktuell baselinecommit.
- Skapa verktygsgrenen `tooling/parameter-optimizer`.
- Flytta eller återskapa `optimizer/SPEC.md` och denna plan där.
- Verifiera att worktreet är rent före första implementationen.

### Kontroller

```bash
git status --short --branch
git rev-parse HEAD
cat artifacts/baseline/current.json
```

### Acceptansgrind

- Worktreet bygger exakt från beslutad baseline.
- Inga orelaterade motorfiler är ändrade.
- Dokumentfilerna är de enda första ändringarna.

## 5. Fas 1: namngivet parameterregister i Go

### Syfte

Skapa ett stabilt gränssnitt mellan anonyma interna parametrar och
optimeringssystemet.

### Leverans

- Definiera en versionsmärkt Go-struktur för pilotgruppen.
- Ge varje parameter stabilt namn och dokumenterad betydelse.
- Lägg min/max/default och heltalssteg i ett centralt register.
- Implementera validering av okända, saknade och otillåtna värden.
- Implementera kanonisk export av baselines standardvärden.
- Låt befintlig intern kod läsa från samma centrala värden.

Pilotgruppen bör omfatta 5–10 befintliga evalvikter som:

- redan används av motorn,
- inte ändrar schackregler,
- inte är samma avvisade grupp som kandidat 023,
- har begriplig MG-/EG-semantik,
- kan testas utan ny evalfunktionalitet.

Exakt grupp väljs innan implementationen av denna fas. Ett rimligt första val
är en liten mobilitets- eller pjäsaktivitetsgrupp. Fribönder, remilogik och
kungssäkerhet ska inte blandas in samtidigt.

### Tester

- standardexport ger exakt nuvarande baselinevärden,
- export följd av import ändrar inget,
- okänd parameter avvisas,
- saknad obligatorisk parameter avvisas,
- min/max och steg valideras,
- olika JSON-ordning ger samma kanoniska representation och hash,
- standardparameterfil ger samma fixed-depth bestmove, score och noder som
  motorn utan explicit parameterfil.

### Acceptansgrind

Standardparameterfilen är exakt sökekvivalent med baseline i den
deterministiska benchmarksviten.

## 6. Fas 2: ParameterFile i GoAlaric

### Leverans

- Lägg till UCI-optionen `ParameterFile`.
- Använd en enda central laddningsfunktion.
- Spara aktiv registerversion och parameter-SHA i motorprocessen.
- Ge ett UCI-diagnostikkommando eller `info string` som rapporterar aktiv
  parameteridentitet.
- Rensa TT, history, killer, eval-cache och pawn-cache när en ny fil tas i
  bruk före sökning.
- Avvisa byte under aktiv sökning.

### Tester

- giltig fil laddas före `isready`,
- felaktig fil ger tydligt fel och ingen sökning startas,
- samma fil ger samma SHA och eval efter omstart,
- baselinefil ger exakt baselinebeteende,
- ändrad pilotparameter ger förväntad evalskillnad i riktad position,
- cachedata från föregående parameteruppsättning återanvänds inte,
- `go test -race ./...` är grönt.

### Acceptansgrind

Två motorprocesser från samma binär kan köras samtidigt med varsin
parameterfil utan delat tillstånd eller identitetsförväxling.

## 7. Fas 3: motorinstansidentitet i testmonitor

### Leverans

- Utöka matchkonfiguration med baseline- och kandidatparameterfil.
- Beräkna och spara parameterfilernas SHA-256.
- Spara registerversion och kanoniska parametrar eller lokal artefaktlänk.
- Definiera `engine_instance_id` som hash av motor-SHA, parameter-SHA och
  registerversion.
- Behåll nuvarande binärspärr utanför optimizerläget.
- Avvisa identiska fullständiga instanser även i optimizerläget.
- Kopiera identiteterna till `config.json`, `status.json` och experimentrapport.

### Tester

- samma binär + samma parametrar avvisas,
- samma binär + olika parametrar godkänns endast i optimizerläge,
- olika binärer + samma parametrar förblir distinkta,
- modifierad parameterfil efter initiering upptäcks före matchstart,
- identiteter överlever `start`, `status`, screening och SPRT,
- befintliga vanliga kandidatkampanjer fungerar oförändrat.

### Acceptansgrind

En falsk Fastchess-integration visar att rätt parameterfil skickas till rätt
motor och att deras identiteter inte kan bytas.

## 8. Fas 4: deterministiska öppningsblock i testmonitor

### Leverans

- Lägg till ett kommando som materialiserar ett öppningsblock från bok, seed,
  blockindex och antal par.
- Varje block ska få en egen PGN- eller EPD-fil med exakt valda öppningar.
- Kör varje block som en vanlig fristående match med `-repeat`.
- Auditera att varje öppning finns exakt två gånger med växlade färger.
- Skriv en kompakt blockrapport atomiskt.

Blockidentitet:

```text
opening_book_sha256
master_seed
partition_name
block_index
pairs_per_block
materialized_openings_sha256
```

### Tester

- samma indata ger byte-identiskt öppningsblock,
- olika blockindex ger disjunkta öppningsgrupper inom kampanjen,
- samma öppning hamnar inte i både tränings- och slutpartition,
- färgparen är kompletta,
- avbrutet block markeras ofullständigt och godkänns inte som resultat,
- omkörning av blocket använder exakt samma öppningar.

### Acceptansgrind

En testkampanj med falsk Fastchess kan avbrytas i block 3, startas om och köra
om endast block 3 utan att block 1–2 räknas igen.

## 9. Fas 5: Python-CLI och SQLite-kärna

### Leverans

Implementera:

```bash
python -m goalaric_optimizer init <campaign.json>
python -m goalaric_optimizer run <campaign-id>
python -m goalaric_optimizer status <campaign-id>
python -m goalaric_optimizer status <campaign-id> --watch
python -m goalaric_optimizer pause <campaign-id>
python -m goalaric_optimizer resume <campaign-id>
python -m goalaric_optimizer stop <campaign-id>
python -m goalaric_optimizer best <campaign-id>
python -m goalaric_optimizer trials <campaign-id> --last 20
```

`init` ska:

- validera kampanj och register,
- låsa baselineidentiteten,
- skapa SQLite-databasen med WAL,
- materialisera baselineparameterfilen,
- spara master-seed och öppningspartitioner,
- vägra återanvända en befintlig kampanjidentitet med annan konfiguration.

### Databasregler

- Foreign keys aktiveras.
- Unik constraint används för parameterhash inom kampanjen.
- Unik constraint används för blockidentitet.
- Statusövergångar valideras centralt.
- Resultat och checkpoint skrivs i samma transaktion.
- PID är diagnostik, inte bevis för att ett jobb fortfarande lever.
- Ett `running`-jobb utan levande process återklassas till `interrupted` vid
  återstart.

### Tester

- schema skapas och migreringsversion sparas,
- dubbla parameteruppsättningar avvisas eller återanvänds deterministiskt,
- förbjudna statusövergångar avvisas,
- rollback lämnar föregående checkpoint intakt,
- statuskommandon gör inga databasskrivningar,
- två optimizerprocesser kan inte äga samma kampanj samtidigt.

### Acceptansgrind

En helt falsk kampanj kan initieras, pausas, återupptas och avslutas med
oförändrad historik efter varje omstart.

## 10. Fas 6: scheduler och säker processhantering

### Leverans

- Kör högst ett block åt gången.
- Starta `testmonitor` som egen processgrupp.
- Spara process- och run-identitet före väntan på resultat.
- Hantera `SIGINT` och `SIGTERM`.
- `pause` stoppar skapandet av nya block men låter normalt aktuellt block bli
  färdigt.
- `stop` begär kontrollerat stopp av aktuellt block och avslutar kampanjen.
- En konfigurerbar `stop_after_current_block` ger säker natt-/manuell paus.
- Abrupt död upptäcks vid nästa `resume`.

### Viktig semantik

- `pause`: avsluta aktuellt block och starta inget nytt.
- `stop`: be `testmonitor stop` avbryta aktuellt block; blocket räknas inte.
- strömavbrott: aktuellt block betraktas som ofullständigt och spelas om.
- färdig match utan databascheckpoint: importera den endast om identiteter och
  terminal status verifieras exakt; annars spela om blocket.

### Tester

- signal under väntande jobb,
- signal mitt i matchblock,
- föräldraprocess dör men barnprocess finns kvar,
- terminal blockrapport finns men checkpoint saknas,
- dubbel `resume`,
- upprepade `pause` och `stop`,
- ingen föräldralös Fastchess- eller motorprocess efter kontrollerat stopp.

### Acceptansgrind

Ett automatiserat stresstest stoppar och återstartar samma kampanj minst 20
gånger utan tappade eller dubblerade block.

## 11. Fas 7: första förslagsalgoritmen

### Leverans

Implementera först deterministisk koordinatsökning för pilotgruppen:

1. börja från baselineparametrarna,
2. ändra en parameter med ett registersteg åt gången,
3. använd fasta parade öppningsblock,
4. förkasta tydligt svaga riktningar,
5. spara alla provade parameterhashar,
6. checkpoint efter varje resultat och nytt förslag.

Koordinatsökning väljs före SPSA därför att den är lätt att verifiera och gör
hela infrastrukturen begriplig. Den är inte tänkt som slutlig algoritm för
många parametrar.

### Resultatmått

Huvudmåttet är poäng i parade partier. Rapportera också:

- Elo-estimat,
- osäkerhetsintervall,
- W-D-L,
- remifrekvens,
- antal partier,
- CPU-tid,
- eventuella krascher eller tidsförluster.

Ingen kandidat ska kallas bättre enbart för att observerad score är över 50 %.
Minimikrav och osäkerhetsregel ska anges i kampanjfilen.

### Tester

- samma seed ger samma förslagssekvens,
- optimizer-state återställs exakt,
- gränser och steg respekteras,
- redan provad hash föreslås inte igen,
- oavgjort eller osäkert resultat hanteras deterministiskt,
- falska matchresultat leder till förväntad sökväg.

### Acceptansgrind

En syntetisk målfunktion med känt optimum hittas även efter flera avbrott och
återstarter.

## 12. Fas 8: Texel-liknande evalgallring

### Leverans

- Återanvänd lärdomarna från `cmd/evaltuner`.
- Bygg tränings-, validerings- och slutpartitioner efter öppningsgrupp.
- Spara SHA-256 för källor och genererade data.
- Kalibrera logistisk skala endast på träningsdata.
- Optimera endast mot träningsdata.
- Rapportera validering utan att använda den för varje parameterbeslut.
- Håll slutpartitionen orörd tills en komplett kandidat valts.

Texel-resultatet ska skapa prioriterade parameterförslag till matchoptimeraren,
inte promovera parametrar direkt.

### Tester

- ingen öppningsgruppsläcka,
- samma indata ger byte-identiska dataset och resultat,
- validerings- och slutdata påverkar inte träningsförslagen,
- svart/vit-resultat orienteras korrekt,
- pawn-/eval-cache rensas korrekt mellan parameteruppsättningar.

### Acceptansgrind

En omkörning på samma data ger identiska parametrar och rapporthashar.

## 13. Fas 9: verklig pilotkampanj

### Förberedelse

- Frys motorbinär, baselineparameterfil, register och öppningspartitioner.
- Kör fulla Go-tester, race, vet, perft, UCI och fixed-depth.
- Kör en kort self-test där baselineparameterfil möter en identisk kopia i ett
  explicit diagnostikläge. Resultatet används endast för att verifiera
  matchkonfigurationen.

### Pilotflöde

1. Texel-gallring eller koordinatförslag.
2. 40–100 parade partier per tidig trial.
3. Automatisk förkastning av tydligt svaga trials.
4. 400-partiersscreening för bästa samlade uppsättning.
5. Bekräftelse med ny seed.
6. Sammanhängande SPRT vid `20+0.2` endast om grindarna passerar.
7. Skapa promotionsrapport och invänta mänskligt beslut.

### Acceptansgrind

Kampanjen ska kunna avslutas utan promotion och ändå lämna en fullständig,
reproducerbar historik över alla parameterförsök.

## 14. Fas 10: SPSA för sökparametrar

Påbörjas först när evalpiloten och återstartstesterna är stabila.

### Leverans

- SPSA-state med dokumenterade koefficienter,
- deterministisk perturbationsgenerator,
- `theta+` och `theta-` med gemensamma öppningsblock,
- projektion till parametergränser och heltalssteg,
- periodisk kontroll mot fryst baseline,
- checkpoint av iteration och RNG-state.

### Särskilda krav

- sökparametrar får inte optimeras med statisk Texel-förlust,
- båda sidorna i SPSA-paret ska få samma färgväxlade öppningar,
- tidskontrollen ska vara tillräcklig för att parametrarna verkligen aktiveras,
- depth pre-scan används för djupberoende grupper,
- slutkandidaten testas på nya öppningar och ny seed.

### Acceptansgrind

SPSA hittar riktningen i en syntetisk testfunktion och kan återupptas exakt
efter avbrott mitt mellan `theta+` och `theta-`.

## 15. Fas 11: skrivskyddad dashboard

Dashboarden byggs sist och får endast läsa SQLite.

### Leverans

```bash
python -m goalaric_optimizer dashboard <campaign-id> \
  --listen 127.0.0.1 --port 8080
```

Den visar status, trials, W-D-L, Elointervall, parameterdifferenser, CPU-tid,
fel och senaste checkpoint.

### Acceptansgrind

Dashboarden kan stoppas, startas och krascha utan att optimizerprocessen eller
databasen påverkas.

## 16. Testmatris per fas

| Fas | Python unit | Go unit | Race/vet | Falsk integration | Riktig motor | Riktig match |
|---|---:|---:|---:|---:|---:|---:|
| 0 Dokumentation/worktree | – | – | – | – | – | – |
| 1 Parameterregister | – | Ja | Ja | – | Ja | – |
| 2 ParameterFile | – | Ja | Ja | Ja | Ja | – |
| 3 Instansidentitet | – | Ja | Ja | Ja | Ja | Kort |
| 4 Öppningsblock | – | Ja | Ja | Ja | – | Kort |
| 5 SQLite/CLI | Ja | – | – | Ja | – | – |
| 6 Scheduler/resume | Ja | – | – | Ja | Kort | Kort |
| 7 Koordinatsökning | Ja | – | – | Ja | – | Pilot |
| 8 Texel-gallring | Ja | Ja | Ja | Ja | Ja | – |
| 9 Pilotkampanj | Ja | Ja | Ja | Ja | Ja | Ja |
| 10 SPSA | Ja | – | – | Ja | Ja | Ja |
| 11 Dashboard | Ja | – | – | Ja | – | – |

## 17. Definition of done för version 1

Version 1 är klar när följande är sant:

- GoAlaric kan läsa en versionsmärkt parameterfil via UCI.
- Baseline med explicit standardfil är exakt sökekvivalent med nuvarande
  baseline.
- `testmonitor` identifierar motor + parametrar utan att försvaga vanliga
  binärspärrar.
- Python-CLI använder SQLite/WAL som enda sanningskälla.
- En kampanj kör ett matchblock åt gången.
- `status`, `--watch`, `pause`, `resume` och `stop` fungerar.
- Avbrutna block spelas om utan dubbelräkning.
- Samma parameterhash testas inte två gånger av misstag.
- En liten evalgrupp kan optimeras med koordinatsökning och Texel-gallring.
- Bästa uppsättningen kan gå genom 400 partier och separat bekräftelse.
- Systemet skapar en kompakt promotionsrapport.
- Baseline och `current.json` förblir orörda tills användaren uttryckligen
  godkänner promotion.

## 18. Rekommenderade commitgränser

Varje punkt bör vara en separat, verifierad commit:

1. Dokumentation och schemaskelett.
2. Namngivet Go-parameterregister.
3. ParameterFile och UCI-tester.
4. Testmonitor motorinstansidentitet.
5. Deterministiska öppningsblock.
6. Pythonpaket och SQLite-schema.
7. Status, pause/resume/stop.
8. Scheduler och falsk Fastchess-integration.
9. Koordinatsökning.
10. Texel-dataset och gallring.
11. Pilotkampanjens konfiguration och rapportering.
12. SPSA, först efter godkänd pilot.
13. Dashboard, sist.

Ingen commit ska blanda en ny optimeringsfunktion med en orelaterad
motorstyrkeändring.

## 19. Första konkreta nästa steg

Nästa arbetssteg ska endast vara fas 0 och början av fas 1:

1. skapa den rena worktreet från aktuell baseline,
2. lägga in dokumentationen där,
3. inventera vilka befintliga parametrar som lämpar sig för pilotgruppen,
4. föreslå gruppen med namn, nuvärden, min/max och steg,
5. invänta godkännande av gruppen innan motorkoden ändras.

Detta undviker att infrastrukturprojektet samtidigt blir ännu en oprövad
evalkandidat.
