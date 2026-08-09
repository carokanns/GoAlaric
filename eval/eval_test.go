package eval

import (
	"fmt"
	"testing"

	"goalaric/board"
	"goalaric/material"
	"goalaric/square"
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

// func Interpolation(eval-mg, eval-eg int, bd *board.Board) int
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
		{"8/1k6/8/8/8/5N2/5BK1/8 w - - 0 1", 766},
		{"8/1k6/8/3K4/8/5N2/5B2/8 w - - 0 1", 795},
		{"k7/8/8/8/8/5N2/5BK1/8 w - - 0 1", 556},
		{"1k6/8/8/8/8/5N2/5BK1/8 w - - 0 1", 762},
		{"2k5/8/8/8/8/5N2/5BK1/8 w - - 0 1", 956},
		{"7k/8/8/8/8/5N2/5BK1/8 w - - 0 1", 1962},
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
		{"3r2k1/5pp1/p7/P1qp1PP1/8/1P1R3K/3Q4/8 w - - 5 39", 5, 10},
		{"3r2k1/5pp1/p7/P1qp1PP1/1P6/3R3K/3Q4/8 b - - 5 39", 50, 90},
		{"3r2k1/5pp1/p7/P2p1PP1/1P6/3R3K/3Q4/6q1 w - - 1 40", -70, -40},
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

func TestBackwardPawn(t *testing.T) {
	tests := []struct {
		name string
		fen  string
		sq   int
		sd   int
		want bool
	}{
		{
			name: "white backward pawn attacked on stop square",
			fen:  "4k3/8/8/4p3/2P1P3/3P4/8/4K3 w - - 0 1",
			sq:   square.D3,
			sd:   WHITE,
			want: true,
		},
		{
			name: "white pawn can advance safely",
			fen:  "4k3/8/8/8/2P1P3/3P4/8/4K3 w - - 0 1",
			sq:   square.D3,
			sd:   WHITE,
			want: false,
		},
		{
			name: "white backward pawn with piece blockade",
			fen:  "4k3/8/8/4p3/2PnP3/3P4/8/4K3 w - - 0 1",
			sq:   square.D3,
			sd:   WHITE,
			want: true,
		},
		{
			name: "white passed pawn is not backward",
			fen:  "4k3/8/8/8/2PnP3/3P4/8/4K3 w - - 0 1",
			sq:   square.D3,
			sd:   WHITE,
			want: false,
		},
		{
			name: "black backward pawn attacked on stop square",
			fen:  "4k3/8/3p4/2p1p3/4P3/8/8/4K3 b - - 0 1",
			sq:   square.D6,
			sd:   BLACK,
			want: true,
		},
		{
			name: "isolated pawn is not backward",
			fen:  "4k3/8/8/4p3/8/3P4/8/4K3 w - - 0 1",
			sq:   square.D3,
			sd:   WHITE,
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			board.SetFen(test.fen, &bd)
			if got := isBackward(test.sq, test.sd, &bd); got != test.want {
				t.Fatalf("isBackward() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestBackwardPawnPenalty(t *testing.T) {
	tests := []struct {
		name   string
		rank   int
		open   bool
		wantMG int
		wantEG int
	}{
		{name: "second rank closed", rank: square.Rank2, wantMG: 24, wantEG: 30},
		{name: "second rank open", rank: square.Rank2, open: true, wantMG: 48, wantEG: 30},
		{name: "third rank closed", rank: square.Rank3, wantMG: 16, wantEG: 20},
		{name: "fifth rank open", rank: square.Rank5, open: true, wantMG: 16, wantEG: 10},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mg, eg := backwardPawnPenalty(test.rank, test.open)
			if mg != test.wantMG || eg != test.wantEG {
				t.Fatalf("backwardPawnPenalty() = (%v, %v), want (%v, %v)", mg, eg, test.wantMG, test.wantEG)
			}
		})
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
			fmt.Println("\nall attacks from", text)
			board.PrintBB(ai.allAtks[sd])

			for pc := material.Pawn; pc <= material.King; pc++ {
				fmt.Printf("\nattacks from lower than %v %v", text, material.ToString(pc))
				//				board.PrintBB(ai.ltAtks[sd][pc])
			}
			text = "Black"
		}
	}
}

func Test_calcDist(t *testing.T) {
	type args struct {
		f      int
		t      int
		weight int
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{"a", args{square.D2, square.E3, 10}, 17},
		{"b", args{square.F2, square.E3, 20}, 33},
		{"c", args{square.E2, square.F2, 10}, 17},
		{"d", args{square.G2, square.F2, 20}, 33},
		{"e", args{square.E2, square.F5, 10}, 70},
		{"f", args{square.G2, square.F5, 20}, 140},
	}

	fmt.Println("\n\ndistWeight", distWeight)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := calcDist(tt.args.f, tt.args.t, tt.args.weight); got != tt.want {
				t.Errorf("\n%s: calcDist() = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
