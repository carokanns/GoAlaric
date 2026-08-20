package search

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"goalaric/bit"
	"goalaric/board"
	"goalaric/gen"
	"goalaric/material"
	"goalaric/move"
	"goalaric/square"
	"goalaric/syzygy"
)

// DefaultSyzygyPath is relative to the engine's working directory. Testmonitor
// resolves and supplies an absolute path for reproducible match runs.
const DefaultSyzygyPath = ".tools/syzygy/3-4"

const tablebaseWinScore = 8000

var (
	gameTablebaseHits     atomic.Int64
	gameTablebaseRootWins atomic.Int64
)

// SetSyzygyPath reloads the local tablebase set. Reconfiguration is forbidden
// while a search is active because Fathom owns process-global mapped files.
func SetSyzygyPath(path string) (int, error) {
	if Status == Running {
		return syzygy.Largest(), errors.New("SyzygyPath cannot change during an active search")
	}
	path = strings.TrimSpace(path)
	largest, err := syzygy.Load(path)
	if err != nil {
		return 0, err
	}
	Engine.SyzygyPath = path
	Engine.SyzygyPieces = largest
	return largest, nil
}

func tablebasePosition(bd *board.Board) syzygy.Position {
	ep := uint32(0)
	if bd.EpSq() != square.None {
		ep = uint32(toSyzygySquare(bd.EpSq()))
	}
	return syzygy.Position{
		White:       toSyzygyBitboard(bd.Side(board.WHITE)),
		Black:       toSyzygyBitboard(bd.Side(board.BLACK)),
		Kings:       toSyzygyBitboard(bd.Piece(material.King)),
		Queens:      toSyzygyBitboard(bd.Piece(material.Queen)),
		Rooks:       toSyzygyBitboard(bd.Piece(material.Rook)),
		Bishops:     toSyzygyBitboard(bd.Piece(material.Bishop)),
		Knights:     toSyzygyBitboard(bd.Piece(material.Knight)),
		Pawns:       toSyzygyBitboard(bd.Piece(material.Pawn)),
		Rule50:      uint32(bd.HalfmoveClock()),
		Castling:    uint32(bd.Flags()),
		EnPassant:   ep,
		WhiteToMove: bd.Stm() == board.WHITE,
	}
}

// GoAlaric stores A1,A2,... by file, while Syzygy uses A1,B1,... by rank.
func toSyzygyBitboard(bb bit.BB) uint64 {
	var result uint64
	for bb != 0 {
		sq := bit.First(bb)
		result |= uint64(1) << uint(toSyzygySquare(sq))
		bb = bit.Rest(bb)
	}
	return result
}

func toSyzygySquare(sq int) int {
	return square.Rank(sq)*8 + square.File(sq)
}

func fromSyzygySquare(sq int) int {
	return square.Make(sq&7, sq>>3)
}

func tablebaseScore(wdl syzygy.WDL, ply int) int {
	switch wdl {
	case syzygy.Win:
		return tablebaseWinScore - ply
	case syzygy.CursedWin:
		return 1
	case syzygy.BlessedLoss:
		return -1
	case syzygy.Loss:
		return -tablebaseWinScore + ply
	default:
		return 0
	}
}

func probeTablebaseScore(bd *board.Board) (int, bool) {
	// Keep the overwhelmingly common non-tablebase path entirely in Go and
	// avoid converting eight bitboards before we know a native probe is useful.
	if Engine.SyzygyPieces == 0 || bit.Count(bd.All()) > Engine.SyzygyPieces ||
		bd.Flags() != 0 || bd.HalfmoveClock() != 0 || bd.InNullMoveSubtree() {
		return 0, false
	}
	wdl, ok := syzygy.ProbeWDL(tablebasePosition(bd))
	if !ok {
		return 0, false
	}
	recordTablebaseHit(false)
	return tablebaseScore(wdl, bd.Ply()), true
}

func recordTablebaseHit(rootWin bool) {
	gameTablebaseHits.Add(1)
	if rootWin {
		gameTablebaseRootWins.Add(1)
	}
}

// ResetTablebaseGameStats starts a fresh per-game tablebase counter.
func ResetTablebaseGameStats() {
	gameTablebaseHits.Store(0)
	gameTablebaseRootWins.Store(0)
}

// ConsumeTablebaseGameStats returns and resets the current per-game counters.
func ConsumeTablebaseGameStats() (hits int64, rootWins int64) {
	hits = gameTablebaseHits.Swap(0)
	rootWins = gameTablebaseRootWins.Swap(0)
	return hits, rootWins
}

func probeRootTablebase(bd *board.Board, legal *gen.ScMvList) (syzygy.RootResult, int, bool) {
	if Engine.SyzygyPieces == 0 || bit.Count(bd.All()) > Engine.SyzygyPieces || bd.Flags() != 0 {
		return syzygy.RootResult{}, move.None, false
	}
	result, ok := syzygy.ProbeRoot(tablebasePosition(bd))
	if !ok {
		return syzygy.RootResult{}, move.None, false
	}

	from := fromSyzygySquare(result.From)
	to := fromSyzygySquare(result.To)
	promotion := material.None
	switch result.Promotion {
	case syzygy.PromoteQueen:
		promotion = material.Queen
	case syzygy.PromoteRook:
		promotion = material.Rook
	case syzygy.PromoteBishop:
		promotion = material.Bishop
	case syzygy.PromoteKnight:
		promotion = material.Knight
	}

	for pos := 0; pos < legal.Size(); pos++ {
		mv := legal.Move(pos)
		if move.From(mv) == from && move.To(mv) == to && move.Prom(mv) == promotion {
			return result, mv, true
		}
	}
	tellGUI(fmt.Sprintf("info string Syzygy returned unmatched root move %v%v", result.From, result.To))
	return syzygy.RootResult{}, move.None, false
}
