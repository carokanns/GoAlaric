// +build tune

package main

import (
	"GoAlaric/board"
	"GoAlaric/eval"
	"GoAlaric/parms"
	"GoAlaric/search"
	"GoAlaric/uci"
	"bufio"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
)

// Parms is an array with evaluation values
var Parms = &parms.Parms

/////////////////////////////////////////////////////////
// read in the epd-lines and save the result given by  //
// the c1-tag in EPD                                   //
/////////////////////////////////////////////////////////

var inputFile = "tune.epd"
var outputFile = "newParms.txt"
var res []float32
var fen []string

//var nParms = len(parms.Parms)

func init() {
	fmt.Println("\nstarting tuner")
	f, err := os.Create(outputFile)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	scanEpd(&res, &fen)
	fmt.Println("both lengths should be the same", len(res), len(fen))
	if len(res) != len(fen) {
		panic("Not the same length")
	}
	bestParms := localOptimize(parms.Parms)

	fmt.Println(bestParms)
	_, err = f.WriteString(fmt.Sprintln(bestParms))
	if err != nil {
		panic(err)
	}

	log.Fatalln("tune finished")
}

func localOptimize(initialGuess [parms.Nparms]int) [parms.Nparms]int {
	//	testQs(initialGuess)
	//	panic("ok")
	var bestE = E(initialGuess)
	bestParValues := initialGuess
	fmt.Println("initial best", bestE, bestParValues)
	improved := true
	for improved {
		improved = false
		for pi := 0; pi < parms.Nparms; pi++ {
			newParValues := bestParValues
			newE := 9999.9
			newParValues[pi]++
			fmt.Printf(", %v: %v\n", pi, newParValues[pi])
			if newParValues[pi] <= 1200 { // max value
				newE = E(newParValues)
			}
			if newE < bestE {
				improved = true
				for newE < bestE {
					bestE = newE
					bestParValues = newParValues
					fmt.Println("\nnew best", bestE, bestParValues)
					newParValues[pi]++
					fmt.Printf(", %v: %v\n", pi, newParValues[pi])
					if newParValues[pi] <= 1200 { // max value
						newE = E(newParValues)
					}
				}
			} else {
				if newParValues[pi] > 2 { // never set parm <= 0
					newParValues[pi] -= 2
					fmt.Printf(", %v: %v\n", pi, newParValues[pi])
					newE = E(newParValues)
					if newE < bestE {
						improved = true
						for newE < bestE {
							bestE = newE
							bestParValues = newParValues
							fmt.Println("\nnew best", bestE, bestParValues)
							newParValues[pi]--
							fmt.Printf(", %v: %v\n", pi, newParValues[pi])
							if newParValues[pi] > 0 { // min value
								newE = E(newParValues)
							}

						}
					}
				}
			}
		}
	}
	return bestParValues
}

// E gives evaluation error
func E(newParms [parms.Nparms]int) float64 {
	parms.Parms = newParms
	//fmt.Println(parms.Parms)
	//testQs(newParms)
	nPositions := len(res)
	diff := float64(0)

	for pos := 0; pos < nPositions; pos++ {
		//setup position epd[pos]
		uci.SetPosition("position fen " + fen[pos])
		//		var pawnHash eval.PawnHash
		//		pawnHash.Clear()
		q := getQs()
		if uci.Bd.Stm() == board.BLACK {
			q = -q
		}
		/* 		pawnHash.Clear()
		   		e := eval.CompEval(&uci.Bd, &pawnHash)
		   		if e != q {
		   			fmt.Printf("q=%v e=%v\n", q, e)
		   		}
		*/
		x := float64(res[pos]) - sigmoid(float64(q))
		diff += x * x
	}
	fmt.Printf("diff=%1.2f Nparms=%v return=%1.2f\n", diff, parms.Nparms, diff/float64(parms.Nparms))
	return diff / float64(parms.Nparms)
}

func sigmoid(q float64) float64 {
	const K = 1.13
	return 1.0 / (1.0 + math.Pow(10, (-K*q)/400))
}

func scanEpd(r *[]float32, epd *[]string) {
	file, err := os.Open(inputFile)
	if err != nil {
		panic(err)
	}

	fscanner := bufio.NewScanner(file)
	ix, ix2 := 0, 0
	for fscanner.Scan() {
		epdLine := fscanner.Text()
		if len(epdLine) == 0 {
			continue
		}
		if ix = strings.Index(epdLine, "c0"); ix < 0 {
			log.Fatalln("c0 saknas i", epdLine)
		}

		*epd = append(*epd, epdLine[:ix])

		if ix = strings.Index(epdLine, "c1"); ix < 0 {
			log.Fatalln("c1 saknas i", epdLine)
		}

		if ix2 = strings.Index(epdLine[ix:], "1-0"); ix2 > 0 {
			*r = append(*r, 1.0)
			continue
		}
		if ix2 = strings.Index(epdLine[ix:], "0-1"); ix2 > 0 {
			*r = append(*r, 0.0)
			continue
		}
		if ix2 = strings.Index(epdLine[ix:], "1/2-1/2"); ix2 > 0 {
			*r = append(*r, 0.5)
			continue
		}
		log.Fatalln("resultat saknas i", epdLine)
	}
}

func getQs() int {
	const maxScore = 9999
	var sl search.SearchLocal
	sl.ClearHash()
	sl.ID = 0
	sl.Board = uci.Bd
	eval.Update()
	//	fmt.Println("parms i getQs", parms.Parms[23], parms.Parms[24])
	val := search.Qs(&sl, maxScore, 0)
	return val
}

/// Note: a copy from search /////
//const maxScore = +9999

/*
// ASSERT is asserting message f cond is false
func ASSERT(cond bool, msg ...interface{}) {

	//	if !cond {
	//		if len(msg) > 0 {
	//			panic("assert failed. ")
	//		} else {
	//			panic("assert failed. ")
	//		}
	//	}
}
*/
func testQs(newParms [parms.Nparms]int) {
	/*
		325, // MG knight value
		325, // EG knight value
		//ix=25 (next)
		325, // MG bishop value
		325, // EG bisop value
		460, // MG rook value
		540, // EG rook value
		975, // MG queen value
		//ix=30 (next)
		975, // EG queen value
	*/
	posar := []string{"r1bqkbnr/pppp1ppp/8/4n3/4P3/8/PPPP1PPP/RNBQKB1R w KQkq - 0 4",
		"r1bqkb1r/1ppp1ppp/5n2/1p2n3/4P3/P7/1PPP1PPP/R1BQKB1R w KQkq - 0 7",
		"r1bqkb1r/pppp1ppp/5n2/4N3/4P3/2N5/PPQP1PPP/R1B1KB1R w KQkq - 1 6"}
	resar := []float64{0.5, 0, 1}
	nightar := []int{320, 200, 326}

	diff := float64(0)
	for ix, pos := range posar {
		fmt.Println(pos)
		for _, kn := range nightar {
			parms.Parms[23] = kn
			parms.Parms[24] = kn
			eval.Update()
			uci.SetPosition("position fen " + pos)
			q := getQs()
			if uci.Bd.Stm() == board.BLACK {
				q = -q
			}
			x := float64(resar[ix]) - sigmoid(float64(q))
			diff = x * x
			fmt.Println("pos", ix, "N", kn, "q", q, "res", resar[ix], diff)
		}
	}
}
