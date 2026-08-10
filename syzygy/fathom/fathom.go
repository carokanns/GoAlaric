// Package fathom provides a small, synchronized Go wrapper around the
// vendored Fathom Syzygy probing code.
package fathom

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

const (
	Loss        = 0
	BlessedLoss = 1
	Draw        = 2
	CursedWin   = 3
	Win         = 4
)

// Position uses ordinary Syzygy bitboards where A1 is bit 0, B1 is bit 1.
type Position struct {
	White, Black                           uint64
	Kings, Queens, Rooks, Bishops, Knights uint64
	Pawns                                  uint64
	Rule50                                 uint
	Castling                               uint
	EnPassant                              uint
	WhiteToMove                            bool
}

// RootResult is the WDL/DTZ-optimal move returned by Fathom.
type RootResult struct {
	WDL       int
	From      int
	To        int
	Promotion int
	EnPassant bool
	DTZ       int
}

var (
	errNotCompiled = errors.New("Syzygy support is unavailable in this build (cgo disabled)")
	probeMu        sync.RWMutex
	largest        int
	enabled        bool
)

// SetPath closes the previous tables and opens the table files in path. An
// empty path disables probing and always succeeds, including without cgo.
func SetPath(path string) error {
	probeMu.Lock()
	defer probeMu.Unlock()

	backendFree()
	largest = 0
	enabled = false
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if !backendCompiled() {
		return errNotCompiled
	}
	ok, found := backendInit(path)
	if !ok {
		return fmt.Errorf("could not initialize Syzygy path %q", path)
	}
	if found == 0 {
		backendFree()
		return fmt.Errorf("no Syzygy tables found in %q", path)
	}
	largest = found
	enabled = true
	return nil
}

// Enabled reports whether at least one table has been loaded.
func Enabled() bool {
	probeMu.RLock()
	defer probeMu.RUnlock()
	return enabled
}

// Largest reports the largest available table cardinality.
func Largest() int {
	probeMu.RLock()
	defer probeMu.RUnlock()
	return largest
}

// Compiled reports whether this binary contains the cgo Fathom backend.
func Compiled() bool { return backendCompiled() }

// ProbeWDL performs the thread-safe search probe.
func ProbeWDL(position Position) (int, bool) {
	probeMu.RLock()
	defer probeMu.RUnlock()
	if !enabled {
		return 0, false
	}
	return backendProbeWDL(position)
}

// ProbeRoot performs the non-thread-safe root DTZ probe under an exclusive
// lock so it cannot overlap another root or path reconfiguration.
func ProbeRoot(position Position) (RootResult, bool) {
	probeMu.Lock()
	defer probeMu.Unlock()
	if !enabled {
		return RootResult{}, false
	}
	return backendProbeRoot(position)
}
