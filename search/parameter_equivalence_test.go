package search

import (
	"path/filepath"
	"runtime"
	"testing"

	"goalaric/board"
	"goalaric/eval"
	"goalaric/move"
	"goalaric/parms"
)

type fixedDepthResult struct {
	bestmove string
	score    int
	nodes    int64
}

func runFixedDepth(t *testing.T, position *board.Board) fixedDepthResult {
	t.Helper()
	SG.Trans.Clear()
	NewSearch()
	SetMaxDepth(5)
	SetMaxNodes(0)
	SetMaxTime(0)
	SetInfinite(false)
	SetStop(false)
	searchGo(position)
	return fixedDepthResult{
		bestmove: move.ToString(Best.move),
		score:    Best.Score,
		nodes:    current.node,
	}
}

func TestExportedDefaultParameterFileIsSearchEquivalent(t *testing.T) {
	original := parms.Parms
	t.Cleanup(func() {
		parms.Parms = original
		eval.Update()
		SetStop(false)
	})

	var position board.Board
	board.SetFen("r1bq1rk1/ppp1p1bp/5np1/3Ppp2/8/1QP3P1/PP2PPBP/RNBR2K1 w - - 3 10", &position)

	baseline := runFixedDepth(t, &position)
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(source), "..", "optimizer", "registries", "eval-pilot-v1-default.json")
	file, _, err := parms.LoadParameterFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := eval.ApplyParameterFile(file); err != nil {
		t.Fatal(err)
	}
	fromFile := runFixedDepth(t, &position)

	if fromFile != baseline {
		t.Fatalf("default parameter file changed fixed-depth result: baseline=%+v from_file=%+v", baseline, fromFile)
	}
}

func TestAspirationProfilingDoesNotChangeSearch(t *testing.T) {
	originalEnabled := Engine.AspirationProfile
	t.Cleanup(func() { Engine.AspirationProfile = originalEnabled })

	var position board.Board
	board.SetFen("r1bq1rk1/ppp1p1bp/5np1/3Ppp2/8/1QP3P1/PP2PPBP/RNBR2K1 w - - 3 10", &position)
	Engine.AspirationProfile = false
	baseline := runFixedDepth(t, &position)
	Engine.AspirationProfile = true
	profiled := runFixedDepth(t, &position)

	if profiled != baseline {
		t.Fatalf("profiling changed search: baseline=%+v profiled=%+v", baseline, profiled)
	}
	if aspirationProfile.Depths == 0 || aspirationProfile.WindowSearches == 0 {
		t.Fatalf("profiling collected no aspiration work: %+v", aspirationProfile)
	}
}
