package move

import (

	// "GoAlaric/board"  går ej!!

	"GoAlaric/material"
	"GoAlaric/square"
	"strings"
)

// flags
const (
	flagsBits int = 9
	flagsSize int = 1 << uint(flagsBits)
	flagsMask int = flagsSize - 1
)

// hash
const (
	Bits int = flagsBits + 12
	size int = 1 << uint(Bits)
	Mask int = size - 1
)

// score
const (
	scoreBits int = 32 - Bits
	scoreSize int = 1 << uint(scoreBits)
	ScoreMask int = scoreSize - 1
)

//
const (
	None = 0
	Null = 1
)

// Build builds up a move from its different parts
func Build(fr int, to int, pc int, cp int, pp int) int {

	// assert(f < square::SIZE);
	// assert(t < square::SIZE);
	// assert(pc < piece::SIZE);
	// assert(cp < piece::SIZE);
	// assert(pp < piece::SIZE);
	// assert(pc != piece::None);
	// assert(pp == piece::None || pc == piece::PAWN);

	//temp := (pc << 18) | (cp << 15) | (pp << 12) | (f << 6) | t
	//fmt.Printf("f: %v t: %v pc: %v cp: %v pp: %v mv_fr: %v  mv_to: %v\n", f, t, pc, cp, pp, From(temp), To(temp))

	return (pc << 18) | (cp << 15) | (pp << 12) | (fr << 6) | to
}

// From extracts the from-sq from a move
func From(mv int) int {
	// assert(mv != None);
	// assert(mv != NULL_);
	return (mv >> 6) & 077
}

// To extracts the to-sq from a move
func To(mv int) int {
	// assert(mv != None);
	// assert(mv != NULL_);
	return mv & 077
}

// Piece extracts the fr-piece from a move
func Piece(mv int) int {
	// assert(mv != None);
	// assert(mv != NULL_);
	return (mv >> 18) & 7
}

// Capt extracts the captured piece from a move
func Capt(mv int) int {
	// assert(mv != None);
	// assert(mv != NULL_);
	return (mv >> 15) & 7
}

// Prom extracts the prom-piece from a move
func Prom(mv int) int {
	// assert(mv != None);
	//assert(mv != NULL_);
	return (mv >> 12) & 7
}

// ToString converts a move from internal coding to a string
func ToString(mv int) string {

	// assert(mv != None);

	if mv == None {
		return "0000"
	}
	if mv == Null {
		return "0000"
	}

	s := ""

	s += square.ToString(From(mv))
	s += square.ToString(To(mv))

	if Prom(mv) != material.None {
		s += strings.ToLower(material.ToString(Prom(mv)))
	}

	return s
}

// IsTactical returns true if the moves is a capture or a promotion
func IsTactical(mv int) bool {
	return IsCapture(mv) || IsPromotion(mv)
}

// IsCapture returns true if the move is a capture
func IsCapture(mv int) bool {
	return Capt(mv) != material.None
}

// IsCastling returns true if the move is a castling move
func IsCastling(mv int) bool {
	return Piece(mv) == material.King && Iabs(To(mv)-From(mv)) == square.CastlingDelta
}

// IsPromotion returns true if the move is a promotion
func IsPromotion(mv int) bool {
	return Prom(mv) != material.None
}

// CaptMax returns the maximum value that can be earned from a capture
func CaptMax(mv int) int {
	sc := material.Value[Capt(mv)]

	pp := Prom(mv)
	if pp != material.None {
		sc += material.Value[pp] - material.PawnValue
	}
	return sc
}

// Iabs returns the absolute value of an int
func Iabs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}
