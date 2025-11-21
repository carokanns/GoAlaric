package gen

import (
	"goalaric/move"
)

const maxPly = 100

type killers struct {
	k1 int
	k2 int
}

// Killer holds the killer moves per Ply
type Killer struct {
	entry [maxPly]killers
}

// Clear killer moves
func (k *Killer) Clear() {
	for ply := 0; ply < maxPly; ply++ {
		k.entry[ply].k1 = move.None
		k.entry[ply].k2 = move.None
	}
}

// Add killer 1 or 2
func (k *Killer) Add(mv, ply int) {
	if k.entry[ply].k1 != mv {
		k.entry[ply].k2 = k.entry[ply].k1
		k.entry[ply].k1 = mv
	}
}

// Killer1 is first killer move
func (k *Killer) Killer1(ply int) int {
	return k.entry[ply].k1
}

// Killer2 is the second killer move
func (k *Killer) Killer2(ply int) int {
	return k.entry[ply].k2
}
