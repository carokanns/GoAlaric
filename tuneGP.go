//go:build tunegp
// +build tunegp

package main

////////////////////////////////////////////////////////////
// COMPILE WITH go build -tags tunegp   (små bokstäver)	  //
////////////////////////////////////////////////////////////
// Buid a file for the GPTune project use to tune with GP //
// Will read in the epd-lines and 1. compute rooteval and //
// 2.search(). It create a new epd-file with the same  	  //
//  fen and the rooteval in c0 and searchEval in c1.  	  //
////////////////////////////////////////////////////////////
///////////////////////////////////////////
// start with 'goalaric parms'     		 //
// in order to save eval-parameters		 //
// otherwise use goalaric  in order		 //
// to generate epd-file with c0-eval and //
// c1-search.							 //
///////////////////////////////////////////

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"goalaric/board"
	"goalaric/eval"
	"goalaric/search"
	"goalaric/uci"
)

////////////////// dont forget +build tunegp

//var inputFile = "tune/cleanForGP.epd"
var fullGPFile = "fullGP2.epd"
var parmGPFile = "parmsGP.epd"
var epd []string

//var nParms = len(parms.Parms)

func init() {
	fmt.Println("\nstarting GP tuner")
	fmt.Println(os.Args)
	useParms := false
	if len(os.Args) > 1 {
		if os.Args[1] == "parms" {
			fmt.Println("Parameter: parms")
			useParms = true
		} else {
			fmt.Println("Parameter not known:", os.Args[1:])
		}
	} else {
		fmt.Println("No Parameter")
	}

	time.Sleep(time.Second * 2)

	fmt.Println("Use file", inputFile)
	scanEpd(&epd) // läs in alla epd-positioner
	if useParms {
		runGetParms()
	} else {
		runGetEval()
	}

	fmt.Println("tune will finish here")
	os.Exit(2)
}

// Create a file where c0 is qs value and c1 is a list of parm-values
func runGetParms() {
	f, err := os.Create(parmGPFile)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	//startTime := time.Now()
	nPositions := len(epd)
	chInput := make(chan string)
	chSearch := make(chan int)
	chBestmove := make(chan string)

	go getInput(chInput)
	go search.StartSearch(chSearch, chBestmove, &uci.Bd)

	fmt.Println("starting get parms")
	for pos := 0; pos < nPositions; pos++ {
		fields := strings.Fields(epd[pos])
		if fields[1] == "b" {
			epd[pos] = mirror(epd[pos])
		}

		fen := strings.Split(epd[pos], "c0")

		_ = uci.HandleInput("ucinewgame", &chSearch)
		_ = uci.HandleInput("position fen "+fen[0], &chSearch)

		var ph eval.PawnHash
		ph.Clear()

		evRoot, vector := eval.CompEvalGP(&uci.Bd, &ph)

		if pos%1000 == 0 {
			fmt.Println("\nPOS", pos, "====================== rootEv", evRoot)
			time.Sleep(time.Millisecond * 500)
		}
		strVector := fmt.Sprintf("%v", vector)
		strVector = strings.Replace(strVector, "]", "", -1)
		strVector = strings.Replace(strVector, "[", "", -1)
		_, err = f.WriteString(fmt.Sprintf("%v c0 %v c1 %v\n", fen[0], evRoot, strVector))
		if err != nil {
			panic(err)
		}
	}
	fmt.Println("no of positions", nPositions)
}

// Create a file where c0 is qs value and c1 is eval after xs
func runGetEval() {
	f, err := os.Create(fullGPFile)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	//startTime := time.Now()
	nPositions := len(epd)
	diffHigh := 0
	chInput := make(chan string)
	chSearch := make(chan int)
	chBestmove := make(chan string)

	go getInput(chInput)
	go search.StartSearch(chSearch, chBestmove, &uci.Bd)

	for pos := 0; pos < nPositions; pos++ {
		fields := strings.Fields(epd[pos])
		if fields[1] == "b" {
			epd[pos] = mirror(epd[pos])
		}

		fen := strings.Split(epd[pos], "c0")

		//		uci.SetPosition("position fen " + fen[0])
		_ = uci.HandleInput("ucinewgame", &chSearch)
		_ = uci.HandleInput("position fen "+fen[0], &chSearch)

		var ph eval.PawnHash
		ph.Clear()
		evRoot, _ := eval.CompEvalGP(&uci.Bd, &ph)

		//////////////////////////
		var sl search.Local
		sl.ID = 0
		sl.Board = uci.Bd
		sl.ClearHash()
		eval.Update()
		/* 		qs := search.Qs(&sl, search.EvalMAX, 0)
		   		///////////////////////

		   		_ = uci.HandleInput("ucinewgame", &chSearch)
		   		_ = uci.HandleInput("position fen "+fen[0], &chSearch)
		   		eval.Update()
		   		_ = uci.HandleInput("go depth 1", &chSearch)
		   		_ = <-chBestmove
		   		e1 := search.Best.Score
		   		//		q := getQs()
		   		if abs(qs-evRoot) > 75 {
		   			diffHigh++
		   		}
		   		if uci.Bd.Stm() == board.BLACK {
		   			e1 = -e1
		   			panic("NOOOOOOOOOOOOOOOOOOOOOOO")
		   		}
		*/
		//uci.HandleGo("go movetime 10000", &chSearch)
		uci.HandleGo("go movetime 100", &chSearch)
		_ = <-chBestmove

		evSearch := search.Best.Score
		if uci.Bd.Stm() == board.BLACK {
			evSearch = -evSearch
			panic("NOOOOOOOOOOOOOOOOOOOOOOO")
		}

		if pos%1000 == 0 {
			fmt.Println("\n\n\nPOS", pos, "====== rootEv", evRoot, "ev100", evSearch)
		}
		_, err = f.WriteString(fmt.Sprintf("%v c0 %v c1 %v\n", fen[0], evRoot, evSearch))
		if err != nil {
			panic(err)
		}
	}
	fmt.Println("no of diffHigh", diffHigh, "of", nPositions)
}

// ///
func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
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

	search.SG.Trans.Clear()
	search.SG.History.Clear()
	sl.ID = 0
	sl.Board = uci.Bd
	eval.Update()
	val := search.Qs(&sl, maxScore, 0)
	return val
}

func mirror(epd string) string {
	fields := strings.Fields(epd)
	if len(fields) < 4 {
		panic("invalid epd - missing data " + epd)
	}

	rows := strings.Split(fields[0], "/")
	rows[0], rows[1], rows[2], rows[3], rows[4], rows[5], rows[6], rows[7] = rows[7], rows[6], rows[5], rows[4], rows[3], rows[2], rows[1], rows[0]
	temp := strings.Join(rows, "/")
	up := []string{"P", "N", "B", "R", "Q", "K"}
	lo := []string{"p", "n", "b", "r", "q", "k"}
	xd := []string{"@", "£", "$", "€", "<", ">"}
	for i := 0; i < len(up); i++ {
		temp = strings.Replace(temp, up[i], xd[i], -1)
	}
	for i := 0; i < len(lo); i++ {
		temp = strings.Replace(temp, lo[i], up[i], -1)
	}
	for i := 0; i < len(xd); i++ {
		temp = strings.Replace(temp, xd[i], lo[i], -1)
	}
	fields[0] = temp
	if fields[1] == "w" {
		fields[1] = "b"
	} else {
		fields[1] = "w"
	}
	if fields[2] != "-" {
		newField := ""
		if strings.Contains(fields[2][:], "k") {
			newField = newField + "K"
		}
		if strings.Contains(fields[2], "q") {
			newField = newField + "Q"
		}
		if strings.Contains(fields[2], "K") {
			newField = newField + "k"
		}
		if strings.Contains(fields[2], "Q") {
			newField = newField + "q"
		}
		//fmt.Printf("%v %#v\n", fields[2], newField)
		fields[2] = newField
	}

	if fields[3] != "-" {
		n, _ := strconv.Atoi(fields[3][1:])
		n = 9 - n
		fields[3] = fields[3][0:1] + strconv.Itoa(n)
	}

	newEpd := strings.Join(fields, " ")
	return newEpd
}

// byt ut run() mot denna
func testRun() {
	tests := []struct {
		name string
		epd  string
		want string
	}{
		{"", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - c0 alaric1-alaric2 TuneGames Ingared 2018.07.31; c1 1-0;", "b"},
		{"", "r2qkbnr/ppp1pppp/2n5/3P4/2p1P1b1/5N2/PP3PPP/RNBQKB1R b KQkq - c0 alaric2-alaric1 TuneGames Ingared 2018.07.31; c1 1-0;", "w"},
	}
	for _, tt := range tests {
		got := mirror(tt.epd)
		x := strings.Fields(got)[1]

		if x != tt.want {
			fmt.Printf("mirror() = %v, want %v\n", x, tt.want)
		}
	}

}
