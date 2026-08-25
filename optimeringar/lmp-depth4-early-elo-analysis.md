# Tidiga Elo-förlopp och fortsatt LMP-kampanj

Analyserad: 2026-08-25.

## Fråga och metod

Analysen prövar observationen att GoAlarics kandidater alltid skulle börja med
hög Elo och därefter falla mot noll. Endast sparade verkliga matchresultat
användes. SQLite lästes read-only. För äldre körningar där tusentals partier
sparades som ett enda aggregerat SQLite-block rekonstruerades ordningen från
PGN, eftersom den äldre blockimporten lagrade alla vinster, remier och förluster
grupperade och därför inte bevarade kronologin.

Alla resultat är ur kandidatens perspektiv. Kontrollpunkterna är kumulativa och
Elo beräknas med samma logistiska omvandling och kontinuitetskorrektion som
optimeraren.

## Längre kampanjer

| Kampanj | 64 | 128 | 256 | 400 | 800 | 1 600 | 2 000 | 4 000 | Slut |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| LMR 175, första matchen | -43,0 | -21,6 | -2,7 | +4,3 | +0,4 | -1,7 | +1,9 | +1,5 | +5,6 vid 6 000 |
| LMR 175, oberoende fortsättning | +5,3 | -5,4 | -23,0 | -17,3 | -5,2 | -2,2 | -1,6 | — | -1,6 vid 2 000 |
| `activity_shift=3` | -21,4 | 0,0 | -5,4 | -0,9 | -7,8 | -4,3 | -2,8 | — | -1,4 vid 2 504 |
| `aspiration_min_depth=5` | +5,3 | -13,5 | -8,1 | -6,9 | -1,7 | -3,0 | -3,0 | — | -1,7 vid 2 508 |
| `aspiration_initial_margin_cp=15` | -10,7 | +8,1 | +20,3 | +11,3 | +9,5 | +6,9 | +5,7 | +3,1 | +4,0 vid 4 550 |
| dynamiskt LMP-djup 4 från iteration 12 | +16,0 | +16,2 | +6,8 | +11,3 | +11,7 | +12,2 | +9,6 | — | +6,2 vid 2 284 |

Tre av de sex längre serierna var positiva och tre negativa efter 64 partier.
Efter 128 partier var två positiva, tre negativa och en exakt jämn. Det finns
alltså inget stöd för att längre kampanjer systematiskt börjar positivt.

Som bredare kontroll analyserades 127 söktrials med minst 64 partier och
parvis lagrade block. Efter 64 partier var 54 positiva, 9 jämna och 64
negativa; deras ovägda medelscore var 49,705 procent. Efter att den stora
100-trialskampanjen tagits bort var motsvarande antal 16 positiva, 1 jämn och
11 negativa med 50,084 procents medelscore. Åtta separata bekräftelser gav inte
heller någon ensidig startfördel.

Slutsatsen är att minnesbilden bäst förklaras av urval och uppmärksamhet:
positiva starter är mer minnesvärda, och kandidater som går vidare har ofta
redan valts efter ett positivt pilotresultat.

## Avbruten LMP-kampanj

`lmp-depth4-iteration12-6000-20260824` stoppades efter 1 142 kompletta
färgväxlade öppningspar:

- partier: 2 284
- W-D-L: 538-1 249-497
- score: 50,89754816 procent
- Elo: +6,23474312
- dashboardens 95-procentsintervall: -3,35160898 till +15,83606936 Elo
- status: `interrupted`
- ett påbörjat block avbröts och räknades inte
- 1 142 av 1 142 färdiga block hade unika öppningshashar
- inga kampanjprocesser fanns kvar efter stoppet

En SQLite-säkerhetskopia finns som
`artifacts/development/lmp-depth4-iteration12-6000-20260824/campaign-at-stop-2284.db`.

Eftersom resultaten kommer från färgväxlade par kontrollerades även ett
parklustrat intervall. Det blev -2,74 till +15,22 Elo. Det nuvarande
spelbaserade intervallet är alltså något bredare och därmed konservativt för
den här datan; det förklarar inte en falskt positiv start.

## Verifierade driftsbrister

Två separata driftsbrister upptäcktes:

1. `optimizer stop` och `optimizer pause` försökte ta samma OS-lås som en
   långvarig `optimizer optimize` höll. Kontrollkommandona kunde därför svara
   `campaign ... is busy` när de behövdes. De gör nu sina idempotenta
   SQLite-/processåtgärder utan att vänta på invokationslåset.
2. `real.concurrency` lästes från kampanjfilen och testmonitor/Fastchess stödde
   värdet, men Pythonbryggan avvisade allt utom 1. Positiva värden tillåts nu.
   Ett färgväxlat tvåpartiersblock kan därför köras med `concurrency=2` utan att
   ändra block-, återstarts- eller dubbelräkningssemantiken.

## Fortsättning

Den oberoende fortsättningen ska innehålla exakt 3 716 nya partier, alltså
1 858 kompletta öppningspar. Tillsammans med de 2 284 sparade partierna blir
det 6 000. Den använder samma låsta motor, samma parameterfiler, `8+0.05`,
`Threads=1`, Hash 128 och Syzygy 3+4, men en ny kampanjidentitet och
`concurrency=2`.

Öppningskällan filtreras så att inga öppningar från den första kampanjens
1 142 kompletta eller ett avbrutet par kan återkomma. Den nya kampanjen får
ingen automatisk promotion. Slutbedömningen ska redovisa båda delresultaten
separat och sammanlagt.
