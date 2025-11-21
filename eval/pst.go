package eval

import (
	"fmt"

	"goalaric/material"
	"goalaric/square"
)

var knightLine = [8]int{-4, -2, 0, +1, +1, 0, -2, -4}
var scoreLine = [8]int{-3, -1, 0, +1, +1, 0, -1, -3}
var kingFile = [8]int{+1, +2, 0, -2, -2, 0, +2, +1}
var kingRank = [8]int{+1, 0, -2, -4, -6, -8, -10, -12}

var advanceRank = [8]int{-3, -2, -1, 0, +1, +2, +1, 0}

var scoreTab [material.SideSize][square.BoardSize][stageSize]int

// Score returns the piece square table value for a given piece on a given square. Stage = MG/EG
func Score(p12, sq, stage int) int {
	return scoreTab[p12][sq][stage]
}

// PstInit intits the pieces-square-tables when the program starts
func PstInit() {
	fmt.Println("info string PstInit startar")
	for p12 := 0; p12 < material.SideSize; p12++ {
		for sq := 0; sq < square.BoardSize; sq++ {
			scoreTab[p12][sq][MG] = 0
			scoreTab[p12][sq][EG] = 0
		}
	}

	for sq := 0; sq < square.BoardSize; sq++ {

		fl := square.File(sq)
		rk := square.Rank(sq)

		scoreTab[material.WhitePawn][sq][MG] = 0
		scoreTab[material.WhitePawn][sq][EG] = 0

		scoreTab[material.WhiteKnight][sq][MG] = (knightLine[fl] + knightLine[rk] + advanceRank[rk]) * 4
		scoreTab[material.WhiteKnight][sq][EG] = (knightLine[fl] + knightLine[rk] + advanceRank[rk]) * 4

		scoreTab[material.WhiteBishop][sq][MG] = (scoreLine[fl] + scoreLine[rk]) * 2
		scoreTab[material.WhiteBishop][sq][EG] = (scoreLine[fl] + scoreLine[rk]) * 2

		scoreTab[material.WhiteRook][sq][MG] = scoreLine[fl] * 5
		scoreTab[material.WhiteRook][sq][EG] = 0

		scoreTab[material.WhiteQueen][sq][MG] = (scoreLine[fl] + scoreLine[rk]) * 1
		scoreTab[material.WhiteQueen][sq][EG] = (scoreLine[fl] + scoreLine[rk]) * 1

		scoreTab[material.WhiteKing][sq][MG] = (kingFile[fl] + kingRank[rk]) * 8
		scoreTab[material.WhiteKing][sq][EG] = (scoreLine[fl] + scoreLine[rk] + advanceRank[rk]) * 8
	}

	for pc := material.Pawn; pc <= material.King; pc++ {

		wP12 := material.MakeP12(pc, WHITE)
		bP12 := material.MakeP12(pc, BLACK)

		for bSq := 0; bSq < square.BoardSize; bSq++ {

			wSq := square.OppositRank(bSq)
			scoreTab[bP12][bSq][MG] = scoreTab[wP12][wSq][MG]
			scoreTab[bP12][bSq][EG] = scoreTab[wP12][wSq][EG]
		}
	}
}
