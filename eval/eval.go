package eval

import (
	"GoAlaric/material"
	"fmt"
	"math"
	//"GoAlaric/attack"
	"GoAlaric/bit"
	"GoAlaric/board"
	"GoAlaric/hash"
	//"GoAlaric/pawn"
	//"GoAlaric/move"
	"GoAlaric/castling"
	"GoAlaric/piece"
	//	"GoAlaric/score"
	"GoAlaric/square"
	"GoAlaric/util"
)

//
// WHITE is 0 and means the white color pieces Black is 1
const (
	WHITE int = iota
	BLACK
)

// MG is the game Stage Middlegame
// EG is the game Stage Endgame
var (
	MG = board.MG
	EG = board.EG
)

// BITS is the number of bits needed to address the Eval hash table
// SIZE is the size of the Eval hash Table
// MASK is used to get the Index out of the hashkey for the Eval hash table
const (
	BITS = 16
	SIZE = 1 << BITS
	MASK = SIZE - 1
)
const stageSize int = 2 // number of game stages

var smallCentre, mediumCentre, largeCentre bit.BB
var centre0, centre1 bit.BB
var sideArea [2]bit.BB
var kingArea [2][square.BoardSize]bit.BB
var attackWeight = [piece.Size]int{0, 4, 4, 2, 1, 4, 0}
var attackedWeight = [piece.Size]int{0, 1, 1, 2, 4, 8, 0}

var mobWeight [32]int
var distWeight [8]int // for king-passer distance

// Init inits eval values
func init() {
	PstInit()
	PawnInit()
	fmt.Println("info string eval Init startar")

	smallCentre = 0
	mediumCentre = 0
	largeCentre = 0

	for sq := 0; sq < square.BoardSize; sq++ {

		fl := square.File(sq)
		rk := square.Rank(sq)

		if fl >= square.FileD && fl <= square.FileE && rk >= square.Rank4 && rk <= square.Rank5 {
			bit.Set(&smallCentre, sq)
		}

		if fl >= square.FileC && fl <= square.FileF && rk >= square.Rank3 && rk <= square.Rank6 {
			bit.Set(&mediumCentre, sq)
		}

		if fl >= square.FileB && fl <= square.FileG && rk >= square.Rank2 && rk <= square.Rank7 {
			bit.Set(&largeCentre, sq)
		}
	}

	largeCentre &= ^mediumCentre
	mediumCentre &= ^smallCentre

	centre0 = smallCentre | largeCentre
	centre1 = smallCentre | mediumCentre

	sideArea[WHITE] = 0
	sideArea[BLACK] = 0

	for sq := 0; sq < square.BoardSize; sq++ {
		if square.Rank(sq) <= square.Rank4 {
			bit.Set(&sideArea[WHITE], sq)
		} else {
			bit.Set(&sideArea[BLACK], sq)
		}
	}

	for ks := 0; ks < square.BoardSize; ks++ {

		kingArea[WHITE][ks] = 0
		kingArea[BLACK][ks] = 0

		for as := 0; as < square.BoardSize; as++ {

			df := square.File(as) - square.File(ks)
			dr := square.Rank(as) - square.Rank(ks)

			if util.Iabs(df) <= 1 && dr >= -1 && dr <= +2 {
				bit.Set(&kingArea[WHITE][ks], as)
			}

			if util.Iabs(df) <= 1 && dr >= -2 && dr <= +1 {
				bit.Set(&kingArea[BLACK][ks], as)
			}
		}
	}

	for i := 0; i < 32; i++ {
		x := float64(i) * 0.5
		y := 1.0 - math.Exp(-x)
		mobWeight[i] = int(math.Floor((y*512.0 - 256) + 0.5)) //util.round av positiva integers
	}

	for i := 0; i < 8; i++ {
		x := float64(i) - 3.0
		y := 1.0 / (1.0 + math.Exp(-x))
		distWeight[i] = int(math.Floor((y * 7.0 * 256.0) + 0.5))
	}

	AtkInit()
}

type attackInfo struct {
	pieceAtks    [square.BoardSize]bit.BB
	allAtks      [2]bit.BB
	multipleAtks [2]bit.BB

	gePieces [2][piece.Size]bit.BB

	ltAtks [2][piece.Size]bit.BB
	leAtks [2][piece.Size]bit.BB

	kingEvasions [2]bit.BB

	pinned bit.BB
}

// entry is one entry int EvalHas
type entry struct {
	lock uint32
	eval int
}

// Hash struct for Eval
type Hash struct { // Eval
	// private:

	entries [SIZE]entry

	// public:
}

// Clear sets lock and eval to 0 for all indexes
func (t *Hash) Clear() {
	for index := 0; index < SIZE; index++ {
		t.entries[index].lock = 0
		t.entries[index].eval = 0
	}
}

// Eval is called by evalu8 (search) and if it cannot use eval-hash-table it calls comp_eval.
// it returns score for white
func (t *Hash) Eval(bd *board.Board, pawnTable *PawnHash) int { // NOTE: score for white

	key := bd.EvalKey()

	index := hash.Index(key) & MASK
	lock := uint32(hash.Lock(key))

	entry := t.entries[index]

	if entry.lock == lock {
		return entry.eval
	}

	eval := CompEval(bd, pawnTable)

	entry.lock = lock
	entry.eval = eval

	return eval
}

// Eval is called by evalu8 (in search). It calls table.eval and returns the stm evaluation
//func Eval(bd *board.Board, table *Table, pawn_table *PawnTable) int {
//	return score.Side_score(table.Eval(bd, pawn_table), bd.Turn())
//}

// CompEval makes a complete evaluation for white in current position
func CompEval(bd *board.Board, pawnHash *PawnHash) int { // NOTE: score for white

	var ai attackInfo
	compAttacks(&ai, bd)
	pEntry := pawnHash.getEntry(bd)

	eval := 0
	mg := 0
	eg := 0

	var shelter [2]int

	sd := 0
	for ; sd < 2; sd++ {
		shelter[sd] = shelterScore(bd.King(sd), sd, bd, pEntry)
	}

	for sd = 0; sd < 2; sd++ {

		xd := board.Opposit(sd)

		myKing := bd.King(sd)
		opKing := bd.King(xd)

		target := ^(bd.PieceSd(piece.Pawn, sd) | PawnAttacksFrom(xd, bd))

		kingN := 0
		kingPower := 0

		// pawns

		myPawns := bd.PieceSd(piece.Pawn, sd)
		front := bit.Front(square.Rank3)
		if sd == BLACK {
			front = bit.Rear(square.Rank6)
		}

		for pc := myPawns & pEntry.Passed & front; pc != 0; pc = bit.Rest(pc) {

			sq := bit.First(pc)
			rk := square.RankSd(sq, sd)

			if passerUnstoppable(sq, sd, bd) {

				weight := util.Imax(rk-square.Rank3, 0.0)
				//util.ASSERT(weight >= 0 && weight < 5)

				eg += (piece.QueenValue - piece.PawnValue) * weight / 5
				//fmt.Println("col:", sd, "unstop sq:", sq, "eg:", eg)

			} else {
				sc := evalPassed(sq, sd, bd, &ai)
				//fmt.Println("col:", sd, "Passed sq:", sq, "sc:", sc)
				scMg := sc * 20
				scEg := sc * 25

				stop := square.Stop(sq, sd)
				scEg -= calcDist(myKing, stop, 10)
				scEg += calcDist(opKing, stop, 20)
				//fmt.Println("col:", sd, "Passed sq;", sq, "stop:", stop, "dist myK:", my_distance(myKing, stop, 10), "dist opK:", my_distance(opKing, stop, 20))

				mg += passedScore(scMg, rk)
				eg += passedScore(scEg, rk)
				//fmt.Println("col:", sd, "Passed sq:", sq, "mg:", mg, "eg:", eg)
			}
		}
		//fmt.Println("col:", sd, "P eval1:  ", eval, "mg:", mg, "eg:", eg)

		eval += bit.Count(pawnMovesFrom(sd, bd)&bd.Empty())*4 - bd.Count(piece.Pawn, sd)*2
		//fmt.Println("col:", sd, "P evalCnt:", eval, "mg:", mg, "eg:", eg)

		eval += evalPawnCap(sd, bd, &ai)
		//fmt.Println("col:", sd, "P evalCap:", eval, "mg:", mg, "eg:", eg)
		// --- end pawns ----

		// pieces

		for pc := piece.Knight; pc <= piece.King; pc++ {

			p12 := piece.MakeP12(pc, sd) // for PST

			n := bd.Count(pc, sd)
			mg += n * material.Score(pc, MG)
			eg += n * material.Score(pc, EG)
			//fmt.Println("material col:", sd, "pc:", pc, "n=", n, "mg:", mg, "eg:", eg)

			for b := bd.PieceSd(pc, sd); b != 0; b = bit.Rest(b) {

				sq := bit.First(b)

				fl := square.File(sq)
				rk := square.RankSd(sq, sd)

				// compute safe attacks

				tsAll := ai.pieceAtks[sq]
				tsPawnSafe := tsAll & target

				safe := ^ai.allAtks[xd] | ai.multipleAtks[sd]

				if pc >= piece.Bishop && pc <= piece.Queen { // battery (slider) support

					bishops := bd.PieceSd(piece.Bishop, sd) | bd.PieceSd(piece.Queen, sd)
					rooks := bd.PieceSd(piece.Rook, sd) | bd.PieceSd(piece.Queen, sd)

					support := bishops & PseudoAttacksTo(piece.Bishop, sd, sq)
					support |= rooks & PseudoAttacksTo(piece.Rook, sd, sq)
					for b := tsAll & support; b != 0; b = bit.Rest(b) {
						f := bit.First(b)
						//util.ASSERT(Line_is_empty(f, sq, bd))
						safe |= Behind[f][sq]
					}
				}

				tsSafe := tsPawnSafe & ^ai.ltAtks[xd][pc] & safe

				mg += Score(p12, sq, MG)
				eg += Score(p12, sq, EG)

				if pc == piece.King {
					eg += mobilityScore(tsSafe)
				} else {
					eval += mobilityScore(tsSafe)
				}

				if pc != piece.King {
					mg += attackMgScore(pc, sd, tsPawnSafe)
				}

				eg += attackEgScore(pc, sd, tsPawnSafe, pEntry)

				eval += captureScore(pc, sd, tsAll&(ai.gePieces[xd][pc]|target), bd, &ai)

				if pc != piece.King {
					eval += checkNumber(pc, sd, tsSafe, opKing, bd) * material.Power(pc) * 6
				}

				if pc != piece.King && (tsSafe&kingArea[xd][opKing]) != 0 { // king attack
					kingN++
					kingPower += material.Power(pc)
				}

				if (pc == piece.Knight || pc == piece.Bishop) && rk >= square.Rank5 && rk <= square.Rank6 && fl >= square.FileC && fl <= square.FileF { // outpost
					eval += evalOutpost(sq, sd, bd, pEntry) * 5
				}

				if (pc == piece.Knight || pc == piece.Bishop) && rk >= square.Rank5 && !bit.IsOne(ai.allAtks[sd], sq) { // loose minor
					mg -= 10
				}

				if (pc == piece.Knight || pc == piece.Bishop) && rk >= square.Rank3 && rk <= square.Rank4 && bd.SquareIs(square.Stop(sq, sd), piece.Pawn, sd) { // shielded minor
					mg += 10
				}

				// Rook on open file and/or 7th rank
				if pc == piece.Rook {

					sc := pEntry.Open[fl][sd]

					minors := bd.PieceSd(piece.Knight, xd) | bd.PieceSd(piece.Bishop, xd)
					if sc >= 10 && (minors&bit.File(fl) & ^target) != 0 { // blocked by minor
						sc = 5
					}

					eval += int(sc - 10)

					if sc >= 10 && util.Iabs(square.File(opKing)-fl) <= 1 { // open file on king
						weight := 1
						if square.File(opKing) == fl {
							weight = 2
						}

						mg += int(sc) * weight / 2
					}

					if rk == square.Rank7 { // 7th rank

						pawns := bd.PieceSd(piece.Pawn, xd) & bit.Rank(square.Rank(sq))

						if square.RankSd(opKing, sd) >= square.Rank7 || pawns != 0 {
							mg += 10
							eg += 20
						}
					}
				}

				if pc == piece.King { // king out

					dl := (pEntry.leftFile - 1) - int8(fl)
					if dl > 0 {
						eg -= int(dl) * 20
					}

					dr := int8(fl) - (pEntry.rightFile + 1)
					if dr > 0 {
						eg -= int(dr) * 20
					}
				}
			}
		}
		//fmt.Println("col:", sd, "Pi eval:", eval, "mg:", mg, "eg:", eg)
		// ---- end pieces ---

		if bd.Count(piece.Bishop, sd) >= 2 {
			mg += 30
			eg += 50
		}

		if evalKBNK(bd, sd) {
			sqB := bit.First(bd.Piece(piece.Bishop))

			//min distance till A1 och H8
			d := util.Imin(square.Distance(opKing, square.A1), square.Distance(opKing, square.H8))
			if square.SameColor(square.H1, sqB) { // if bishop_sq_color is the other
				//min distance till H1 och A8
				d = util.Imin(square.Distance(opKing, square.H1), square.Distance(opKing, square.A8))
			}
			kd := square.Distance(myKing, opKing)
			mkc := square.Distance(myKing, square.E4) // dist center
			okc := square.Distance(opKing, square.E4) // dist center
			eval += 2000 - int(d*200.0) - 10*kd + mkc - okc
			if sd == BLACK {
				return -eval //NOTE!!!
			}

			return eval
		}

		mg += shelter[sd]
		//fmt.Println("col:", sd, "shelter mg:", mg)
		mg += mulShift(kingScore(kingPower*30, kingN), 32-shelter[xd], 5)
		//fmt.Println("col:", sd, "mul_sh king_score mg:", mg)

		eval = -eval
		mg = -mg
		eg = -eg
		//fmt.Println("neg col", sd, "eval:", eval, "mg:", mg, "eg:", eg)
	}

	mg += int(pEntry.mg)
	eg += int(pEntry.eg)

	eval += evalFiancetto(bd)
	//fmt.Println("innan interp", "eval:", eval, "mg:", mg, "eg:", eg)
	eval += Interpolation(mg, eg, bd)
	//fmt.Println("efter interp", "eval:", eval, "mg:", mg, "eg:", eg)
	//fmt.Println()
	if eval != 0 { // draw multiplier
		if eval > 0 {
			return mulShift(eval, drawMul(WHITE, bd, pEntry), 4)
		}
		return mulShift(eval, drawMul(BLACK, bd, pEntry), 4)
	}
	//util.ASSERT(eval >= score.EVAL_MIN && eval <= score.EVAL_MAX)
	return eval
}

// -- comp_eval END -- //

func evalKBNK(bd *board.Board, sd int) bool {
	if bit.Count(bd.All()) == 4 &&
		bd.Count(piece.Bishop, sd) == 1 &&
		bd.Count(piece.Knight, sd) == 1 {
		return true
	}
	return false
}

func kingScore(sc, n int) int {
	weight := 256 - (uint(256) >> uint(n))
	return mulShift(sc, int(weight), 8)
}

func mobilityScore(ts bit.BB) int {

	mob := bit.Count(ts)
	return mulShift(20, mobWeight[mob], 8)
}
func attackMgScore(pc, sd int, ts bit.BB) int {

	//util.ASSERT(pc < piece.SIZE)

	c0 := bit.Count(ts & centre0)
	c1 := bit.Count(ts & centre1)
	sc := c1*2 + c0

	sc += bit.Count(ts & sideArea[board.Opposit(sd)])

	return (sc - 4) * attackWeight[pc] / 2
}
func attackEgScore(pc, sd int, ts bit.BB, pi *pawnEntry) int {
	//util.ASSERT(pc < piece.SIZE)
	return bit.Count(ts&pi.target[sd]) * attackWeight[pc] * 4
}
func drawMul(sd int, bd *board.Board, pi *pawnEntry) int {

	xd := board.Opposit(sd)

	var pawn [2]int
	pawn[WHITE] = bd.Count(piece.Pawn, WHITE)
	pawn[BLACK] = bd.Count(piece.Pawn, BLACK)

	force := force(sd, bd) - force(xd, bd)

	// Rook-file pawns

	if board.LoneKingOrBishop(sd, bd) && pawn[sd] != 0 {

		b := bd.PieceSd(piece.Bishop, sd)

		if (bd.PieceSd(piece.Pawn, sd) & ^bit.File(square.FileA)) == 0 &&
			(bd.PieceSd(piece.Pawn, xd)&bit.File(square.FileB)) == 0 {

			prom := square.A1
			if sd == WHITE {
				prom = square.A8
			}

			if square.Distance(bd.King(xd), prom) <= 1 {
				if b == 0 || !square.SameColor(bit.First(b), prom) {
					return 1
				}
			}
		}

		if (bd.PieceSd(piece.Pawn, sd) & ^bit.File(square.FileH)) == 0 &&
			(bd.PieceSd(piece.Pawn, xd)&bit.File(square.FileG)) == 0 {

			prom := square.H1
			if sd == WHITE {
				prom = square.H8
			}

			if square.Distance(bd.King(xd), prom) <= 1 {
				if b == 0 || !square.SameColor(bit.First(b), prom) {
					return 1
				}
			}
		}
	}

	if pawn[sd] == 0 && board.LoneKingOrMinor(sd, bd) {
		return 1
	}

	if pawn[sd] == 0 && board.TwoKnights(sd, bd) {
		return 2
	}

	if pawn[sd] == 0 && force <= 1 {
		return 2
	}

	if pawn[sd] == 1 && force == 0 && hasMinor(xd, bd) {
		return 4
	}

	if pawn[sd] == 1 && force == 0 {

		king := bd.King(xd)
		pawn := bit.First(bd.PieceSd(piece.Pawn, sd))
		stop := square.Stop(pawn, sd)

		if king == stop || (square.RankSd(pawn, sd) <= square.Rank6 && king == square.Stop(stop, sd)) {
			return 4
		}
	}

	if pawn[sd] == 2 && pawn[xd] >= 1 && force == 0 && hasMinor(xd, bd) && (bd.PieceSd(piece.Pawn, sd)&pi.Passed) == 0 {
		return 8
	}

	if board.LoneBishop(WHITE, bd) && board.LoneBishop(BLACK, bd) && util.Iabs(pawn[WHITE]-pawn[BLACK]) <= 2 { // opposit-colour bishops

		wb := bit.First(bd.PieceSd(piece.Bishop, WHITE))
		bb := bit.First(bd.PieceSd(piece.Bishop, BLACK))

		if !square.SameColor(wb, bb) {
			return 8
		}
	}

	return 16
}

func captureScore(pc, sd int, ts bit.BB, bd *board.Board, ai *attackInfo) int {

	//util.ASSERT(pc < piece.SIZE)

	sc := 0

	for b := ts & bd.Pieces(board.Opposit(sd)); b != 0; b = bit.Rest(b) {

		t := bit.First(b)

		cp := bd.Square(t)
		sc += attackedWeight[cp]
		if bit.IsOne(ai.pinned, t) {
			sc += attackedWeight[cp] * 2
		}
	}

	return attackWeight[pc] * sc * 4
}
func checkNumber(pc, sd int, ts bit.BB, king int, bd *board.Board) int {

	//util.ASSERT(pc != piece.King)

	xd := board.Opposit(sd)
	checks := ts & ^bd.Side(sd) & PseudoAttacksTo(pc, sd, king)

	if !(pc >= piece.Bishop && pc <= piece.Queen) { // not slider
		return bit.CountLoop(checks)
	}

	n := 0

	b := checks & PseudoAttacksTo(piece.King, xd, king) // contact checks
	n += bit.CountLoop(b) * 2
	checks &= ^b

	for b := checks; b != 0; b = bit.Rest(b) {

		t := bit.First(b)

		if LineIsEmpty(t, king, bd) {
			n++
		}
	}

	return n
}

func force(sd int, bd *board.Board) int { // for draw eval

	force := 0

	for pc := piece.Knight; pc <= piece.Queen; pc++ {
		force += bd.Count(pc, sd) * material.Power(pc)
	}

	return force
}
func hasMinor(sd int, bd *board.Board) bool {
	return bd.Count(piece.Knight, sd)+bd.Count(piece.Bishop, sd) != 0
}

// Interpolation is using the stage (Phase) of the game to find out
// the weight for endgame and middlegame and compute the score
func Interpolation(mg, eg int, bd *board.Board) int {

	phase := util.Imin(bd.Phase(), material.TotalPhase)
	//util.ASSERT(phase >= 0 && phase <= material.TOTAL_PHASE)

	weight := material.PhaseWeight[phase]
	return (mg*weight + eg*(256-weight) + 128) >> 8
}
func evalFiancetto(bd *board.Board) int {

	eval := 0

	// fianchetto

	if bd.SquareIs(square.B2, piece.Bishop, WHITE) &&
		bd.SquareIs(square.B3, piece.Pawn, WHITE) &&
		bd.SquareIs(square.C2, piece.Pawn, WHITE) {
		eval += 20
	}

	if bd.SquareIs(square.G2, piece.Bishop, WHITE) &&
		bd.SquareIs(square.G3, piece.Pawn, WHITE) &&
		bd.SquareIs(square.F2, piece.Pawn, WHITE) {
		eval += 20
	}

	if bd.SquareIs(square.B7, piece.Bishop, BLACK) &&
		bd.SquareIs(square.B6, piece.Pawn, BLACK) &&
		bd.SquareIs(square.C7, piece.Pawn, BLACK) {
		eval -= 20
	}

	if bd.SquareIs(square.G7, piece.Bishop, BLACK) &&
		bd.SquareIs(square.G6, piece.Pawn, BLACK) &&
		bd.SquareIs(square.F7, piece.Pawn, BLACK) {
		eval -= 20
	}

	return eval
}
func evalOutpost(sq, sd int, bd *board.Board, pi *pawnEntry) int {

	//util.ASSERT(square.RankSd(sq, sd) >= square.Rank5)

	xd := board.Opposit(sd)

	weight := 0

	if bit.IsOne(pi.safe, sq) { // safe square
		weight += 2
	}

	if bd.SquareIs(square.Stop(sq, sd), piece.Pawn, xd) { // shielded
		weight++
	}

	if IsAttacked(sq, sd, bd) { // defended
		weight++
	}

	return weight - 2
}
func evalPawnCap(sd int, bd *board.Board, ai *attackInfo) int {

	ts := PawnAttacksFrom(sd, bd)

	sc := 0

	for b := ts & bd.Pieces(board.Opposit(sd)); b != 0; b = bit.Rest(b) {

		t := bit.First(b)

		cp := bd.Square(t)
		if cp == piece.King {
			continue
		}

		sc += piece.Value[cp] - 50
		if bit.IsOne(ai.pinned, t) {
			sc += (piece.Value[cp] - 50) * 2
		}
	}

	return sc / 10
}
func evalPassed(sq, sd int, bd *board.Board, ai *attackInfo) int {

	fl := square.File(sq)
	xd := board.Opposit(sd)

	weight := 4

	// blocker
	//util.ASSERT(sq < 63 && sq > 0)
	if bd.Square(square.Stop(sq, sd)) != piece.None {
		weight--
	}

	// free path

	front := bit.File(fl) & bit.FrontSd(sq, sd)
	rear := bit.File(fl) & bit.RearSd(sq, sd)

	if (bd.All() & front) == 0 {

		majorBehind := false
		majors := bd.PieceSd(piece.Rook, xd) | bd.PieceSd(piece.Queen, xd)

		for b := majors & rear; b != 0; b = bit.Rest(b) {

			f := bit.First(b)

			if LineIsEmpty(f, sq, bd) {
				majorBehind = true
			}
		}

		if !majorBehind && (ai.allAtks[xd]&front) == 0 {
			weight += 2
		}
	}

	return weight
}
func compAttacks(ai *attackInfo, bd *board.Board) {

	// prepare for adding defended opponent pieces

	var pc int
	sd := 0
	for ; sd < 2; sd++ {

		b := bit.BB(0)

		for pc = piece.King; pc >= piece.Bishop; pc-- {
			b |= bd.PieceSd(pc, sd)
			ai.gePieces[sd][pc] = b
		}

		ai.gePieces[sd][piece.Knight] = ai.gePieces[sd][piece.Bishop] // minors are equal
	}

	// pawn attacks

	pc = piece.Pawn

	for sd = 0; sd < 2; sd++ {

		b := PawnAttacksFrom(sd, bd)

		ai.ltAtks[sd][pc] = 0 // not needed
		ai.leAtks[sd][pc] = b // all pawn attcks per side (sd)
		ai.allAtks[sd] = b
	}

	// piece attacks

	ai.multipleAtks[WHITE] = 0
	ai.multipleAtks[BLACK] = 0

	for pc = piece.Knight; pc <= piece.King; pc++ {
		lowerPiece := piece.Pawn
		if pc > piece.Bishop {
			lowerPiece = pc - 1 // HACK: direct access to piece number
		}

		for sd = 0; sd < 2; sd++ {
			ai.ltAtks[sd][pc] = ai.leAtks[sd][lowerPiece]

			for fs := bd.PieceSd(pc, sd); fs != 0; fs = bit.Rest(fs) {

				sq := bit.First(fs)

				ts := PieceAttacksFrom(pc, sq, bd)
				ai.pieceAtks[sq] = ts

				ai.multipleAtks[sd] |= ts & ai.allAtks[sd]
				ai.allAtks[sd] |= ts
			}

			ai.leAtks[sd][pc] = ai.allAtks[sd]
			//util.ASSERT((ai.le_attacks[sd][pc] & ai.lt_attacks[sd][pc]) == ai.lt_attacks[sd][pc])

			if pc == piece.Bishop { // minors are equal
				ai.leAtks[sd][piece.Knight] = ai.leAtks[sd][piece.Bishop]
			}
		}
	}

	for sd = 0; sd < 2; sd++ {
		king := bd.King(sd)
		ts := PseudoAttacksFrom(piece.King, sd, king)
		ai.kingEvasions[sd] = ts & ^bd.Side(sd) & ^ai.allAtks[board.Opposit(sd)]
	}

	// pinned pieces

	ai.pinned = 0

	for sd = 0; sd < 2; sd++ {
		sq := bd.King(sd)
		ai.pinned |= bd.Side(sd) & PinnedBy(sq, board.Opposit(sd), bd)
	}
}

func shelterScore(sq int, sd int, bd *board.Board, pi *pawnEntry) int {

	if square.RankSd(sq, sd) > square.Rank2 {
		return 0
	}

	s0 := pi.Shelter[square.File(sq)][sd]

	s1 := 0

	for wg := 0; wg < 2; wg++ {

		index := castling.Index(sd, wg)

		if castling.Flag(bd.Flags(), uint(index)) {
			fl := shelterFile[wg]
			s1 = util.Imax(s1, int(pi.Shelter[fl][sd]))
		}
	}

	if s1 > int(s0) {
		return (int(s0) + s1) / 2
	}
	return int(s0)

}
func calcDist(f, t, weight int) int {
	dist := square.Distance(f, t)
	return mulShift(distWeight[dist], weight, 8)
}
func mulShift(a, b, c int) int {
	bias := 1 << uint(c-1)
	return (a*b + bias) >> uint(c)
}

func passedScore(sc, rk int) int {
	passedWeight := []int{0, 0, 0, 2, 6, 12, 20, 0}
	return mulShift(sc, passedWeight[rk], 4)
}

/*


    struct Entry {
 var lock; uint32
 var eval; int
    };

   // class Table {

       static const int BITS = 16;
       static const int SIZE = 1 << BITS;
       static const int MASK = SIZE - 1;

       Entry pc_table[SIZE];




 func  clear( )  void{
          for  index := 0; index < SIZE; index++ ) {
             pc_table[index].lock = 0;
             pc_table[index].eval = 0;
          }
       }




 var mob_weight[32]; int
 var dist_weight[8]; int // for king-passer distance



 var side_area[2]; bit.Bit_t
 var king_area[2][square.SIZE]; bit.Bit_t






 func  comp_eval( &pawn_table pawn.Table,)  int{

       Attack_Info ai;
       comp_attacks(ai, bd);

 var pawn.Info const & pi = pawn_table.info(bd);

 var eval int = 0;
 var mg int = 0;
 var eg int = 0;

 var shelter[2]; int

       for  sd := 0; sd < 2; sd++ ) {
          shelter[sd] = shelter_score(bd.king(sd), sd, bd, pi);
       }

       for  sd := 0; sd < 2; sd++ ) {

 var xd int = opposit(sd);

 var my_king int = bd.king(sd);
 var op_king int = bd.king(xd);

 var target bit.Bit_t = ~(bd.piece(piece.PAWN, sd) | attack.pawn_attacks_from(xd, bd));

 var king_n int = 0;
 var king_power int = 0;

          // pawns

          {
 var fs bit.Bit_t = bd.piece(piece.PAWN, sd);

 var front bit.Bit_t = (sd == WHITE) ? bit.front(square.Rank3) : bit.rear(square.RANK_6);

             for  b := fs &pi.passed &front; b != 0; b = bit.rest(b )) {

 var sq int = bit.first(b);

 var rk int = square.Rank(sq, sd);

                if (passer_is_unstoppable(sq, sd, bd)) {

 var weight int = std.max(rk - square.Rank3, 0);
                   //util.ASSERT(weight >= 0 and weight < 5);

                   eg += (piece.Queen_VALUE - piece.PAWN_VALUE) * weight / 5;

                } else {

 var sc int = eval_passed(sq, sd, bd, ai);

 var sc_mg int = sc * 20;
 var sc_eg int = sc * 25;

 var stop int = square.stop(sq, sd);
                   sc_eg -= my_distance(my_king, stop, 10);
                   sc_eg += my_distance(op_king, stop, 20);

                   mg += passed_score(sc_mg, rk);
                   eg += passed_score(sc_eg, rk);
                }
             }

             eval += bit.count(attack.pawn_moves_from(sd, bd) & bd.empty()) * 4 - bd.count(piece.PAWN, sd) * 2;

             eval += eval_pawn_cap(sd, bd, ai);
          }

          // pieces

          for  pc := piece.Knight; pc <= piece.King; pc++ ) {

 var p12 int = piece.make(pc, sd); // for PST

             {
 var n int = bd.count(pc, sd);
                mg += n * material.score(pc, stage.MG);
                eg += n * material.score(pc, stage.EG);
             }

             for   sd); b != 0; b = bit.rest(b)) {

 var sq int = bit.first(b);

 var fl int = square.File(sq);
 var rk int = square.Rank(sq, sd);

                // compute safe attacks

 var ts_all bit.Bit_t = ai.piece_attacks[sq];
 var ts_pawn_safe bit.Bit_t = ts_all & target;

 var safe bit.Bit_t = ~ai.all_attacks[xd] | ai.multiple_attacks[sd];

                if (true && piece.is_slider(pc)) { // battery support

 var bishops bit.Bit_t = bd.piece(piece.Bishop, sd) | bd.piece(piece.Queen, sd);
 var rooks bit.Bit_t = bd.piece(piece.Rook, sd) | bd.piece(piece.Queen, sd);

 var support bit.Bit_t = 0;
                   support |= bishops & attack.pseudo_attacks_to(piece.Bishop, sd, sq);
                   support |= rooks   & attack.pseudo_attacks_to(piece.Rook,   sd, sq);

                   for  b := ts_all &support; b != 0; b = bit.rest(b )) {
 var f int = bit.first(b);
                      assert(attack.line_is_empty(f, sq, bd));
                      safe |= attack.Behind[f][sq];
                   }
                }

 var ts_safe bit.Bit_t = ts_pawn_safe & ~ai.lt_attacks[xd][pc] & safe;

                mg += pst.score(p12, sq, stage.MG);
                eg += pst.score(p12, sq, stage.EG);

                if (pc == piece.King) {
                   eg += mobility_score(pc, ts_safe);
                } else {
                   eval += mobility_score(pc, ts_safe);
                }

                if (pc != piece.King) {
                   mg += attack_mg_score(pc, sd, ts_pawn_safe);
                }

                eg += attack_eg_score(pc, sd, ts_pawn_safe, pi);

                eval += capture_score(pc, sd, ts_all & (ai.ge_pieces[xd][pc] | target), bd, ai);

                if (pc != piece.King) {
                   eval += check_number(pc, sd, ts_safe, op_king, bd) * material.power(pc) * 6;
                }

                if (pc != piece.King && (ts_safe & king_area[xd][op_king]) != 0) { // king attack
                   king_n++;
                   king_power += material.power(pc);
                }

                if (piece.is_minor(pc) && rk >= square.Rank5 && rk <= square.RANK_6 && fl >= square.FileC && fl <= square.FileF) { // outpost
                   eval += eval_outpost(sq, sd, bd, pi) * 5;
                }

                if (piece.is_minor(pc) && rk >= square.Rank5 && !bit.is_set(ai.all_attacks[sd], sq)) { // loose minor
                   mg -= 10;
                }

                if (piece.is_minor(pc) && rk >= square.Rank3 && rk <= square.Rank4 && bd.square_is(square.stop(sq, sd), piece.PAWN, sd)) { // shielded minor
                   mg += 10;
                }

                if (pc == piece.Rook) { // open file

 var sc int = pi.open[fl][sd];

 var minors bit.Bit_t = bd.piece(piece.Knight, xd) | bd.piece(piece.Bishop, xd);
                   if (true && sc >= 10 && (minors & bit.file(fl) & ~target) != 0) { // blocked by minor
                      sc = 5;
                   }

                   eval += sc - 10;

                   if (sc >= 10 && std.abs(square.File(op_king) - fl) <= 1) { // open file on king
 var weight int = (square.File(op_king) == fl) ? 2 : 1;
                      mg += sc * weight / 2;
                   }
                }

                if (pc == piece.Rook && rk == square.Rank7) { // 7th rank

 var pawns bit.Bit_t = bd.piece(piece.PAWN, xd) & bit.rank(square.Rank(sq));

                   if (square.Rank(op_king, sd) >= square.Rank7 || pawns != 0) {
                      mg += 10;
                      eg += 20;
                   }
                }

                if (pc == piece.King) { // king out

 var dl int = (pi.left_file - 1) - fl;
                   if (dl > 0) eg -= dl * 20;

 var dr int = fl - (pi.right_file + 1);
                   if (dr > 0) eg -= dr * 20;
                }
             }
          }

          if (bd.count(piece.Bishop, sd) >= 2) {
             mg += 30;
             eg += 50;
          }

          mg += shelter[sd];
          mg += mul_shift(king_score(king_power * 30, king_n), 32 - shelter[xd], 5);

          eval = -eval;
          mg = -mg;
          eg = -eg;
       }

       mg += pi.mg;
       eg += pi.eg;

       eval += eval_pattern(bd);

       eval += material.interpolation(mg, eg, bd);

       if (eval != 0) { // draw multiplier
 var winner int = (eval > 0) ? WHITE : BLACK;
          eval = mul_shift(eval, draw_mul(winner, bd, pi), 4);
       }

       assert(eval >= score.EVAL_MIN && eval <= score.EVAL_MAX);
       return eval;
    }

*/
