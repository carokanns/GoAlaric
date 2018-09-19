// +build tunegp

package main

//////////////////////////////////////////////////////////
// COMPILE WITH go build -tags tunegp   (små bokstäver)	//
//////////////////////////////////////////////////////////
// Buid a file to GPTune.go  use for tuning with GP 	//
// Will read in the epd-lines and run 1.qs and       	//
// 2.search(). It create a new epd-file with the same  	//
//  fen and the Qs-eval in c0 and searchEval in c1.    	//
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
	"strconv"
	"strings"
)

////////////////// dont forget +build tunegp

var inputFile = "tune/verifyTune.epd"
var outputFile = "gpVerify.epd"
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
	//testRun()
	log.Fatalln("tune finished")
}

// cerate a file where c0 is qs value and c1 is eval after 10s
func run() {
	f, err := os.Create(outputFile)
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

	for pos := 0; pos < nPositions; pos++ {
		/* 	if pos < 725 {
			continue
		}
		*/
		fields := strings.Fields(epd[pos])
		if fields[1] == "b" {
			epd[pos] = mirror(epd[pos])
		}

		fen := strings.Split(epd[pos], "c0")

		//		uci.SetPosition("position fen " + fen[0])
		_ = uci.HandleInput("position fen "+fen[0], &chSearch)
		_ = uci.HandleInput("go depth 1", &chSearch)
		_ = <-chBestmove
		q := search.Best.Score
		//		q := getQs()
		if uci.Bd.Stm() == board.BLACK {
			q = -q
			panic("NOOOOOOOOOOOOOOOOOOOOOOO")
		}
		uci.HandleGo("go movetime 10000", &chSearch)
		_ = <-chBestmove

		e := search.Best.Score
		if uci.Bd.Stm() == board.BLACK {
			e = -e
		}

		fmt.Println("POS ===================================================  qs", q, "ev", e, pos)
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

//byt ut run() mot denna
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
