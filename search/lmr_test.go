package search

import (
	"math"
	"testing"

	"goalaric/board"
	"goalaric/gen"
	"goalaric/material"
	"goalaric/move"
	"goalaric/parms"
	"goalaric/square"
)

func TestLMRTableGrowsWithDepthAndMoveNumber(t *testing.T) {
	setLMRDivisorForTest(t, 225)
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

func TestDefaultLMRTableMatchesPreviousFormula(t *testing.T) {
	setLMRDivisorForTest(t, 225)
	for depth := 1; depth <= maxDepth; depth++ {
		for moveNumber := 1; moveNumber < lmrMoveLimit; moveNumber++ {
			want := int(0.75 + math.Log(float64(depth))*math.Log(float64(moveNumber))/2.25)
			if want > depth-2 {
				want = depth - 2
			}
			if want < 1 {
				want = 0
			}
			if got := lmrReductions[depth][moveNumber]; got != want {
				t.Fatalf("default LMR[%d][%d] = %d, want %d", depth, moveNumber, got, want)
			}
		}
	}
}

func TestLMRDivisorMonotonicity(t *testing.T) {
	setLMRDivisorForTest(t, 225)
	baseline := lmrReductions
	setLMRDivisorForTest(t, 175)
	aggressive := lmrReductions
	setLMRDivisorForTest(t, 275)
	conservative := lmrReductions

	for depth := 1; depth <= maxDepth; depth++ {
		for moveNumber := 1; moveNumber < lmrMoveLimit; moveNumber++ {
			if aggressive[depth][moveNumber] < baseline[depth][moveNumber] {
				t.Fatalf("175 reduced less than 225 at depth=%d move=%d", depth, moveNumber)
			}
			if conservative[depth][moveNumber] > baseline[depth][moveNumber] {
				t.Fatalf("275 reduced more than 225 at depth=%d move=%d", depth, moveNumber)
			}
		}
	}
}

func TestRefreshRuntimeParametersRebuildsLMRTable(t *testing.T) {
	setLMRDivisorForTest(t, 225)
	baseline := LMRReduction(12, 30)
	setLMRDivisorForTest(t, 175)
	if got := LMRReduction(12, 30); got <= baseline {
		t.Fatalf("175 LMR reduction = %d, want greater than default %d", got, baseline)
	}
	setLMRDivisorForTest(t, 225)
	if got := LMRReduction(12, 30); got != baseline {
		t.Fatalf("restored default LMR reduction = %d, want %d", got, baseline)
	}
}

func setLMRDivisorForTest(t *testing.T, divisor int) {
	t.Helper()
	original := parms.Search
	t.Cleanup(func() {
		parms.Search = original
		RefreshRuntimeParameters()
	})
	parms.Search.LMRDivisorX100 = divisor
	RefreshRuntimeParameters()
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
