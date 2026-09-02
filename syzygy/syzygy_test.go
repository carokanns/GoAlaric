package syzygy

import (
	"os"
	"slices"
	"testing"

	"goalaric/board"
)

func TestSquareAndBitboardTranspose(t *testing.T) {
	tests := []struct{ goSquare, syzygySquare int }{
		{0, 0},  // a1
		{1, 8},  // a2
		{56, 7}, // h1
		{63, 63},
	}
	for _, test := range tests {
		if got := toSyzygySquare(test.goSquare); got != test.syzygySquare {
			t.Fatalf("toSyzygySquare(%d) = %d, want %d", test.goSquare, got, test.syzygySquare)
		}
		if got := fromSyzygySquare(test.syzygySquare); got != test.goSquare {
			t.Fatalf("fromSyzygySquare(%d) = %d, want %d", test.syzygySquare, got, test.goSquare)
		}
	}
	if got := transpose((uint64(1) << 1) | (uint64(1) << 56)); got != (uint64(1)<<8)|(uint64(1)<<7) {
		t.Fatalf("transpose = %#x", got)
	}
}

func TestTableCardinalityGate(t *testing.T) {
	if !withinTableCardinality(3, 3) {
		t.Fatal("a position matching the largest loaded table should be probeable")
	}
	if withinTableCardinality(4, 3) {
		t.Fatal("a four-piece position must not reach a three-piece tablebase")
	}
	if withinTableCardinality(2, 0) {
		t.Fatal("probing must remain disabled when no tables are loaded")
	}
}

func TestConfiguredTablesRejectExcessCardinality(t *testing.T) {
	path := os.Getenv("GOALARIC_SYZYGY_PATH")
	if path == "" {
		t.Skip("GOALARIC_SYZYGY_PATH is not set")
	}
	if err := SetPath(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = SetPath("") })

	// Eight pieces is above Fathom's supported table cardinality and above
	// the small local set. The wrapper must reject it before Fathom is called.
	var bd board.Board
	board.SetFen("kqrb4/8/8/8/8/8/8/KQRB4 w - - 0 1", &bd)
	ResetHits()
	if _, ok := ProbeWDL(&bd, 100, 1); ok {
		t.Fatal("WDL probe above loaded cardinality succeeded")
	}
	if _, ok := ProbeRoot(&bd); ok {
		t.Fatal("root probe above loaded cardinality succeeded")
	}
	if Hits() != 0 {
		t.Fatalf("Hits() = %d after rejected probes, want 0", Hits())
	}
}

func TestTablebaseOracle(t *testing.T) {
	path := os.Getenv("GOALARIC_SYZYGY_PATH")
	if path == "" {
		t.Skip("GOALARIC_SYZYGY_PATH is not set")
	}
	if err := SetPath(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = SetPath("") })

	tests := []struct {
		name      string
		fen       string
		wdl       WDL
		dtz       int
		rootMoves []string
		searchWDL bool
	}{
		{
			name: "win", fen: "8/8/8/8/8/8/8/QK1k4 w - - 0 1",
			wdl: Win, dtz: 9, searchWDL: true,
			rootMoves: []string{"b1a2", "b1b2", "a1a2", "a1b2", "a1a3", "a1c3", "a1a4", "a1d4", "a1a5", "a1e5", "a1a6", "a1f6", "a1a7", "a1g7", "a1a8", "a1h8"},
		},
		{
			name: "loss", fen: "8/8/8/8/8/8/8/QK1k4 b - - 0 1",
			wdl: Loss, dtz: 14, searchWDL: true,
			rootMoves: []string{"d1e1", "d1d2", "d1e2"},
		},
		{
			name: "draw", fen: "8/7k/8/8/8/8/8/K1B5 w - - 0 1",
			wdl: Draw, dtz: 0, searchWDL: true,
			rootMoves: []string{"a1b1", "a1a2", "a1b2", "c1b2", "c1d2", "c1a3", "c1e3", "c1f4", "c1g5", "c1h6"},
		},
		{
			name: "fifty move cursed win", fen: "8/8/8/8/8/8/8/QK1k4 w - - 99 1",
			wdl: CursedWin, dtz: 9,
			rootMoves: []string{"b1a2", "b1b2", "a1a2", "a1b2", "a1a3", "a1c3", "a1a4", "a1d4", "a1a5", "a1e5", "a1a6", "a1f6", "a1a7", "a1g7", "a1a8", "a1h8"},
		},
		{
			name: "en passant", fen: "8/8/8/3pP3/8/8/8/K6k w - d6 0 1",
			wdl: Win, dtz: 1, searchWDL: true,
			rootMoves: []string{"a1b1", "a1a2", "a1b2", "e5d6", "e5e6"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var bd board.Board
			board.SetFen(test.fen, &bd)
			root, ok := ProbeRoot(&bd)
			if !ok {
				t.Fatal("root probe failed")
			}
			if root.WDL != test.wdl || root.DTZ != test.dtz || !slices.Contains(test.rootMoves, root.Move) {
				t.Fatalf("root = %+v, want WDL=%d DTZ=%d move in %v", root, test.wdl, test.dtz, test.rootMoves)
			}
			wdl, ok := ProbeWDL(&bd, 4, 1)
			if ok != test.searchWDL {
				t.Fatalf("search probe success = %v, want %v", ok, test.searchWDL)
			}
			if ok && wdl != test.wdl {
				t.Fatalf("search WDL = %d, want %d", wdl, test.wdl)
			}
		})
	}

	var castling board.Board
	board.SetFen("r3k3/8/8/8/8/8/8/R3K3 w Qq - 0 1", &castling)
	if _, ok := ProbeRoot(&castling); ok {
		t.Fatal("position with castling rights must not be probed")
	}

	var fullCardinality board.Board
	fullCardinalityFEN := "2K5/k7/8/8/5q2/8/3B4/8 w - - 0 1"
	if Largest() == 5 {
		fullCardinalityFEN = "2K5/k7/8/8/5q2/8/3B4/4N3 w - - 0 1"
	}
	board.SetFen(fullCardinalityFEN, &fullCardinality)
	if _, ok := ProbeWDL(&fullCardinality, 0, 1); ok {
		t.Fatal("full-cardinality table should respect probe depth")
	}
	if _, ok := ProbeWDL(&fullCardinality, 1, 1); !ok {
		t.Fatal("full-cardinality table should probe at configured depth")
	}
}

func TestPositionConversion(t *testing.T) {
	var bd board.Board
	board.SetFen("8/8/8/3k4/8/8/4P3/4K3 w - - 17 1", &bd)
	got := position(&bd)
	if got.White != (uint64(1)<<4)|(uint64(1)<<12) {
		t.Fatalf("white = %#x", got.White)
	}
	if got.Black != uint64(1)<<35 {
		t.Fatalf("black = %#x", got.Black)
	}
	if got.Rule50 != 17 || !got.WhiteToMove || got.Castling != 0 || got.EnPassant != 0 {
		t.Fatalf("position metadata = %+v", got)
	}
	board.SetFen("8/8/8/3pP3/8/8/8/K6k w - d6 0 1", &bd)
	if got := position(&bd).EnPassant; got != 43 { // d6 in A1=0 Syzygy coordinates
		t.Fatalf("en passant square = %d, want 43", got)
	}
}

func TestDisabledWithoutTables(t *testing.T) {
	if err := SetPath(""); err != nil {
		t.Fatal(err)
	}
	var bd board.Board
	board.SetFen("8/8/8/3k4/8/8/4P3/4K3 w - - 0 1", &bd)
	if _, ok := ProbeWDL(&bd, 8, 1); ok {
		t.Fatal("disabled WDL probe succeeded")
	}
	if _, ok := ProbeRoot(&bd); ok {
		t.Fatal("disabled root probe succeeded")
	}
	if Hits() != 0 {
		t.Fatalf("Hits() = %d, want 0", Hits())
	}
}

func TestInvalidPathDisablesProbing(t *testing.T) {
	err := SetPath("/definitely/not/a/goalaric/syzygy/path")
	if err == nil {
		t.Fatal("invalid path succeeded")
	}
	if Enabled() || Largest() != 0 {
		t.Fatalf("enabled=%v largest=%d after invalid path", Enabled(), Largest())
	}
}
