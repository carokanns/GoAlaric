package search

import (
	"testing"

	"goalaric/board"
	"goalaric/gen"
	"goalaric/material"
	"goalaric/move"
	"goalaric/square"
)

func TestLMRTableGrowsWithDepthAndMoveNumber(t *testing.T) {
	if got := lmrReductions[2][20]; got != 0 {
		t.Fatalf("depth-2 reduction = %d, want 0", got)
	}
	if shallow, deep := lmrReductions[4][8], lmrReductions[12][8]; deep < shallow {
		t.Fatalf("reduction decreased with depth: shallow=%d deep=%d", shallow, deep)
	}
	if early, late := lmrReductions[10][4], lmrReductions[10][30]; late < early {
		t.Fatalf("reduction decreased with move number: early=%d late=%d", early, late)
	}
}

func TestReductionProtectsImportantMoves(t *testing.T) {
	var local Local
	board.SetFen(board.StartFen, &local.Board)
	quiet := move.Build(square.E2, square.E4, material.Pawn, material.None, material.None)
	tactical := move.Build(square.E2, square.D3, material.Pawn, material.Pawn, material.None)

	if got := reduction(&local, quiet, 12, false, false, 20, false); got == 0 {
		t.Fatal("late quiet move was not reduced")
	}
	for name, got := range map[string]int{
		"in check":    reduction(&local, quiet, 12, false, true, 20, false),
		"interesting": reduction(&local, quiet, 12, false, false, 20, true),
		"tactical":    reduction(&local, tactical, 12, false, false, 20, false),
	} {
		if got != 0 {
			t.Errorf("%s move reduction = %d, want 0", name, got)
		}
	}
	local.killer.Add(quiet, local.Board.Ply())
	if got := reduction(&local, quiet, 12, false, false, 20, false); got != 0 {
		t.Fatalf("killer reduction = %d, want 0", got)
	}
}

func TestReductionIsLowerForPVAndStrongHistory(t *testing.T) {
	var local Local
	board.SetFen(board.StartFen, &local.Board)
	quiet := move.Build(square.E2, square.E4, material.Pawn, material.None, material.None)
	SG.History.Clear()
	base := reduction(&local, quiet, 12, false, false, 20, false)
	pv := reduction(&local, quiet, 12, true, false, 20, false)
	if pv != base-1 {
		t.Fatalf("PV reduction = %d, want %d", pv, base-1)
	}
	var searched gen.ScMvList
	SG.History.Add(quiet, &searched, 20, &local.Board)
	strong := reduction(&local, quiet, 12, false, false, 20, false)
	if strong != base-1 {
		t.Fatalf("strong-history reduction = %d, want %d", strong, base-1)
	}
}

func TestLMRResearchStages(t *testing.T) {
	if needsFullDepthSearch(2, 10, 10) {
		t.Fatal("reduced fail-low requested a full-depth search")
	}
	if !needsFullDepthSearch(2, 11, 10) {
		t.Fatal("reduced fail-high did not request a full-depth null-window search")
	}
	if needsFullDepthSearch(0, 11, 10) {
		t.Fatal("unreduced move requested a duplicate full-depth search")
	}

	if !needsFullWindowSearch(true, 11, 10, 20) {
		t.Fatal("PV improvement inside the window did not request a full-window search")
	}
	for name, got := range map[string]bool{
		"non-PV":      needsFullWindowSearch(false, 11, 10, 20),
		"fail-low":    needsFullWindowSearch(true, 10, 10, 20),
		"beta cutoff": needsFullWindowSearch(true, 20, 10, 20),
	} {
		if got {
			t.Errorf("%s requested a full-window search", name)
		}
	}
}
