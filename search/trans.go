package search

import (
	"GoAlaric/hash"
	"GoAlaric/move"
	"fmt"
)

// score limits
const (
	EvalMin = -8999
	EvalMAX = +8999
)

//
// score types
const (
	//no scoretype = 0
	scoreTypeLower   = 0x1                             // sc > alpha
	scoreTypeUpper   = 0x2                             // sc < beta
	scoreTypeBetween = scoreTypeLower | scoreTypeUpper // alpha < sc < beta
)

// scoreType sets if it is an upper or lower score
func scoreType(sc, alpha, beta int) int {

	scoreType := 0
	if sc > alpha {
		scoreType |= scoreTypeLower
	}
	if sc < beta {
		scoreType |= scoreTypeUpper
	}

	return scoreType
}

const sizeEntry = 16

type entry struct { // 16 bytes
	lock      uint32
	move      uint32 // TODO: maybe uint16 (fr+to+pr)
	utfyllnad uint16 // endast utfyllnad
	score     int16
	date      uint8
	depth     int8
	scoreType uint8
	tomt      uint8 // endast utfyllnad
}

// Table is a "header" to hash tables
type transTable struct {
	entries []entry
	cntBits int
	size    uint64
	mask    uint64

	generation int
	cntUsed    uint64
}

// IncDate increments the date for the hahs table.
// The date is used to know if an entry is fresh or old
func (t *transTable) IncDate() {
	t.generation = (t.generation + 1) % 256
	t.cntUsed = 0
}

func sizeToBits(size int) int {
	bits := 0
	for entries := (uint64(size) << 20) / sizeEntry; entries > 1; entries /= 2 {
		bits++
	}

	return bits
}

//   public:

// InitTable nullfies all the "header" values
func (t *transTable) InitTable() {
	fmt.Println("info string Trans init startar")
	t.entries = nil
	t.cntBits = 0
	t.size = 1
	t.mask = 0

	t.generation = 0
	t.cntUsed = 0
}

// Clear all entries
func (t *transTable) Clear() {
	var e entry
	clearEntry(&e)

	for i := uint64(0); i < t.size; i++ {
		t.entries[i] = e
	}

	t.generation = 1
	t.cntUsed = 0
}

// Store one position in Hash table.
// The key is computed from the position. The lock value is the 32 first bits in the key
// From the key we get an index to the table.
// We will try 4 entries in a sequence until a lock is found
// We always try to replace another generation and/or a lower searched depth
func (t *transTable) Store(key hash.Key, depth, ply, mv, sc, scoreType int) {
	//fmt.Println(key, depth, ply, mv, sc, flags)
	//util.ASSERT(depth >= 0 && depth < 100)
	//util.ASSERT(mv != move.NULL_)
	//util.ASSERT(sc >= score.MIN && sc <= score.MAX)

	sc = RemMatePly(sc, ply)

	index := hash.Index(key) & int64(t.mask)
	lock := hash.Lock(key)

	var be *entry
	bs := -1

	for i := uint64(0); i < 4; i++ {
		idx := (uint64(index) + i) & t.mask
		//util.ASSERT(idx < t.p_size)
		entry := &t.entries[idx]

		if entry.lock == lock {
			if int(entry.date) != t.generation {
				entry.date = uint8(t.generation)
				t.cntUsed++
			}

			if depth >= int(entry.depth) {
				if mv != move.None {
					entry.move = uint32(mv)
				}
				entry.depth = int8(depth)
				entry.score = int16(sc)
				entry.scoreType = uint8(scoreType)
				return
			}

			if entry.move == move.None {
				entry.move = uint32(mv)
			}

			return
		}

		sc2 := 99 - int(entry.depth) // NOTE: entry.depth can be -1
		if entry.date != uint8(t.generation) {
			sc2 += 101
		}
		//util.ASSERT(sc2 >= 0 && sc2 < 202)

		if sc2 > bs {
			be = entry
			bs = sc2
		}
	}

	//util.ASSERT(be != nil)

	if be.date != uint8(t.generation) {
		t.cntUsed++
	}

	be.lock = lock
	be.date = uint8(t.generation)
	be.move = uint32(mv)
	be.depth = int8(depth)
	be.score = int16(sc)
	be.scoreType = uint8(scoreType)
}

// SetSize sets the size that will be used next time we Allocate a new Hash Table
func (t *transTable) SetSize(size int) {
	bits := sizeToBits(size)
	if bits == t.cntBits {
		return
	}

	t.cntBits = bits
	t.size = 1 << uint64(bits)
	t.mask = t.size - 1
}

// Alloc makes a Hash table with the size that is set by SetSize
func (t *transTable) Alloc() {
	t.entries = make([]entry, t.size)
	t.Clear()
}

// Retrieve gets info from the Hash Table if the key and lock is correct
// if no entry is matching return false else return true
// the pointers mv (move), sc (score) and flags (UPPER/LOWER) are used to return values
// We will try the 4 entries in sequence until lock match otherwise return false
func (t *transTable) Retrieve(key hash.Key, depth, ply int, mv *int, sc *int, flags *int) bool {

	//util.ASSERT(depth >= 0 && depth < 100)

	index := uint64(hash.Index(key)) & t.mask
	lock := hash.Lock(key)

	for i := uint64(0); i < 4; i++ {

		idx := (index + i) & t.mask
		//util.ASSERT(idx < t.p_size)
		entry := &t.entries[idx]

		if entry.lock == lock { // there is a matching position already here
			if int(entry.date) != t.generation { // from another generation?
				entry.date = uint8(t.generation) // touch entry
				t.cntUsed++
			}
			*mv = int(entry.move)
			*sc = AddMatePly(int(entry.score), ply)
			*flags = int(entry.scoreType)

			if int(entry.depth) >= depth {
				return true
			}

			if IsMateScore(*sc) {
				*flags &= ^scoreTypeUpper
				if *sc < 0 {
					*flags &= ^scoreTypeLower
				}
				//flags &= ~(score < 0 ? FLAGS_LOWER : FLAGS_UPPER);
				return true
			}

			return false
		}
	}

	return false
}

// Used returns how much of the Hash Table that is used where 500 means 50%
func (t *transTable) Used() int {
	return int((t.cntUsed*1000 + t.size/2) / t.size)
}

/////////////// END CLASS Table

func clearEntry(entry *entry) {
	//Obs entry skall vara 16 bytes
	entry.lock = 0
	entry.move = move.None
	// entry.utfyllnad = 0  behövs inte
	entry.score = 0
	entry.date = 0
	entry.depth = -1
	entry.scoreType = 0
	//entry.tomt = 0   behövs inte
}

// RemMatePly removes ply from the score value (score - ply) if mate
// in order to mix up different depths
func RemMatePly(sc, ply int) int {
	if sc < EvalMin {
		return sc - ply
	} else if sc > EvalMAX {
		return sc + ply
	} else {
		return sc
	}
}

// AddMatePly adjusts mate value with ply if mate score
func AddMatePly(sc, ply int) int {
	if sc < EvalMin {
		return sc + ply
	} else if sc > EvalMAX {
		return sc - ply
	}
	return sc
}

// IsMateScore returns true if the score is a mate score
func IsMateScore(sc int) bool {
	return sc < EvalMin || sc > EvalMAX
}
