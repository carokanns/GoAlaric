@echo off
if -%1-==-help- goto helpme
if -%1- == -- goto helpme
if -%2- == -- goto helpme

copy %1 cpu%2.pprof
go tool pprof -text  goalaric.exe cpu%2.pprof  >report%2.txt
go tool pprof -pdf goalaric.exe cpu%2.pprof  >report%2.pdf
report%2.txt
report%2.pdf
goto end
:helpme
echo --
echo Syntax:
echo profile "full path to profile.pprof" #number 
echo example: profile c:\xxx\cpu.pprof 5 will result in report5.txt and cpu5.pdf
echo --
:end