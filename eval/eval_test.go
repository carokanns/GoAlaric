// eval_test.go
package eval

import (
	"GoAlaric/board"
	"GoAlaric/material"
	"fmt"
	"testing"
)

func initAll() { // copy of main initSession()
	// input.Init()
	//engine.Init()
	//material.Init()
	//PstInit()  //eval
	//PawnInit() //eval
	//Init()     //eval
	//AtkInit()  //eval
	//	search.Init()
	//bit.InitBits()
	//hash.Init()
	//castling.Init()
}

var bd board.Board

//func Interpolation(eval-mg, eval-eg int, bd *board.Board) int
func TestInterpolation(t *testing.T) {
	type interStruct struct {
		fen  string
		eg   int
		mg   int
		answ int
	}

	var interTest = [...]interStruct{
		{"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1", 0, 0, 0},
		{"r1bq1rk1/ppp1p1bp/5np1/3Ppp2/8/1QP3P1/PP2PPBP/RNBR2K1 b - - 3 10", 25, 100, 99},
		{"8/1k6/8/5K2/8/6P1/8/8 w - - 0 1", 140, 90, 140},
	}

	initAll()

	for ix, tst := range interTest {
		board.SetFen(tst.fen, &bd)
		i := Interpolation(tst.mg, tst.eg, &bd)
		if i != tst.answ {
			t.Errorf("Testcase %v: Borde bli %v. Men blev %v", ix+1, tst.answ, i)
		}
	}
}

func TestKBNK(t *testing.T) {
	type evalStruct struct {
		fen  string
		eval int
	}

	var evalTest = [...]evalStruct{
		{"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1", 0},
		{"8/1k6/8/8/8/5N2/5BK1/8 w - - 0 1", 787},
		{"8/1k6/8/3K4/8/5N2/5B2/8 w - - 0 1", 816},
		{"k7/8/8/8/8/5N2/5BK1/8 w - - 0 1", 576},
		{"1k6/8/8/8/8/5N2/5BK1/8 w - - 0 1", 782},
		{"2k5/8/8/8/8/5N2/5BK1/8 w - - 0 1", 977},
		{"7k/8/8/8/8/5N2/5BK1/8 w - - 0 1", 1983},
	}

	initAll()
	var pawnTab PawnHash
	pawnTab.Clear()
	for ix, tst := range evalTest {
		board.SetFen(tst.fen, &bd)
		if ix > 0 && !evalKBNK(&bd, bd.Stm()) {
			t.Errorf("Testcase %v: KBNK ej true", ix+1)
		}
		e := CompEval(&bd, &pawnTab) // NOTE: score for white
		if e != tst.eval {
			t.Errorf("Testcase %v: Borde bli %v. Men blev %v", ix+1, tst.eval, e)
		}
	}
}

func TestEval(t *testing.T) {
	type evalStruct struct {
		fen     string
		evalMin int
		evalMax int
	}

	var evalTest = [...]evalStruct{
		{"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1", 0, 0},
		{"7k/8/8/8/8/5N2/5BK1/8 w - - 0 1", 1900, 2000},
		{"3r2k1/5pp1/p7/P1qp1PP1/8/1P1R3K/3Q4/8 w - - 5 39", -50, -10},
		{"3r2k1/5pp1/p7/P1qp1PP1/1P6/3R3K/3Q4/8 b - - 5 39", 50, 90},
		{"3r2k1/5pp1/p7/P2p1PP1/1P6/3R3K/3Q4/6q1 w - - 1 40", -90, -50},
	}

	initAll()
	var pawnTab PawnHash
	pawnTab.Clear()
	for ix, tst := range evalTest {
		board.SetFen(tst.fen, &bd)
		e := CompEval(&bd, &pawnTab) // NOTE: score for white
		if e < tst.evalMin || e > tst.evalMax {
			t.Errorf("Testcase %v: Borde vara inom [%v, %v]. Men blev %v", ix+1, tst.evalMin, tst.evalMax, e)
		}
	}
}

func TestCompAttacks(t *testing.T) {
	type evalStruct struct {
		fen       string
		something int
	}

	var evalTest = [...]evalStruct{
		{"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1", 0},
		{"rnbqk2r/pppp1ppp/4pn2/8/1bPP4/2N5/PP2PPPP/R1BQKBNR w KQkq - 2 4", 1900},
	}

	initAll()
	for ix, tst := range evalTest {
		fmt.Printf("case %v\n", ix+1)
		board.SetFen(tst.fen, &bd)
		var ai attackInfo
		compAttacks(&ai, &bd)
		text := "White"
		for sd := 0; sd < 2; sd++ {
			fmt.Println("all attacks from", text)
			board.PrintBB(ai.allAtks[sd])

			for pc := material.Pawn; pc <= material.King; pc++ {
				fmt.Printf("attacks from lower than %v %v", text, material.ToString(pc))
				//				board.PrintBB(ai.ltAtks[sd][pc])
			}
			text = "Black"
		}
	}
}
