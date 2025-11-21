package board

import (
	"fmt"

	"goalaric/hash"
	"goalaric/square"
)

type info struct {
	KingFr,
	KingTo,
	RookFr,
	RokTo int
}

// CastleInfo is the castling Info
var CastleInfo = [...]info{{square.E1, square.G1, square.H1, square.F1},
	{square.E1, square.C1, square.A1, square.D1},
	{square.E8, square.G8, square.H8, square.F8},
	{square.E8, square.C8, square.A8, square.D8},
}

// flags for castling
var (
	castleMask [square.BoardSize]int
	castleKey  [1 << 4]hash.Key
)

// CastleIndex of the castling
// sd: side to move, wg: castle side - short or long (that is king side or queen side)
func CastleIndex(sd, wg int) int {
	return sd*2 + wg
}

// castleSide returns which side(s) can be castled to
func castleSide(index int) int {
	return index / 2
}

// CastleFlag returns one of the castling flags
func CastleFlag(flags int, index uint) bool {
	//assert(index < 4);
	return ((flags >> index) & 1) != 0
}

// setFlag sets one of the castling flags
func setFlag(flags *int, index uint) {
	//assert(index < 4);
	*flags |= (1 << index)
}

// Init the castling flags
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
func computeFlagsKey(flags int) hash.Key {

	key := hash.Key(0)

	for index := uint(0); index < 4; index++ {
		if CastleFlag(flags, index) {
			key ^= hash.FlagKey(index)
		}
	}

	return key
}
