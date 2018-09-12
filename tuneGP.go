// +build tunegp

package main

//////////////////////////////////////////////////////////
// COMPILE WITH go build tags tuneGP					//
//////////////////////////////////////////////////////////
// Buid a file for GPTune.go to use for tuning with GP 	//
// read in the epd-lines and run 1.qs and 2.search()   	//
// create a new epd-file with the same fen and the  	//
// Qs-eval in c0 and searchEval in c1.                 	//
//////////////////////////////////////////////////////////

import (
	"GoAlaric/board"
	"GoAlaric/eval"
	"GoAlaric/search"
	"GoAlaric/uci"
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
)

var inputFile = "tune.epd"
var outputFile = "gp.epd"
var epd []string

//var nParms = len(parms.Parms)

func init() {
	fmt.Println("\nstarting GP tuner")
	f, err := os.Create(outputFile)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	scanEpd(&epd) // läs in alla epd-positioner
	run()
	log.Fatalln("tune finished")
}

// cerate a file where c0 is qs value and c1 is eval after 10s
func run() {
	f, err := os.Create(outputFile)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	nPositions := len(epd)
	chInput := make(chan string)
	chSearch := make(chan int)
	chBestmove := make(chan string)

	go getInput(chInput)
	go search.StartSearch(chSearch, chBestmove, &uci.Bd)

	for pos := 0; pos < nPositions; pos++ {
		fen := strings.Split(epd[pos], "c0")
		uci.SetPosition("position fen " + fen[0])
		q := getQs()
		if uci.Bd.Stm() == board.BLACK {
			q = -q
		}
		uci.HandleGo("go movetime 10000", &chSearch)
		bm := <-chBestmove
		e := search.Best.Score
		if uci.Bd.Stm() == board.BLACK {
			e = -e
		}

		fmt.Println(q, e, bm)
		_, err = f.WriteString(fmt.Sprintf("%v c0 %v c1 %v\n", fen[0], q, e))
		if err != nil {
			panic(err)
		}
	}
}

func scanEpd(epd *[]string) {
	file, err := os.Open(inputFile)
	if err != nil {
		panic(err)
	}

	fscanner := bufio.NewScanner(file)
	ix := 0
	for fscanner.Scan() {
		epdLine := fscanner.Text()
		if len(epdLine) == 0 {
			continue
		}
		if ix = strings.Index(epdLine, "c0"); ix < 0 {
			log.Fatalln("c0 saknas i", epdLine)
		}

		*epd = append(*epd, epdLine[:ix])
	}
}

func getQs() int {
	const maxScore = 9999
	var sl search.Local
	sl.ClearHash()
	sl.ID = 0
	sl.Board = uci.Bd
	eval.Update()
	val := search.Qs(&sl, maxScore, 0)
	return val
}
