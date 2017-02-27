package fastgen

import (
	"GoAlaric/board"
	"GoAlaric/move"
	"GoAlaric/piece"
	"GoAlaric/sort"
	"GoAlaric/square"
)

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
func (h *HistoryTab) Add(bm int, searched *sort.ScMvList, bd *board.Board) {

	//util.ASSERT(bm != move.None)

	h.updateGood(bm, bd)

	for pos := 0; pos < searched.Size(); pos++ {
		mv := searched.Move(pos)
		if mv != bm {
			h.updateBad(mv, bd)
		}
	}
}

// History is sorting history moves
func History(ml *sort.ScMvList, bd *board.Board, history *HistoryTab) {

	for pos := 0; pos < ml.Size(); pos++ {
		mv := ml.Move(pos)
		sc := history.Score(mv, bd)
		ml.SetScore(pos, sc)
	}

	ml.Sort()
}
