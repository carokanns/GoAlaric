package syzygy

import (
	"path/filepath"
	"testing"
)

func loadLocalTables(t *testing.T) {
	t.Helper()
	if !Available() {
		t.Skip("Syzygy probing requires cgo")
	}
	path, err := filepath.Abs(filepath.Join("..", ".tools", "syzygy", "3-4"))
	if err != nil {
		t.Fatal(err)
	}
	largest, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if largest != 4 {
		t.Fatalf("largest loaded tablebase = %d, want 4", largest)
	}
}

func TestLocalThreeAndFourPieceTables(t *testing.T) {
	loadLocalTables(t)

	const (
		d1 = uint64(1) << 3
		e1 = uint64(1) << 4
		e8 = uint64(1) << 60
	)
	position := Position{
		White:       d1 | e1,
		Black:       e8,
		Kings:       e1 | e8,
		Queens:      d1,
		WhiteToMove: true,
	}
	if got, ok := ProbeWDL(position); !ok || got != Win {
		t.Fatalf("KQvK WDL = %v, ok=%v, want win", got, ok)
	}

	position.Queens = 0
	position.White = e1
	if got, ok := ProbeWDL(position); !ok || got != Draw {
		t.Fatalf("KvK WDL = %v, ok=%v, want draw", got, ok)
	}
}

func TestRootDTZReturnsWinningMove(t *testing.T) {
	loadLocalTables(t)

	const (
		d1 = uint64(1) << 3
		e1 = uint64(1) << 4
		e8 = uint64(1) << 60
	)
	result, ok := ProbeRoot(Position{
		White:       d1 | e1,
		Black:       e8,
		Kings:       e1 | e8,
		Queens:      d1,
		WhiteToMove: true,
	})
	if !ok {
		t.Fatal("KQvK root DTZ probe failed")
	}
	if result.WDL != Win || result.From == result.To {
		t.Fatalf("unexpected root result: %+v", result)
	}
}
