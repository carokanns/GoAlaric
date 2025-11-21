package gen

import (
	"goalaric/board"
	"goalaric/material"
	"goalaric/move"
	"goalaric/square"
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
	entry [material.SideSize * square.BoardSize]int
}

func (h *HistoryTab) index(mv int, bd *board.Board) int {

	// assert(!move::is_tactical(mv));

	sd := bd.SquareSide(move.From(mv))
	p12 := material.MakeP12(move.Piece(mv), sd)

	return p12*square.BoardSize + move.To(mv)
}

func (h *HistoryTab) good(mv int, bd *board.Board) {
	if !move.IsTactical(mv) {
		h.entry[h.index(mv, bd)] += (histOne - h.entry[h.index(mv, bd)]) >> histShift
	}
}

func (h *HistoryTab) bad(mv int, bd *board.Board) {
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

	for ix := 0; ix < material.SideSize*square.BoardSize; ix++ {
		h.entry[ix] = histHalf
	}
}

// Add score into history table
func (h *HistoryTab) Add(bm int, searched *ScMvList, bd *board.Board) {
	h.good(bm, bd)

	for pos := 0; pos < searched.Size(); pos++ {
		mv := searched.Move(pos)
		if mv != bm {
			h.bad(mv, bd)
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
