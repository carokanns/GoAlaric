package search

import (
	"reflect"
	"testing"

	"goalaric/board"
	"goalaric/eval"
	"goalaric/gen"
	"goalaric/move"
	"goalaric/syzygy"
)

func TestSingleReplyExtensionAtRoot(t *testing.T) {
	var local Local
	slInitEarly(&local, 0)
	slInitLate(&local)
	board.SetFen("7k/5K2/8/8/8/8/PP6/6R1 b - - 0 1", &local.Board)
	local.Board.SetRoot()
	var legal gen.ScMvList
	gen.LegalMoves(&legal, &local.Board)
	if legal.Size() != 1 {
		t.Fatalf("root legal moves = %d, want one", legal.Size())
	}

	var events []singleReplyEvent
	local.singleReplyObserver = func(event singleReplyEvent) { events = append(events, event) }
	SG.Trans.Clear()
	SG.History.Clear()
	SetStop(false)
	searchRoot(&local, &legal, 2, minScore, maxScore)

	if len(events) != 1 {
		t.Fatalf("root single-reply events = %d, want one", len(events))
	}
	if got := move.ToString(events[0].move); got != "h8h7" {
		t.Fatalf("root extended move = %s, want h8h7", got)
	}
	if events[0].depth != 2 || events[0].ply != 0 {
		t.Fatalf("root event depth/ply = %d/%d, want 2/0", events[0].depth, events[0].ply)
	}
}

func TestSingleReplyExtensionAtCompleteSearchNode(t *testing.T) {
	tests := []struct {
		name     string
		fen      string
		wantMove string
		wantRoot bool
	}{
		{
			name:     "only legal check evasion",
			fen:      "7k/8/5K2/8/8/8/PP6/7R b - - 0 1",
			wantMove: "h8g8",
			wantRoot: true,
		},
		{
			name:     "only legal quiet reply",
			fen:      "7k/5K2/8/8/8/8/PP6/6R1 b - - 0 1",
			wantMove: "h8h7",
			wantRoot: true,
		},
		{
			name:     "two legal check evasions",
			fen:      "7k/8/4K3/8/8/8/PP6/7R b - - 0 1",
			wantRoot: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var local Local
			slInitEarly(&local, 0)
			slInitLate(&local)
			board.SetFen(test.fen, &local.Board)
			local.Board.SetRoot()

			var legal gen.ScMvList
			gen.LegalMoves(&legal, &local.Board)
			if test.wantRoot && legal.Size() != 1 {
				t.Fatalf("legal moves = %d, want exactly one", legal.Size())
			}
			if !test.wantRoot && legal.Size() < 2 {
				t.Fatalf("legal moves = %d, want at least two", legal.Size())
			}

			beforeKey := local.Board.Key()
			var rootEvents []singleReplyEvent
			local.singleReplyObserver = func(event singleReplyEvent) {
				if event.ply == 0 {
					rootEvents = append(rootEvents, event)
				}
			}
			SG.Trans.Clear()
			SG.History.Clear()
			SetStop(false)
			oldPath := Engine.SyzygyPath
			if err := syzygy.SetPath(""); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = syzygy.SetPath(oldPath) })

			var pv pvStruct
			search(&local, 3, minScore, maxScore, &pv)

			if test.wantRoot && len(rootEvents) == 0 {
				t.Fatal("root single-reply extension was not observed")
			}
			if !test.wantRoot && len(rootEvents) != 0 {
				t.Fatalf("root single-reply events = %d, want none", len(rootEvents))
			}
			mainDepthObserved := false
			for _, event := range rootEvents {
				if test.wantMove != "" && move.ToString(event.move) != test.wantMove {
					t.Fatalf("extended move = %s, want %s", move.ToString(event.move), test.wantMove)
				}
				if event.depth == 3 {
					mainDepthObserved = true
				}
			}
			if test.wantRoot && !mainDepthObserved {
				t.Fatal("single reply was not extended in the requested depth-3 search")
			}
			if local.Board.Key() != beforeKey || local.Board.Ply() != 0 {
				t.Fatal("search did not restore the root position")
			}
		})
	}
}

func TestSingleReplyExtensionDoesNotStackOrEnterQuiescence(t *testing.T) {
	var local Local
	board.SetFen("7k/8/5K2/8/8/8/PP6/7R b - - 0 1", &local.Board)
	var legal gen.ScMvList
	gen.LegalMoves(&legal, &local.Board)
	if legal.Size() != 1 {
		t.Fatalf("legal moves = %d, want one", legal.Size())
	}
	mv := legal.Move(0)
	observed := 0
	local.singleReplyObserver = func(singleReplyEvent) { observed++ }

	if got := extension(&local, mv, 3, true, true); got != 1 {
		t.Fatalf("combined single-reply/PV/check extension = %d, want 1", got)
	}
	if got := extension(&local, mv, 0, true, true); got > 1 {
		t.Fatalf("quiescence extension = %d, want at most the existing non-single-reply extension", got)
	}
	if observed != 1 {
		t.Fatalf("single-reply observer calls = %d, want only the positive-depth call", observed)
	}
}

func TestSingleReplyCannotBeInferredFromHardPrunedGenerator(t *testing.T) {
	firstMove := 1
	if singleReplyLookAheadAllowed(4, true, firstMove) {
		t.Fatal("hard-pruned generator was treated as a complete legal move list")
	}
	if !singleReplyLookAheadAllowed(4, false, firstMove) {
		t.Fatal("complete positive-depth generator did not allow single-reply detection")
	}
	if singleReplyLookAheadAllowed(0, false, firstMove) {
		t.Fatal("quiescence node allowed single-reply detection")
	}
	if singleReplyLookAheadAllowed(4, false, move.None) {
		t.Fatal("empty move list allowed single-reply detection")
	}
}

func TestSingleReplyBypassesLateMovePruning(t *testing.T) {
	if lateMovePrune(false, 3, 0, 100, true) {
		t.Fatal("single reply was late-move pruned")
	}
}

func TestSingleReplyLookAheadPreservesMoveClassification(t *testing.T) {
	var position board.Board
	board.SetFen(board.StartFen, &position)
	var attacks eval.Attacks
	eval.InitAttacks(&attacks, position.Stm(), &position)
	var killer gen.Killer
	var history gen.HistoryTab
	killer.Clear()
	history.Clear()

	newList := func() *gen.List {
		var list gen.List
		list.Init(6, &position, &attacks, move.None, &killer, &history, false)
		return &list
	}
	readAll := func(list *gen.List) []generatedMove {
		var result []generatedMove
		for item := nextGeneratedMove(list); item.move != move.None; item = nextGeneratedMove(list) {
			result = append(result, item)
		}
		return result
	}
	readWithLookAhead := func(list *gen.List) []generatedMove {
		current := nextGeneratedMove(list)
		next := nextGeneratedMove(list)
		var result []generatedMove
		for ; current.move != move.None; current = advanceGeneratedMove(list, true, &next) {
			result = append(result, current)
		}
		return result
	}

	direct := readAll(newList())
	lookedAhead := readWithLookAhead(newList())
	if !reflect.DeepEqual(lookedAhead, direct) {
		t.Fatal("lookahead changed move order or Candidate classification")
	}
}
