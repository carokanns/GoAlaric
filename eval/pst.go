package eval

import (
	"GoAlaric/piece"
	"GoAlaric/square"
	"fmt"
)

var knightLine = [8]int{-4, -2, 0, +1, +1, 0, -2, -4}
var scoreLine = [8]int{-3, -1, 0, +1, +1, 0, -1, -3}
var kingFile = [8]int{+1, +2, 0, -2, -2, 0, +2, +1}
var kingRank = [8]int{+1, 0, -2, -4, -6, -8, -10, -12}

var advanceRank = [8]int{-3, -2, -1, 0, +1, +2, +1, 0}

var scoreTab [piece.SideSize][square.BoardSize][stageSize]int

// Score returns the piece square table value for a given piece on a given square. Stage = MG/EG
func Score(p12, sq, stage int) int {
	return scoreTab[p12][sq][stage]
}

// PstInit intits the pieces-square-tables when the program starts
func PstInit() {
	fmt.Println("info string PstInit startar")
	for p12 := 0; p12 < piece.SideSize; p12++ {
		for sq := 0; sq < square.BoardSize; sq++ {
			scoreTab[p12][sq][MG] = 0
			scoreTab[p12][sq][EG] = 0
		}
	}

	for sq := 0; sq < square.BoardSize; sq++ {

		fl := square.File(sq)
		rk := square.Rank(sq)

		scoreTab[piece.WhitePawn][sq][MG] = 0
		scoreTab[piece.WhitePawn][sq][EG] = 0

		scoreTab[piece.WhiteKnight][sq][MG] = (knightLine[fl] + knightLine[rk] + advanceRank[rk]) * 4
		scoreTab[piece.WhiteKnight][sq][EG] = (knightLine[fl] + knightLine[rk] + advanceRank[rk]) * 4

		scoreTab[piece.WhiteBishop][sq][MG] = (scoreLine[fl] + scoreLine[rk]) * 2
		scoreTab[piece.WhiteBishop][sq][EG] = (scoreLine[fl] + scoreLine[rk]) * 2

		scoreTab[piece.WhiteRook][sq][MG] = scoreLine[fl] * 5
		scoreTab[piece.WhiteRook][sq][EG] = 0

		scoreTab[piece.WhiteQueen][sq][MG] = (scoreLine[fl] + scoreLine[rk]) * 1
		scoreTab[piece.WhiteQueen][sq][EG] = (scoreLine[fl] + scoreLine[rk]) * 1

		scoreTab[piece.WhiteKing][sq][MG] = (kingFile[fl] + kingRank[rk]) * 8
		scoreTab[piece.WhiteKing][sq][EG] = (scoreLine[fl] + scoreLine[rk] + advanceRank[rk]) * 8
	}

	for pc := piece.Pawn; pc <= piece.King; pc++ {

		wp := piece.MakeP12(pc, WHITE)
		bp := piece.MakeP12(pc, BLACK)

		for bs := 0; bs < square.BoardSize; bs++ {

			ws := square.OppositRank(bs)
			scoreTab[bp][bs][MG] = scoreTab[wp][ws][MG]
			scoreTab[bp][bs][EG] = scoreTab[wp][ws][EG]
		}
	}
}
