package square

import (
	"math"
	"strconv"
)

const (
	FileSize  int = 8
	rankSize  int = 8
	BoardSize     = FileSize * rankSize
)

// Wite is zero, Black is 1
const (
	WHITE int = iota
	BLACK
)

// All Files
const (
	FileA int = iota
	FileB
	FileC
	FileD
	FileE
	FileF
	FileG
	FileH
)

// All ranks
const (
	rank1 int = iota
	Rank2
	Rank3
	Rank4
	Rank5
	Rank6
	Rank7
	rank8
)

// All squares
const (
	None int = -1 + iota
	A1
	A2
	A3
	A4
	A5
	A6
	A7
	A8
	B1
	B2
	B3
	B4
	B5
	B6
	B7
	B8
	C1
	C2
	C3
	C4
	C5
	C6
	C7
	C8
	D1
	D2
	D3
	D4
	D5
	D6
	D7
	D8
	E1
	E2
	E3
	E4
	E5
	E6
	E7
	E8
	F1
	F2
	F3
	F4
	F5
	F6
	F7
	F8
	G1
	G2
	G3
	G4
	G5
	G6
	G7
	G8
	H1
	H2
	H3
	H4
	H5
	H6
	H7
	H8
)

var strFiles = "abcdefgh"

// what to add/subtract for left and right incrementations
const (
	IncLeft  int = -8
	IncRight int = +8
)

// The distance between K and R in castling and between double pawns
const (
	CastlingDelta   int = 16
	DoublePawnDelta int = 2
)

// FromFen converts a fen square number to internal form
func FromFen(sq int) int {
	return Make(sq&7, (sq>>3)^7)
}

// Make builds an internal square
func Make(fl, rk int) int {

	//assert(fl < 8);
	//assert(rk < 8);

	return (fl << 3) | rk
}

// MakeSd builds internal square from file, rank, and color
func MakeSd(fl, rk, col int) int {
	return Make(fl, (rk^-col)&7)
}

// SameColor returns true if sq1 and sq2 has the same color
func SameColor(sq1, sq2 int) bool {
	diff := sq1 ^ sq2
	return (((diff >> 3) ^ diff) & 1) == 0
}

// Rank is absolute rank (from white side)
func Rank(sq int) int { //se RankSd(...)  namnkonflikt gjorde mig tvingad
	return sq & 7
}

// RankSd is rank depending on side of the board
func RankSd(sq, sd int) int { //renamed från rank
	return (sq ^ -sd) & 7
}

// File returns the file number of a square
func File(sq int) int {
	return sq >> 3
}

func fileDistance(s0, s1 int) int {
	return int(math.Abs(float64(File(s1) - File(s0))))
}

func rankDistance(s0, s1 int) int {
	return int(math.Abs(float64(Rank(s1) - Rank(s0))))
}

// Distance between two squares
func Distance(sq1, sq2 int) int {
	return int(math.Max(float64(fileDistance(sq1, sq2)), float64(rankDistance(sq1, sq2))))
}

//func Opposit_file(sq int) int {
//	return sq ^ 070
//}

// OppositRank is the mirror of a rank
func OppositRank(sq int) int {
	return sq ^ 007
}

// Promotion builds a promotion move
func Promotion(sq, sd int) int {
	return MakeSd(File(sq), rank8, sd)
}

// PawnInc returns the increment from a pawn with WHITE or BLACK color
func PawnInc(col int) int {
	if col == WHITE {
		return +1
	}
	return -1
}

// Stop returns the square one pawnmove from sq
func Stop(sq, sd int) int {
	return sq + PawnInc(sd)
}

// FromString converts a string square to internal format
func FromString(s string) int {
	return Make(int(s[0]-'a'), int(s[1]-'1'))
}

// IsPromotion checks if a square is promotion square (rank 1 or 8)
func IsPromotion(sq int) bool {
	rk := Rank(sq)
	return rk == rank1 || rk == rank8
}

// IsValid88 checks if a 88-format square is valid
func IsValid88(s88 int) bool {
	return (s88 & (^0x77)) == 0
}

// To88 convers internal dquare to 88-format
func To88(sq int) int {
	return sq + (sq & 070)
}

// From88 converts a 88-format square to internal form
func From88(s88 int) int {
	// assert(is_valid_88(s88))
	return (s88 + (s88 & 7)) >> 1
}

// ToString converts internal square to a string
func ToString(sq int) string {
	f := File(sq)
	s := ""
	//fmt.Printf("sq: %v fileNr: %v  rankNr %v\n", sq, f, Rank(sq))
	s += strFiles[f : f+1]
	s += strconv.Itoa(Rank(sq) + 1)

	return s
}
