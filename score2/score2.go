package score

// score types
const (
	None    = -10000
	Min     = -9999
	evalMin = -8999
	EvalMAX = +8999
	Max     = +9999
	Mate    = +10000
)

// Castling flags
const (
	FlagsNone  = 0
	FlagsLower = 1 << 0
	FlagsUpper = 1 << 1
	FlagsExact = FlagsLower | FlagsUpper
)

// IsMateScore returns true if the score is a mate score
func IsMateScore(sc int) bool {
	return sc < evalMin || sc > EvalMAX
}

// MateWithSign put +/- to a mate score
func MateWithSign(sc int) int {
	if sc < evalMin { // -MATE
		return -(Mate + sc) / 2
	} else if sc > EvalMAX { // +MATE
		return (Mate - sc + 1) / 2
	}
	// assert(false);
	return 0
}

//func Side_score(sc, sd int) int {
//	if sd == board.WHITE {
//		return +sc
//	}
//	return -sc
//}

// HashScore puts back ply to the score value (mate)
func HashScore(sc, ply int) int {
	if sc < evalMin {
		return sc + ply
	} else if sc > EvalMAX {
		return sc - ply
	}
	return sc
}

// ToHashScore removes ply from the score value (mate)
func ToHashScore(sc, ply int) int {
	if sc < evalMin {
		return sc - ply
	} else if sc > EvalMAX {
		return sc + ply
	} else {
		return sc
	}
}

// Flags sets if it is an upper or lower score
func Flags(sc, alpha, beta int) int {

	flags := FlagsNone
	if sc > alpha {
		flags |= FlagsLower
	}
	if sc < beta {
		flags |= FlagsUpper
	}

	return flags
}
