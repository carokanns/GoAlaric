package search

import (
	"reflect"
	"testing"

	"goalaric/board"
	"goalaric/gen"
	"goalaric/move"
	"goalaric/syzygy"
)

func TestSiblingBottomKIsOrderIndependentAndStratified(t *testing.T) {
	config := SiblingTraceConfig{Depths: []int{6, 8}, PerDepth: 3, SampleSeed: 7}
	first, err := NewSiblingTraceCollector(config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewSiblingTraceCollector(config)
	if err != nil {
		t.Fatal(err)
	}
	items := make([]SiblingTraceSnapshot, 0, 20)
	for ix := 0; ix < 10; ix++ {
		for _, depth := range []int{6, 8} {
			items = append(items, SiblingTraceSnapshot{PositionKey: uint64(ix + 1), Depth: depth, SourceIndex: ix, FEN: "test"})
		}
	}
	for _, item := range items {
		first.consider(item)
	}
	for ix := len(items) - 1; ix >= 0; ix-- {
		second.consider(items[ix])
	}
	if !reflect.DeepEqual(first.Snapshots(), second.Snapshots()) {
		t.Fatalf("bottom-k depends on order")
	}
	for _, depth := range []int{6, 8} {
		count := 0
		for _, item := range first.Snapshots() {
			if item.Depth == depth {
				count++
			}
		}
		if count != 3 {
			t.Fatalf("depth %d count=%d want=3", depth, count)
		}
	}
}

func TestIterativeTraceIsDeterministicAndHasWarmTT(t *testing.T) {
	withoutSyzygy(t)
	var position board.Board
	board.SetFen(board.StartFen, &position)
	run := func() (FixedDepthResult, []SiblingTraceSnapshot) {
		collector, err := NewSiblingTraceCollector(SiblingTraceConfig{Depths: []int{4}, PerDepth: 20, SampleSeed: 11})
		if err != nil {
			t.Fatal(err)
		}
		result, err := RunIterativeSiblingTrace(&position, 6, 0, collector)
		if err != nil {
			t.Fatal(err)
		}
		return result, collector.Snapshots()
	}
	firstResult, first := run()
	secondResult, second := run()
	if firstResult != secondResult || !reflect.DeepEqual(first, second) {
		t.Fatalf("iterative trace is not deterministic")
	}
	if len(first) == 0 {
		t.Fatal("trace is empty")
	}
	found := false
	for _, snapshot := range first {
		if snapshot.TTFound && snapshot.TTDepth > 0 && snapshot.TTMove != "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("iterative warm-up produced no trace node with a stored TT move/depth")
	}
}

func TestTracingDoesNotAffectLaterFixedDepthSearch(t *testing.T) {
	withoutSyzygy(t)
	var position board.Board
	board.SetFen(board.StartFen, &position)
	before, err := FixedDepthSearch(&position, 4)
	if err != nil {
		t.Fatal(err)
	}
	collector, _ := NewSiblingTraceCollector(SiblingTraceConfig{Depths: []int{3}, PerDepth: 5, SampleSeed: 1})
	if _, err := RunIterativeSiblingTrace(&position, 5, 0, collector); err != nil {
		t.Fatal(err)
	}
	after, err := FixedDepthSearch(&position, 4)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("diagnostic state leaked: before=%+v after=%+v", before, after)
	}
}

func TestIterativeTracePreservesInputPosition(t *testing.T) {
	withoutSyzygy(t)
	var position board.Board
	board.SetFen(board.StartFen, &position)
	key, fen := position.Key(), position.CreateFen()
	collector, _ := NewSiblingTraceCollector(SiblingTraceConfig{Depths: []int{3}, PerDepth: 4})
	if _, err := RunIterativeSiblingTrace(&position, 5, 2, collector); err != nil {
		t.Fatal(err)
	}
	if position.Key() != key || position.CreateFen() != fen {
		t.Fatalf("input changed: %s", position.CreateFen())
	}
}

func TestSiblingTraceRejectsInvalidConfiguration(t *testing.T) {
	for _, config := range []SiblingTraceConfig{{Depths: nil, PerDepth: 1}, {Depths: []int{6}, PerDepth: 0}, {Depths: []int{1}, PerDepth: 1}, {Depths: []int{6, 6}, PerDepth: 1}} {
		if _, err := NewSiblingTraceCollector(config); err == nil {
			t.Fatalf("invalid config accepted: %+v", config)
		}
	}
}

func TestTransDiagnosticProbeIsReadOnly(t *testing.T) {
	var position board.Board
	board.SetFen(board.StartFen, &position)
	SG.Trans.Clear()
	SG.Trans.Store(position.Key(), 5, 0, 1234, 42, scoreTypeLower)
	used, generation := SG.Trans.cntUsed, SG.Trans.generation
	probe := SG.Trans.probeDiagnostic(position.Key(), 0)
	if !probe.Found || probe.Depth != 5 || probe.Move != 1234 || probe.Score != 42 || probe.Bound != scoreTypeLower {
		t.Fatalf("unexpected probe: %+v", probe)
	}
	if SG.Trans.cntUsed != used || SG.Trans.generation != generation {
		t.Fatal("diagnostic probe changed transposition metadata")
	}
}

func TestDiagnosticExtensionClassifierMatchesSearch(t *testing.T) {
	fens := []string{
		board.StartFen,
		"4k3/8/8/8/8/8/4Q3/4K3 w - -",
		"4k3/P7/8/8/8/8/8/4K3 w - -",
		"4k3/8/8/3q4/4P3/8/8/4K3 w - -",
	}
	for _, fen := range fens {
		var position board.Board
		board.SetFen(fen, &position)
		var legal gen.ScMvList
		gen.LegalMoves(&legal, &position)
		for depth := 1; depth <= 6; depth++ {
			for _, pvNode := range []bool{false, true} {
				for ix := 0; ix < legal.Size(); ix++ {
					mv := legal.Move(ix)
					local := Local{Board: position}
					got := ClassifyExistingExtension(&position, mv, depth, pvNode).Extended
					want := extension(&local, mv, depth, pvNode) == 1
					if got != want {
						t.Fatalf("fen=%q move=%d depth=%d pv=%t got=%t want=%t", fen, mv, depth, pvNode, got, want)
					}
				}
			}
		}
	}
}

func TestDiagnosticExtensionUsesRecordedRecaptureSquare(t *testing.T) {
	var position board.Board
	board.SetFen("4k3/8/8/3q4/4P3/8/8/4K3 w - -", &position)
	mv := board.FromString("e4d5", &position)
	withoutHistory := ClassifyExistingExtension(&position, mv, 4, false)
	withRecordedSquare := ClassifyExistingExtensionAtNode(&position, mv, 4, false, move.To(mv))
	if withoutHistory.Recapture || withoutHistory.Extended {
		t.Fatalf("FEN unexpectedly retained recapture state: %+v", withoutHistory)
	}
	if !withRecordedSquare.Recapture || !withRecordedSquare.Extended {
		t.Fatalf("recorded recapture was not classified: %+v", withRecordedSquare)
	}
}

func TestSingularTTMoveRequiresSafeRuntimePreconditions(t *testing.T) {
	var position board.Board
	board.SetFen(board.StartFen, &position)
	ttMove := board.FromString("e2e4", &position)
	SG.Trans.Clear()
	SG.Trans.Store(position.Key(), 5, position.Ply(), ttMove, 25, scoreTypeLower)
	probe := SG.Trans.probeDiagnostic(position.Key(), position.Ply())
	if got := singularTTMove(&position, 6, false, probe); got != ttMove {
		t.Fatalf("valid singular TT move rejected: got=%d want=%d", got, ttMove)
	}
	for _, test := range []struct {
		name  string
		depth int
		check bool
		probe transDiagnostic
		want  int
	}{
		{name: "too shallow", depth: 9, probe: probe},
		{name: "in check", depth: 6, check: true, probe: probe},
		{name: "upper bound", depth: 6, probe: transDiagnostic{Found: true, Move: ttMove, Score: 25, Depth: 5, Bound: scoreTypeUpper}},
		{name: "missing entry", depth: 6, probe: transDiagnostic{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := singularTTMove(&position, test.depth, test.check, test.probe); got != test.want {
				t.Fatalf("got=%d want=%d", got, test.want)
			}
		})
	}
	var sparse board.Board
	board.SetFen("4k3/8/8/8/8/8/4R3/4K3 w - -", &sparse)
	mv := board.FromString("e2e3", &sparse)
	if got := singularTTMove(&sparse, 6, false, transDiagnostic{Found: true, Move: mv, Score: 25, Depth: 5, Bound: scoreTypeLower}); got != 0 {
		t.Fatalf("sparse material accepted: %d", got)
	}
}

func TestSingularExtensionIsAppliedToTTMove(t *testing.T) {
	withoutSyzygy(t)
	run := func(enabled bool) FixedDepthResult {
		singularExtensionEnabled = enabled
		defer func() { singularExtensionEnabled = true }()
		var position board.Board
		board.SetFen(board.StartFen, &position)
		ttMove := board.FromString("e2e4", &position)
		clear()
		var local Local
		slInitEarly(&local, 0)
		slInitLate(&local)
		slSetRoot(&local, &position)
		SetStop(false)
		ClearSearchCaches()
		initSg()
		SG.Trans.Store(position.Key(), 5, position.Ply(), ttMove, 25, scoreTypeLower)
		var pv pvStruct
		score := search(&local, 6, minScore, maxScore, &pv)
		best := move.None
		if pv.getSize() != 0 {
			best = pv.getMove(0)
		}
		return FixedDepthResult{Score: score, BestMove: move.ToString(best), Nodes: local.node}
	}
	without := run(false)
	with := run(true)
	if with.Nodes <= without.Nodes {
		t.Fatalf("singular extension did not add the expected ply: enabled=%+v disabled=%+v", with, without)
	}
}

func TestQuiescenceStopsBeforeMaxPlyGeneratorAccess(t *testing.T) {
	var position board.Board
	board.SetFen(board.StartFen, &position)
	position.SetRoot()
	for range maxPly {
		position.MoveNull()
	}

	var local Local
	slInitEarly(&local, 0)
	slInitLate(&local)
	local.Board = position
	if local.Board.Ply() != maxPly {
		t.Fatalf("test position ply = %d, want %d", local.Board.Ply(), maxPly)
	}

	// This used to panic at genList[local.ID][maxPly].
	_ = Qs(&local, maxScore, 100)
}

func withoutSyzygy(t *testing.T) {
	t.Helper()
	oldPath := Engine.SyzygyPath
	if err := syzygy.SetPath(""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { Engine.SyzygyPath = oldPath; _ = syzygy.SetPath("") })
}
