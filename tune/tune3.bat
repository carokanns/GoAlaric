rem Create results.epd from the results.pgn file from the goAlaric-goAlaric match
C:\Users\JP\Documents\Schack\pgn-extract\pgn-extract-17-55.exe -Wepd -o results.epd results.pgn

rem Pick the first epd-line in each game and create tune.epd
..\..\epdReader\epdReader first

rem Start tuning
cd ..
go build -tags tune
cd tune
..\GoAlaric