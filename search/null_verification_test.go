package search

import (
	"slices"
	"testing"

	"goalaric/board"
	"goalaric/eval"
	"goalaric/hash"
	"goalaric/move"
)

func TestNullMoveVerificationMaterialAndDepthPolicy(t *testing.T) {
	tests := []struct {
		name  string
		fen   string
		depth int
		want  bool
	}{
		{"king and pawns", "8/8/8/8/8/5k2/P7/K7 w - - 0 1", 6, true},
		{"king knight and pawns", "8/8/8/8/8/5k2/P7/K1N5 w - - 0 1", 6, true},
		{"king bishop and pawns", "8/8/8/8/8/5k2/P7/K1B5 w - - 0 1", 6, true},
		{"king rook and pawns", "8/8/8/8/8/5k2/P7/K1R5 w - - 0 1", 6, true},
		{"king queen and pawns", "8/8/8/8/8/5k2/P7/K1Q5 w - - 0 1", 6, true},
		{"opponent material does not matter", "q6r/8/8/8/8/5k2/P7/K1N5 w - - 0 1", 6, true},
		{"king and two pieces", "8/8/8/8/8/5k2/P7/KBN5 w - - 0 1", 6, false},
		{"side to move material is authoritative", "8/8/8/8/8/5K2/P7/kbn5 b - - 0 1", 6, false},
		{"at threshold depth", "8/8/8/8/8/5k2/P7/K1N5 w - - 0 1", nullVerificationReduction, false},
		{"above threshold depth", "8/8/8/8/8/5k2/P7/K1N5 w - - 0 1", nullVerificationReduction + 1, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var position board.Board
			board.SetFen(test.fen, &position)
			requireQuietKings(t, &position)
			if got := needsNullMoveVerification(&position, position.Stm(), test.depth); got != test.want {
				t.Fatalf("needsNullMoveVerification(depth=%d) = %v, want %v", test.depth, got, test.want)
			}
		})
	}
}

func requireQuietKings(t *testing.T, position *board.Board) {
	t.Helper()
	if eval.IsInCheck(position) {
		t.Fatal("test position has side-to-move in check")
	}
	position.MoveNull()
	opponentInCheck := eval.IsInCheck(position)
	position.UndoNull()
	if opponentInCheck {
		t.Fatal("test position illegally leaves the opponent in check")
	}
}

type nullWindowResult struct {
	score      int
	pv         pvStruct
	events     []nullMoveEvent
	beforeKey  hash.Key
	afterKey   hash.Key
	beforePly  int
	afterPly   int
	beforeSide int
	afterSide  int
}

func runNullWindow(t *testing.T, fen string, depth, beta int) nullWindowResult {
	return runNullWindowWithVerifier(t, fen, depth, beta, nil)
}

func runNullWindowWithVerifier(t *testing.T, fen string, depth, beta int, verifier nullMoveVerifier) nullWindowResult {
	t.Helper()
	var local Local
	slInitEarly(&local, 0)
	slInitLate(&local)
	board.SetFen(fen, &local.Board)
	SG.Trans.Clear()
	SG.History.Clear()
	SetStop(false)
	t.Cleanup(func() { SetStop(false) })

	result := nullWindowResult{
		beforeKey:  local.Board.Key(),
		beforePly:  local.Board.Ply(),
		beforeSide: local.Board.Stm(),
	}
	local.nullMoveObserver = func(event nullMoveEvent) {
		result.events = append(result.events, event)
	}
	local.nullMoveVerifier = verifier
	result.score = search(&local, depth, beta-1, beta, &result.pv)
	result.afterKey = local.Board.Key()
	result.afterPly = local.Board.Ply()
	result.afterSide = local.Board.Stm()
	return result
}

func rootEventKinds(events []nullMoveEvent) []nullMoveEventKind {
	var kinds []nullMoveEventKind
	for _, event := range events {
		if event.ply == 0 {
			kinds = append(kinds, event.kind)
		}
	}
	return kinds
}

func requireBoardRestored(t *testing.T, result nullWindowResult) {
	t.Helper()
	if result.afterKey != result.beforeKey || result.afterPly != result.beforePly || result.afterSide != result.beforeSide {
		t.Fatalf("search did not restore board: key %v -> %v, ply %d -> %d, side %d -> %d",
			result.beforeKey, result.afterKey, result.beforePly, result.afterPly, result.beforeSide, result.afterSide)
	}
}

func TestNullMoveVerificationAcceptsOnlyAfterRealMoveSearch(t *testing.T) {
	const (
		fen   = "8/8/8/8/p1p5/p1k5/8/K1N5 w - - 0 1"
		depth = 6
		beta  = -500
	)
	var position board.Board
	board.SetFen(fen, &position)
	requireQuietKings(t, &position)
	result := runNullWindow(t, fen, depth, beta)
	requireBoardRestored(t, result)

	want := []nullMoveEventKind{
		nullMoveAttempt,
		nullMovePreliminaryCutoff,
		nullMoveVerificationStarted,
		nullMoveVerificationAccepted,
	}
	if got := rootEventKinds(result.events); !slices.Equal(got, want) {
		t.Fatalf("root null events = %v, want %v; events=%+v", got, want, result.events)
	}
	if result.score < beta {
		t.Fatalf("verified score = %d, want cutoff at beta %d", result.score, beta)
	}
	if result.pv.getSize() == 0 || result.pv.getMove(0) == move.None {
		t.Fatal("verification cutoff has no real-move PV")
	}

	var storedMove, storedScore, storedType int
	if !SG.Trans.Retrieve(result.beforeKey, depth, 0, &storedMove, &storedScore, &storedType) {
		t.Fatal("verified cutoff was not stored at the original search depth")
	}
	if storedType != scoreTypeLower || storedScore < beta || storedMove == move.None {
		t.Fatalf("stored verified cutoff = move %s score %d type %d", move.ToString(storedMove), storedScore, storedType)
	}
}

func TestNullMoveVerificationRejectsPreliminaryCutoffAndContinues(t *testing.T) {
	const (
		fen   = "8/8/8/8/2p5/2k5/8/K1N5 w - - 0 1"
		depth = 6
		beta  = -97
	)
	var position board.Board
	board.SetFen(fen, &position)
	requireQuietKings(t, &position)
	verificationCalls := 0
	verifier := func(sl *Local, gotDepth, alpha, gotBeta, transMove int, pv *pvStruct) int {
		verificationCalls++
		if gotDepth != depth-nullVerificationReduction || gotBeta != beta {
			t.Fatalf("verification called with depth=%d beta=%d", gotDepth, gotBeta)
		}
		return beta - 1
	}
	result := runNullWindowWithVerifier(t, fen, depth, beta, verifier)
	requireBoardRestored(t, result)
	if verificationCalls != 1 {
		t.Fatalf("verification calls = %d, want 1", verificationCalls)
	}

	want := []nullMoveEventKind{
		nullMoveAttempt,
		nullMovePreliminaryCutoff,
		nullMoveVerificationStarted,
		nullMoveVerificationRejected,
	}
	if got := rootEventKinds(result.events); !slices.Equal(got, want) {
		t.Fatalf("root null events = %v, want %v; events=%+v", got, want, result.events)
	}
	if result.pv.getSize() == 0 || result.pv.getMove(0) == move.None {
		t.Fatal("search did not continue with real moves after rejected verification")
	}

	var storedMove, storedScore, storedType int
	if !SG.Trans.Retrieve(result.beforeKey, depth, 0, &storedMove, &storedScore, &storedType) {
		t.Fatal("continued real-move search did not store its result")
	}
	if storedMove == move.None {
		t.Fatal("preliminary null cutoff leaked into the transposition table")
	}
}

func TestNullMoveVerificationScoreBoundary(t *testing.T) {
	const (
		fen   = "8/8/8/8/2p5/2k5/8/K1N5 w - - 0 1"
		depth = 6
		beta  = -97
	)
	tests := []struct {
		name      string
		score     int
		wantEvent nullMoveEventKind
	}{
		{"below beta rejects", beta - 1, nullMoveVerificationRejected},
		{"equal beta accepts", beta, nullMoveVerificationAccepted},
		{"above beta accepts", beta + 1, nullMoveVerificationAccepted},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verifier := func(sl *Local, depth, alpha, beta, transMove int, pv *pvStruct) int {
				return test.score
			}
			result := runNullWindowWithVerifier(t, fen, depth, beta, verifier)
			if !slices.Contains(rootEventKinds(result.events), test.wantEvent) {
				t.Fatalf("root events = %v, want %v", rootEventKinds(result.events), test.wantEvent)
			}
		})
	}
}

func TestNullMovePreliminaryFailLowSkipsVerification(t *testing.T) {
	const fen = "8/8/8/8/2p5/2k5/8/K1N5 w - - 0 1"
	verifier := func(sl *Local, depth, alpha, beta, transMove int, pv *pvStruct) int {
		t.Fatal("verification called after preliminary fail-low")
		return 0
	}
	result := runNullWindowWithVerifier(t, fen, 6, 3, verifier)
	if got, want := rootEventKinds(result.events), []nullMoveEventKind{nullMoveAttempt}; !slices.Equal(got, want) {
		t.Fatalf("root null events = %v, want %v", got, want)
	}
}

func TestNullMoveStopRestoresBoardAndSkipsVerification(t *testing.T) {
	const (
		fen   = "8/8/8/8/p1p5/p1k5/8/K1N5 w - - 0 1"
		depth = 6
		beta  = -500
	)
	var local Local
	slInitEarly(&local, 0)
	slInitLate(&local)
	board.SetFen(fen, &local.Board)
	SG.Trans.Clear()
	SG.History.Clear()
	SetStop(false)
	t.Cleanup(func() { SetStop(false) })
	beforeKey := local.Board.Key()
	beforeSide := local.Board.Stm()
	local.nullMoveObserver = func(event nullMoveEvent) {
		if event.kind == nullMoveAttempt && event.ply == 0 {
			SetStop(true)
		}
	}
	local.nullMoveVerifier = func(sl *Local, depth, alpha, beta, transMove int, pv *pvStruct) int {
		t.Fatal("verification started after search stop")
		return 0
	}
	var pv pvStruct
	search(&local, depth, beta-1, beta, &pv)
	if local.Board.Key() != beforeKey || local.Board.Ply() != 0 || local.Board.Stm() != beforeSide {
		t.Fatal("stopped null search did not restore its board")
	}
}

func TestNullMoveIsNotAttemptedInCheck(t *testing.T) {
	const fen = "4r1k1/8/8/8/8/8/8/4K1N1 w - - 0 1"
	result := runNullWindow(t, fen, 6, -500)
	if got := rootEventKinds(result.events); len(got) != 0 {
		t.Fatalf("root null events = %v, want none while in check", got)
	}
}

func TestNullMoveIsNotAttemptedAtPVNode(t *testing.T) {
	const fen = "8/8/8/8/2p5/2k5/8/K1N5 w - - 0 1"
	var local Local
	slInitEarly(&local, 0)
	slInitLate(&local)
	board.SetFen(fen, &local.Board)
	SG.Trans.Clear()
	SG.History.Clear()
	var events []nullMoveEvent
	local.nullMoveObserver = func(event nullMoveEvent) { events = append(events, event) }
	var pv pvStruct
	search(&local, 6, -500, -97, &pv)
	if got := rootEventKinds(events); len(got) != 0 {
		t.Fatalf("root null events = %v, want none at a PV node", got)
	}
}

func TestNullMoveVerificationDoesNotReenterNullAtVerificationRoot(t *testing.T) {
	const fen = "8/8/8/8/p1p5/p1k5/8/K1N5 w - - 0 1"
	result := runNullWindow(t, fen, 6, -500)

	insideVerification := false
	for _, event := range result.events {
		if event.ply != 0 {
			continue
		}
		switch event.kind {
		case nullMoveVerificationStarted:
			insideVerification = true
		case nullMoveAttempt:
			if insideVerification {
				t.Fatal("verification root attempted another null move")
			}
		case nullMoveVerificationAccepted, nullMoveVerificationRejected:
			insideVerification = false
		}
	}
	if insideVerification {
		t.Fatal("verification did not produce a terminal decision")
	}
}

func TestNullMoveDirectCutoffOutsideVerificationScope(t *testing.T) {
	tests := []struct {
		name  string
		fen   string
		depth int
		beta  int
	}{
		{"two non-pawn pieces", "8/8/8/8/8/6k1/P7/BNK5 w - - 0 1", 6, -500},
		{"verification depth not reached", "8/8/8/8/8/5k2/P7/K1N5 w - - 0 1", nullVerificationReduction, 100},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runNullWindow(t, test.fen, test.depth, test.beta)
			requireBoardRestored(t, result)
			want := []nullMoveEventKind{nullMoveAttempt, nullMovePreliminaryCutoff, nullMoveDirectCutoff}
			if got := rootEventKinds(result.events); !slices.Equal(got, want) {
				t.Fatalf("root null events = %v, want %v", got, want)
			}
		})
	}
}

func TestNullMoveIsNotAttemptedWithoutNonPawnMaterial(t *testing.T) {
	const fen = "8/8/8/8/8/6k1/P7/2K5 w - - 0 1"
	result := runNullWindow(t, fen, 6, -500)
	if got := rootEventKinds(result.events); len(got) != 0 {
		t.Fatalf("root null events = %v, want none for king-and-pawn material", got)
	}
}
