package piece

import (
	"GoAlaric/util"
	"strings"
)

// number of pieces + 1 (with color and without color)
const (
	SideSize int = 12
	Size     int = 7
)

// Piece types (no color)
const (
	Pawn int = iota
	Knight
	Bishop
	Rook
	Queen
	King
	None
)

// Internal representation of all pieces. white even and black uneven
const (
	WhitePawn int = iota
	BlackPawn
	WhiteKnight
	BlackKnight
	WhiteBishop
	BlackBishop
	WhiteRook
	BlackRook
	WhiteQueen
	BlackQueen
	WhiteKing
	BlackKing
)

// the evaluation values of piece types
const (
	PawnValue   int = 100
	KnightValue int = 325
	BishopValue int = 325
	RookValue   int = 500
	QueenValue  int = 975
	KingValue   int = 10000 // for SEE
)

// Value is the evaluation value of a piece type
var Value = [...]int{
	PawnValue,
	KnightValue,
	BishopValue,
	RookValue,
	QueenValue,
	KingValue,
	0,
}

// piece representations
const (
	Char    = "PNBRQK?"
	fenChar = "PpNnBbRrQqKk"
)

// FromChar returns the internal form of a string piece
func FromChar(c string) int {
	return strings.Index(Char, c)
}

// ToString returns the string  of a piece type
func ToString(pc int) string {
	//util.ASSERT(pc < SIZE);
	return Char[pc : pc+1]
}

// ToFen converts a p12 to a string
func ToFen(p12 int) string {
	util.ASSERT(p12 < SideSize)
	return fenChar[p12 : p12+1]
}

// FromFen converts a piece as string to internal form
func FromFen(c string) int {
	return strings.Index(fenChar, c)
}

// Piece returns the pc from p12
func Piece(p12 int) int {
	// util.ASSERT(p12 < SIDE_SIZE);
	return p12 >> 1
}

// Color returns the color of a p12 form
func Color(p12 int) int {
	// util.ASSERT(p12 < SIDE_SIZE);
	return p12 & 0x1
}

// MakeP12 returns the p12 from pc and sd
func MakeP12(pc, sd int) int {
	// util.ASSERT(pc < SIZE);
	// util.ASSERT(pc != None);
	// util.ASSERT(sd < side::SIZE);
	return (pc << 1) | sd
}

//func Is_minor(pc int) bool {
//	//util.ASSERT(pc < SIZE)
//	return pc == Knight || pc == Bishop
//}

//func Is_major(pc int) bool {
//	//util.ASSERT(pc < SIZE)
//	return pc == Rook || pc == Queen
//}

//func IsSlider(pc int) bool {
//	// // util.ASSERT(pc < SIZE);
//	return pc >= Bishop && pc <= Queen
//}

//func Valuation(pc int) int {
//	// util.ASSERT(pc < SIZE);
//	return Value[pc]
//}

//func Score(pc int) int { // for MVV/LVA
//	// util.ASSERT(pc < SIZE);
//	// util.ASSERT(pc != None);
//	return pc
//}

/*
   bool is_slider(int pc) {
      // util.ASSERT(pc < SIZE);
      return pc >= BISHOP && pc <= Queen;
   }





   int from_char(char c) {
      return util::string_find(Char, c);
   }

   char to_char(int pc) {
      // util.ASSERT(pc < SIZE);
      return Char[pc];
   }

*/
