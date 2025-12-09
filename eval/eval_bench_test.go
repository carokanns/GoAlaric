package eval

import (
	"testing"

	"goalaric/board"
)

func benchmarkCompEval(b *testing.B, fen string) {
	var bd board.Board
	board.SetFen(fen, &bd)
	var pawnHash PawnHash

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CompEval(&bd, &pawnHash)
	}
}

func BenchmarkCompEvalStartPos(b *testing.B) {
	benchmarkCompEval(b, board.StartFen)
}

func BenchmarkCompEvalTactical(b *testing.B) {
	benchmarkCompEval(b, "r1bq1rk1/ppp2ppp/2np1n2/3N4/2B1P3/2N5/PPP2PPP/R1BQ1RK1 w - - 1 8")
}

func BenchmarkHashEvalVariedPositions(b *testing.B) {
	fens := []string{
		board.StartFen,
		"r1bqkbnr/pppp1ppp/2n5/4p3/2B1P3/5N2/PPPP1PPP/RNBQK2R w KQkq - 2 3",
		"8/1p3pk1/p6p/3p4/3P4/P7/1P4PP/6K1 w - - 0 35",
		"4rrk1/1pq2pbp/p2p1np1/2pP4/4P3/1PQ2NP1/PP2BPBP/R2R2K1 w - - 4 18",
	}

	boards := make([]board.Board, len(fens))
	for i, fen := range fens {
		board.SetFen(fen, &boards[i])
	}

	var pawnHash PawnHash
	var evalHash Hash

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bd := &boards[i%len(boards)]
		_ = evalHash.Eval(bd, &pawnHash)
	}
}
