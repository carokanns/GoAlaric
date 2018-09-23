rem Run matches between GoAlaric-GoAlaric from selected.epd - This creates results.pgn
Rem The reason is to get results to the selected.epd games
cd ..
go build
cd tune
C:\Users\JP\Documents\Schack\cutechess/cutechess-cli -engine conf=GoAlaric name=alaric1 -engine conf=GoAlaric name=alaric2 -each tc=40/20+1 timemargin=200 -event TuneGames -rounds x?  -concurrency 3 -draw movenumber=40 movecount=5 score=25 -resign movecount=3 score=300 -site Ingared -openings file=selected.epd format=epd order=sequential start=0 -pgnout results.pgn

rem if you break the match. Take note of which game it was - say x. Next time start with start=x
rem remove the bad game from results.pgn
rem start this bat again
rem
rem run tune3.bat