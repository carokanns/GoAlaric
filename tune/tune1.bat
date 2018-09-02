rem This is the complete procedure:
rem First save x games from scid in pgn-format to scidgames.pgn

rem Start this bat-file tune1.bat
rem create scidgames.epd
C:\Users\JP\Documents\Schack\pgn-extract\pgn-extract-17-55.exe -Wepd -o scidgames.epd scidgames.pgn

rem Randomly select 10% positions from scidgames.epd to selected.epd
..\..\epdReader\epdReader select 10

rem start tune2.bat