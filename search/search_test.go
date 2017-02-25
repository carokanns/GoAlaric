// search_test.go
package search

import (
	"strings"
	//	"GoAlaric/attack"

	"GoAlaric/board"
	_ "GoAlaric/engine"
	"GoAlaric/eval"
	"GoAlaric/gen"
	"GoAlaric/gen2"
	_ "GoAlaric/material"
	"GoAlaric/move"
	"GoAlaric/piece"
	"GoAlaric/score"
	"GoAlaric/sort"
	"fmt"
	"time"

	//	"GoAlaric/input"

	//	"GoAlaric/pawn"
	//"GoAlaric/pst"
	"testing"
)

var bd board.Board
var sl searchLocal

func Test_Next(t *testing.T) {
	board.SetFen("8/6kp/5p2/3n2pq/3N1n1R/1P3P2/P6P/4QK2 w - - 2 2", &bd)
	var attacks eval.Attacks
	var killer sort.Killer
	var history sort.HistoryTab
	var ml gen2.List

	killer.Clear()
	history.Clear()
	eval.InitAttacks(&attacks, bd.Stm(), &bd)
	transMove := move.None
	useFP := false
	depth := 1
	ml.Init(depth, &bd, &attacks, transMove, &killer, &history, useFP)

	bFail := false
	for mv := ml.Next(); mv != move.None; mv = ml.Next() {
		if mv == 945916 {
			break
		}
		bFail = true
		mov := move.ToString(mv)
		fmt.Printf(mov + " ")
	}
	fmt.Println()
	if bFail {
		t.Errorf("H4H5 borde sorterats först. Blev dragen ovan")
	}

}

// testing a bug in prom. Where pawn captured forward 8 squares when prom
func Test_promGen(t *testing.T) {
	board.SetFen("r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPbbPPP/R3K2R w KQkq -", &bd)
	board.FenMoves([]string{"d5e6", "h3g2", "h1g1"}, &bd)
	var ml gen.ScMvList

	gen.LegalMoves(&ml, &bd)
	bFound := false
	for pos := 0; pos < ml.Size(); pos++ {
		mv := ml.Move(pos)
		tMve := move.ToString(mv)
		if tMve == "G2G1q" ||
			tMve == "G212n" ||
			tMve == "G2G1r" ||
			tMve == "G2G1b" {
			bFound = true
			break
		}
	}
	if bFound {
		t.Errorf("case 1: prom is impossible")
	}

	board.SetFen("k7/8/8/8/8/8/6p1/K5N1 b - - 0 1", &bd)
	ml.Clear()
	gen.LegalMoves(&ml, &bd)
	bFound = false
	for pos := 0; pos < ml.Size(); pos++ {
		mv := ml.Move(pos)
		tMve := strings.TrimSpace(move.ToString(mv))
		if tMve == "g2g1q" ||
			tMve == "g2g1n" ||
			tMve == "g2g1r" ||
			tMve == "g2g1b" {
			bFound = true
			break
		}
	}
	if bFound {
		t.Errorf("case 2: prom is impossible")
	}

}

func Test_QS(t *testing.T) {
	type seeStruct struct {
		fen     string
		val     int
		comment string
	}

	var seeTest = [...]seeStruct{
		{"rnbqkbnr/ppppp2p/8/5pp1/3PP3/8/PPP2PPP/RNBQKBNR b KQkq - 0 3", -12, "After fxe4 and Bxg5 is it material equal"},
		{"rnbqkbnr/ppppp2p/8/5pp1/3PP3/8/PPPK1PPP/RNBQ1BNR b kq - 1 3", 32, "fxe4 and black is pawn up"},
		{"rnbqkbnr/ppppp2p/8/5pB1/3PP3/8/PPP2PPP/RN1QKBNR b KQkq - 0 3", -122, "equal after fxe4"},
		{"rnbqkbnr/p1pppppp/8/1p6/2P5/8/PP1PPPPP/RNBQKBNR w KQkq - 0 2", piece.PawnValue - 16, "Pawn can take unguarded pawn"},
		// Pawn
		{"rnbqkbnr/ppp1pppp/8/3p4/2P5/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", -28, "Pawn captures guarded pawn"},
		{"rnb1kbnr/ppp1pppp/8/3n4/2P5/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", piece.PawnValue + piece.QueenValue - 31, "Pawn captures unguarded knight. Now under with queen and pawn"},
		{"rnbqkbnr/ppp1pppp/8/3p4/2P5/8/PP1PPPPP/RNBQKBNR b KQkq - 0 1", piece.PawnValue + 28, "Bl Pawn captures unguarded W pawn"},
		// Knight
		{"rnbqkbnr/ppp1pppp/8/3p4/1N6/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", piece.KnightValue - piece.PawnValue - 8, "White is up knight-pawn"},
		{"rnb1kbnr/ppp1pppp/8/3n4/5N2/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", piece.KnightValue + piece.QueenValue - 21, "White is up knight and queen"},
		{"rnbqkbnr/ppp1pppp/8/3n4/5N2/8/PP1PPPPP/RNBQKBNR b KQkq - 0 1", piece.KnightValue + 39, "Bl Knigh captures guarded W queen"},
		// Bishop
		{"rnb1kbnr/ppp1pppp/8/3p4/2B5/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", piece.BishopValue + piece.QueenValue + 4, "White is up bishop and queen"},
		{"rnbqkbnr/ppp1pppp/8/3n4/4B3/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", -3, "Bishop captures guarded knight"},
		{"rnbqkbnr/ppp1pppp/8/3b4/4Q3/8/PP1PPPPP/RNBQKBNR b KQkq - 0 1", piece.BishopValue + 53, "Black is up a bishop"},
		// Rook
		{"rnb1kbnr/ppp1pppp/8/3p4/3R4/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", piece.RookValue + piece.QueenValue + 1, "White is up Rook and queen"},
		{"rnbqkbnr/ppp1pppp/8/3n4/3R4/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", piece.RookValue - piece.KnightValue - 24, "White is up Rook vs knight"},
		{"rnbqkbnr/ppp1pppp/8/3r1B2/8/8/PP1PPPPP/RNBQKBNR b KQkq - 0 1", piece.RookValue + 13, "Black is up a Rook"},
		// Queen
		{"rnbqkbnr/ppp1pppp/8/3p4/4Q3/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", piece.QueenValue - piece.PawnValue - 39, "White is up a Queen vs a Pawn"},
		{"rnb1kbnr/ppp1pppp/8/3n4/2Q5/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", piece.QueenValue*2 + 52, "White is up 2 Queens"},
		{"rnbqkbnr/ppp1pppp/8/3q4/4Q3/8/PP1PPPPP/RNBQKBNR b KQkq - 0 1", piece.QueenValue + 1, "Black Queen captures guarded W queen and is up a Queen"},
		// King
		{"rnb1kbnr/ppp1pppp/8/3p4/2K5/8/PP1PPPPP/RNBQ1BNR w KQkq - 0 1", piece.QueenValue - 67, "White is Queen up after King captures unguarded pawn"},
		{"rnbqkbnr/ppp1pppp/8/3n4/3K4/8/PP1PPPPP/RNBQ1BNR w KQkq - 0 1", -piece.KnightValue - 277, "Black is knight up"},
		{"rnbq1bnr/ppp1pppp/8/3k4/4N3/8/PP1PPPPP/RNBQKBNR b KQkq - 0 1", -104, "Bl King captures unguarded W knight. It's equal"},
	}
	var bd board.Board
	sl.ID = 0
	sl.board = bd
	for ix, ss := range seeTest {
		board.SetFen(ss.fen, &sl.board)
		//alpha := 0
		//beta := score.EVAL_MAX
		//mv := move.Make(fr, to, pc, cp, pr)
		rightVal := ss.val

		val := qs(&sl, score.Max, 0)

		if val != rightVal {
			t.Errorf("Case %v: gav %v istf %v - %v", ix+1, val, rightVal, ss.comment)
		}
	}
}

func Test_RootSearch(t *testing.T) {
	chSearch := make(chan int)
	chBestmove := make(chan string)
	Infinite = false
	board.SetFen("8/7p/5pk1/3n2pq/3N1nR1/1P3P2/P6P/4QK2 w - - 0 1" /*board.Start_fen*/, &bd)
	//	var moves = []string{"e2e4", "f7f5", "d2d4", "g7g5", "d1e2"}
	//	board.FenMoves(moves, &bd)
	go StartSearch(chSearch, chBestmove, &bd)

	// search condition
	Infinite = true
	chSearch <- Simple
	var bm = "     "
	for ix := 0; ix < 5; ix++ {
		select {
		case bm = <-chBestmove:
			break
		default:
			SetStop(true)
			time.Sleep(time.Millisecond * 100)

		}
	}
	fmt.Println(bm)
}

func TestSetHard(t *testing.T) {
	board.SetFen("8/6kp/5p2/3n2pq/3N1n1R/1P3P2/P6P/4QK2 w - - 2 2", &bd)

	wtime := int64(1 * 60 * 1000)
	btime := wtime
	mtg := int64(0)
	winc := int64(0)
	binc := int64(0)
	SetHard(&bd, wtime, btime, winc, binc, mtg)
	//	p_time.hard = true
	//	p_time.limit_0 = int64(math.Min(float64(alloc), float64(max)))
	//	p_time.limit_1 = int64(math.Min(float64(alloc*4), float64(max)))
	//	p_time.limit_2 = int64(max)
	//	p_time.last_score = score.None
	//	p_time.drop = false
	if !timeInfo.hard {
		t.Errorf("hard ej satt")
	}
	if false {
		t.Errorf("Kolla upp wtime %v", wtime)
		t.Errorf("Kolla upp p_time.limit_0 %v", timeInfo.limitA)
		t.Errorf("Kolla upp p_time.limit_1 %v", timeInfo.limitB)
		t.Errorf("Kolla upp p_time.limit_2 %v", timeInfo.limitC)
	}

}

func BenchmarkSearch(b *testing.B) {
	chSearch := make(chan int)
	chBestmove := make(chan string)
	Infinite = false
	board.SetFen(board.StartFen, &bd)
	go StartSearch(chSearch, chBestmove, &bd)

	// search condition
	Infinite = true
	chSearch <- Simple
	var bm = "     "
	for i := 0; i < b.N; i++ {
		select {
		case bm = <-chBestmove:
			break
		default:
			time.Sleep(time.Millisecond * 100)
		}
	}
	SetStop(true)
	_ = bm
}
