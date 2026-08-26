package main

import (
	"os"
	"path/filepath"
	"testing"

	"goalaric/board"
)

func TestParsePGNAndSelectPositionAfterBook(t *testing.T) {
	pgn := `[Event "sample"]
[Result "*"]

1. e2e4 {book} e7e5 {book} 2. g1f3 {depth=8} b8c6 3. f1b5 a7a6 *
`
	games, err := parsePGN(pgn)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 || len(games[0].moves) != 6 {
		t.Fatalf("games=%+v", games)
	}
	position, ply, bookPlies, ok, err := selectPGNPosition(games[0], 0, 20, 17)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("no selected position")
	}
	if bookPlies != 2 || ply <= bookPlies {
		t.Fatalf("ply=%d book=%d", ply, bookPlies)
	}
	if position.CreateFen() == board.StartFen {
		t.Fatal("selected starting position")
	}
}

func TestPGNSelectionIsDeterministic(t *testing.T) {
	pgn := `[Event "sample"]

1. e2e4 {book} e7e5 {book} 2. g1f3 b8c6 3. f1b5 a7a6 4. b5a4 g8f6 *
`
	games, _ := parsePGN(pgn)
	first, firstPly, _, _, err := selectPGNPosition(games[0], 3, 20, 99)
	if err != nil {
		t.Fatal(err)
	}
	second, secondPly, _, _, err := selectPGNPosition(games[0], 3, 20, 99)
	if err != nil {
		t.Fatal(err)
	}
	if firstPly != secondPly || first.CreateFen() != second.CreateFen() {
		t.Fatal("selection changed")
	}
}

func TestPGNSelectionExcludesSimpleTablebaseEnding(t *testing.T) {
	pgn := `[Event "tablebase"]
[FEN "4k3/8/8/8/8/8/4K3/R7 w - -"]

1. a1a2 *
`
	games, err := parsePGN(pgn)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, ok, err := selectPGNPosition(games[0], 0, 20, 1)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("position with at most four pieces was selected")
	}
}

func TestWilson95(t *testing.T) {
	got := wilson95(3, 100)
	if !(got.Low > 0 && got.Low < 3 && got.High > 3 && got.High < 9) {
		t.Fatalf("unexpected interval %+v", got)
	}
}

func TestDecisionGate(t *testing.T) {
	passed := decideGate(depthSummary{Qualified: 100, StableSingular: 4, StablePercent: 4, TTTruePositive: 3, TTFalsePositive: 6, TTRecallPercent: 75})
	if !passed.Passed {
		t.Fatalf("gate should pass: %+v", passed)
	}
	failed := decideGate(depthSummary{Qualified: 100, StableSingular: 2, StablePercent: 2, TTTruePositive: 2, TTRecallPercent: 100})
	if failed.Passed {
		t.Fatalf("gate should fail: %+v", failed)
	}
}

func TestWriteJSONProducesValidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	if err := writeJSON(path, map[string]int{"answer": 42}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{\n  \"answer\": 42\n}\n" {
		t.Fatalf("unexpected JSON %q", data)
	}
}
