// GoAlaric project GoAlaric.go
package main

import (
	"bufio"
	"fmt"
	"goalaric/search"
	"goalaric/uci"
	"io"
	"os"
	// Följande 3 imports samt "go func" först i main() för pprof
	// "log"
	// "net/http"
	// _ "net/http/pprof"
)

// main är yttre loopen
// Den avbryts av uci-kommandot "quit" (som avslutar programmet)
func main() {
	// Följande för pprof
	/*	go func() {
			log.Println(http.ListenAndServe("localhost:8080", nil))
		}()
	*/ // starta från kommandotolken för en 30-sekunders CPU profil:
	//     go tool pprof http://localhost:8080/debug/pprof/profile
	//     Detaljer i: https://golang.org/pkg/net/http/pprof/

	printTODO()
	initSession()

	chInput := make(chan string)
	chSearch := make(chan int)
	chBestmove := make(chan string)

	go getInput(chInput)
	go search.StartSearch(chSearch, chBestmove, &uci.Bd)

	search.Infinite = false
	savedBm := ""
	uci.SetPosition("position startpos")

	for s := ""; s != "quit"; {
		bm := ""
		line := ""
		select {
		case bm = <-chBestmove:
			if search.Infinite {
				savedBm = bm // Save Bestmove until GUI sends "stop"
				// nothing more should come from the engine now
			} else {
				tellGUI(bm)
			}
		case line = <-chInput:
			s = uci.HandleInput(line, &chSearch)
			if search.Infinite && (s == "stop" || //we are waiting for "stop" in order to send bestmove
				s == "s") {
				tellGUI(savedBm)
				search.Infinite = false
			}
		}
	}

	fmt.Println("info string program exits")
}

var reader = bufio.NewReader(os.Stdin)

// getInput gets the next input from stdin (the GUI)
func getInput(line chan<- string) {
	//reader = bufio.NewReader(os.Stdin)
	for {
		text, err := reader.ReadString('\n')
		if err != io.EOF && len(text) > 0 {
			line <- text
		}
	}
}

// initSession görs endast en gång; när programmet startas
func initSession() {
	//engine.Init()

	//bit.InitBits()
	//material.Init()
	//eval.PstInit()
	//eval.PawnInit()
	//eval.Init()
	//search.Init()
	//hash.Init()
	//castling.Init()
	//eval.AtkInit()
}

// TellGUI prints a line to stdout (to the GUI)
func tellGUI(line string) {
	fmt.Println(line)
}

func printTODO() {
	tellGUI(":")
	tellGUI("TODO-list is currently outcommented")
	/*	tellGUI("info string Lyft upp lokala variabler till globala för att avlasta GC")
		tellGUI("info string Tex: Allokera movelist globalt och skicka slice ner i trädet")
		tellGUI("info string Kör benchmark tester före och efter")
		tellGUI("\nFÖRBÄTTRINGAR")
		tellGUI("info string Inför trivila slutspel")
		tellGUI("info string Byt namn allmänt")
		tellGUI("info string  - 'Get' istf retrieve mfl")
		tellGUI("info string  returnera flera värden istf pointers i retrieve")
		tellGUI("info string  Rationalisera bort search_id och search_asp ")
		tellGUI("info string  byt namn på p_time till constraint e.d.")
		tellGUI("info string  kolla om preprocessor kan ge något")

		tellGUI("\nBUGGAR/VIKTIGA ÄNDRINGAR")
		tellGUI("info string Kolla varför monitorsf felaktigt får score 0 ibland från GoAlaric")
		tellGUI("info string Rekursera qs 1 ggn. Vissa score indikerar att enkla motslag missas")
		tellGUI("info string Görs verkligen en riktig omallokering av Trans-tabellen vid setoption Hash?")
		tellGUI("info string Jämför med sp om eval ger samma värden. Depth 2 och 3")
		tellGUI("")
	*/
}
