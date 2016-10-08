// gen2_test.go
package gen2

import (
	//	"GoAlaric/attack"

	"GoAlaric/board"
	"GoAlaric/move"
	//	"GoAlaric/pawn"
	"GoAlaric/piece"
	//	"GoAlaric/pst"
	"GoAlaric/score"
	"GoAlaric/square"

	"testing"
)

var bd board.Board

//var See SEE
func initAll() { // copy of main initSession()
	// input.Init()
	//engine.Init()
	//material.Init()
	//eval.PstInit()
	//eval.PawnInit()
	//eval.Init()
	//	search.Init()
	//bit.InitBits()
	//hash.Init()
	//castling.Init()
	//eval.AtkInit()
}

func Test_SEE(t *testing.T) {
	type seeStruct struct {
		fen                     string
		fr, to, pc, cp, pr, val int
		comment                 string
	}

	// King Value is more than max beta so we use 9025 for QS here
	kingValue := 9025

	var seeTest = [...]seeStruct{
		// Pawn
		{"rnbqkbnr/ppp1pppp/8/3p4/2P5/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.C4, square.D5, piece.Pawn, piece.Pawn, piece.None, 0, "Pawn captures guarded pawn"},
		{"rnbqkbnr/ppp1pppp/8/3n4/2P5/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.C4, square.D5, piece.Pawn, piece.Knight, piece.None, piece.KnightValue - piece.PawnValue, "Pawn captures guarded knight"},
		{"rnbqkbnr/ppp1pppp/8/3b4/2P5/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.C4, square.D5, piece.Pawn, piece.Bishop, piece.None, piece.BishopValue - piece.PawnValue, "Pawn captures unguarded bishop"},
		{"rnbqkbnr/ppp1pppp/8/3r4/4P3/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.E4, square.D5, piece.Pawn, piece.Rook, piece.None, piece.RookValue - piece.PawnValue, "Pawn captures unguarded Rook"},
		{"rnbqkbnr/ppp1pppp/8/3q4/4P3/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.E4, square.D5, piece.Pawn, piece.Queen, piece.None, piece.QueenValue - piece.PawnValue, "Pawn captures unguarded queen"},
		{"rnbqkbnr/ppp1pppp/8/3p4/2P5/8/PP1PPPPP/RNBQKBNR b KQkq - 0 1", square.D5, square.C4, piece.Pawn, piece.Pawn, piece.None, piece.PawnValue, "Bl Pawn captures unguarded W pawn"},

		// Knight
		{"8/1k6/8/8/8/1n6/6K1/N7 w - - 0 1", square.A1, square.B3, piece.Knight, piece.Knight, piece.None, piece.KnightValue, "Knight captures unguarded knight"},
		{"8/1k6/8/8/8/1n6/6K1/N7 b - - 0 1", square.B3, square.A1, piece.Knight, piece.Knight, piece.None, piece.KnightValue, "Bl Knight captures unguarded w knight"},
		{"8/8/8/8/1k6/1p6/6K1/N7 w - - 0 1", square.A1, square.B3, piece.Knight, piece.Pawn, piece.None, piece.PawnValue - piece.KnightValue, "Knight captures guarded pawn"},
		{"rnb1kbnr/ppp1pppp/8/3p4/1N6/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.B4, square.D5, piece.Knight, piece.Pawn, piece.None, piece.PawnValue, "Knigh captures unguarded pawn"},
		{"rnbqkbnr/ppp1pppp/8/3n4/5N2/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.F4, square.D5, piece.Knight, piece.Knight, piece.None, 0, "Knigh captures guarded knight"},
		{"rnbqkbnr/ppp1pppp/8/3b4/5N2/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.F4, square.D5, piece.Knight, piece.Bishop, piece.None, 0, "Knigh captures guarded bishop"},
		{"rnbqkbnr/ppp1pppp/8/3r4/1N6/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.B4, square.D5, piece.Knight, piece.Rook, piece.None, piece.RookValue - piece.KnightValue, "Knigh captures guarded Rook"},
		{"rnbqkbnr/ppp1pppp/8/3q4/1N6/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.B4, square.D5, piece.Knight, piece.Queen, piece.None, piece.QueenValue - piece.KnightValue, "Knigh captures guarded queen"},
		{"rnbqkbnr/ppp1pppp/8/3n4/5N2/8/PP1PPPPP/RNBQKBNR b KQkq - 0 1", square.D5, square.F4, piece.Knight, piece.Knight, piece.None, piece.KnightValue, "Bl Knigh captures unguarded W queen"},

		// Bishop
		{"rnb1kbnr/ppp1pppp/8/3p4/2B5/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.C4, square.D5, piece.Bishop, piece.Pawn, piece.None, piece.PawnValue, "Bishop captures unguarded pawn"},
		{"rnbqkbnr/ppp1pppp/8/3n4/4B3/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.E4, square.D5, piece.Bishop, piece.Knight, piece.None, 0, "Bishop captures guarded knight"},
		{"rnb1kbnr/ppp1pppp/8/3b4/2B5/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.C4, square.D5, piece.Bishop, piece.Bishop, piece.None, piece.BishopValue, "Bishop captures unguarded bishop"},
		{"rnbqkbnr/ppp1pppp/8/3r4/4B3/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.E4, square.D5, piece.Bishop, piece.Rook, piece.None, piece.RookValue - piece.BishopValue, "Bishop captures guarded Rook"},
		{"rnbqkbnr/ppp1pppp/8/3q4/4B3/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.E4, square.D5, piece.Bishop, piece.Queen, piece.None, piece.QueenValue - piece.BishopValue, "Bishop captures guarded queen"},
		{"rnbqkbnr/ppp1pppp/8/3b4/4Q3/8/PP1PPPPP/RNBQKBNR b KQkq - 0 1", square.D5, square.E4, piece.Bishop, piece.Queen, piece.None, piece.QueenValue, "Bl Bishop captures unguarded W queen"},

		// Rook
		{"rnbqkbnr/ppp1pppp/8/3p4/3R4/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.D4, square.D5, piece.Rook, piece.Pawn, piece.None, piece.PawnValue - piece.RookValue, "Rook captures guarded pawn"},
		{"rnb1kbnr/ppp1pppp/8/3n4/3R4/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.D4, square.D5, piece.Rook, piece.Knight, piece.None, piece.KnightValue, "Rook captures unguarded knight"},
		{"rnbqkbnr/ppp1pppp/8/3b4/3R4/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.D4, square.D5, piece.Rook, piece.Bishop, piece.None, piece.BishopValue - piece.RookValue, "Rook captures guarded bishop"},
		{"rnbqkbnr/ppp1pppp/8/3r4/8/3R4/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.D3, square.D5, piece.Rook, piece.Rook, piece.None, 0, "Rook captures guarded Rook"},
		{"rnbqkbnr/ppp1pppp/8/3q1R2/8/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.F5, square.D5, piece.Rook, piece.Queen, piece.None, piece.QueenValue - piece.RookValue, "Rook captures guarded queen"},
		{"rnbqkbnr/ppp1pppp/8/3r1B2/8/8/PP1PPPPP/RNBQKBNR b KQkq - 0 1", square.D5, square.F5, piece.Rook, piece.Bishop, piece.None, piece.BishopValue, "Bl Rook captures unguarded W bishop"},

		// Queen
		{"rnb1kbnr/ppp1pppp/8/3p4/4Q3/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.E4, square.D5, piece.Queen, piece.Pawn, piece.None, piece.PawnValue, "Queen captures unguarded pawn"},
		{"rnbqkbnr/ppp1pppp/8/3n4/2Q5/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.C4, square.D5, piece.Queen, piece.Knight, piece.None, piece.KnightValue - piece.QueenValue, "Queen captures guarded knight"},
		{"rnb1kbnr/ppp1pppp/8/3b4/3Q4/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.D4, square.D5, piece.Queen, piece.Bishop, piece.None, piece.BishopValue, "Queen captures unguarded bishop"},
		{"rnbqkbnr/ppp1pppp/8/3r4/4Q3/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.E4, square.D5, piece.Queen, piece.Rook, piece.None, piece.RookValue - piece.QueenValue, "Queen captures guarded Rook"},
		{"rnb1kbnr/ppp1pppp/8/3q4/4Q3/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.E4, square.D5, piece.Queen, piece.Queen, piece.None, piece.QueenValue, "Queen captures unguarded queen"},
		{"rnbqkbnr/ppp1pppp/8/3q4/4Q3/8/PP1PPPPP/RNBQKBNR b KQkq - 0 1", square.D5, square.E4, piece.Queen, piece.Queen, piece.None, piece.QueenValue, "Bl Queen captures unguarded W queen"},

		// King
		{"rnb1kbnr/ppp1pppp/8/3p4/2K5/8/PP1PPPPP/RNBQ1BNR w KQkq - 0 1", square.C4, square.D5, piece.King, piece.Pawn, piece.None, piece.PawnValue, "King captures unguarded pawn"},
		{"rnbqkbnr/ppp1pppp/8/3n4/3K4/8/PP1PPPPP/RNBQ1BNR w KQkq - 0 1", square.D4, square.D5, piece.King, piece.Knight, piece.None, piece.KnightValue - kingValue, "King captures guarded knight"},
		{"rnb1kbnr/ppp1pppp/8/3b4/2K5/8/PP1PPPPP/RNBQ1BNR w KQkq - 0 1", square.C4, square.D5, piece.King, piece.Bishop, piece.None, piece.BishopValue, "King captures unguarded bishop"},
		{"rnbqkbnr/ppp1pppp/8/3r4/3K4/8/PP1PPPPP/RNBQ1BNR w KQkq - 0 1", square.D4, square.D5, piece.King, piece.Rook, piece.None, piece.RookValue - kingValue, "King captures guarded Rook"},
		{"rnbqkbnr/ppp1pppp/8/3q4/4K3/8/PP1PPPPP/RNBQ1BNR w KQkq - 0 1", square.E4, square.D5, piece.King, piece.Queen, piece.None, piece.QueenValue - kingValue, "King captures guarded queen"},
		{"rnbq1bnr/ppp1pppp/8/3k4/4N3/8/PP1PPPPP/RNBQKBNR b KQkq - 0 1", square.D5, square.E4, piece.King, piece.Knight, piece.None, piece.KnightValue, "Bl King captures unguarded W queen"},
	}

	var bd board.Board
	initAll()
	var se SEE
	for ix, ss := range seeTest {
		board.SetFen(ss.fen, &bd)
		fr := ss.fr
		to := ss.to
		pc := ss.pc
		cp := ss.cp
		pr := ss.pr
		alpha := 0
		beta := score.EvalMAX
		mv := move.Build(fr, to, pc, cp, pr)
		rightVal := ss.val
		if fr >= square.BoardSize || to >= square.BoardSize || pc >= piece.Size || pr >= piece.Size ||
			pc == piece.None || (pr != piece.None && pr != piece.Pawn) {
			t.Fatalf("Felaktig input till testcase %v. fr:%v to:%v pc:%v cp:%v prom:%v", ix+1, fr, to, pc, cp, pr)
		}

		val, _ := se.See(mv, alpha, beta, &bd)

		if val != rightVal {
			t.Errorf("Testcase %v: %v. Borde bli %v men blev %v", ix+1, ss.comment, rightVal, val)
		}
	}
}

func Test_Lva(t *testing.T) {
	var bd board.Board
	var se SEE

	type lvaStruct struct {
		fen     string
		fr, to  int
		comment string
	}

	var lvaTest = [...]lvaStruct{
		// Pawn
		//		{"rnbqkbnr/ppp1pppp/8/3p4/2P5/8/PP1PPPPP/RNBQKBNR w KQkq - 0 1", square.A1, square.D5, square.None, "Pawn captures guarded pawn"},

		{"8/1k6/8/2n5/8/1q6/6K1/N7 w - - 0 1", square.A1, square.B3, "Knight a1 only one"},
		{"8/1k6/8/8/8/1n6/6K1/N7 b - - 0 1", square.B3, square.A1, "Bl Knight b3 only one"},
		{"6b1/1k6/1r6/8/8/1n4Q1/2P3K1/N7 w - - 0 1", square.C2, square.B3, "Pawn on c2 the lowest value"},
		{"6b1/1k6/1r6/8/8/1n4Q1/2P3K1/N7 w - - 0 1", square.None, square.D4, "No one attacks d4"},
		{"6b1/1k6/1r6/8/8/1n4Q1/2P3K1/N7 w - - 0 1", square.G8, square.B3, "Bl Bishop g8 the lowest value"},
	}
	initAll()
	for ix, lva := range lvaTest {
		board.SetFen(lva.fen, &bd)
		se.board = &bd
		// pc := se.p_board.Square(lva.fr1)
		sd := se.board.SquareSide(lva.fr)
		se.init(lva.to, sd)

		// capVal := se.move(lva.fr1)
		rightFr := lva.fr
		nextFr := se.pickLva()
		if nextFr != rightFr {
			t.Errorf("Testcase %v: %v. Borde bli %v men blev %v", ix+1, lva.comment, rightFr, nextFr)
		}
	}
}
