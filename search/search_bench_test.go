package search

import (
	"testing"

	"goalaric/board"
)

func benchmarkSearchID(b *testing.B, depth int, fen string) {
	var base board.Board
	board.SetFen(fen, &base)

	oldTell := tellGUI
	tellGUI = func(string) {}
	defer func() { tellGUI = oldTell }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bd := base

		NewSearch()
		SetMaxDepth(depth)
		SetStop(false)

		clear()
		initSg()
		slInitEarly(&slEntries[0], 0)
		slInitLate(&slEntries[0])
		rootSP.initRoot(&slEntries[0])

		searchID(&bd)
	}
}

func BenchmarkSearchIDDepth2(b *testing.B) {
	benchmarkSearchID(b, 2, board.StartFen)
}

func BenchmarkSearchIDDepth3(b *testing.B) {
	benchmarkSearchID(b, 3, "r1bqkbnr/pppp1ppp/2n5/4p3/2B1P3/5N2/PPPP1PPP/RNBQK2R w KQkq - 2 3")
}
