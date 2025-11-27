// Castling utilities: precomputed square mappings, masks, and Zobrist keys.
package board

import (
	"fmt"

	"goalaric/hash"
	"goalaric/square"
)

// info holds the from/to squares for king and rook in a castling move.
type info struct {
	KingFr,
	KingTo,
	RookFr,
	RokTo int
}

// CastleInfo describes the square mappings for each of the four castling options.
var CastleInfo = [...]info{
	{square.E1, square.G1, square.H1, square.F1},
	{square.E1, square.C1, square.A1, square.D1},
	{square.E8, square.G8, square.H8, square.F8},
	{square.E8, square.C8, square.A8, square.D8},
}

// Castling helpers: mask tracks which moves reset flags; castleKey holds Zobrist keys per flag set.
var (
	castleMask [square.BoardSize]int
	castleKey  [1 << 4]hash.Key
)

// CastleIndex returns the index in CastleInfo for side sd and wing wg (king/queen side).
func CastleIndex(sd, wg int) int {
	return sd*2 + wg
}

// castleSide returns which side(s) can be castled to for a given CastleInfo index.
func castleSide(index int) int {
	return index / 2
}

// CastleFlag reports whether a specific castling right (0..3) is set in flags.
func CastleFlag(flags int, index uint) bool {
	//assert(index < 4);
	return ((flags >> index) & 1) != 0
}

// setFlag sets a specific castling right bit in flags.
func setFlag(flags *int, index uint) {
	//assert(index < 4);
	*flags |= (1 << index)
}

// init precomputes castling masks and Zobrist keys used by Board hashing.
func init() {
	fmt.Println("info string Castling init startar")
	for sq := 0; sq < square.BoardSize; sq++ {
		castleMask[sq] = 0
	}

	setFlag(&castleMask[square.E1], 0)
	setFlag(&castleMask[square.E1], 1)
	setFlag(&castleMask[square.H1], 0)
	setFlag(&castleMask[square.A1], 1)

	setFlag(&castleMask[square.E8], 2)
	setFlag(&castleMask[square.E8], 3)
	setFlag(&castleMask[square.H8], 2)
	setFlag(&castleMask[square.A8], 3)

	for flags := 0; flags < (1 << 4); flags++ {
		castleKey[flags] = computeFlagsKey(flags)
	}
}

// computeFlagsKey builds the Zobrist key for the current castling flags.
func computeFlagsKey(flags int) hash.Key {

	key := hash.Key(0)

	for index := uint(0); index < 4; index++ {
		if CastleFlag(flags, index) {
			key ^= hash.FlagKey(index)
		}
	}

	return key
}
