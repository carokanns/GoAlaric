//go:build cgo

package fathom

/*
#cgo CFLAGS: -O3 -std=gnu11 -I${SRCDIR}
#cgo LDFLAGS: -pthread
#include <stdlib.h>
#include "tbprobe.h"

static unsigned go_tb_get_wdl(unsigned result) { return TB_GET_WDL(result); }
static unsigned go_tb_get_from(unsigned result) { return TB_GET_FROM(result); }
static unsigned go_tb_get_to(unsigned result) { return TB_GET_TO(result); }
static unsigned go_tb_get_promotes(unsigned result) { return TB_GET_PROMOTES(result); }
static unsigned go_tb_get_ep(unsigned result) { return TB_GET_EP(result); }
static unsigned go_tb_get_dtz(unsigned result) { return TB_GET_DTZ(result); }
*/
import "C"

import "unsafe"

func backendCompiled() bool { return true }

func backendInit(path string) (bool, int) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	if !bool(C.tb_init(cPath)) {
		return false, 0
	}
	return true, int(C.TB_LARGEST)
}

func backendFree() { C.tb_free() }

func backendProbeWDL(p Position) (int, bool) {
	result := C.tb_probe_wdl(
		C.uint64_t(p.White), C.uint64_t(p.Black),
		C.uint64_t(p.Kings), C.uint64_t(p.Queens), C.uint64_t(p.Rooks),
		C.uint64_t(p.Bishops), C.uint64_t(p.Knights), C.uint64_t(p.Pawns),
		C.uint(p.Rule50), C.uint(p.Castling), C.uint(p.EnPassant), C.bool(p.WhiteToMove),
	)
	if result == C.TB_RESULT_FAILED {
		return 0, false
	}
	return int(result), true
}

func backendProbeRoot(p Position) (RootResult, bool) {
	result := C.tb_probe_root(
		C.uint64_t(p.White), C.uint64_t(p.Black),
		C.uint64_t(p.Kings), C.uint64_t(p.Queens), C.uint64_t(p.Rooks),
		C.uint64_t(p.Bishops), C.uint64_t(p.Knights), C.uint64_t(p.Pawns),
		C.uint(p.Rule50), C.uint(p.Castling), C.uint(p.EnPassant), C.bool(p.WhiteToMove), nil,
	)
	if result == C.TB_RESULT_FAILED || result == C.TB_RESULT_CHECKMATE || result == C.TB_RESULT_STALEMATE {
		return RootResult{}, false
	}
	return RootResult{
		WDL:       int(C.go_tb_get_wdl(result)),
		From:      int(C.go_tb_get_from(result)),
		To:        int(C.go_tb_get_to(result)),
		Promotion: int(C.go_tb_get_promotes(result)),
		EnPassant: C.go_tb_get_ep(result) != 0,
		DTZ:       int(C.go_tb_get_dtz(result)),
	}, true
}
