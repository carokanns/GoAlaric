package gen2

//namespace gen_sort {
//   // HACK: outside of List class because of C++ "static const" limitations :(
import (
	"GoAlaric/bit"
	"GoAlaric/board"
	"GoAlaric/eval"
	//	"GoAlaric/gen"
	"GoAlaric/move"
	"GoAlaric/piece"
	"GoAlaric/sort"
	"GoAlaric/square"
	"GoAlaric/util"
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
	todoList sort.ScMvList
	doneList sort.ScMvList
	badList  sort.ScMvList

	transMove int
	idx       int
	genSV     int
	postSV    int
	pos       int
	candidate bool

	board       *board.Board
	attacks     *eval.Attacks
	killerMoves *sort.Killer
	historyTab  *sort.HistoryTab
	progPointer *genSV
}

// Init the list before generating moves
func (l *List) Init(depth int, bd *board.Board, attacks *eval.Attacks, transMove int, killer *sort.Killer, history *sort.HistoryTab, useFP bool /* = false */) {
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

func (l *List) gen() bool {

	l.todoList.Clear()
	l.pos = 0

	switch l.genSV {

	case genEvasion:

		sort.AddEvasions(&l.todoList, l.board.Stm(), l.board, l.attacks)
		sort.Evasions(&l.todoList, l.transMove)

	case genTrans:

		mv := l.transMove

		if mv != move.None && board.IsMove(mv, l.board) {
			l.todoList.Add(mv)
		}

		l.candidate = true

	case genTactical:

		sort.AddCaptures(&l.todoList, l.board.Stm(), l.board)
		sort.AddProms(&l.todoList, l.board.Stm(), l.board.Empty(), l.board)
		sort.Tacticals(&l.todoList)

		l.candidate = true

	case genKiller:

		k0 := l.killerMoves.Killer1(l.board.Ply())

		if k0 != move.None && sort.IsQuiet(k0, l.board) {
			l.todoList.Add(k0)
		}

		k1 := l.killerMoves.Killer2(l.board.Ply())

		if k1 != move.None && sort.IsQuiet(k1, l.board) {
			l.todoList.Add(k1)
		}

		l.candidate = true

	case genCheck:

		sort.AddChecks(&l.todoList, l.board.Stm(), l.board)

		l.candidate = true // not needed yet

	case genPawn:

		sort.AddCastling(&l.todoList, l.board.Stm(), l.board)
		sort.PawnPushes(&l.todoList, l.board.Stm(), l.board)

		l.candidate = true // not needed yet

	case genQuiet:

		sort.AddQuietMoves(&l.todoList, l.board.Stm(), l.board)
		sort.History(&l.todoList, l.board, l.historyTab)

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

		if !IsSafe(mv, l.board) {
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

		if !IsSafe(mv, l.board) {
			l.badList.Add(mv)
			return false
		}

	case postBad:

		//util.ASSERT(Is_legal(mv, l.p_board, l.p_attacks))

	default:
		util.ASSERT(false)
	}

	return true
}

// Next move from list
func (l *List) Next() int {

	for true {
		for l.pos >= l.todoList.Size() {
			l.genSV = (*l.progPointer)[l.idx]
			l.idx++
			l.postSV = (*l.progPointer)[l.idx]
			l.idx++
			if !l.gen() {
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

	se.value = piece.Value[pc]
	se.color = sd
}

func (se *SEE) move(fr int) int {

	// assert(bit::is_set(p_all, fr));
	bit.Clear(&se.bbAlll, fr)

	pc := se.board.Square(fr)
	// assert(pc != piece::None && p_board->square_side(fr) == p_side);

	val := se.value
	se.value = piece.Value[pc]

	if pc == piece.Pawn && square.IsPromotion(se.to) {
		delta := piece.QueenValue - piece.PawnValue
		val += delta
		se.value += delta
	}

	if val == piece.KingValue { // stop at king capture
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

	capVal := se.move(fr) // NOTE: has side effect

	v, ct := se.seeRec(capVal-beta, capVal-alpha)

	s1 := capVal - v

	return int(math.Max(float64(s0), float64(s1))), ct + 1 //cnt+1 for the move above
}

func (se *SEE) pickLva() int {

	sd := se.color

	for pc := piece.Pawn; pc <= piece.King; pc++ {
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
	capVal := se.move(fr) // NOTE: assumes queen promotion

	if pc == piece.Pawn && square.IsPromotion(to) { // adjust for under-promotion
		delta := piece.QueenValue - piece.Value[move.Prom(mv)]
		capVal -= delta
		se.value -= delta
	}
	v, ct := se.seeRec(capVal-beta, capVal-alpha)
	return capVal - v, ct + 1 //cnt+1 for the move above
}

// IsSafe ...
func IsSafe(mv int, bd *board.Board) bool {

	pc := move.Piece(mv)
	cp := move.Capt(mv)
	pp := move.Prom(mv)

	if pc == piece.King {
		return true
	} else if piece.Value[cp] >= piece.Value[pc] {
		return true
	} else if pp != piece.None && pp != piece.Queen { // under-promotion
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
		return sort.IsLegalMv(mv, bd)
	}

	if move.Piece(mv) == piece.King {
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

	// assert(is_tactical(mv));

	pc := move.Piece(mv)
	cp := move.Capt(mv)
	pp := move.Prom(mv)

	if pc == piece.King {
		return true
	} else if piece.Value[cp] > piece.Value[pc] {
		return true
	} else if pp != piece.None && pp != piece.Queen { // when it is under-promotion
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
