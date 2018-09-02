package bit

import (
	"GoAlaric/square"
	"fmt"
)

///////////////////////////
///// bit twiddling ///////
///////////////////////////
var index = [64]int{
	0, 1, 2, 7, 3, 13, 8, 19,
	4, 25, 14, 28, 9, 34, 20, 40,
	5, 17, 26, 38, 15, 46, 29, 48,
	10, 31, 35, 54, 21, 50, 41, 57,
	63, 6, 12, 18, 24, 27, 33, 39,
	16, 37, 45, 47, 30, 53, 49, 56,
	62, 11, 23, 32, 36, 44, 52, 55,
	61, 22, 43, 51, 60, 42, 59, 58,
}

//
const (
	WHITE int = iota
	BLACK
)

var leftBB [8]BB
var rightBB [8]BB
var frontBB [8]BB
var rearBB [8]BB

var frontSd [2][8]BB
var rearSd [2][8]BB

// BB is the biboard type. 64 bits representing some fact of the board
type BB uint64

// Set bit n to 1
func Set(bb *BB, n int) {
	*bb |= Bit(n)
}

// Bit returns a bitboard with bit n set to 1
func Bit(n int) BB {
	return BB(1) << uint(n)
}

// Single is true if there is only one and only one bit set to 1
// Do not use for bitboards == 0
func Single(b BB) bool {
	return Rest(b) == 0
}

// Clear bit n in biboard bb
func Clear(bb *BB, n int) {
	*bb &= ^Bit(n)
}

// Count counts number of bits set to 1. Use this when many bits are expected
func Count(b BB) int {
	b = b - ((b >> 1) & BB(0x5555555555555555))
	b = (b & BB(0x3333333333333333)) + ((b >> 2) & BB(0x3333333333333333))
	b = (b + (b >> 4)) & BB(0x0F0F0F0F0F0F0F0F)
	return int((b * BB(0x0101010101010101)) >> 56)

	/*
	   return __builtin_popcountll(b); // GCC
	*/
}

// CountLoop counts number of bits set to 1. Use this when just a few bits are expected
func CountLoop(b BB) int {
	n := 0

	for ; b != 0; b = Rest(b) {
		n++
	}

	return n

	/*
	   return __builtin_popcountll(b); // GCC
	*/
}

// File returns a bitboard with all sq on file fl set to 1
func File(fl int) BB {
	return BB(uint64(0xFF) << uint(fl*8))
}

// AdjFiles returns a bitboard the given file and its close neigbors set to 1
func AdjFiles(fl int) BB {
	file := File(fl)
	return (file << 8) | file | (file >> 8)
}

// Rank returns a bitboard with all bits on row rk is set to 1
func Rank(rk int) BB {
	return BB(uint64(0x0101010101010101) << uint(rk))
}

// Front returns a bitboard with all rows in front of the given row, set to 1
func Front(row int) BB {
	return frontBB[row]
}

// Rear returns a bitboard with all rows behind the given row, set to 1
func Rear(row int) BB {
	return rearBB[row]
}

// FrontSd returns a bitboard with all bits set to 1 for rows in fornt of the sq row.
// Depending on side WHITE/BLACK
func FrontSd(sq, sd int) BB {
	rk := square.Rank(sq)
	return frontSd[sd][rk]
}

// RearSd returns a bitboard with all bits set to 1 for rows behind the sq row.
// Depending on side WHITE/BLACK
func RearSd(sq, sd int) BB { //name conflict rear
	rk := square.Rank(sq)
	return rearSd[sd][rk]
}

// Rest returns a new bitboard with the first bitpos set to 1 removed
func Rest(b BB) BB {
	return b & (b - 1)
}

// First returns the first bitpos set to 1 in a bitboard
func First(b BB) int {
	return index[(uint64(b&-b)*uint64(0x218A392CD3D5DBF))>>(64-6)]

	//return __builtin_ctzll(b); // in GCC but I can't find it in my env
}

// init set bits for what is left,right,in front of and behind a square
// This should be run before other inits for pawns and eval
func init() {
	fmt.Println("info string Bit init startar")
	bf := BB(0)
	br := BB(0)
	i := 0 // Note: will be reused
	for ; i < 8; i++ {
		leftBB[i] = bf
		rearBB[i] = br
		bf |= File(i)
		br |= Rank(i)
	}

	bf = 0 // reuse variable
	br = 0 // reuse variable

	for i = 7; i >= 0; i-- { // NOTE: reuse i
		rightBB[i] = bf
		frontBB[i] = br
		bf |= File(i)
		br |= Rank(i)
	}

	for i = 0; i < 8; i++ { // NOTE: Reuse i
		frontSd[WHITE][i] = Front(i)
		frontSd[BLACK][i] = Rear(i)
		rearSd[WHITE][i] = Rear(i)
		rearSd[BLACK][i] = Front(i)
	}
}

//IsOne checks if bit n in a bitboard is set to 1
func IsOne(b BB, n int) bool {
	return (b & Bit(n)) != 0
}

/*
   static const int index[64] = {
       0,  1,  2,  7,  3, 13,  8, 19,
       4, 25, 14, 28,  9, 34, 20, 40,
       5, 17, 26, 38, 15, 46, 29, 48,
      10, 31, 35, 54, 21, 50, 41, 57,
      63,  6, 12, 18, 24, 27, 33, 39,
      16, 37, 45, 47, 30, 53, 49, 56,
      62, 11, 23, 32, 36, 44, 52, 55,
      61, 22, 43, 51, 60, 42, 59, 58,
   };

   return index[((b & -b) * U64(0x218A392CD3D5DBF)) >> (64 - 6)];
*/