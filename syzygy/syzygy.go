// Package syzygy adapts GoAlaric's board representation to optional Syzygy
// WDL and DTZ probing. Empty or unavailable table paths leave search intact.
package syzygy

import (
	"math/bits"
	"strings"
	"sync/atomic"

	"goalaric/board"
	"goalaric/material"
	"goalaric/square"
	"goalaric/syzygy/fathom"
)

// WDL is the tablebase result from the perspective of the side to move.
type WDL int

const (
	Loss        WDL = -2
	BlessedLoss WDL = -1
	Draw        WDL = 0
	CursedWin   WDL = 1
	Win         WDL = 2
)

// RootResult contains Fathom's 50-move-aware root choice.
type RootResult struct {
	Move string
	WDL  WDL
	DTZ  int
}

var hits atomic.Uint64

// SetPath configures table files. An empty path disables Syzygy.
func SetPath(path string) error {
	hits.Store(0)
	return fathom.SetPath(path)
}

// Compiled reports whether cgo/Fathom is present in this binary.
func Compiled() bool { return fathom.Compiled() }

// Enabled reports whether usable tables were found.
func Enabled() bool { return fathom.Enabled() }

// Largest reports the largest loaded table cardinality.
func Largest() int { return fathom.Largest() }

// ResetHits starts a new per-search tbhits counter.
func ResetHits() { hits.Store(0) }

// Hits returns successful WDL and root probes in the current search.
func Hits() uint64 { return hits.Load() }

// ProbeWDL probes search nodes. Full-cardinality tables respect probeDepth;
// smaller endings are always probed once tables are configured.
func ProbeWDL(bd *board.Board, depth, probeDepth int) (WDL, bool) {
	if !fathom.Enabled() || bd.Flags() != 0 || bd.HalfmoveClock() != 0 {
		return Draw, false
	}
	count := bits.OnesCount64(uint64(bd.All()))
	largest := fathom.Largest()
	if count > largest || (count == largest && depth < probeDepth) {
		return Draw, false
	}
	result, ok := fathom.ProbeWDL(position(bd))
	if !ok {
		return Draw, false
	}
	hits.Add(1)
	return WDL(result - fathom.Draw), true
}

// ProbeRoot returns a 50-move-aware DTZ move for a root tablebase position.
func ProbeRoot(bd *board.Board) (RootResult, bool) {
	if !fathom.Enabled() || bd.Flags() != 0 || bits.OnesCount64(uint64(bd.All())) > fathom.Largest() {
		return RootResult{}, false
	}
	result, ok := fathom.ProbeRoot(position(bd))
	if !ok {
		return RootResult{}, false
	}
	move := square.ToString(fromSyzygySquare(result.From)) + square.ToString(fromSyzygySquare(result.To))
	move += promotionSuffix(result.Promotion)
	hits.Add(1)
	return RootResult{Move: move, WDL: WDL(result.WDL - fathom.Draw), DTZ: result.DTZ}, true
}

func position(bd *board.Board) fathom.Position {
	ep := uint(0)
	if bd.EpSq() != square.None {
		ep = uint(toSyzygySquare(bd.EpSq()))
	}
	return fathom.Position{
		White:       transpose(uint64(bd.Side(board.WHITE))),
		Black:       transpose(uint64(bd.Side(board.BLACK))),
		Kings:       transpose(uint64(bd.Piece(material.King))),
		Queens:      transpose(uint64(bd.Piece(material.Queen))),
		Rooks:       transpose(uint64(bd.Piece(material.Rook))),
		Bishops:     transpose(uint64(bd.Piece(material.Bishop))),
		Knights:     transpose(uint64(bd.Piece(material.Knight))),
		Pawns:       transpose(uint64(bd.Piece(material.Pawn))),
		Rule50:      uint(bd.HalfmoveClock()),
		Castling:    uint(bd.Flags()),
		EnPassant:   ep,
		WhiteToMove: bd.Stm() == board.WHITE,
	}
}

func transpose(bb uint64) uint64 {
	var result uint64
	for bb != 0 {
		sq := bits.TrailingZeros64(bb)
		result |= uint64(1) << uint(toSyzygySquare(sq))
		bb &= bb - 1
	}
	return result
}

func toSyzygySquare(sq int) int {
	return square.Rank(sq)*8 + square.File(sq)
}

func fromSyzygySquare(sq int) int {
	return square.Make(sq&7, sq>>3)
}

func promotionSuffix(promotion int) string {
	return [...]string{"", "q", "r", "b", "n"}[promotion]
}

// NormalizePath preserves case while trimming UCI whitespace.
func NormalizePath(path string) string { return strings.TrimSpace(path) }
