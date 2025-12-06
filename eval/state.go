package eval

import (
	"goalaric/bit"
	"goalaric/board"
	"goalaric/material"
)

// evalState caches board-derived data used during evaluation to avoid
// repeated lookups while walking the board.
type evalState struct {
	kings       [2]int
	empty       bit.BB
	side        [2]bit.BB
	pawnAttacks [2]bit.BB
	pieceBB     [2][material.Size]bit.BB
	counts      [2][material.Size]int
}

func newEvalState(bd *board.Board) evalState {
	var st evalState

	st.empty = bd.Empty()
	for sd := 0; sd < 2; sd++ {
		st.kings[sd] = bd.King(sd)
		st.side[sd] = bd.Side(sd)
		st.pawnAttacks[sd] = PawnAttacksFrom(sd, bd)

		for pc := material.Pawn; pc <= material.King; pc++ {
			bb := bd.PieceSd(pc, sd)
			st.pieceBB[sd][pc] = bb
			st.counts[sd][pc] = bd.Count(pc, sd)
		}
	}

	return st
}
