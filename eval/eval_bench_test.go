package eval

import (
	"testing"

	"goalaric/board"
)

func BenchmarkCompEval(b *testing.B) {
	positions := []struct {
		name string
		fen  string
	}{
		{name: "Start", fen: board.StartFen},
		{name: "OpenCenter", fen: "r1bq1rk1/ppp1p1bp/5np1/3Ppp2/8/1QP3P1/PP2PPBP/RNBR2K1 b - - 3 10"},
		{name: "Endgame", fen: "8/1k6/8/5K2/8/6P1/8/8 w - - 0 1"},
	}

	var pawnHash PawnHash

	for _, tc := range positions {
		b.Run(tc.name, func(b *testing.B) {
			var bd board.Board
			board.SetFen(tc.fen, &bd)
			pawnHash.Clear()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				CompEval(&bd, &pawnHash)
			}
		})
	}
}

func BenchmarkHashEval(b *testing.B) {
	var (
		evalHash Hash
		pawnHash PawnHash
		bd       board.Board
	)

	board.SetFen("3r2k1/5pp1/p7/P1qp1PP1/8/1P1R3K/3Q4/8 w - - 5 39", &bd)
	pawnHash.Clear()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		evalHash.Eval(&bd, &pawnHash)
	}
}
