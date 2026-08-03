package gen

import (
	"goalaric/board"
	"goalaric/material"
	"goalaric/move"
	"goalaric/square"
)

// History tab bits
const (
	histBits     = 11
	histOne      = 1 << histBits
	histMax      = histOne - 1
	histHalf     = 1 << (histBits - 1)
	histBonusMax = 400
)

// HistoryTab holds history tab entries
type HistoryTab struct {
	entry [material.SideSize * square.BoardSize]int
}

func (h *HistoryTab) index(mv int, bd *board.Board) int {

	// assert(!move::is_tactical(mv));

	sd := bd.SquareSide(move.From(mv))
	p12 := material.MakeP12(move.Piece(mv), sd)

	return p12*square.BoardSize + move.To(mv)
}

func historyBonus(depth int) int {
	if depth <= 0 {
		return 0
	}
	if depth >= 20 {
		return histBonusMax
	}
	return depth * depth
}

func (h *HistoryTab) good(mv int, depth int, bd *board.Board) {
	if !move.IsTactical(mv) {
		ix := h.index(mv, bd)
		h.entry[ix] += historyBonus(depth)
		if h.entry[ix] > histMax {
			h.entry[ix] = histMax
		}
	}
}

func (h *HistoryTab) bad(mv int, depth int, bd *board.Board) {
	if !move.IsTactical(mv) {
		ix := h.index(mv, bd)
		h.entry[ix] -= historyBonus(depth)
		if h.entry[ix] < 0 {
			h.entry[ix] = 0
		}
	}
}

// Score returns the history tab score
func (h *HistoryTab) Score(mv int, bd *board.Board) int {
	return h.entry[h.index(mv, bd)]
}

// IsStrong reports whether a quiet move has accumulated clearly positive
// history. Search uses this to reduce promising late moves less aggressively.
func (h *HistoryTab) IsStrong(mv int, bd *board.Board) bool {
	return !move.IsTactical(mv) && h.Score(mv, bd) >= histHalf+histBonusMax/2
}

// Clear history table
func (h *HistoryTab) Clear() {
	for ix := range h.entry {
		h.entry[ix] = histHalf
	}

	for ix := 0; ix < material.SideSize*square.BoardSize; ix++ {
		h.entry[ix] = histHalf
	}
}

// Add score into history table
func (h *HistoryTab) Add(bm int, searched *ScMvList, depth int, bd *board.Board) {
	h.good(bm, depth, bd)

	for pos := 0; pos < searched.Size(); pos++ {
		mv := searched.Move(pos)
		if mv != bm {
			h.bad(mv, depth, bd)
		}
	}
}

// Sort is sorting history moves
func (h *HistoryTab) Sort(ml *ScMvList, bd *board.Board) {

	for pos := 0; pos < ml.Size(); pos++ {
		mv := ml.Move(pos)
		sc := h.Score(mv, bd)
		ml.SetScore(pos, sc)
	}

	ml.Sort()
}
