package eval

//namespace pawn { // HACK: early declaration for pawn-pushes move type

import (
	"fmt"

	//	"GoAlaric/attack" //går ej!
	"GoAlaric/bit"
	"GoAlaric/board"
	"GoAlaric/hash"
	// "GoAlaric/eval" //går ej
	"GoAlaric/material"
	"GoAlaric/move"
	"GoAlaric/piece"
	//"GoAlaric/pst"
	// "GoAlaric/pst"  // går ej
	"GoAlaric/square"
)

//
const (
	SIDE = 2 // two sides
)
const (
	pawnBITS = 12
	pawnSIZE = 1 << pawnBITS
	pawnMASK = pawnSIZE - 1
)

var shelterFile = [2]int{square.FileG, square.FileB} // for pawn-shelter eval
var passedMe [square.BoardSize][2]bit.BB
var passedOpp [square.BoardSize][2]bit.BB
var pair [square.BoardSize]bit.BB

// ShelterFile returns 1 if a pawn is on file=fl on rank 1
//             returns 2 if a pawn is on file=fl on rank 2
//             return 0 in alla other cases
func ShelterFile(fl, sd int, bd *board.Board) int {

	//util.ASSERT(fl >= 0 && fl < 8)

	if bd.SquareIs(square.MakeSd(fl, square.Rank2, sd), piece.Pawn, sd) {
		return 2
	}
	if bd.SquareIs(square.MakeSd(fl, square.Rank3, sd), piece.Pawn, sd) {
		return 1
	}

	return 0 //default
}

func shelterFiles(fl, sd int, bd *board.Board) int {

	left := fl - 1
	if fl == square.FileA {
		left = fl + 1
	}
	right := fl + 1
	if fl == square.FileH {
		right = fl - 1
	}

	sc := ShelterFile(fl, sd, bd)*2 + ShelterFile(left, sd, bd) + ShelterFile(right, sd, bd)
	//util.ASSERT(sc >= 0 && sc <= 8)

	return sc
}
func isEmpty(sq int, bd *board.Board) bool {
	return bd.Square(sq) != piece.Pawn
}
func isControlled(sq, sd int, bd *board.Board) bool {

	attackers := bd.PieceSd(piece.Pawn, sd) & pawnAttacksTo(sd, sq)
	defenders := bd.PieceSd(piece.Pawn, board.Opposit(sd)) & pawnAttacksTo(board.Opposit(sd), sq)

	return bit.CountLoop(attackers) > bit.CountLoop(defenders)
}

// PawnInit is done ince per session to set bits used to detect passed and isolated pawns
func PawnInit() {
	fmt.Println("info string PawnInit startar")
	for sq := 0; sq < square.BoardSize; sq++ {

		fl := square.File(sq)
		rk := square.Rank(sq)

		passedMe[sq][WHITE] = bit.File(fl) & bit.Front(rk)
		passedMe[sq][BLACK] = bit.File(fl) & bit.Rear(rk)
		passedOpp[sq][WHITE] = bit.AdjFiles(fl) & bit.Front(rk)
		passedOpp[sq][BLACK] = bit.AdjFiles(fl) & bit.Rear(rk)

		b := bit.BB(0)
		if fl != square.FileA {
			bit.Set(&b, sq+square.IncLeft)
		}
		if fl != square.FileH {
			bit.Set(&b, sq+square.IncRight)
		}
		pair[sq] = b
	}
}

type pawnEntry struct { // 80 bytes; TODO: merge some bitboards and/or file info?
	Open      [square.FileSize][SIDE]int8
	Shelter   [square.FileSize][SIDE]uint8
	target    [SIDE]bit.BB
	Passed    bit.BB
	safe      bit.BB
	Lock      uint32
	mg        int16
	eg        int16
	leftFile  int8
	rightFile int8
}

func (pawnHash *PawnHash) getEntry(bd *board.Board) *pawnEntry {

	key := bd.PawnKey()

	index := hash.Index(key) & pawnMASK
	lock := uint32(hash.Lock(key))

	entry := &pawnHash.entries[index]

	if entry.Lock != lock {
		entry.Lock = lock
		compPawnHash(entry, bd)
	}

	return entry
}

////// flyttat från  pawn ///////////
func isPassed(sq, sd int, bd *board.Board) bool {
	return (bd.PieceSd(piece.Pawn, board.Opposit(sd))&passedOpp[sq][sd]) == 0 &&
		(bd.PieceSd(piece.Pawn, sd)&passedMe[sq][sd]) == 0
}

func passerUnstoppable(sq, sd int, bd *board.Board) bool {

	if !board.LoneKing(board.Opposit(sd), bd) {
		return false
	}

	fl := square.File(sq)
	front := bit.File(fl) & bit.FrontSd(sq, sd)

	if bd.All()&front != 0 { // path not free
		return false
	}

	if squareDistance(bd.King(board.Opposit(sd)), sq, sd) >= 2 { // opponent king outside square
		return true
	}

	if (front & ^PseudoAttacksFrom(piece.King, sd, bd.King(sd))) == 0 { // king controls promotion path
		return true
	}

	return false
}
func squareDistance(ks, ps, sd int) int {
	prom := square.Promotion(ps, sd)
	return square.Distance(ks, prom) - square.Distance(ps, prom)
}

// IsEnPassant returns true if mv is en passant
func IsEnPassant(mv int, bd *board.Board) bool {
	return move.Piece(mv) == piece.Pawn && move.To(mv) == bd.EpSq()
}

// IsPawnPush return true if it is a pawn push
func IsPawnPush(mv int, bd *board.Board) bool {

	if move.IsTactical(mv) {
		return false
	}

	fr := move.From(mv)
	to := move.To(mv)

	pc := bd.Square(fr)
	sd := bd.SquareSide(fr)

	return pc == piece.Pawn && square.RankSd(to, sd) >= square.Rank6 && isPassed(to, sd, bd) && !move.IsCapture(mv)
}
func possibleAttacks(sq, sd int, bd *board.Board) bit.BB {

	inc := square.PawnInc(sd)

	attacks := pawnAttacksFrom(sd, sq)

	for sq += inc; !square.IsPromotion(sq) && isSafe(sq, sd, bd); sq += inc {
		attacks |= pawnAttacksFrom(sd, sq)
	}

	return attacks
}

// clearEntry clears one entry in pawnHash
func clearEntry(entry *pawnEntry) {

	entry.Passed = bit.BB(0)
	entry.safe = bit.BB(0)
	entry.Lock = uint32(1) // board w/o pawns has key 0!
	entry.mg = int16(0)
	entry.eg = int16(0)
	entry.leftFile = int8(square.FileA)
	entry.rightFile = int8(square.FileH)
	entry.target[BLACK] = 0
	entry.target[WHITE] = 0

	for fl := 0; fl < square.FileSize; fl++ {
		for sd := 0; sd < SIDE; sd++ {
			entry.Open[fl][sd] = int8(0)
			entry.Shelter[fl][sd] = uint8(0)
		}
	}
}

func compPawnHash(entry *pawnEntry, bd *board.Board) {

	entry.Passed = 0
	entry.safe = 0

	entry.mg = 0
	entry.eg = 0

	entry.leftFile = int8(square.FileH + 1)
	entry.rightFile = int8(square.FileA - 1)

	entry.target[BLACK] = 0
	entry.target[WHITE] = 0

	var weak bit.BB
	var strong bit.BB
	full := ^uint64(0)
	safe := [2]bit.BB{bit.BB(full), bit.BB(full)}

	for sd := 0; sd < 2; sd++ {

		p12 := piece.MakeP12(piece.Pawn, sd)

		strong |= bd.PieceSd(piece.Pawn, sd) & PawnAttacksFrom(sd, bd) // defended pawns

		{
			n := bd.Count(piece.Pawn, sd)
			entry.mg += int16(n * material.Score(piece.Pawn, MG))
			entry.eg += int16(n * material.Score(piece.Pawn, EG))
		}

		for b := bd.PieceSd(piece.Pawn, sd); b != 0; b = bit.Rest(b) {

			sq := bit.First(b)

			fl := int8(square.File(sq))
			rk := int8(square.RankSd(sq, sd))

			if fl < entry.leftFile {
				entry.leftFile = fl
			}
			if fl > entry.rightFile {
				entry.rightFile = fl
			}

			entry.mg += int16(Score(p12, sq, MG))
			entry.eg += int16(Score(p12, sq, EG))

			if isIsolated(sq, sd, bd) {

				bit.Set(&weak, sq)

				entry.mg -= 10
				entry.eg -= 20

			} else if isWeak(sq, sd, bd) {

				bit.Set(&weak, sq)

				entry.mg -= 5
				entry.eg -= 10
			}

			if isDoubled(sq, sd, bd) {
				entry.mg -= 5
				entry.eg -= 10
			}

			if isPassed(sq, sd, bd) {

				bit.Set(&entry.Passed, sq)

				entry.mg += 10
				entry.eg += 20

				if rk >= int8(square.Rank5) {
					stop := square.Stop(sq, sd)
					if isPawnPair(sq, sd, bd) && rk <= int8(square.Rank6) {
						stop += square.PawnInc(sd)
					} // stop one line "later" for duos
					bit.Set(&entry.target[board.Opposit(sd)], stop)
				}
			}

			safe[board.Opposit(sd)] &= ^possibleAttacks(sq, sd, bd)
		}

		for fl := 0; fl < square.FileSize; fl++ {
			entry.Shelter[fl][sd] = uint8(shelterFiles(fl, sd, bd) * 4)
		}

		entry.mg = -entry.mg
		entry.eg = -entry.eg
	}

	weak &= ^strong // defended doubled pawns are not weak
	//util.ASSERT((weak & strong) == 0)

	entry.target[WHITE] |= bd.PieceSd(piece.Pawn, BLACK) & weak
	entry.target[BLACK] |= bd.PieceSd(piece.Pawn, WHITE) & weak

	entry.safe = (safe[WHITE] & bit.Front(square.Rank4)) |
		(safe[BLACK] & bit.Rear(square.Rank5))

	if entry.leftFile > entry.rightFile { // no pawns
		entry.leftFile = int8(square.FileA)
		entry.rightFile = int8(square.FileH)
	}

	//util.ASSERT(info.left_file <= info.right_file)

	// file "openness"

	for sd := 0; sd < 2; sd++ {

		for fl := 0; fl < square.FileSize; fl++ {

			file := bit.File(fl)

			open := 0

			// if false {
			//} else if ((bd.piece(piece::PAWN, sd) & file) != 0) {
			//   open = 0;
			if (bd.PieceSd(piece.Pawn, board.Opposit(sd)) & file) == 0 {
				open = 4
			} else if (strong & file) != 0 {
				open = 1
			} else if (weak & file) != 0 {
				open = 3
			} else {
				open = 2
			}

			entry.Open[fl][sd] = int8(open * 5)
		}
	}
}

// PawnHash is the struct the pawn hash table
type PawnHash struct {
	entries [pawnSIZE]pawnEntry
}

// Clear all pawnhash entries
func (pawnHash *PawnHash) Clear() {

	var info pawnEntry
	clearEntry(&info)

	for index := 0; index < pawnSIZE; index++ {
		//pawnHash.entries[index] = info
		pawnHash.entries[index].Lock = 1 // the fast version... board w/o pawns has key 0!
	}
}

func isIsolated(sq, sd int, bd *board.Board) bool {

	fl := square.File(sq)
	files := bit.AdjFiles(fl) & ^bit.File(fl)

	return (bd.PieceSd(piece.Pawn, sd) & files) == 0
}

func isPawnPair(sq, sd int, bd *board.Board) bool {
	return (bd.PieceSd(piece.Pawn, sd) & pair[sq]) != 0
}

func isSafe(sq, sd int, bd *board.Board) bool {
	return isEmpty(sq, bd) && !isControlled(sq, board.Opposit(sd), bd)
}

func isWeak(sq, sd int, bd *board.Board) bool {

	fl := square.File(sq)
	rk := square.RankSd(sq, sd)

	pawns := bd.PieceSd(piece.Pawn, sd)
	inc := square.PawnInc(sd)

	// already fine?

	if (pawns & pair[sq]) != 0 {
		return false
	}

	if IsAttacked(sq, sd, bd) {
		return false
	}

	// can advance next to other pawn in one move?

	s1 := sq + inc
	s2 := s1 + inc

	if (pawns&pair[s1]) != 0 && isSafe(s1, sd, bd) {
		return false
	}

	if rk == square.Rank2 && (pawns&pair[s2]) != 0 && isSafe(s1, sd, bd) && isSafe(s2, sd, bd) {
		return false
	}

	// can be defended in one move?

	if fl != square.FileA {

		s0 := sq + square.IncLeft
		s1 := s0 - inc
		s2 := s1 - inc
		s3 := s2 - inc

		//util.ASSERT(sq >= square.A2 && sq < 64)
		//util.ASSERT(sd == 0 || sd == 1)

		if rk == square.Rank5 && bd.SquareIs(s3, piece.Pawn, sd) && isSafe(s2, sd, bd) && isSafe(s1, sd, bd) {
			return false
		}
	}

	if fl != square.FileH {

		s0 := sq + square.IncRight
		s1 := s0 - inc
		s2 := s1 - inc
		s3 := s2 - inc

		if rk >= square.Rank4 && bd.SquareIs(s2, piece.Pawn, sd) && isSafe(s1, sd, bd) {
			return false
		}

		if rk == square.Rank5 && bd.SquareIs(s3, piece.Pawn, sd) && isSafe(s2, sd, bd) && isSafe(s1, sd, bd) {
			return false
		}
	}

	return true
}

func isDoubled(sq, sd int, bd *board.Board) bool {
	fl := square.File(sq)
	return (bd.PieceSd(piece.Pawn, sd) & bit.File(fl) & bit.RearSd(sq, sd)) != 0
}
