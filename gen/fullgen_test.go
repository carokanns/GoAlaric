package gen

import (
	"GoAlaric/board"
	"testing"
)

func Test_genLegals(t *testing.T) {

	var bd board.Board
	var ml ScMvList
	board.SetFen(board.StartFen, &bd)

	LegalMoves(&ml, &bd)
	if ml.Size() != 20 {
		t.Errorf("Test 1: Borde vara 20 drag men är %v", ml.Size())
		PrintAllMoves(&ml)
	}

	board.SetFen("r3k2r/pppqbppp/2n1bn2/3pp3/3PP3/2N1BN2/PPPQBPPP/R3K2R w KQkq - 10 8", &bd)
	LegalMoves(&ml, &bd)
	if ml.Size() != 40 {
		t.Errorf("Test 2: Borde vara 37 drag men är %v", ml.Size())
		PrintAllMoves(&ml)
	}

	board.SetFen("rnbqkbnr/pp2pppp/8/2ppP3/8/8/PPPP1PPP/RNBQKBNR w KQkq d6 0 3", &bd)
	LegalMoves(&ml, &bd)
	if ml.Size() != 31 {
		t.Errorf("Test 3: Borde vara 31 drag men är %v", ml.Size())
		PrintAllMoves(&ml)
	}

	board.SetFen("rnbqkbnr/pp1pp1pp/8/2p1Pp2/8/8/PPPP1PPP/RNBQKBNR w KQkq f6 0 3", &bd)
	LegalMoves(&ml, &bd)
	if ml.Size() != 31 {
		t.Errorf("Test 4: Borde vara 31 drag men är %v", ml.Size())
		PrintAllMoves(&ml)
	}
	board.SetFen("3K4/PPP1PPPP/8/8/8/8/ppp1pppp/3k4 w - - 1 58", &bd)
	LegalMoves(&ml, &bd)
	if ml.Size() != 31 {
		t.Errorf("Test 5: Borde vara 31 drag men är %v", ml.Size())
		PrintAllMoves(&ml)
	}
}

//////////////////// HACK //////////////////////////
// LegalMoves is generating psudomoves and selecting the legal ones
//func LegalMoves(ml *ScMvList, bd *board.Board) {
//	var pseudos ScMvList
//	GenPseudos(&pseudos, bd)
//	selectLegals(ml, &pseudos, bd)
//}

//func selectLegals(legals, src *ScMvList, bd *board.Board) {

//	legals.Clear()

//	for pos := 0; pos < src.Size(); pos++ {

//		mv := src.Move(pos)

//		if IsLegalMv(mv, bd) {
//			legals.Add(mv)
//		}
//	}
//}
