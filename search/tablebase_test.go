package search

import (
	"path/filepath"
	"testing"

	"goalaric/bit"
	"goalaric/board"
	"goalaric/gen"
	"goalaric/square"
	"goalaric/syzygy"
)

func TestSyzygySquareAndBitboardConversion(t *testing.T) {
	tests := []struct {
		internal int
		standard int
	}{
		{square.A1, 0},
		{square.A2, 8},
		{square.B1, 1},
		{square.H8, 63},
	}
	for _, test := range tests {
		if got := toSyzygySquare(test.internal); got != test.standard {
			t.Fatalf("square %d converted to %d, want %d", test.internal, got, test.standard)
		}
		if got := fromSyzygySquare(test.standard); got != test.internal {
			t.Fatalf("square %d converted back to %d, want %d", test.standard, got, test.internal)
		}
	}

	internal := bit.Bit(square.A2) | bit.Bit(square.B1) | bit.Bit(square.H8)
	want := uint64(1)<<8 | uint64(1)<<1 | uint64(1)<<63
	if got := toSyzygyBitboard(internal); got != want {
		t.Fatalf("bitboard conversion = %#x, want %#x", got, want)
	}
}

func TestGoAlaricRootProbeUsesLocalTables(t *testing.T) {
	if !syzygy.Available() {
		t.Skip("Syzygy probing requires cgo")
	}
	oldPath := Engine.SyzygyPath
	defer func() { _, _ = SetSyzygyPath(oldPath) }()

	path, err := filepath.Abs(filepath.Join("..", ".tools", "syzygy", "3-4"))
	if err != nil {
		t.Fatal(err)
	}
	if largest, err := SetSyzygyPath(path); err != nil || largest != 4 {
		t.Fatalf("SetSyzygyPath() = %d, %v", largest, err)
	}

	var bd board.Board
	board.SetFen("4k3/8/8/8/8/8/8/3QK3 w - - 0 1", &bd)
	var legal gen.ScMvList
	gen.LegalMoves(&legal, &bd)
	result, mv, ok := probeRootTablebase(&bd, &legal)
	if !ok || result.WDL != syzygy.Win || mv == 0 {
		t.Fatalf("root probe = %+v, move=%d, ok=%v", result, mv, ok)
	}
}

func TestSearchWDLProbeHonorsHalfmoveClock(t *testing.T) {
	if !syzygy.Available() {
		t.Skip("Syzygy probing requires cgo")
	}
	oldPath := Engine.SyzygyPath
	defer func() { _, _ = SetSyzygyPath(oldPath) }()
	path, err := filepath.Abs(filepath.Join("..", ".tools", "syzygy", "3-4"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SetSyzygyPath(path); err != nil {
		t.Fatal(err)
	}

	var bd board.Board
	board.SetFen("4k3/8/8/8/8/8/8/3QK3 w - - 0 1", &bd)
	if score, ok := probeTablebaseScore(&bd); !ok || score != tablebaseWinScore {
		t.Fatalf("zero-clock WDL score = %d, ok=%v", score, ok)
	}

	board.SetFen("4k3/8/8/8/8/8/8/3QK3 w - - 1 1", &bd)
	if _, ok := probeTablebaseScore(&bd); ok {
		t.Fatal("WDL probe accepted a non-zero halfmove clock")
	}
}

func TestTablebaseGameCountersAreConsumable(t *testing.T) {
	ResetTablebaseGameStats()
	recordTablebaseHit(false)
	recordTablebaseHit(true)
	if hits, rootWins := ConsumeTablebaseGameStats(); hits != 2 || rootWins != 1 {
		t.Fatalf("stats = hits %d root wins %d", hits, rootWins)
	}
	if hits, rootWins := ConsumeTablebaseGameStats(); hits != 0 || rootWins != 0 {
		t.Fatalf("consumed stats repeated: hits %d root wins %d", hits, rootWins)
	}
}
