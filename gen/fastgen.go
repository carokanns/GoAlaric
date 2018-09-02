package gen

//namespace gen_sort {
//   // HACK: outside of List class because of C++ "static const" limitations :(
import (
	"GoAlaric/bit"
	"GoAlaric/board"
	"GoAlaric/eval"
	"GoAlaric/material"
	"GoAlaric/move"
	"GoAlaric/square"
	"fmt"
	"math"
)

//
const (
	genEvasion = iota
	genTrans
	genTactical
	genKiller
	genCheck
	genPawn
	genQuiet
	genBad
	genEnd
	postMove
	postMoveSEE
	postKiller
	postKillerSEE
	postBad
)

type genSV []int

var (
	progMain    = genSV{genTrans, postKiller, genTactical, postMoveSEE, genKiller, postKillerSEE, genQuiet, postMoveSEE, genBad, postBad, genEnd, genEnd}
	progQSRoot  = genSV{genTrans, postKiller, genTactical, postMove, genCheck, postKiller, genPawn, postMove, genEnd, genEnd}
	progQS      = genSV{genTrans, postKiller, genTactical, postMove, genEnd, genEnd}
	progEvasion = genSV{genEvasion, postMoveSEE, genBad, postBad, genEnd, genEnd}
)

// List is the gen move table
type List struct {
	todoList ScMvList
	doneList ScMvList
	badList  ScMvList

	transMove int
	idx       int
	genSV     int
	postSV    int
	pos       int
	candidate bool

	board       *board.Board
	attacks     *eval.Attacks
	killerMoves *Killer
	historyTab  *HistoryTab
	progPointer *genSV
}

// Init the list before generating moves
func (l *List) Init(depth int, bd *board.Board, attacks *eval.Attacks, transMove int, killer *Killer, history *HistoryTab, useFP bool /* = false */) {
	l.board = bd
	l.attacks = attacks
	l.killerMoves = killer
	l.historyTab = history
	l.transMove = transMove

	if attacks.Size != 0 { // in check
		l.progPointer = &progEvasion
	} else if depth < 0 {
		l.progPointer = &progQS
	} else if depth == 0 {
		l.progPointer = &progQSRoot
	} else if useFP {
		l.progPointer = &progQSRoot
	} else {
		l.progPointer = &progMain
	}

	l.todoList.Clear()
	l.doneList.Clear()
	l.badList.Clear()

	l.idx = 0
	l.genSV = 0
	l.postSV = 0
	l.pos = 0
	l.candidate = false
}

// Here is where the moves are actually generated
func (l *List) gen() bool {
	l.todoList.Clear()
	l.pos = 0
	switch l.genSV {

	case genEvasion:

		AddEvasions(&l.todoList, l.board.Stm(), l.board, l.attacks)
		evasions(&l.todoList, l.transMove) // sort them

	case genTrans:

		mv := l.transMove

		if mv != move.None && board.IsMove(mv, l.board) {
			l.todoList.Add(mv)
		}

		l.candidate = true

	case genTactical:

		AddCaptures(&l.todoList, l.board.Stm(), l.board)
		AddProms(&l.todoList, l.board.Stm(), l.board.Empty(), l.board)
		Tacticals(&l.todoList)

		l.candidate = true

	case genKiller:

		k0 := l.killerMoves.Killer1(l.board.Ply())

		if k0 != move.None && IsQuiet(k0, l.board) {
			l.todoList.Add(k0)
		}

		k1 := l.killerMoves.Killer2(l.board.Ply())

		if k1 != move.None && IsQuiet(k1, l.board) {
			l.todoList.Add(k1)
		}

		l.candidate = true

	case genCheck:

		AddChecks(&l.todoList, l.board.Stm(), l.board)

		l.candidate = true // not needed yet

	case genPawn:

		AddCastling(&l.todoList, l.board.Stm(), l.board)
		PawnPushes(&l.todoList, l.board.Stm(), l.board)

		l.candidate = true // not needed yet

	case genQuiet:

		AddQuietMoves(&l.todoList, l.board.Stm(), l.board)
		l.historyTab.Sort(&l.todoList, l.board)

		l.candidate = false
	case genBad:

		l.todoList = l.badList

		l.candidate = false
	case genEnd:

		return false

	default:

		// assert(false);

	}

	return true
}

// post makes post move functions
func (l *List) post(mv int) bool {

	//util.ASSERT(mv != move.None)

	switch l.postSV {

	case postMove:

		if l.doneList.Contain(mv) {
			return false
		}

		if !isLegal(mv, l.board, l.attacks) {
			return false
		}

	case postMoveSEE:

		if l.doneList.Contain(mv) {
			return false
		}

		if !isLegal(mv, l.board, l.attacks) {
			return false
		}

		if !NoSacrifice(mv, l.board) {
			l.badList.Add(mv)
			return false
		}

	case postKiller:

		if l.doneList.Contain(mv) {
			return false
		}

		if !isLegal(mv, l.board, l.attacks) {
			return false
		}

		l.doneList.Add(mv)

	case postKillerSEE:

		if l.doneList.Contain(mv) {
			return false
		}

		if !isLegal(mv, l.board, l.attacks) {
			return false
		}

		l.doneList.Add(mv)

		if !NoSacrifice(mv, l.board) {
			l.badList.Add(mv)
			return false
		}

	case postBad:

		//util.ASSERT(Is_legal(mv, l.p_board, l.p_attacks))

	default:
		panic("don't go here List.post")
	}

	return true
}

// Next move from list
//   gen adds to Size for each generated move.
//   l.pos adds one for each used move.
//   l.idx is the index to the next phase (SV=Status Variable) of moves (checks, tatical etc)
func (l *List) Next() int {
	// 	progMain    = genSV{ genTrans, postKiller, genTactical, postMoveSEE, genKiller, postKillerSEE, genQuiet, postMoveSEE, genBad, postBad, genEnd, genEnd}

	for true {
		for l.pos >= l.todoList.Size() { // we have no unused generated moves
			l.genSV = (*l.progPointer)[l.idx] // set next SV
			l.idx++
			l.postSV = (*l.progPointer)[l.idx] // set next SV
			l.idx++
			if !l.gen() { // generate more moves (sometimes not for that SV!)
				return move.None
			}
		}

		mv := l.todoList.Move(l.pos)
		l.pos++
		if l.post(mv) {
			return mv
		}
	}
	//panic("skall inte komma hit")
	return move.None
}

// Candidate returns true if we are in SV for moves that candidates to be good
func (l *List) Candidate() bool {
	return l.candidate
}

// SEE is the struct holding values needed for the See algorithm
type SEE struct {
	board  *board.Board
	to     int
	bbAlll bit.BB

	value int
	color int
}

func (se *SEE) init(to int, sd int) {

	se.to = to
	se.bbAlll = se.board.All()

	pc := se.board.Square(to)

	se.value = material.Value[pc]
	se.color = sd
}

func (se *SEE) moveVal(fr int) int {

	// assert(bit::is_set(p_all, fr));
	bit.Clear(&se.bbAlll, fr)

	pc := se.board.Square(fr)
	// assert(pc != piece::None && p_board->square_side(fr) == p_side);

	val := se.value
	se.value = material.Value[pc]

	if pc == material.Pawn && square.IsPromotion(se.to) {
		delta := material.QueenValue - material.PawnValue
		val += delta
		se.value += delta
	}

	if val == material.KingValue { // stop at king capture
		se.bbAlll = 0 // HACK: erase all attackers
	}

	se.color = board.Opposit(se.color)

	return val
}

// see_rec makes recursive calls to evluate see
func (se *SEE) seeRec(alpha, beta int) (val, cnt int) {

	// assert(alpha < beta);

	s0 := 0

	if s0 > alpha {
		alpha = s0
		if s0 >= beta {
			return s0, 0
		}
	}

	if se.value <= alpha { // FP, NOTE: fails for promotions
		return se.value, 0
	}

	fr := se.pickLva()

	if fr == square.None {
		return s0, 0
	}

	capVal := se.moveVal(fr) // NOTE: has side effect

	v, ct := se.seeRec(capVal-beta, capVal-alpha)

	s1 := capVal - v

	return int(math.Max(float64(s0), float64(s1))), ct + 1 //cnt+1 for the move above
}

func (se *SEE) pickLva() int {

	sd := se.color

	for pc := material.Pawn; pc <= material.King; pc++ {
		fs := se.board.PieceSd(pc, sd) & eval.PseudoAttacksTo(pc, sd, se.to) & se.bbAlll

		for b := fs; b != 0; b = bit.Rest(b) {

			fr := bit.First(b)

			if (se.bbAlll & eval.Between[fr][se.to]) == 0 {
				return fr
			}
		}
	}

	return square.None
}

// See (Static Exchange Evaluation) returns static material value of a move
func (se *SEE) See(mv int, alpha int, beta int, bd *board.Board) (val, cnt int) {

	// assert(alpha < beta)

	se.board = bd

	fr := move.From(mv)
	to := move.To(mv)

	pc := se.board.Square(fr)
	sd := se.board.SquareSide(fr)

	se.init(to, sd)
	capVal := se.moveVal(fr) // NOTE: assume queen promotion to start with

	if pc == material.Pawn && square.IsPromotion(to) { // adjust for under-promotion
		delta := material.QueenValue - material.Value[move.Prom(mv)]
		capVal -= delta
		se.value -= delta
	}
	v, ct := se.seeRec(capVal-beta, capVal-alpha)
	return capVal - v, ct + 1 //cnt+1 for the move above
}

// NoSacrifice is true if the move doesn't directly sacrifice itself
func NoSacrifice(mv int, bd *board.Board) bool {

	pc := move.Piece(mv)
	cp := move.Capt(mv)
	pp := move.Prom(mv)

	if pc == material.King {
		return true
	} else if material.Value[cp] >= material.Value[pc] {
		return true
	} else if pp != material.None && pp != material.Queen { // under-promotion
		return false
	}

	var se SEE
	val, _ := se.See(mv, -1, 0, bd)
	return val >= 0
}

func isLegal(mv int, bd *board.Board, attacks *eval.Attacks) bool {

	sd := bd.Stm()

	fr := move.From(mv)
	to := move.To(mv)

	if eval.IsEnPassant(mv, bd) {
		return IsLegalMv(mv, bd)
	}

	if move.Piece(mv) == material.King {
		return !eval.IsAttacked(to, board.Opposit(sd), bd)
	}

	if !bit.IsOne(attacks.Pinned, fr) {
		return true
	}

	if bit.IsOne(eval.Ray(bd.King(sd), fr), to) {
		return true
	}

	return false
}

// IsWin returns true if a move is maybe winning
func IsWin(mv int, bd *board.Board) bool {

	pc := move.Piece(mv)
	cp := move.Capt(mv)
	pp := move.Prom(mv)

	if pc == material.King {
		return true
	} else if material.Value[cp] > material.Value[pc] {
		return true
	} else if pp != material.None && pp != material.Queen { // when it is under-promotion
		return false
	}

	var se SEE
	val, _ := se.See(mv, 0, +1, bd)
	return val > 0

}

// IsRecapture checks if a move is a recapture
func IsRecapture(mv int, bd *board.Board) bool {
	return move.To(mv) == bd.Recap() && IsWin(mv, bd)
}

// evasions is sorting evasion moves
func evasions(ml *ScMvList, transMv int) {

	for pos := 0; pos < ml.Size(); pos++ {
		ml.SetScore(pos, evasionScore(ml.Move(pos), transMv))
	}

	ml.Sort()
}

// evasionScore sets a score for an evasion move
func evasionScore(mv, transMv int) int {

	if mv == transMv {
		return move.ScoreMask
	} else if move.IsTactical(mv) {
		return tacticalScore(move.Piece(mv), move.Capt(mv), move.Prom(mv)) + 1
		// assert(sc >= 1 && sc < 41)
	}
	return 0
}

func tacticalScore(pc, cp, pp int) int {

	if cp != material.None {
		return captScore(pc, cp) + 4
	}
	return promotionScore(pp)

}

func captScore(pc, cp int) int {
	sc := cp*6 + (5 - pc)
	// assert(sc >= 0 && sc < 36);
	return sc
}

func promotionScore(pp int) int {
	switch pp {
	case material.Queen:
		return 3
	case material.Knight:
		return 2
	case material.Rook:
		return 1
	case material.Bishop:
		return 0
	default:
		// assert(false)
		return 0
	}
}

// Tacticals is sorting tactical moves
func Tacticals(ml *ScMvList) {

	for pos := 0; pos < ml.Size(); pos++ {
		mv := ml.Move(pos)
		sc := tacticalScore(move.Piece(mv), move.Capt(mv), move.Prom(mv))
		ml.SetScore(pos, sc)
	}

	ml.Sort()
}

// IsQuiet returns true if the given move is legal and not capture and not prom
func IsQuiet(mv int, bd *board.Board) bool {

	sd := bd.Stm()

	fr := move.From(mv)
	to := move.To(mv)

	pc := move.Piece(mv)
	// assert(move.cap(mv) == material.None);
	// assert(move.prom(mv) == material.None);

	if !(bd.Square(fr) == pc && bd.SquareSide(fr) == sd) {
		return false
	}

	if bd.Square(to) != material.None {
		return false
	}

	if pc == material.Pawn {

		inc := square.PawnInc(sd)

		if to-fr == inc && !square.IsPromotion(to) {
			return true
		} else if to-fr == inc*2 && square.RankSd(fr, sd) == square.Rank2 {
			return bd.Square(fr+inc) == material.None
		}
		return false
	}

	return eval.PieceAttack(pc, fr, to, bd)
}

// LegalMoves is generating pseudomoves and selecting the legal ones
func LegalMoves(ml *ScMvList, bd *board.Board) {
	var pseudos ScMvList
	genPseudos(&pseudos, bd)
	selectLegals(ml, &pseudos, bd)
}

func selectLegals(legals, src *ScMvList, bd *board.Board) {

	legals.Clear()

	for pos := 0; pos < src.Size(); pos++ {

		mv := src.Move(pos)

		if IsLegalMv(mv, bd) {
			legals.Add(mv)
		}
	}
}

// IsLegalMv returns true if the given move doesnt leave the king in chess
func IsLegalMv(mv int, bd *board.Board) bool {

	bd.Move(mv)

	sd := bd.Stm() // new new side

	answer := !eval.IsAttacked(bd.King(sd^1), sd, bd)

	bd.Undo()

	return answer
}

const maxSize = 256

// ScMvList holds a list of moves with score integrated
type ScMvList struct {
	size int
	scMv [maxSize]uint32 // 21 bits score + 11 bits mv
}

// Clear the Score/Move List by setting its size to 0
func (list *ScMvList) Clear() { list.size = 0 }

// Size returns the current size of the Score/Move list
func (list *ScMvList) Size() int {
	return list.size
}

// Move returns the move part from the Score/Move list
func (list *ScMvList) Move(pos int) int {
	return int(list.scMv[pos] & uint32(move.Mask))
}

// Add a move t0 the Score/Move list with the score set to 0
func (list *ScMvList) Add(mv int) {
	// assert(mv >= 0 && mv < move.SIZE);
	// assert(sc >= 0 && sc < move.SCORE_SIZE);
	// assert(!contain(mv));
	list.addPair((uint32(0) << uint32(move.Bits)) | uint32(mv))
}

func (list *ScMvList) addPair(pair uint32) {
	// assert(p_size < SIZE);
	list.scMv[list.size] = pair
	list.size++
}

// Score returns the score part in a Score/Move pair
func (list *ScMvList) Score(pos int) int {
	return int(list.scMv[pos] >> uint(move.Bits))
}

// MoveToFront puts the entry in pos to the top
func (list *ScMvList) MoveToFront(pos int) {
	list.moveTo(pos, 0)
}
func (list *ScMvList) moveTo(frPos, toPos int) {

	// assert(pt <= pf && pf < p_size)

	scMv := list.scMv[frPos]

	for i := frPos; i > toPos; i-- {
		list.scMv[i] = list.scMv[i-1]
	}

	list.scMv[toPos] = scMv
}

// SetScore sets the score paired with a move in the Score/Move list
func (list *ScMvList) SetScore(pos int, sc int) {
	//util.ASSERT(pos < l.p_size)
	//util.ASSERT(sc >= 0 && sc < move.SCORE_SIZE)
	list.scMv[pos] = uint32((sc << uint32(move.Bits)) | list.Move(pos))
	//util.ASSERT(l.Score(pos) == sc)
}

// Sort is sorting the moves in the Score/Move list according to the score per move
func (list *ScMvList) Sort() {
	bSwap := true
	for bSwap {
		bSwap = false
		for i := 0; i < list.size-1; i++ {
			if list.scMv[i+1] > list.scMv[i] {
				list.scMv[i], list.scMv[i+1] = list.scMv[i+1], list.scMv[i]
				bSwap = true
			}
		}
	}
	for i := 0; i < list.size-1; i++ {
		//util.ASSERT(l.p_pair[i] >= l.p_pair[i+1])
	}
}

// Contain returns true if we can find the given move in the Score/Move list
func (list *ScMvList) Contain(mv int) bool {

	for pos := 0; pos < list.Size(); pos++ {
		if list.Move(pos) == mv {
			return true
		}
	}

	return false
}

// AddEvasions is adding King evasions to the Score/Move list
func AddEvasions(ml *ScMvList, sd int, bd *board.Board, attacks *eval.Attacks) {

	// assert(attacks.size > 0);

	king := bd.King(sd)

	pieceMovesFr(ml, king, ^bd.Side(sd) & ^attacks.Avoid, bd)

	if attacks.Size == 1 {

		to := attacks.Square[0]

		addEvasionCaptures(ml, sd, to, bd)
		addEnPassant(ml, sd, bd)

		ts := eval.Between[king][to]
		// assert(eval.line_is_empty(king, to, bd));

		if ts != 0 {
			addPawnQuiets(ml, sd, ts, bd)
			AddProms(ml, sd, ts, bd)
			pieceEvasionMoves(ml, sd, ts, bd)
		}
	}
}

// AddCaptures is adding captures to the Score/Move list
func AddCaptures(ml *ScMvList, sd int, bd *board.Board) {

	ts := bd.Side(board.Opposit(sd))

	addPawnCaptures(ml, sd, ts, bd)
	addPieceCaptures(ml, sd, ts, bd)
	addEnPassant(ml, sd, bd)
}

// addEvasionCaptures add captures (not king captuers) for King evasion
func addEvasionCaptures(ml *ScMvList, sd int, to int, bd *board.Board) {
	for pc := material.Pawn; pc <= material.Queen; pc++ { // skip king
		for bb := bd.PieceSd(pc, sd) & eval.AttacksTo(pc, sd, to, bd); bb != 0; bb = bit.Rest(bb) {
			fr := bit.First(bb)
			addMove(ml, fr, to, bd)
		}
	}
}

// PawnPushes is adding pawn pushes to the Score/Move list
func PawnPushes(ml *ScMvList, sd int, bd *board.Board) {

	ts := bit.BB(0)

	if sd == board.WHITE {

		ts |= bit.Rank(square.Rank7)
		ts |= bit.Rank(square.Rank6) & ^eval.PawnAttacksFrom(board.BLACK, bd) & (^bd.Piece(material.Pawn) >> 1) // HACK: direct access

	} else {

		ts |= bit.Rank(square.Rank2)
		ts |= bit.Rank(square.Rank3) & ^eval.PawnAttacksFrom(board.WHITE, bd) & (^bd.Piece(material.Pawn) << 1) // HACK: direct access
	}

	addPawnQuiets(ml, sd, ts&bd.Empty(), bd)
}

func canCastle(sd int, wg int, bd *board.Board) bool {

	index := board.CastleIndex(sd, wg)

	if board.CastleFlag(bd.Flags(), uint(index)) {

		kf := board.CastleInfo[index].KingFr
		// int kt = board.info[index].kt;
		rf := board.CastleInfo[index].RookFr
		rt := board.CastleInfo[index].RokTo

		// assert(bd.square_is(kf, material.King, sd))
		// assert(bd.square_is(rf, material.Rook, sd))

		return eval.LineIsEmpty(kf, rf, bd) && !eval.IsAttacked(rt, board.Opposit(sd), bd)
	}

	return false
}

func genPseudos(ml *ScMvList, bd *board.Board) {

	ml.Clear()

	sd := bd.Stm()

	if eval.IsInCheck(bd) {
		var attacks eval.Attacks
		eval.InitAttacks(&attacks, sd, bd)
		AddEvasions(ml, sd, bd, &attacks)
	} else {
		AddCaptures(ml, sd, bd)
		AddProms(ml, sd, bd.Empty(), bd)
		AddQuietMoves(ml, sd, bd)
	}
}
func addMove(ml *ScMvList, fr, to int, bd *board.Board) {
	if bd.Square(fr) == material.Pawn {
		addPawnMv(ml, fr, to, bd)
	} else {
		addPieceMv(ml, fr, to, bd)
	}
}

// AddChecks to the Score/Move list
func AddChecks(ml *ScMvList, sd int, bd *board.Board) {

	opp := sd ^ 1

	king := bd.King(opp)
	pinned := eval.PinnedBy(king, sd, bd)
	empty := bd.Empty()
	empty &= ^eval.PawnAttacksFrom(opp, bd) // pawn-safe

	// discovered checks

	for fs := bd.Pieces(sd) & pinned; fs != 0; fs = bit.Rest(fs) { // TODO: pawns
		fr := bit.First(fs)
		ts := empty & ^eval.Ray(king, fr) // needed only for pawns
		pieceMovesFr(ml, fr, ts, bd)
	}

	// direct checks, pawns

	{
		ts := eval.PseudoAttacksTo(material.Pawn, sd, king) & empty

		addPawnQuiets(ml, sd, ts, bd)
	}

	// direct checks, knights

	{
		pc := material.Knight

		attacks := eval.PseudoAttacksTo(pc, sd, king) & empty

		for b := bd.PieceSd(pc, sd) & ^pinned; b != 0; b = bit.Rest(b) {

			fr := bit.First(b)

			moves := eval.PseudoAttacksFrom(pc, sd, fr)

			for bb := moves & attacks; bb != 0; bb = bit.Rest(bb) {
				to := bit.First(bb)
				addPieceMv(ml, fr, to, bd)
			}
		}
	}

	// direct checks, sliders

	for pc := material.Bishop; pc <= material.Queen; pc++ {

		attacks := eval.PseudoAttacksTo(pc, sd, king) & empty

		for b := bd.PieceSd(pc, sd) & ^pinned; b != 0; b = bit.Rest(b) {

			fr := bit.First(b)

			moves := eval.PseudoAttacksFrom(pc, sd, fr)

			for bb := moves & attacks; bb != 0; bb = bit.Rest(bb) {

				to := bit.First(bb)

				if eval.LineIsEmpty(fr, to, bd) && eval.LineIsEmpty(to, king, bd) {
					addPieceMv(ml, fr, to, bd)
				}
			}
		}
	}
}

func addPieceMoves(ml *ScMvList, sd int, ts bit.BB, bd *board.Board) {

	// assert(ts != 0);
	for pc := material.Knight; pc <= material.King; pc++ {

		for b := bd.PieceSd(pc, sd); b != 0; b = bit.Rest(b) {

			fr := bit.First(b)

			pieceMovesFr(ml, fr, ts, bd)
		}
	}
}

// pieceMovesFr is adding piece moves from the given square to the Score/Move list
func pieceMovesFr(ml *ScMvList, fr int, ts bit.BB, bd *board.Board) {

	pc := bd.Square(fr)

	for b := eval.PieceAttacksFrom(pc, fr, bd) & ts; b != 0; b = bit.Rest(b) {
		to := bit.First(b)
		//		fmt.Printf("  Fr: %v    To: %v\n", square.To_string(fr), square.To_string(to))
		addPieceMv(ml, fr, to, bd)
		//		mv := ml.Move(ml.Size() - 1)
		//		fmt.Printf("mvFr: %v, mvTo: %v\n", square.To_string(move.From(mv)), square.To_string(move.To(mv)))
	}
}

// pieceEvasionMoves is adding evasion moves (not from the king) to the Score/Move list
func pieceEvasionMoves(ml *ScMvList, sd int, ts bit.BB, bd *board.Board) { // for evasions but not for the king

	// assert(ts != 0);

	for pc := material.Knight; pc <= material.Queen; pc++ { // skip king
		for b := bd.PieceSd(pc, sd); b != 0; b = bit.Rest(b) {
			fr := bit.First(b)
			pieceMovesFr(ml, fr, ts, bd)
		}
	}
}
func addPieceCaptures(ml *ScMvList, sd int, ts bit.BB, bd *board.Board) {

	// assert(ts != 0);

	for pc := material.Knight; pc <= material.King; pc++ {

		for b := bd.PieceSd(pc, sd); b != 0; b = bit.Rest(b) {

			fr := bit.First(b)

			for bb := eval.PseudoAttacksFrom(pc, sd, fr) & ts; bb != 0; bb = bit.Rest(bb) {

				to := bit.First(bb)

				if eval.LineIsEmpty(fr, to, bd) {
					addPieceMv(ml, fr, to, bd)
				}
			}
		}
	}
}

// AddCastling adding castling moves to the Score/Mv list
func AddCastling(ml *ScMvList, sd int, bd *board.Board) {

	for wg := 0; wg < 2; wg++ {
		if canCastle(sd, wg, bd) {
			index := board.CastleIndex(sd, wg)
			addPieceMv(ml, board.CastleInfo[index].KingFr, board.CastleInfo[index].KingTo, bd)
		}
	}
}

// AddProms is adding promotion moves to the Score/Mv list
func AddProms(ml *ScMvList, sd int, ts bit.BB, bd *board.Board) {

	pawns := bd.PieceSd(material.Pawn, sd)
	if sd == board.WHITE {
		for b := pawns & (ts >> 1) & bit.Rank(square.Rank7); b != 0; b = bit.Rest(b) {
			fr := bit.First(b)
			to := fr + 1
			//util.ASSERT(bd.Square(to) == material.None)
			// //util.ASSERT(square.is_promotion(to));
			addPawnMv(ml, fr, to, bd)
		}

	} else {
		for b := pawns & (ts << 1) & bit.Rank(square.Rank2); b != 0; b = bit.Rest(b) {
			fr := bit.First(b)
			to := fr - 1
			//util.ASSERT(bd.Square(to) == material.None)
			// //util.ASSERT(square.is_promotion(to));
			addPawnMv(ml, fr, to, bd)
		}
	}

}
func addPieceMv(ml *ScMvList, fr int, to int, bd *board.Board) {
	// assert(bd.square(fr) != material.PAWN);
	ml.Add(move.Build(fr, to, bd.Square(fr), bd.Square(to), material.None))
}
func addPawnMv(ml *ScMvList, fr, to int, bd *board.Board) {

	// assert(bd.square(fr) == material.PAWN);

	pc := bd.Square(fr)
	cp := bd.Square(to)

	if square.IsPromotion(to) {
		ml.Add(move.Build(fr, to, pc, cp, material.Queen))
		ml.Add(move.Build(fr, to, pc, cp, material.Knight))
		ml.Add(move.Build(fr, to, pc, cp, material.Rook))
		ml.Add(move.Build(fr, to, pc, cp, material.Bishop))
	} else {
		ml.Add(move.Build(fr, to, pc, cp, material.None))
	}
}
func addEnPassant(ml *ScMvList, sd int, bd *board.Board) {

	to := bd.EpSq()

	if to != square.None {

		fs := bd.PieceSd(material.Pawn, sd) & eval.PawnAttacks[board.Opposit(sd)][to]

		for b := fs; b != 0; b = bit.Rest(b) {
			fr := bit.First(b)
			ml.Add(move.Build(fr, to, material.Pawn, material.Pawn, material.None))
		}
	}
}

// AddQuietMoves is doing just that to the Score/Mv list
func AddQuietMoves(ml *ScMvList, sd int, bd *board.Board) {
	AddCastling(ml, sd, bd)
	addPieceMoves(ml, sd, bd.Empty(), bd)
	addPawnQuiets(ml, sd, bd.Empty(), bd)
}
func addPawnQuiets(ml *ScMvList, sd int, ts bit.BB, bd *board.Board) {

	pawns := bd.PieceSd(material.Pawn, sd)
	empty := bd.Empty()

	if sd == board.WHITE {

		for b := pawns & (ts >> 1) & (empty >> 1) & ^bit.Rank(square.Rank7); b != 0; b = bit.Rest(b) { // don'to generate promotions
			fr := bit.First(b)
			to := fr + 1
			// assert(bd.square(to) == material.None);
			// assert(!square.is_promotion(to));
			addPawnMv(ml, fr, to, bd)
		}
		// for (b := pawns & (ts >> 2) & (empty >> 1) & bit.Rank(square.Rank2); b != 0; b = bit.Rest(b)) {
		for b := pawns & (ts >> 2) & (empty >> 1) & bit.Rank(square.Rank2); b != 0; b = bit.Rest(b) {
			fr := bit.First(b)
			to := fr + 2
			// assert(bd.square(to) == material.None);
			// assert(!square.is_promotion(to));
			addPawnMv(ml, fr, to, bd)
		}

	} else {
		for b := pawns & (ts << 1) & ^bit.Rank(square.Rank2); b != 0; b = bit.Rest(b) { // don'to generate promotions
			fr := bit.First(b)
			to := fr - 1
			// assert(bd.square(to) == material.None);
			// assert(!square.is_promotion(to));
			addPawnMv(ml, fr, to, bd)
		}

		for b := pawns & (ts << 2) & (empty << 1) & bit.Rank(square.Rank7); b != 0; b = bit.Rest(b) {
			fr := bit.First(b)
			to := fr - 2
			// assert(bd.square(to) == material.None);
			// assert(!square.is_promotion(to));
			addPawnMv(ml, fr, to, bd)
		}
	}
}
func addPawnCaptures(ml *ScMvList, sd int, ts bit.BB, bd *board.Board) {

	pawns := bd.PieceSd(material.Pawn, sd)
	ts &= bd.Side(board.Opposit(sd)) // not needed

	if sd == board.WHITE {

		for b := (ts << 7) & pawns; b != 0; b = bit.Rest(b) {
			fr := bit.First(b)
			to := fr - 7
			addPawnMv(ml, fr, to, bd)
		}

		for b := (ts >> 9) & pawns; b != 0; b = bit.Rest(b) {
			fr := bit.First(b)
			to := fr + 9
			addPawnMv(ml, fr, to, bd)
		}

	} else {

		for b := (ts << 9) & pawns; b != 0; b = bit.Rest(b) {
			fr := bit.First(b)
			to := fr - 9
			addPawnMv(ml, fr, to, bd)
		}

		for b := (ts >> 7) & pawns; b != 0; b = bit.Rest(b) {
			fr := bit.First(b)
			to := fr + 7
			addPawnMv(ml, fr, to, bd)
		}
	}
}

/////////////// för tester ////////////////////////////////////

// PrintAllMoves printes all moves in the move list ml
func PrintAllMoves(ml *ScMvList) {
	fmt.Printf("antal=%v, ", ml.Size())
	for pos := 0; pos < ml.Size(); pos++ {
		mv := ml.Move(pos)
		strMove := move.ToString(mv)
		fmt.Print(strMove + " ")
	}
	fmt.Println()
}
