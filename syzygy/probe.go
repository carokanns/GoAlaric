// Package syzygy provides local probing of Syzygy WDL and DTZ tablebases.
package syzygy

import "math/bits"

// WDL is the tablebase result from the point of view of the side to move.
type WDL uint8

const (
	Loss WDL = iota
	BlessedLoss
	Draw
	CursedWin
	Win
)

func (w WDL) String() string {
	switch w {
	case Loss:
		return "loss"
	case BlessedLoss:
		return "blessed-loss"
	case Draw:
		return "draw"
	case CursedWin:
		return "cursed-win"
	case Win:
		return "win"
	default:
		return "unknown"
	}
}

// Promotion is the piece used by a root tablebase move.
type Promotion uint8

const (
	PromoteNone Promotion = iota
	PromoteQueen
	PromoteRook
	PromoteBishop
	PromoteKnight
)

// Position uses standard chess bitboards: A1 is bit 0 and H8 is bit 63.
type Position struct {
	White, Black                  uint64
	Kings, Queens, Rooks, Bishops uint64
	Knights, Pawns                uint64
	Rule50, Castling, EnPassant   uint32
	WhiteToMove                   bool
}

// PieceCount returns the number of pieces represented by the position.
func (p Position) PieceCount() int {
	return bits.OnesCount64(p.White | p.Black)
}

// RootResult is the DTZ result and suggested move for a root position.
type RootResult struct {
	WDL       WDL
	DTZ       int
	From      int
	To        int
	Promotion Promotion
	EnPassant bool
}
