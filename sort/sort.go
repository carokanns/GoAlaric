package sort

import (
	"GoAlaric/board"
	//"GoAlaric/gen"
	"GoAlaric/move"
	"GoAlaric/piece"
	"GoAlaric/square"
)

//

/*
namespace sort {

   class Killer {
*/
type list struct {
	k1 int
	k2 int
}

const maxPly = 100

// Killer holds the killer moves per Ply
type Killer struct {
	entry [maxPly]list
}

// Clear killer moves
func (k *Killer) Clear() {
	for ply := 0; ply < maxPly; ply++ {
		k.entry[ply].k1 = move.None
		k.entry[ply].k2 = move.None
	}
}

// Add killer 1 or 2
func (k *Killer) Add(mv, ply int) {
	if k.entry[ply].k1 != mv {
		k.entry[ply].k2 = k.entry[ply].k1
		k.entry[ply].k1 = mv
	}
}

// Killer1 is first killer move
func (k *Killer) Killer1(ply int) int {
	return k.entry[ply].k1
}

// Killer2 is the second killer move
func (k *Killer) Killer2(ply int) int {
	return k.entry[ply].k2
}

// History tab bits
const (
	histBits  = 11
	histOne   = 1 << histBits
	histHalf  = 1 << (histBits - 1)
	histShift = 5
)

// HistoryTab holds history tab entries
type HistoryTab struct {
	entry [piece.SideSize * square.BoardSize]int
}

func (h *HistoryTab) index(mv int, bd *board.Board) int {

	// assert(!move::is_tactical(mv));

	sd := bd.SquareSide(move.From(mv))
	p12 := piece.MakeP12(move.Piece(mv), sd)

	return p12*square.BoardSize + move.To(mv)
}

func (h *HistoryTab) updateGood(mv int, bd *board.Board) {
	if !move.IsTactical(mv) {
		h.entry[h.index(mv, bd)] += (histOne - h.entry[h.index(mv, bd)]) >> histShift
	}
}

func (h *HistoryTab) updateBad(mv int, bd *board.Board) {
	if !move.IsTactical(mv) {
		h.entry[h.index(mv, bd)] -= h.entry[h.index(mv, bd)] >> histShift
	}
}

// Score returns the history tab score
func (h *HistoryTab) Score(mv int, bd *board.Board) int {
	return h.entry[h.index(mv, bd)]
}

// Clear history table
func (h *HistoryTab) Clear() {
	for ix := range h.entry {
		h.entry[ix] = histHalf
	}

	for ix := 0; ix < piece.SideSize*square.BoardSize; ix++ {
		//util.ASSERT(h.entry[ix] == PROB_HALF)
		h.entry[ix] = histHalf
	}
}

// Add score into history table
func (h *HistoryTab) Add(bm int, searched *ScMvList, bd *board.Board) {

	//util.ASSERT(bm != move.None)

	h.updateGood(bm, bd)

	for pos := 0; pos < searched.Size(); pos++ {
		mv := searched.Move(pos)
		if mv != bm {
			h.updateBad(mv, bd)
		}
	}
}

// ******* utanför class - ej methods ****************

// Evasions is sorting evasion moves
func Evasions(ml *ScMvList, transMv int) {

	for pos := 0; pos < ml.Size(); pos++ {
		ml.SetScore(pos, evasionScore(ml.Move(pos), transMv))
	}

	ml.Sort()
}

func evasionScore(mv, transMv int) int {

	if mv == transMv {
		return move.ScoreMask
	} else if move.IsTactical(mv) {
		return tacticalScore(move.Piece(mv), move.Capt(mv), move.Prom(mv)) + 1
		// assert(sc >= 1 && sc < 41)
	}
	return 0

}

func tacticalScore(pc, cp, pp int) int {

	if cp != piece.None {
		return captScore(pc, cp) + 4
	}
	return promotionScore(pp)

}

func captScore(pc, cp int) int {
	sc := cp*6 + (5 - pc)
	// assert(sc >= 0 && sc < 36);
	return sc
}

func promotionScore(pp int) int {
	switch pp {
	case piece.Queen:
		return 3
	case piece.Knight:
		return 2
	case piece.Rook:
		return 1
	case piece.Bishop:
		return 0
	default:
		// assert(false)
		return 0
	}
}

//func tacticalScore(mv int) int {
//	// assert(move::is_tactical(mv));
//	return tacticalScore(move.Piece(mv), move.Capt(mv), move.Prom(mv))
//}

// Tacticals is sorting tactical moves
func Tacticals(ml *ScMvList) {

	for pos := 0; pos < ml.Size(); pos++ {
		mv := ml.Move(pos)
		sc := tacticalScore(move.Piece(mv), move.Capt(mv), move.Prom(mv))
		ml.SetScore(pos, sc)
	}

	ml.Sort()
}

// History is sorting history moves
func History(ml *ScMvList, bd *board.Board, history *HistoryTab) {

	for pos := 0; pos < ml.Size(); pos++ {
		mv := ml.Move(pos)
		sc := history.Score(mv, bd)
		ml.SetScore(pos, sc)
	}

	ml.Sort()
}
