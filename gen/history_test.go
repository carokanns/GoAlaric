package gen

import (
	"testing"

	"goalaric/board"
	"goalaric/material"
	"goalaric/move"
	"goalaric/square"
)

func TestHistoryBonusUsesCappedDepthSquare(t *testing.T) {
	tests := []struct {
		depth int
		want  int
	}{
		{depth: 0, want: 0},
		{depth: 1, want: 1},
		{depth: 4, want: 16},
		{depth: 19, want: 361},
		{depth: 20, want: histBonusMax},
		{depth: 100, want: histBonusMax},
	}
	for _, test := range tests {
		if got := historyBonus(test.depth); got != test.want {
			t.Errorf("historyBonus(%d) = %d, want %d", test.depth, got, test.want)
		}
	}
}

func TestHistoryAddScalesAndSaturates(t *testing.T) {
	var bd board.Board
	board.SetFen(board.StartFen, &bd)
	good := move.Build(square.E2, square.E4, material.Pawn, material.None, material.None)
	bad := move.Build(square.D2, square.D4, material.Pawn, material.None, material.None)

	var history HistoryTab
	history.Clear()
	var searched ScMvList
	searched.Add(bad)
	history.Add(good, &searched, 4, &bd)
	if got, want := history.Score(good, &bd), histHalf+16; got != want {
		t.Fatalf("good score = %d, want %d", got, want)
	}
	if got, want := history.Score(bad, &bd), histHalf-16; got != want {
		t.Fatalf("bad score = %d, want %d", got, want)
	}

	for range 10 {
		history.good(good, 20, &bd)
		history.bad(bad, 20, &bd)
	}
	if got := history.Score(good, &bd); got != histMax {
		t.Fatalf("saturated good score = %d, want %d", got, histMax)
	}
	if got := history.Score(bad, &bd); got != 0 {
		t.Fatalf("saturated bad score = %d, want 0", got)
	}
}

func TestHistoryIgnoresTacticalMoves(t *testing.T) {
	var bd board.Board
	board.SetFen(board.StartFen, &bd)
	tactical := move.Build(square.E2, square.D3, material.Pawn, material.Pawn, material.None)

	var history HistoryTab
	history.Clear()
	history.good(tactical, 20, &bd)
	history.bad(tactical, 20, &bd)
	if got := history.Score(tactical, &bd); got != histHalf {
		t.Fatalf("tactical history score = %d, want %d", got, histHalf)
	}
}

func TestHistoryStrongThreshold(t *testing.T) {
	var bd board.Board
	board.SetFen(board.StartFen, &bd)
	quiet := move.Build(square.E2, square.E4, material.Pawn, material.None, material.None)
	tactical := move.Build(square.E2, square.D3, material.Pawn, material.Pawn, material.None)

	var history HistoryTab
	history.Clear()
	if history.IsStrong(quiet, &bd) {
		t.Fatal("neutral quiet move reported as strong")
	}
	history.good(quiet, 20, &bd)
	if !history.IsStrong(quiet, &bd) {
		t.Fatal("positive quiet history was not reported as strong")
	}
	if history.IsStrong(tactical, &bd) {
		t.Fatal("tactical move reported as strong quiet history")
	}
}
