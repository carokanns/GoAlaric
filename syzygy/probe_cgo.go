//go:build cgo

package syzygy

/*
#cgo CFLAGS: -O3 -std=gnu11 -I${SRCDIR}
#cgo linux CFLAGS: -pthread
#cgo linux LDFLAGS: -pthread
#include <stdbool.h>
#include <stdlib.h>
#include "tbprobe.h"

static unsigned go_tb_probe_wdl(
    uint64_t white, uint64_t black, uint64_t kings, uint64_t queens,
    uint64_t rooks, uint64_t bishops, uint64_t knights, uint64_t pawns,
    unsigned rule50, unsigned castling, unsigned ep, bool turn) {
    return tb_probe_wdl(white, black, kings, queens, rooks, bishops,
        knights, pawns, rule50, castling, ep, turn);
}

static unsigned go_tb_probe_root(
    uint64_t white, uint64_t black, uint64_t kings, uint64_t queens,
    uint64_t rooks, uint64_t bishops, uint64_t knights, uint64_t pawns,
    unsigned rule50, unsigned castling, unsigned ep, bool turn) {
    return tb_probe_root(white, black, kings, queens, rooks, bishops,
        knights, pawns, rule50, castling, ep, turn, NULL);
}

static unsigned go_tb_get_wdl(unsigned result) { return TB_GET_WDL(result); }
static unsigned go_tb_get_dtz(unsigned result) { return TB_GET_DTZ(result); }
static unsigned go_tb_get_from(unsigned result) { return TB_GET_FROM(result); }
static unsigned go_tb_get_to(unsigned result) { return TB_GET_TO(result); }
static unsigned go_tb_get_promotes(unsigned result) { return TB_GET_PROMOTES(result); }
static unsigned go_tb_get_ep(unsigned result) { return TB_GET_EP(result); }
*/
import "C"

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
)

var (
	loadedPieces atomic.Int32
	loadMu       sync.Mutex
	rootMu       sync.Mutex
)

// Available reports whether this build contains the native Fathom prober.
func Available() bool { return true }

// Load initializes Fathom with one or more platform-separated directories.
// An empty path disables tablebase probing.
func Load(path string) (int, error) {
	loadMu.Lock()
	defer loadMu.Unlock()

	path = strings.TrimSpace(path)
	if path == "" {
		C.tb_free()
		loadedPieces.Store(0)
		return 0, nil
	}

	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	if C.tb_init(cPath) == C.bool(false) {
		loadedPieces.Store(0)
		return 0, errors.New("Fathom could not initialize the Syzygy path")
	}

	largest := int(C.TB_LARGEST)
	loadedPieces.Store(int32(largest))
	return largest, nil
}

// Largest returns the largest loaded tablebase cardinality, or zero if none
// are available.
func Largest() int {
	return int(loadedPieces.Load())
}

// ProbeWDL probes a non-root position. Fathom's search probe deliberately
// accepts only positions with no castling rights and a zero half-move clock.
func ProbeWDL(p Position) (WDL, bool) {
	if Largest() == 0 || p.PieceCount() > Largest() || p.Castling != 0 || p.Rule50 != 0 {
		return Draw, false
	}

	result := uint32(C.go_tb_probe_wdl(
		C.uint64_t(p.White), C.uint64_t(p.Black), C.uint64_t(p.Kings),
		C.uint64_t(p.Queens), C.uint64_t(p.Rooks), C.uint64_t(p.Bishops),
		C.uint64_t(p.Knights), C.uint64_t(p.Pawns), C.uint(p.Rule50),
		C.uint(p.Castling), C.uint(p.EnPassant), C.bool(p.WhiteToMove),
	))
	if result == uint32(C.TB_RESULT_FAILED) || result > uint32(Win) {
		return Draw, false
	}
	return WDL(result), true
}

// ProbeRoot probes DTZ once at the root and returns a WDL-preserving move.
func ProbeRoot(p Position) (RootResult, bool) {
	if Largest() == 0 || p.PieceCount() > Largest() || p.Castling != 0 {
		return RootResult{}, false
	}

	rootMu.Lock()
	defer rootMu.Unlock()
	result := uint32(C.go_tb_probe_root(
		C.uint64_t(p.White), C.uint64_t(p.Black), C.uint64_t(p.Kings),
		C.uint64_t(p.Queens), C.uint64_t(p.Rooks), C.uint64_t(p.Bishops),
		C.uint64_t(p.Knights), C.uint64_t(p.Pawns), C.uint(p.Rule50),
		C.uint(p.Castling), C.uint(p.EnPassant), C.bool(p.WhiteToMove),
	))
	if result == uint32(C.TB_RESULT_FAILED) {
		return RootResult{}, false
	}

	wdl := WDL(C.go_tb_get_wdl(C.uint(result)))
	if wdl > Win {
		return RootResult{}, false
	}
	return RootResult{
		WDL:       wdl,
		DTZ:       int(C.go_tb_get_dtz(C.uint(result))),
		From:      int(C.go_tb_get_from(C.uint(result))),
		To:        int(C.go_tb_get_to(C.uint(result))),
		Promotion: Promotion(C.go_tb_get_promotes(C.uint(result))),
		EnPassant: C.go_tb_get_ep(C.uint(result)) != 0,
	}, true
}
