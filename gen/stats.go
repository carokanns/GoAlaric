package gen

import (
	"sync"
	"sync/atomic"

	"goalaric/hash"
)

// SeeStats is temporary diagnostic accounting for SEE/NoSacrifice usage.
type SeeStats struct {
	NoSacrificeCalls          uint64
	GeneratorNoSacrificeCalls uint64
	SearchNoSacrificeCalls    uint64
	SeeCalls                  uint64
	SeeNodes                  uint64
	DuplicateNoSacrificeCalls uint64
	CrossSourceDuplicates     uint64
}

var seeStats struct {
	noSacrificeCalls          atomic.Uint64
	generatorNoSacrificeCalls atomic.Uint64
	searchNoSacrificeCalls    atomic.Uint64
	seeCalls                  atomic.Uint64
	seeNodes                  atomic.Uint64
	duplicateNoSacrificeCalls atomic.Uint64
	crossSourceDuplicates     atomic.Uint64
	seenNoSacrifice           map[noSacrificeKey]uint8
	seenMu                    sync.Mutex
}

type noSacrificeKey struct {
	key hash.Key
	mv  int
}

const (
	noSacrificeGenerator uint8 = 1 << iota
	noSacrificeSearch
)

func recordNoSacrifice(key hash.Key, mv int, source uint8) {
	seeStats.seenMu.Lock()
	defer seeStats.seenMu.Unlock()
	if seeStats.seenNoSacrifice == nil {
		seeStats.seenNoSacrifice = make(map[noSacrificeKey]uint8)
	}
	statKey := noSacrificeKey{key: key, mv: mv}
	previous := seeStats.seenNoSacrifice[statKey]
	if previous != 0 {
		seeStats.duplicateNoSacrificeCalls.Add(1)
	}
	if previous != 0 && previous&source == 0 {
		seeStats.crossSourceDuplicates.Add(1)
	}
	seeStats.seenNoSacrifice[statKey] = previous | source
}

func CountGeneratorNoSacrifice(key hash.Key, mv int) {
	seeStats.generatorNoSacrificeCalls.Add(1)
	recordNoSacrifice(key, mv, noSacrificeGenerator)
}

func CountSearchNoSacrifice(key hash.Key, mv int) {
	seeStats.searchNoSacrificeCalls.Add(1)
	recordNoSacrifice(key, mv, noSacrificeSearch)
}

func SnapshotSeeStats() SeeStats {
	return SeeStats{
		NoSacrificeCalls:          seeStats.noSacrificeCalls.Load(),
		GeneratorNoSacrificeCalls: seeStats.generatorNoSacrificeCalls.Load(),
		SearchNoSacrificeCalls:    seeStats.searchNoSacrificeCalls.Load(),
		SeeCalls:                  seeStats.seeCalls.Load(),
		SeeNodes:                  seeStats.seeNodes.Load(),
		DuplicateNoSacrificeCalls: seeStats.duplicateNoSacrificeCalls.Load(),
		CrossSourceDuplicates:     seeStats.crossSourceDuplicates.Load(),
	}
}
