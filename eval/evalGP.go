//go:build tunegp
// +build tunegp

// this file is for the GPTune project and is compiled together with GPTune.go
// See more in GPTune.go that is the starting point
////////////////////////////////////////////////////////////
// COMPILE WITH go build -tags tunegp   (små bokstäver)	  //
////////////////////////////////////////////////////////////

package eval

import (
	"fmt"
	"math"

	"goalaric/bit"
	"goalaric/board" //"goalaric/hash"
	"goalaric/material"
	"goalaric/move"
	"goalaric/parms"
	"goalaric/square"
)

// Parms is an array with evaluation values
var (
	Parms  = &parms.Parms
	Nparms = parms.Nparms
)

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
var attackWeight = [material.Size]int{0, Parms[35], Parms[36], Parms[37], Parms[38], Parms[39], 0}   // 4, 4, 2, 1, 4,
var attackedWeight = [material.Size]int{0, Parms[40], Parms[41], Parms[42], Parms[43], Parms[44], 0} //1, 1, 2, 4, 8,

var mobWeight [32]int
var distWeight [8]int // for king-passer distance

// Init inits eval values
func init() {
	if len(Parms) != Nparms {
		panic(fmt.Sprintf("constant Nparms is not equal len(Parms)"))
	}
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

			if move.Iabs(df) <= 1 && dr >= -1 && dr <= +2 {
				bit.Set(&kingArea[WHITE][ks], as)
			}

			if move.Iabs(df) <= 1 && dr >= -2 && dr <= +1 {
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
		x := float64(i) - float64(Parms[13]) //3.0
		y := 1.0 / (1.0 + math.Exp(-x))
		distWeight[i] = int(math.Floor((y*float64(Parms[14]*Parms[15]) + 0.5))) //7.0, 256.0
	}

	AtkInit()
}

// Update is for tuning. See tune.go
func Update() {
	attackWeight = [material.Size]int{0, Parms[35], Parms[36], Parms[37], Parms[38], Parms[39], 0}   // 4, 4, 2, 1, 4,
	attackedWeight = [material.Size]int{0, Parms[40], Parms[41], Parms[42], Parms[43], Parms[44], 0} //1, 1, 2, 4, 8,
	material.Update()
}

type attackInfo struct {
	pieceAtks    [square.BoardSize]bit.BB
	allAtks      [2]bit.BB
	multipleAtks [2]bit.BB

	gePieces [2][material.Size]bit.BB

	ltAtks [2][material.Size]bit.BB
	leAtks [2][material.Size]bit.BB

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

// Eval is called by evalWithSign (search) and if it cannot use eval-hash-table it calls comp_eval.
// it returns score for white

func (t *Hash) Eval(bd *board.Board, pawnTable *PawnHash) int { // NOTE: score for white
	//fmt.Println("i hash.Eval", parms.Parms[23], Parms[24])
	/* 	key := bd.EvalKey()

	   	index := hash.Index(key) & MASK
	   	lock := uint32(hash.Lock(key))

	   	entry := t.entries[index]

	   	if entry.lock == lock {
	   		return entry.eval,
	   	}
	*/
	eval := CompEval(bd, pawnTable)

	//entry.lock = lock
	//entry.eval = eval

	return eval
}

// Eval is called by evalWithSign (in search). It calls table.eval and returns the stm evaluation
//func Eval(bd *board.Board, table *Table, pawn_table *PawnTable) int {
//	return score.Side_score(table.Eval(bd, pawn_table), bd.Turn())
//}

// CompEval makes a complete evaluation for white in current position
func CompEval(bd *board.Board, pawnHash *PawnHash) int { // NOTE: score for white
	//fmt.Println("i CompEval", parms.Parms[23], Parms[24])
	var ai attackInfo
	compAttacks(&ai, bd)
	pEntry := pawnHash.getEntry(bd)

	eval := 0
	mg := 0
	eg := 0

	var shelter [2]int

	sd := 0
	for ; sd < 2; sd++ {
		shelter[sd], _ = shelterScore(bd.King(sd), sd, bd, pEntry)
	}

	for sd = 0; sd < 2; sd++ {

		xd := board.Opposit(sd)

		myKing := bd.King(sd)
		opKing := bd.King(xd)

		target := ^(bd.PieceSd(material.Pawn, sd) | PawnAttacksFrom(xd, bd))

		kingN := 0
		kingPower := 0

		// pawns

		myPawns := bd.PieceSd(material.Pawn, sd)
		front := bit.Front(square.Rank3)
		if sd == BLACK {
			front = bit.Rear(square.Rank6)
		}

		for pc := myPawns & pEntry.Passed & front; pc != 0; pc = bit.Rest(pc) {

			sq := bit.First(pc)
			rk := square.RankSd(sq, sd)

			if passerUnstoppable(sq, sd, bd) {

				weight := Imax(rk-square.Rank3, 0)

				eg += (material.QueenValue - material.PawnValue) * weight / Parms[0] //5

			} else {
				sc := evalPassed(sq, sd, bd, &ai)
				scMg := sc * Parms[1] //20
				scEg := sc * Parms[2] //25

				stop := square.Stop(sq, sd)
				scEg -= calcDist(myKing, stop, Parms[3]) //10
				scEg += calcDist(opKing, stop, Parms[4]) //20

				mg += passedScore(scMg, rk)
				eg += passedScore(scEg, rk)
			}
		}

		eval += bit.Count(pawnMovesFrom(sd, bd)&bd.Empty())*Parms[5] - bd.Count(material.Pawn, sd)*Parms[6] //4 och 2

		e, _ := evalPawnCap(sd, bd, &ai)
		eval += e
		// --- end pawns ----

		// pieces
		//fmt.Println("i Score", parms.Parms[23], material.Score(material.Knight, MG), Parms[24], material.Score(material.Knight, EG))
		for pc := material.Knight; pc <= material.King; pc++ {

			p12 := material.MakeP12(pc, sd) // for PST

			n := bd.Count(pc, sd)
			mg += n * material.Score(pc, MG)
			eg += n * material.Score(pc, EG)

			for b := bd.PieceSd(pc, sd); b != 0; b = bit.Rest(b) {

				sq := bit.First(b)

				fl := square.File(sq)
				rk := square.RankSd(sq, sd)

				// compute safe attacks
				tsAll := ai.pieceAtks[sq]
				tsPawnSafe := tsAll & target

				safe := ^ai.allAtks[xd] | ai.multipleAtks[sd]

				if pc >= material.Bishop && pc <= material.Queen { // battery (slider) support

					bishops := bd.PieceSd(material.Bishop, sd) | bd.PieceSd(material.Queen, sd)
					rooks := bd.PieceSd(material.Rook, sd) | bd.PieceSd(material.Queen, sd)

					support := bishops & PseudoAttacksTo(material.Bishop, sd, sq)
					support |= rooks & PseudoAttacksTo(material.Rook, sd, sq)
					for b := tsAll & support; b != 0; b = bit.Rest(b) {
						f := bit.First(b)
						//util.ASSERT(Line_is_empty(f, sq, bd))
						safe |= Behind[f][sq]
					}
				}

				tsSafe := tsPawnSafe & ^ai.ltAtks[xd][pc] & safe

				//TODO in order to tune these values we need to run pstInit before each getQs() in tune.go
				mg += Score(p12, sq, MG) // from piece/sq table
				eg += Score(p12, sq, EG) // from piece/sq table

				//TODO in order to tune mobWeight we must run the init (in init()) before each getQs in tune.go
				if pc == material.King {
					e, _ = mobilityScore(tsSafe)
					eg += e
				} else {
					e, _ = mobilityScore(tsSafe)
					eval += e
				}

				if pc != material.King {
					e, _ = attackMgScore(pc, sd, tsPawnSafe)
					mg += e
				}

				eg += attackEgScore(pc, sd, tsPawnSafe, pEntry)

				e, _ = captureScore(pc, sd, tsAll&(ai.gePieces[xd][pc]|target), bd, &ai)
				eval += e

				if pc != material.King {
					e, _ = checkNumber(pc, sd, tsSafe, opKing, bd)
					eval += e * material.Power(pc) * 6
				}

				if pc != material.King && (tsSafe&kingArea[xd][opKing]) != 0 { // king attack
					kingN++
					kingPower += material.Power(pc)
				}

				if (pc == material.Knight || pc == material.Bishop) && rk >= square.Rank5 && rk <= square.Rank6 && fl >= square.FileC && fl <= square.FileF { // outpost
					eval += evalOutpost(sq, sd, bd, pEntry) * Parms[52] //5
				}

				// mg for not uarded minor piece
				if (pc == material.Knight || pc == material.Bishop) && rk >= square.Rank5 && !bit.IsOne(ai.allAtks[sd], sq) { // not guarded minor
					mg -= Parms[57] //10
				}

				// mg for shielded minor
				if (pc == material.Knight || pc == material.Bishop) && rk >= square.Rank3 && rk <= square.Rank4 && bd.SquareIs(square.Stop(sq, sd), material.Pawn, sd) { // shielded minor
					mg += Parms[58] //10
				}

				// Rook on open file and/or 7th rank
				if pc == material.Rook {

					sc := pEntry.Open[fl][sd]

					// Rook blocked by minor
					minors := bd.PieceSd(material.Knight, xd) | bd.PieceSd(material.Bishop, xd)
					if sc >= int8(Parms[59]) && (minors&bit.File(fl) & ^target) != 0 { //10 // blocked by minor
						sc = int8(Parms[60]) //5
					}

					eval += int(sc - int8(Parms[61])) //10

					//R on open file with K
					if sc >= int8(Parms[62]) && move.Iabs(square.File(opKing)-fl) <= 1 { // open file on king //10
						weight := Parms[63] //1
						if square.File(opKing) == fl {
							weight = Parms[64] //2
						}

						mg += int(sc) * weight / Parms[65] //2
					}

					if rk == square.Rank7 { // 7th rank

						pawns := bd.PieceSd(material.Pawn, xd) & bit.Rank(square.Rank(sq))

						if square.RankSd(opKing, sd) >= square.Rank7 || pawns != 0 {
							mg += Parms[66] //10
							eg += Parms[67] //20
						}
					}
				}

				if pc == material.King { // king distance from A and H

					dl := (pEntry.leftFile - 1) - int8(fl)
					if dl > 0 {
						eg -= int(dl) * Parms[68] //20
					}

					dr := int8(fl) - (pEntry.rightFile + 1)
					if dr > 0 {
						eg -= int(dr) * Parms[68] //20
					}
				}
			}
		} // end pieces

		if bd.Count(material.Bishop, sd) >= 2 { // bishop pair bonus
			mg += Parms[69] //30
			eg += Parms[70] //50
		}

		if evalKBNK(bd, sd) {
			sqB := bit.First(bd.Piece(material.Bishop))

			//min distance till A1 och H8
			d := Imin(square.Distance(opKing, square.A1), square.Distance(opKing, square.H8))
			if square.SameColor(square.H1, sqB) { // if bishop_sq_color is the other
				//min distance till H1 och A8
				d = Imin(square.Distance(opKing, square.H1), square.Distance(opKing, square.A8))
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
		mg += mulShift(kingScore(kingPower*Parms[72], kingN), Parms[73]-shelter[xd], Parms[74]) //30 32 5
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

// CompEvalGP is a copy of CompEval but it also returns parms for GP
func CompEvalGP(bd *board.Board, pawnHash *PawnHash) (int, []int) { // NOTE: score for white
	//fmt.Println("i CompEval", parms.Parms[23], Parms[24])
	vector := [352]int{}
	var x int
	var v int
	var ai attackInfo
	compAttacks(&ai, bd)
	pEntry := pawnHash.getEntry(bd)

	eval := 0
	mg := 0
	eg := 0

	var shelter [2]int

	sd := 0
	e := 0
	for ; sd < 2; sd++ {
		shelter[sd], v = shelterScore(bd.King(sd), sd, bd, pEntry)
		_ = v
		vector[x] = shelter[sd]
		x++
	}

	sign := 1
	x = 2
	for sd = 0; sd < 2; sd++ {
		//		fmt.Println("sd",sd,"x", x)
		xd := board.Opposit(sd)

		myKing := bd.King(sd)
		opKing := bd.King(xd)

		target := ^(bd.PieceSd(material.Pawn, sd) | PawnAttacksFrom(xd, bd))

		kingN := 0
		kingPower := 0

		// PAWNS

		myPawns := bd.PieceSd(material.Pawn, sd)
		front := bit.Front(square.Rank3)
		if sd == BLACK {
			front = bit.Rear(square.Rank6)
		}
		//		PASSED PAWNS
		y := x // 2
		for pc := myPawns & pEntry.Passed & front; pc != 0; pc = bit.Rest(pc) {
			//			fmt.Printf("x=%v\n",x)
			//			board.PrintBB(pc)
			sq := bit.First(pc)
			rk := square.RankSd(sq, sd)

			if passerUnstoppable(sq, sd, bd) {
				weight := Imax(rk-square.Rank3, 0)
				e = (material.QueenValue - material.PawnValue) * weight / Parms[0] //5
				eg += e
				//				vector[x] = sign*e
				//				x++
			} else {
				sc := evalPassed(sq, sd, bd, &ai)
				scMg := sc * Parms[1] //20
				scEg := sc * Parms[2] //25
				//				vector[x] = sign*scMg
				//				x++
				stop := square.Stop(sq, sd)
				e = calcDist(myKing, stop, Parms[3]) //10
				scEg -= e
				//				vector[x] = -sign * e
				//				x++
				e = calcDist(opKing, stop, Parms[4]) //20
				scEg += e
				//				vector[x] = sign *  e
				//				x++
				e = passedScore(scMg, rk)
				mg += e
				//				vector[x] = sign *  e
				//				x++
				e = passedScore(scEg, rk)
				eg += e
				//				vector[x] = sign *  e
				//				x++
			}
		}
		if x > y+20 {
			panic(fmt.Sprintf("warning............. x>y+20 x=%v y=%v diff=%v", x, y, x-y))
		}
		x = y + 20
		// 5
		e = bit.Count(pawnMovesFrom(sd, bd)&bd.Empty())*Parms[5] - bd.Count(material.Pawn, sd)*Parms[6] //4 och 2
		eval += e
		//		vector[x] = sign *  e
		//		x++
		//		vector[x] = sign *  bd.Count(material.Pawn, sd)
		//		x++

		e, v = evalPawnCap(sd, bd, &ai)
		eval += e
		//		vector[x] = sign *  e
		//		x++
		// --- end pawns ----
		y = x // 8

		// PIECES
		for pc := material.Knight; pc <= material.King; pc++ { // for each pt >= Knight
			//			fmt.Printf("pc: %d x=%v\n",pc,y)
			x = y
			p12 := material.MakeP12(pc, sd) // for PST

			n := bd.Count(pc, sd)
			mg += n * material.Score(pc, MG)
			eg += n * material.Score(pc, EG)

			//			vector[x] = sign *  n * material.Score(pc, MG)
			//			x++
			z := x
			for b := bd.PieceSd(pc, sd); b != 0; b = bit.Rest(b) { // for each piece of pt
				sq := bit.First(b)

				fl := square.File(sq)
				rk := square.RankSd(sq, sd)

				// compute safe attacks
				tsAll := ai.pieceAtks[sq]
				tsPawnSafe := tsAll & target

				safe := ^ai.allAtks[xd] | ai.multipleAtks[sd]

				if pc >= material.Bishop && pc <= material.Queen { // battery (slider) support
					bishops := bd.PieceSd(material.Bishop, sd) | bd.PieceSd(material.Queen, sd)
					rooks := bd.PieceSd(material.Rook, sd) | bd.PieceSd(material.Queen, sd)

					support := bishops & PseudoAttacksTo(material.Bishop, sd, sq)
					support |= rooks & PseudoAttacksTo(material.Rook, sd, sq)
					for b := tsAll & support; b != 0; b = bit.Rest(b) {
						f := bit.First(b)
						//util.ASSERT(Line_is_empty(f, sq, bd))
						safe |= Behind[f][sq]
					}
				}

				tsSafe := tsPawnSafe & ^ai.ltAtks[xd][pc] & safe

				//TODO in order to tune these values we need to run pstInit before each getQs() in tune.go
				mg += Score(p12, sq, MG) // from piece/sq table
				eg += Score(p12, sq, EG) // from piece/sq table
				//				vector[x] = sign *  Score(p12, sq, MG)
				//				x++
				//TODO in order to tune mobWeight we must run the init (in init()) before each getQs in tune.go
				v = 0
				if pc == material.King {
					e, v = mobilityScore(tsSafe)
					eg += e
					//					vector[x] = sign *  e
				} else {
					e, v = mobilityScore(tsSafe)
					//					eval += e
					vector[x+1] = sign * e
				}
				//				x+=2
				v = 0
				if pc != material.King {
					e, v = attackMgScore(pc, sd, tsPawnSafe)
					mg += e
					//					vector[x] = sign *  e
				}
				//				x++

				eg += attackEgScore(pc, sd, tsPawnSafe, pEntry)
				v = 0
				e, v = captureScore(pc, sd, tsAll&(ai.gePieces[xd][pc]|target), bd, &ai)
				eval += e
				//				vector[x] = sign *  e
				//				x++
				if pc != material.King {
					v = 0
					e, v = checkNumber(pc, sd, tsSafe, opKing, bd)
					e += e * material.Power(pc) * 6
					eval += e
					//					vector[x] = sign *  e
				}
				//				x++
				if pc != material.King && (tsSafe&kingArea[xd][opKing]) != 0 { // king attack
					kingN++
					kingPower += material.Power(pc)
					//					vector[x] = sign *  material.Power(pc)
				}
				//				x++

				if (pc == material.Knight || pc == material.Bishop) && rk >= square.Rank5 && rk <= square.Rank6 && fl >= square.FileC && fl <= square.FileF { // outpost
					e = evalOutpost(sq, sd, bd, pEntry) * Parms[52] //5
					eval += e
					//					vector[x] = sign * e
				}
				//				x++

				// mg for not guarded minor piece
				if (pc == material.Knight || pc == material.Bishop) && rk >= square.Rank5 && !bit.IsOne(ai.allAtks[sd], sq) { // not guarded minor
					mg -= Parms[57] //10
					//					vector[x] = sign *  -Parms[57]
				}
				//				x++

				// mg for shielded minor
				if (pc == material.Knight || pc == material.Bishop) && rk >= square.Rank3 && rk <= square.Rank4 && bd.SquareIs(square.Stop(sq, sd), material.Pawn, sd) { // shielded minor
					mg += Parms[58] //10
					//					vector[x] = sign *  Parms[58]
				}
				//				x++
				z2 := x
				// Rook on open file and/or 7th rank
				if pc == material.Rook {
					sc := int(pEntry.Open[fl][sd])

					// Rook blocked by minor
					minors := bd.PieceSd(material.Knight, xd) | bd.PieceSd(material.Bishop, xd)
					if sc >= Parms[59] && (minors&bit.File(fl) & ^target) != 0 { //10 // blocked by minor
						sc = Parms[60] //5
					}

					e = sc - Parms[61] //10
					eval += e
					//					vector[x] = sign *  e
					//					x++

					//R on open file with K
					if sc >= Parms[62] && move.Iabs(square.File(opKing)-fl) <= 1 { // open file on king //10
						weight := Parms[63] //1
						if square.File(opKing) == fl {
							weight = Parms[64] //2
						}

						e = int(sc) * weight / Parms[65] //2
						mg += e
						//						vector[x] = sign * e
					}
					//					x++

					if rk == square.Rank7 { // 7th rank
						pawns := bd.PieceSd(material.Pawn, xd) & bit.Rank(square.Rank(sq))
						if square.RankSd(opKing, sd) >= square.Rank7 || pawns != 0 {
							e = Parms[66] //10
							mg += e
							eg += Parms[67] //20
							//							vector[x] = sign *  e
						}
					}
					//					x++
				}
				if x > z2+3 {
					panic(fmt.Sprintf("warning............. x > z2 + 3 x", x, "z2", z2, "diff", x-z2))
				}
				x = z2 + 3
				//				fmt.Printf("pc %v x=%v\n",pc,x)
				if pc == material.King { // king distance from A and H
					dl := (pEntry.leftFile - 1) - int8(fl)
					if dl > 0 {
						eg -= int(dl) * Parms[68] //20
						//						vector[x] = -sign *  int(dl)*Parms[68]
					}

					dr := int8(fl) - (pEntry.rightFile + 1)
					if dr > 0 {
						eg -= int(dr) * Parms[68] //20
						//						vector[x+1] = -sign * int(dr)*Parms[68]
					}
				}
				//				x += 2
			}

			switch x - y {
			case 1:
			case 16:
			case 31:
			default:
				//			fmt.Printf("WARNING: end y=%v x=%v diff %v\n", y, x, x-y)
			}
			if x > z+31 {
				panic(fmt.Sprintf("warning............. x>z+31 x=%v z=%v diff=%v", x, z, x-z))
			}
			if x > y+31 {
				panic(fmt.Sprintf("warning............. x>y+31 x=%v y=%v diff=%v", x, y, x-y))
			}
			y += 30 // 37  66
			//fmt.Println(x,y)
			x = y
		} // end pieces

		if bd.Count(material.Bishop, sd) >= 2 { // bishop pair bonus
			e = Parms[69] //30
			mg += e
			eg += Parms[70] //50
			//			vector[x] = sign *  e
		}
		//		x++ // 69
		//fmt.Println(x)
		if evalKBNK(bd, sd) {
			sqB := bit.First(bd.Piece(material.Bishop))

			//min distance till A1 och H8
			d := Imin(square.Distance(opKing, square.A1), square.Distance(opKing, square.H8))
			if square.SameColor(square.H1, sqB) { // if bishop_sq_color is the other
				//min distance till H1 och A8
				d = Imin(square.Distance(opKing, square.H1), square.Distance(opKing, square.A8))
			}
			kd := square.Distance(myKing, opKing)
			mkc := square.Distance(myKing, square.E4) // dist center
			okc := square.Distance(opKing, square.E4) // dist center
			eval += 2000 - int(d*200.0) - 10*kd + mkc - okc

			if sd == BLACK {
				return -eval, vector[:] //NOTE!!!
			}

			return eval, vector[:]
		}

		mg += shelter[sd]
		//fmt.Println("x at King",x)
		mg += mulShift(kingScore(kingPower*Parms[72], kingN), Parms[73]-shelter[xd], Parms[74]) //30 32 5
		if x >= len(vector) {
			fmt.Println("x", x)
		}
		vector[x] = sign * kingScore(kingPower*Parms[72], kingN)
		x++ // 70
		//fmt.Println("col:", sd, "mul_sh king_score mg:", mg)

		eval = -eval
		mg = -mg
		eg = -eg
		sign = -1
	}

	mg += int(pEntry.mg)
	eg += int(pEntry.eg)
	vector[x] = mg
	x++
	vector[x] = eg
	x++
	vector[x] = eval
	x++
	vector[x] = eval
	x++
	vector[x] = evalFiancetto(bd)
	x++ //
	eval += evalFiancetto(bd)

	eval += Interpolation(mg, eg, bd)
	if eval != 0 { // draw multiplier
		if eval > 0 {
			return mulShift(eval, drawMul(WHITE, bd, pEntry), 4), vector[:]
		}
		return mulShift(eval, drawMul(BLACK, bd, pEntry), 4), vector[:]
	}
	//util.ASSERT(eval >= score.EVAL_MIN && eval <= score.EVAL_MAX)

	if x > len(vector) {
		fmt.Println(x, x <= len(vector))
	}
	//PRINT
	return eval, vector[:]
} // -- comp_eval END -- //

func evalKBNK(bd *board.Board, sd int) bool {
	if bit.Count(bd.All()) == 4 &&
		bd.Count(material.Bishop, sd) == 1 &&
		bd.Count(material.Knight, sd) == 1 {
		return true
	}
	return false
}

func kingScore(sc, n int) int {
	weight := 256 - (uint(256) >> uint(n))
	return mulShift(sc, int(weight), Parms[75]) //8
}

func mobilityScore(ts bit.BB) (int, int) {
	mob := bit.Count(ts)
	return mulShift(Parms[31], mobWeight[mob], Parms[32]), mobWeight[mob] //20 8
}

func attackMgScore(pc, sd int, ts bit.BB) (int, int) {

	c0 := bit.Count(ts & centre0)
	c1 := bit.Count(ts & centre1)
	sc := c1*2 + c0

	sc += bit.Count(ts & sideArea[board.Opposit(sd)])

	return (sc - Parms[33]) * attackWeight[pc] / Parms[34], sc //4 2
}

func attackEgScore(pc, sd int, ts bit.BB, pi *pawnEntry) int {
	//util.ASSERT(pc < material.SIZE)
	return bit.Count(ts&pi.target[sd]) * attackWeight[pc] * 4
}

func drawMul(sd int, bd *board.Board, pi *pawnEntry) int {

	xd := board.Opposit(sd)

	var pawn [2]int
	pawn[WHITE] = bd.Count(material.Pawn, WHITE)
	pawn[BLACK] = bd.Count(material.Pawn, BLACK)

	force := force(sd, bd) - force(xd, bd)

	// Rook-file pawns

	if board.LoneKingOrBishop(sd, bd) && pawn[sd] != 0 {

		b := bd.PieceSd(material.Bishop, sd)

		if (bd.PieceSd(material.Pawn, sd) & ^bit.File(square.FileA)) == 0 &&
			(bd.PieceSd(material.Pawn, xd)&bit.File(square.FileB)) == 0 {

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

		if (bd.PieceSd(material.Pawn, sd) & ^bit.File(square.FileH)) == 0 &&
			(bd.PieceSd(material.Pawn, xd)&bit.File(square.FileG)) == 0 {

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
		pawn := bit.First(bd.PieceSd(material.Pawn, sd))
		stop := square.Stop(pawn, sd)

		if king == stop || (square.RankSd(pawn, sd) <= square.Rank6 && king == square.Stop(stop, sd)) {
			return 4
		}
	}

	if pawn[sd] == 2 && pawn[xd] >= 1 && force == 0 && hasMinor(xd, bd) && (bd.PieceSd(material.Pawn, sd)&pi.Passed) == 0 {
		return 8
	}

	if board.LoneBishop(WHITE, bd) && board.LoneBishop(BLACK, bd) && move.Iabs(pawn[WHITE]-pawn[BLACK]) <= 2 { // opposit-colour bishops

		wb := bit.First(bd.PieceSd(material.Bishop, WHITE))
		bb := bit.First(bd.PieceSd(material.Bishop, BLACK))

		if !square.SameColor(wb, bb) {
			return 8
		}
	}

	return 16
}

func captureScore(pc, sd int, ts bit.BB, bd *board.Board, ai *attackInfo) (int, int) {

	//util.ASSERT(pc < material.SIZE)

	sc := 0

	for b := ts & bd.Pieces(board.Opposit(sd)); b != 0; b = bit.Rest(b) {

		t := bit.First(b)

		cp := bd.Square(t)
		sc += attackedWeight[cp]
		if bit.IsOne(ai.pinned, t) {
			sc += attackedWeight[cp] * Parms[45] //2
		}
	}

	return attackWeight[pc] * sc * Parms[46], attackWeight[pc] * sc //4
}

func checkNumber(pc, sd int, ts bit.BB, king int, bd *board.Board) (int, int) {
	xd := board.Opposit(sd)
	checks := ts & ^bd.Side(sd) & PseudoAttacksTo(pc, sd, king)

	if !(pc >= material.Bishop && pc <= material.Queen) { // not slider
		return bit.CountLoop(checks), bit.CountLoop(checks)
	}

	n := 0
	n2 := 0

	b := checks & PseudoAttacksTo(material.King, xd, king) // contact checks
	n += bit.CountLoop(b) * Parms[51]                      //2
	n2 += bit.CountLoop(b)
	checks &= ^b

	for b := checks; b != 0; b = bit.Rest(b) {

		t := bit.First(b)

		if LineIsEmpty(t, king, bd) {
			n++
			n2++
		}
	}

	return n, n2
}

func force(sd int, bd *board.Board) int { // for draw eval

	force := 0

	for pc := material.Knight; pc <= material.Queen; pc++ {
		force += bd.Count(pc, sd) * material.Power(pc)
	}

	return force
}
func hasMinor(sd int, bd *board.Board) bool {
	return bd.Count(material.Knight, sd)+bd.Count(material.Bishop, sd) != 0
}

// Interpolation is using the stage (Phase) of the game to find out
// the weight for endgame and middlegame and compute the score
func Interpolation(mg, eg int, bd *board.Board) int {

	phase := Imin(bd.Phase(), material.TotalPhase)
	//util.ASSERT(phase >= 0 && phase <= material.TOTAL_PHASE)

	weight := material.PhaseWeight[phase]
	return (mg*weight + eg*(256-weight) + 128) >> 8
}

func evalFiancetto(bd *board.Board) int {

	eval := 0

	// fianchetto

	if bd.SquareIs(square.B2, material.Bishop, WHITE) &&
		bd.SquareIs(square.B3, material.Pawn, WHITE) &&
		bd.SquareIs(square.C2, material.Pawn, WHITE) {
		eval += Parms[76] //20
	}

	if bd.SquareIs(square.G2, material.Bishop, WHITE) &&
		bd.SquareIs(square.G3, material.Pawn, WHITE) &&
		bd.SquareIs(square.F2, material.Pawn, WHITE) {
		eval += Parms[77] //20
	}

	if bd.SquareIs(square.B7, material.Bishop, BLACK) &&
		bd.SquareIs(square.B6, material.Pawn, BLACK) &&
		bd.SquareIs(square.C7, material.Pawn, BLACK) {
		eval -= Parms[76] //20
	}

	if bd.SquareIs(square.G7, material.Bishop, BLACK) &&
		bd.SquareIs(square.G6, material.Pawn, BLACK) &&
		bd.SquareIs(square.F7, material.Pawn, BLACK) {
		eval -= Parms[77] //20
	}

	return eval
}

func evalOutpost(sq, sd int, bd *board.Board, pi *pawnEntry) int {

	//util.ASSERT(square.RankSd(sq, sd) >= square.Rank5)

	xd := board.Opposit(sd)

	weight := 0

	if bit.IsOne(pi.safe, sq) { // safe square
		weight += Parms[53] //2
	}

	if bd.SquareIs(square.Stop(sq, sd), material.Pawn, xd) { // shielded
		weight = +Parms[54] //1
	}

	if IsAttacked(sq, sd, bd) { // defended
		weight += Parms[55] //1
	}

	return weight - Parms[56] //2
}

func evalPawnCap(sd int, bd *board.Board, ai *attackInfo) (int, int) {

	ts := PawnAttacksFrom(sd, bd)

	sc := 0

	for b := ts & bd.Pieces(board.Opposit(sd)); b != 0; b = bit.Rest(b) {

		t := bit.First(b)

		cp := bd.Square(t)
		if cp == material.King {
			continue
		}

		sc += material.Value[cp] - Parms[7]
		if bit.IsOne(ai.pinned, t) {
			sc += (material.Value[cp] - Parms[7]) * Parms[8]
		}
	}

	return sc / Parms[9], sc
}

func evalPassed(sq, sd int, bd *board.Board, ai *attackInfo) int {

	fl := square.File(sq)
	xd := board.Opposit(sd)

	weight := Parms[10] //4

	// blocker
	//util.ASSERT(sq < 63 && sq > 0)
	if bd.Square(square.Stop(sq, sd)) != material.None {
		weight--
	}

	// free path

	front := bit.File(fl) & bit.FrontSd(sq, sd)
	rear := bit.File(fl) & bit.RearSd(sq, sd)

	if (bd.All() & front) == 0 {

		majorBehind := false
		majors := bd.PieceSd(material.Rook, xd) | bd.PieceSd(material.Queen, xd)

		for b := majors & rear; b != 0; b = bit.Rest(b) {

			f := bit.First(b)

			if LineIsEmpty(f, sq, bd) {
				majorBehind = true
			}
		}

		if !majorBehind && (ai.allAtks[xd]&front) == 0 {
			weight += Parms[11] //2
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

		for pc = material.King; pc >= material.Bishop; pc-- {
			b |= bd.PieceSd(pc, sd)
			ai.gePieces[sd][pc] = b
		}

		ai.gePieces[sd][material.Knight] = ai.gePieces[sd][material.Bishop] // minors are equal
	}

	// pawn attacks

	pc = material.Pawn

	for sd = 0; sd < 2; sd++ {

		b := PawnAttacksFrom(sd, bd)

		ai.ltAtks[sd][pc] = 0 // not needed
		ai.leAtks[sd][pc] = b // all pawn attcks per side (sd)
		ai.allAtks[sd] = b
	}

	// piece attacks

	ai.multipleAtks[WHITE] = 0
	ai.multipleAtks[BLACK] = 0

	for pc = material.Knight; pc <= material.King; pc++ {
		lowerPiece := material.Pawn
		if pc > material.Bishop {
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

			if pc == material.Bishop { // minors are equal
				ai.leAtks[sd][material.Knight] = ai.leAtks[sd][material.Bishop]
			}
		}
	}

	for sd = 0; sd < 2; sd++ {
		king := bd.King(sd)
		ts := PseudoAttacksFrom(material.King, sd, king)
		ai.kingEvasions[sd] = ts & ^bd.Side(sd) & ^ai.allAtks[board.Opposit(sd)]
	}

	// pinned pieces

	ai.pinned = 0

	for sd = 0; sd < 2; sd++ {
		sq := bd.King(sd)
		ai.pinned |= bd.Side(sd) & PinnedBy(sq, board.Opposit(sd), bd)
	}
}

func shelterScore(sq int, sd int, bd *board.Board, pi *pawnEntry) (int, int) {

	if square.RankSd(sq, sd) > square.Rank2 {
		return 0, 0
	}

	s0 := int(pi.Shelter[square.File(sq)][sd])

	s1 := 0

	for wing := 0; wing < 2; wing++ {

		index := board.CastleIndex(sd, wing)

		if board.CastleFlag(bd.Flags(), uint(index)) {
			fl := shelterFile[wing]
			s1 = Imax(s1, int(pi.Shelter[fl][sd]))
		}
	}

	if s1 > s0 {
		return (s0 + s1) / Parms[71], (s0 + s1) //200
	}
	return s0, s0

}
func calcDist(f, t, weight int) int {
	dist := square.Distance(f, t)
	return mulShift(distWeight[dist], weight, Parms[12]) //8
}
func mulShift(a, b, c int) int {
	bias := 1 << uint(c-1)
	return (a*b + bias) >> uint(c)
}

func passedScore(sc, rk int) int {
	passedWeight := []int{0, 0, 0, Parms[16], Parms[17], Parms[18], Parms[19], 0} //{0, 0, 0, 2, 6, 12, 20, 0}
	return mulShift(sc, passedWeight[rk], Parms[20])                              // 4
}

// Imax returns maximum value of two ints
func Imax(a, b int) int {
	if a >= b {
		return a
	}
	return b
}

// Imin returns minimum value of two ints
func Imin(a, b int) int {
	if a <= b {
		return a
	}
	return b
}
