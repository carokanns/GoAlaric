package main

import (
	"reflect"
	"testing"

	"goalaric/eval"
)

func TestCoordinateMovesIgnoresCommentsAndVariations(t *testing.T) {
	text := `1. e2e4 {book e7e5} e7e5 2. g1f3 (2. f2f4) b8c6 1-0`
	want := []string{"e2e4", "e7e5", "g1f3", "b8c6"}
	if got := coordinateMoves(text); !reflect.DeepEqual(got, want) {
		t.Fatalf("moves = %v, want %v", got, want)
	}
}

func TestPairedOpeningUsesSameDatasetPartition(t *testing.T) {
	moves := []string{
		"e2e4", "e7e5", "g1f3", "b8c6",
		"f1b5", "a7a6", "b5a4", "g8f6",
		"e1g1", "f8e7", "f1e1", "b7b5",
		"a4b3", "d7d6", "c2c3", "e8g8",
	}
	first := pgnGame{Tags: map[string]string{"White": "Candidate", "Black": "Baseline", "Round": "1"}, Moves: moves, Result: "1-0"}
	second := pgnGame{Tags: map[string]string{"White": "Baseline", "Black": "Candidate", "Round": "2"}, Moves: moves, Result: "0-1"}

	firstRecords, firstValidation, _, err := recordsForGame("games.pgn", first, 42, 20, 16, 8, 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	secondRecords, secondValidation, _, err := recordsForGame("games.pgn", second, 42, 20, 16, 8, 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	if firstValidation != secondValidation {
		t.Fatalf("paired games split across partitions: %v and %v", firstValidation, secondValidation)
	}
	if firstRecords[0].GroupID != secondRecords[0].GroupID {
		t.Fatalf("paired games have different groups: %q and %q", firstRecords[0].GroupID, secondRecords[0].GroupID)
	}
	if firstRecords[0].GameID == secondRecords[0].GameID {
		t.Fatal("paired games unexpectedly have the same game ID")
	}
}

func TestPawnStructureWeightDefaultsMatchCandidate023(t *testing.T) {
	want := eval.PawnStructureWeights{
		IsolatedMG: 15,
		IsolatedEG: 22,
		WeakMG:     0,
		WeakEG:     3,
		DoubledMG:  0,
		DoubledEG:  0,
	}
	if got := eval.CurrentPawnStructureWeights(); got != want {
		t.Fatalf("weights = %+v, want %+v", got, want)
	}
}

func TestResultValue(t *testing.T) {
	for input, want := range map[string]float64{"1-0": 1, "1/2-1/2": 0.5, "0-1": 0} {
		got, ok := resultValue(input)
		if !ok || got != want {
			t.Fatalf("resultValue(%q) = %v, %v; want %v, true", input, got, ok, want)
		}
	}
	if _, ok := resultValue("*"); ok {
		t.Fatal("unfinished result was accepted")
	}
}
