package board

import (

	// "GoAlaric/attack"    // går ej!!
	"GoAlaric/bit"
	"GoAlaric/castling"
	// "GoAlaric/gen2"	  // går ej!!
	"GoAlaric/hash"
	"GoAlaric/material"
	"GoAlaric/move"
	"GoAlaric/piece"
	"GoAlaric/square"
	"GoAlaric/util"
	"fmt"

	"strconv"
	"strings"
)

// White is 0 an Black is 1
const (
	WHITE int = iota
	BLACK
)

// Stages Middlegame, Endgame
const (
	MG int = iota
	EG
)

// The two wings of the board
const (
	WingKING  int = iota // Kungsflygeln
	WingQUEEN            // Damflygeln
)
const scoreNONE int = -10000 // HACK because "score.None" is defined later

type copyStruct struct {
	key     hash.Key
	pawnKey hash.Key
	flags   int
	epSq    int
	moves   int
	recap   int
	phase   int
}

// Undo is the undo struct used to unmake a move
type Undo struct {
	copyS    copyStruct
	move     int
	capSq    int
	castling bool
}

// Board struct is holding a position and all its varables
type Board struct {
	piece [piece.Size]bit.BB // bb per piece
	side  [2]bit.BB          // bb per side
	all   bit.BB             // bb alla pieces

	king  [2]int              // kungpos per side
	count [piece.SideSize]int // counter per piece - varannan vit/svart

	square  [square.BoardSize]int // bräde vridet 90 grader höger
	stm     int                   // vem är vid draget
	copyStr copyStruct

	rootIx  int
	stackIx int
	stack   [1024]Undo
}

// Opposit color - either side to move or piece color
func Opposit(sd int) int {
	return sd ^ 1
}

// SetFen makes en internal board position from the fen-string
func SetFen(fen string, bd *Board) {

	bd.clear()
	pos := 0
	sq := 0

	// Brädet
	for ; pos < len(fen); pos++ {
		c := fen[pos : pos+1]
		//pos++
		if c == " " {
			pos++
			break
		} else if c == "/" {
			continue
		} else {
			i, err := strconv.Atoi(c)
			if err == nil {
				sq += i
			} else {
				// assume piece
				p12 := piece.FromFen(c) //en int från "pPnN..."
				pc := piece.Piece(p12)  // shift >> 1
				sd := piece.Color(p12)  //  & 1
				bd.setSquare(pc, sd, square.FromFen(sq), true)
				sq++
			}
		}
	}
	// vem drar

	bd.stm = WHITE

	if pos < len(fen) {

		bd.stm = strings.IndexAny("wb", fen[pos:pos+1])
		pos++
		if pos < len(fen) {
			// fen[pos] skall vara " "  här
			pos++
		}
	}

	bd.copyStr.key ^= hash.StmKey(bd.stm)

	// castling rights
	bd.copyStr.flags = 0

	if pos < len(fen) {
		for pos < len(fen) {
			c := fen[pos : pos+1]
			pos++
			if c == " " {
				break
			}
			if c == "-" {
				continue
			}

			i := strings.IndexAny("KQkq", c)

			if bd.castleOk(i) {
				castling.SetFlag(&bd.copyStr.flags, uint(i))
			}
		}
	} else { // guess from position
		for i := 0; i < 4; i++ {
			if bd.castleOk(i) {
				castling.SetFlag(&bd.copyStr.flags, uint(i))
			}
		}
	}

	// en-passant
	bd.copyStr.epSq = square.None

	if pos < len(fen) {

		epString := ""

		for pos < len(fen) {
			c := fen[pos : pos+1]
			pos++
			if c == " " {
				break
			}

			epString += c
		}

		if epString != "-" {
			sq := square.FromString(epString)
			if bd.pawnIsAttacked(sq, bd.stm) {
				bd.copyStr.epSq = sq
			}

		}
	}

	bd.update()

}

// FenMoves makes the moves from the position command
func FenMoves(moves []string, bd *Board) {

	for _, strmove := range moves {
		// hämtat från move.fromString
		if len(strings.TrimSpace(strmove)) == 0 {
			continue
		}
		mv := FromString(strmove, bd)

		bd.MakeFenMve(mv) //board.move namn-konflikt med move package
	}
}

// StartFen is the starting position in chess
const StartFen = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq -"

// FromString converts a string-move to an internal move
func FromString(strmve string, bd *Board) int {
	fr := square.FromString(strmve[:2])
	to := square.FromString(strmve[2:4])
	prom := piece.None
	if len(strmve) > 4 {
		prom = piece.FromChar(strings.ToUpper(strmve[4:5]))
	}
	return make(fr, to, prom, bd)
}
func make(fr int, to int, pp int, bd *Board) int {
	///////////  OBS!!!!  måste board class (bd board.Board) skickas med?
	pc := bd.Square(fr)
	cp := bd.Square(to)

	if pc == piece.Pawn && to == bd.EpSq() /*bd.epSq()*/ {
		cp = piece.Pawn
	}

	if pc == piece.Pawn && square.IsPromotion(to) && pp == piece.None { // not needed
		pp = piece.Queen
	}

	return move.Build(fr, to, pc, cp, pp)
}

//////////////////////////////////////////////
//  Tilhör class Board  (I princip allt unde namespace)
/////////////////////////////////////////////

func (bd *Board) update() {
	bd.all = bd.side[WHITE] | bd.side[BLACK]
}

// SetRoot sets the rootIx
func (bd *Board) SetRoot() {
	bd.rootIx = bd.stackIx
}
func (bd *Board) castleOk(index int) bool {

	sd := castling.Side(index)

	return bd.SquareIs(castling.Info[index].KingFr, piece.King, sd) && bd.SquareIs(castling.Info[index].RookFr, piece.Rook, sd)
}
func (bd *Board) setSquare(pc int, sd int, sq int, updateCopy bool) {

	//util.ASSERT(pc < piece.SIZE);
	//util.ASSERT(pc != piece.None);
	//util.ASSERT(sq >= 0 && sq < square.SIZE);

	//util.ASSERT(!bit.is_set(p_piece[pc], sq));
	bit.Set(&bd.piece[pc], sq)

	//util.ASSERT(!bit.is_set(p_side[sd], sq));
	bit.Set(&bd.side[sd], sq)

	//util.ASSERT(p_square[sq] == piece.None);
	bd.square[sq] = pc

	if pc == piece.King {
		bd.king[sd] = sq
	}

	p12 := piece.MakeP12(pc, sd)

	bd.count[p12]++

	if updateCopy {

		HashKey := hash.PieceKey(p12, sq)
		bd.copyStr.key ^= HashKey
		if pc == piece.Pawn {
			bd.copyStr.pawnKey ^= HashKey
		}

		bd.copyStr.phase += material.Phase(pc)
	}
}

// Count returns the number of the given piece with the given color
func (bd *Board) Count(pc, sd int) int {
	// util.ASSERT(pc < piece::SIZE);
	// util.ASSERT(pc != piece::None);
	// return bit::count(piece(pc, sd));
	return bd.count[piece.MakeP12(pc, sd)]
}

// MoveNull makes a NULL move
func (bd *Board) MoveNull() {

	//util.ASSERT(bd.p_sp < 1024)
	undo := &bd.stack[bd.stackIx]
	bd.stackIx++
	undo.move = move.Null

	undo.copyS = bd.copyStr

	bd.flipStm()
	bd.copyStr.epSq = square.None
	bd.copyStr.moves = 0 // HACK: conversion
	bd.copyStr.recap = square.None

	bd.update()
}

// UndoNull takes back a Null move
func (bd *Board) UndoNull() {

	//util.ASSERT(bd.p_sp > 0)
	bd.stackIx--
	undo := bd.stack[bd.stackIx]

	//util.ASSERT(undo.move == move.NULL_, "undo.move=", move.To_string(undo.move))

	bd.flipStm()
	bd.copyStr = undo.copyS

	bd.update()
}

func (bd *Board) clear() {
	for pc := 0; pc < piece.Size; pc++ {
		bd.piece[pc] = 0
	}

	for sd := 0; sd < 2; sd++ {
		bd.side[sd] = 0
	}

	for sq := 0; sq < square.BoardSize; sq++ {
		bd.square[sq] = piece.None
	}

	for sd := 0; sd < 2; sd++ {
		bd.king[sd] = square.None
	}

	for p12 := 0; p12 < piece.SideSize; p12++ {
		bd.count[p12] = 0
	}

	bd.stm = WHITE

	bd.copyStr.key = 0
	bd.copyStr.pawnKey = 0
	bd.copyStr.flags = 0
	bd.copyStr.epSq = square.None
	bd.copyStr.moves = 0
	bd.copyStr.recap = square.None
	bd.copyStr.phase = 0

	bd.rootIx = 0
	bd.stackIx = 0
}

// SquareIs returns true if the square is the given piece with the givien color
func (bd *Board) SquareIs(sq, pc, sd int) bool {
	//util.ASSERT(pc != piece.None)
	return bd.square[sq] == pc && bd.SquareSide(sq) == sd
}
func (bd *Board) pawnIsAttacked(sq, sd int) bool {

	fl := square.File(sq)
	sq -= square.PawnInc(sd)

	return (fl != square.FileA && bd.SquareIs(sq+square.IncLeft, piece.Pawn, sd)) ||
		(fl != square.FileH && bd.SquareIs(sq+square.IncRight, piece.Pawn, sd))
}

// SquareSide returns the color of the piece on given square
func (bd *Board) SquareSide(sq int) int {
	//util.ASSERT(p_square[sq] != piece.None);
	return int((bd.side[BLACK] >> uint(sq)) & 1) // HACK: uses Side internals
}

//Square returns content of given board square
func (bd *Board) Square(sq int) int {
	return bd.square[sq]
}
func (bd *Board) getP12(sq int) int {
	return (bd.Square(sq) << 1) | bd.SquareSide(sq)
}

// Key returns the hash key for the position
func (bd *Board) Key() hash.Key {
	key := bd.copyStr.key
	key ^= castling.FlagsKey[bd.copyStr.flags]
	key ^= hash.EnPassantKey(bd.copyStr.epSq)
	return key
}

// PawnKey returns the Pawn hahs key
func (bd *Board) PawnKey() hash.Key {
	return bd.copyStr.pawnKey
}

// EvalKey returns the Eval hash key
func (bd *Board) EvalKey() hash.Key {
	key := bd.copyStr.key
	key ^= hash.StmKey(bd.stm) // remove incremental STM
	key ^= castling.FlagsKey[bd.copyStr.flags]
	return key
}

// Piece returns the bitboard for the piece type pc
func (bd *Board) Piece(pc int) bit.BB { // se även PieceSd - pga namnkonflikt
	////util.ASSERT(pc < piece.SIZE);
	////util.ASSERT(pc != piece.None);
	return bd.piece[pc]
}

// Phase returns the phase of the game
func (bd *Board) Phase() int {
	return bd.copyStr.phase
}

// PieceSd returns a bitbord for a piece filtered by side (white/black)
func (bd *Board) PieceSd(pc, sd int) bit.BB {
	////util.ASSERT(pc < piece.SIZE)
	// util.ASSERT(pc != piece.None)
	return bd.piece[pc] & bd.side[sd]
}

// King returns the King position for side = sd
func (bd *Board) King(sd int) int {

	// util.ASSERT(bd.king[sd] == bit.first(piece(piece.King, sd)));
	return bd.king[sd]
}

// All returns the bitboard with all pieces
func (bd *Board) All() bit.BB {
	return bd.all
}

// EpSq returns Ep sq
func (bd *Board) EpSq() int {
	return bd.copyStr.epSq
}

// Flags returns castling flags for the position
func (bd *Board) Flags() int {
	return bd.copyStr.flags
}

// Empty returns a bitboard with all empty squares
func (bd *Board) Empty() bit.BB {
	return ^bd.all
}

// Pieces returns a bitboard with all the piece types except pawn
func (bd *Board) Pieces(sd int) bit.BB {
	return bd.side[sd] & ^bd.PieceSd(piece.Pawn, sd)
}

// Side returns a birboard with all the pieces for side=sd
func (bd *Board) Side(sd int) bit.BB {
	return bd.side[sd]
}

// MakeFenMve makes a fenmove converted to an internal mve
func (bd *Board) MakeFenMve(mv int) {
	// util.ASSERT(mv != move.None);
	// util.ASSERT(mv != move.NULL_);

	sd := bd.stm
	xd := Opposit(sd)

	fr := move.From(mv)
	to := move.To(mv)

	pc := move.Piece(mv)
	cp := move.Capt(mv)
	pp := move.Prom(mv)

	// util.ASSERT(p_square[fr] == pc);
	// util.ASSERT(square_side(fr) == sd);

	// util.ASSERT(p_sp < 1024);
	undo := bd.stack[bd.stackIx] //&Undo
	bd.stackIx++
	undo.copyS = bd.copyStr
	undo.move = mv
	undo.castling = false

	bd.copyStr.moves++
	bd.copyStr.recap = square.None

	// capture

	// util.ASSERT(cp != piece.King);

	if pc == piece.Pawn && to == bd.copyStr.epSq {

		capSq := to - square.PawnInc(sd)
		// util.ASSERT(p_square[cap_sq] == cp);
		// util.ASSERT(cp == piece.PAWN);

		undo.capSq = capSq

		bd.clearSquare(cp, xd, capSq, true)

	} else if cp != piece.None {

		// util.ASSERT(p_square[to] == cp);
		// util.ASSERT(square_side(to) == xd);

		undo.capSq = to

		bd.clearSquare(cp, xd, to, true)

	} else {

		// util.ASSERT(p_square[to] == cp);
	}

	// promotion

	if pp != piece.None {
		// util.ASSERT(pc == piece.PAWN);
		bd.clearSquare(piece.Pawn, sd, fr, true)
		bd.setSquare(pp, sd, to, true)
	} else {
		bd.moveSquare(pc, sd, fr, to, true)
	}

	// castling Rook

	if pc == piece.King && util.Iabs(to-fr) == square.CastlingDelta {

		undo.castling = true

		wg := WingKING //Kungsflygeln
		if to < fr {
			wg = WingQUEEN // Damflyfeln
		}
		index := castling.Index(sd, wg)

		// util.ASSERT(castling.Flag(p_copy.flags, index));

		// util.ASSERT(fr == castling.Info[index].kf);
		// util.ASSERT(to == castling.Info[index].kt);

		bd.moveSquare(piece.Rook, sd, castling.Info[index].RookFr, castling.Info[index].RokTo, true)
	}

	// turn

	bd.flipStm()

	// castling flags

	bd.copyStr.flags &= ^castling.FlagsMask[fr]
	bd.copyStr.flags &= ^castling.FlagsMask[to]

	// en-passant square

	bd.copyStr.epSq = square.None

	if pc == piece.Pawn && util.Iabs(to-fr) == square.DoublePawnDelta {
		sq := (fr + to) / 2
		if bd.pawnIsAttacked(sq, xd) {
			bd.copyStr.epSq = sq
		}
	}

	// move counter

	if cp != piece.None || pc == piece.Pawn {
		bd.copyStr.moves = 0 // conversion;
	}

	// recapture

	if cp != piece.None || pp != piece.None {
		bd.copyStr.recap = to
	}

	bd.update()
}
func (bd *Board) clearSquare(pc int, sd int, sq int, updateCopy bool) {

	//util.ASSERT(pc < piece.SIZE)
	//util.ASSERT(pc != piece.None)
	//util.ASSERT(sq >= 0 && sq < square.SIZE)

	//util.ASSERT(pc == bd.p_square[sq])

	//util.ASSERT(bit.Is_set(bd.p_piece[pc], sq))

	bit.Clear(&bd.piece[pc], sq)

	//util.ASSERT(bit.Is_set(bd.p_side[sd], sq))
	bit.Clear(&bd.side[sd], sq)

	//util.ASSERT(bd.p_square[sq] != piece.None)
	bd.square[sq] = piece.None

	p12 := piece.MakeP12(pc, sd)
	//util.ASSERT(p12 < 12 && p12 >= 0, "p12=", p12)
	//util.ASSERT(len(bd.p_count) == 12)
	//util.ASSERT(bd.p_count[p12] >= 0)

	bd.count[p12]--

	if updateCopy {

		key := hash.PieceKey(p12, sq)
		bd.copyStr.key ^= key
		if pc == piece.Pawn {
			bd.copyStr.pawnKey ^= key
		}
		bd.copyStr.phase -= material.Phase(pc)
	}
}

// Ply returns the current ply in search
func (bd *Board) Ply() int {
	// util.ASSERT(p_sp >= p_root);
	return bd.stackIx - bd.rootIx
}
func (bd *Board) flipStm() {
	bd.stm = Opposit(bd.stm)
	bd.copyStr.key ^= hash.StmFlip()
}
func (bd *Board) moveSquare(pc, sd, fr, to int, updateCopy bool) { // TODO
	//util.ASSERT(fr < 64 && fr >= 0)
	//util.ASSERT(to < 64 && to >= 0)
	bd.clearSquare(pc, sd, fr, updateCopy)
	bd.setSquare(pc, sd, to, updateCopy)
}

// Stm returns side to move
func (bd *Board) Stm() int {
	return bd.stm
}

// Move makes a move on the board and uppdates all variables
func (bd *Board) Move(mv int) {

	// util.ASSERT(mv != move.None);
	// util.ASSERT(mv != move.NULL_);
	//bd.Print_board()
	//fmt.Println(move.To_can(mv))
	sd := bd.stm
	xd := Opposit(sd)

	fr := move.From(mv)
	to := move.To(mv)

	pc := move.Piece(mv)
	cp := move.Capt(mv)
	pp := move.Prom(mv)

	// util.ASSERT(p_square[fr] == pc);
	// util.ASSERT(square_side(fr) == sd);

	// util.ASSERT(p_sp < 1024);
	undo := &bd.stack[bd.stackIx]
	bd.stackIx++

	undo.copyS = bd.copyStr
	undo.move = mv
	undo.castling = false

	bd.copyStr.moves++
	bd.copyStr.recap = square.None

	// capture

	// util.ASSERT(cp != piece.King);

	if pc == piece.Pawn && to == bd.copyStr.epSq {

		capSq := to - square.PawnInc(sd)
		// util.ASSERT(p_square[cap_sq] == cp);
		// util.ASSERT(cp == piece.PAWN);

		undo.capSq = capSq

		bd.clearSquare(cp, xd, capSq, true)

	} else if cp != piece.None {

		// util.ASSERT(p_square[to] == cp);
		// util.ASSERT(square_side(to) == xd);

		undo.capSq = to

		bd.clearSquare(cp, xd, to, true)

	} else {

		// util.ASSERT(p_square[to] == cp);
	}

	// promotion

	if pp != piece.None {
		//util.ASSERT(pc == piece.PAWN)
		bd.clearSquare(piece.Pawn, sd, fr, true)
		bd.setSquare(pp, sd, to, true)
	} else {
		bd.moveSquare(pc, sd, fr, to, true)
	}

	// castling Rook

	if pc == piece.King && util.Iabs(to-fr) == square.CastlingDelta {

		undo.castling = true

		wg := WingKING
		if to < fr {
			wg = WingQUEEN
		}
		index := castling.Index(sd, wg)

		// util.ASSERT(castling.flag(p_copy.flags, index));

		// util.ASSERT(fr == castling.info[index].kf);
		// util.ASSERT(to == castling.info[index].kt);

		bd.moveSquare(piece.Rook, sd, castling.Info[index].RookFr, castling.Info[index].RokTo, true)
	}

	// turn

	bd.flipStm()

	// castling flags

	bd.copyStr.flags &= ^castling.FlagsMask[fr]
	bd.copyStr.flags &= ^castling.FlagsMask[to]

	// en-passant square

	bd.copyStr.epSq = square.None

	if pc == piece.Pawn && util.Iabs(to-fr) == square.DoublePawnDelta {
		sq := (fr + to) / 2
		if bd.pawnIsAttacked(sq, xd) {
			bd.copyStr.epSq = sq
		}
	}

	// 50-move rule

	if cp != piece.None || pc == piece.Pawn {
		bd.copyStr.moves = 0 // conversion;
	}

	// recapture

	if cp != piece.None || pp != piece.None {
		bd.copyStr.recap = to
	}

	bd.update()
	//bd.Print_board()
}

// Undo takes back a move
func (bd *Board) Undo() {

	// util.ASSERT(p_sp > 0);
	//bd.Print_board()
	bd.stackIx--
	undo := &bd.stack[bd.stackIx]
	//fmt.Println(bd.p_stack[bd.p_sp])
	//fmt.Println(undo)

	mv := undo.move

	fr := move.From(mv)
	to := move.To(mv)

	pc := move.Piece(mv)
	cp := move.Capt(mv)
	pp := move.Prom(mv)

	xd := bd.stm
	sd := Opposit(xd)

	// util.ASSERT(p_square[to] == pc || p_square[to] == pp);
	// util.ASSERT(square_side(to) == sd);

	// castling Rook

	if undo.castling {
		wg := WingKING
		if to < fr {
			wg = WingQUEEN
		}
		index := castling.Index(sd, wg)

		// util.ASSERT(fr == castling.Info[index].kf);
		// util.ASSERT(to == castling.info[index].kt);

		bd.moveSquare(piece.Rook, sd, castling.Info[index].RokTo, castling.Info[index].RookFr, false)
	}

	// promotion

	if pp != piece.None {
		// util.ASSERT(pc == piece.PAWN);
		bd.clearSquare(pp, sd, to, false)
		bd.setSquare(piece.Pawn, sd, fr, false)
	} else {
		bd.moveSquare(pc, sd, to, fr, false)
	}

	// capture

	if cp != piece.None {
		bd.setSquare(cp, xd, undo.capSq, false)
	}

	bd.flipStm()
	bd.copyStr = undo.copyS

	bd.update()
	//bd.Print_board()
}

// Recap returns the piece if a move was a recapure
func (bd *Board) Recap() int {
	return bd.copyStr.recap
}

// IsDraw returns true if the 50-move rule is passed or if it is 3-move repetition
func (bd *Board) IsDraw() bool {

	if bd.copyStr.moves > 100 { // TODO: check for mate
		return true
	}

	key := bd.copyStr.key // HACK: ignores castling flags and e.p. square

	//util.ASSERT(p_copy.moves <= p_sp);

	for i := 4; i < bd.copyStr.moves; i += 2 {
		if bd.stack[bd.stackIx-i].copyS.key == key {
			return true
		}
	}

	return false
}

// IsMove returns true if it is a possible moves (except checks)
func IsMove(mv int, bd *Board) bool {

	sd := bd.Stm()

	fr := move.From(mv)
	to := move.To(mv)

	pc := move.Piece(mv)
	cp := move.Capt(mv)

	if !(bd.Square(fr) == pc && bd.SquareSide(fr) == sd) {
		return false
	}

	if bd.Square(to) != piece.None && bd.SquareSide(to) == sd {
		return false
	}

	if pc == piece.Pawn && to == bd.EpSq() {
		if cp != piece.Pawn {
			return false
		}
	} else if bd.Square(to) != cp {
		return false
	}

	if cp == piece.King {
		return false
	}

	return true
	//	if pc == piece.PAWN {
	//		return true
	//	} else {
	//		// TODO: castling

	//		// return attack.piece_attack(pc, fr, to, bd);

	//		return true
	//	}

	// util.ASSERT(false);
}

// LoneKing returns true if the side=sd has only King left
func LoneKing(sd int, bd *Board) bool {
	return bd.Count(piece.Knight, sd) == 0 &&
		bd.Count(piece.Bishop, sd) == 0 &&
		bd.Count(piece.Rook, sd) == 0 &&
		bd.Count(piece.Queen, sd) == 0
}

// LoneKingOrBishop returns true if the side=sd has King and Bishop or only King
func LoneKingOrBishop(sd int, bd *Board) bool {

	return bd.Count(piece.Knight, sd) == 0 &&
		bd.Count(piece.Bishop, sd) <= 1 &&
		bd.Count(piece.Rook, sd) == 0 &&
		bd.Count(piece.Queen, sd) == 0
}

// LoneBishop returns true if the side=sd has King and Bishop left
func LoneBishop(sd int, bd *Board) bool {

	return bd.Count(piece.Knight, sd) == 0 &&
		bd.Count(piece.Bishop, sd) == 1 &&
		bd.Count(piece.Rook, sd) == 0 &&
		bd.Count(piece.Queen, sd) == 0
}

// LoneKingOrMinor returns true if the side=sd has King and Minor or only King left
func LoneKingOrMinor(sd int, bd *Board) bool {

	return bd.Count(piece.Knight, sd)+bd.Count(piece.Bishop, sd) <= 1 &&
		bd.Count(piece.Rook, sd) == 0 &&
		bd.Count(piece.Queen, sd) == 0
}

// TwoKnights returns true if sd has exactly two knghts and King
func TwoKnights(sd int, bd *Board) bool {

	return bd.Count(piece.Knight, sd) == 2 &&
		bd.Count(piece.Bishop, sd) == 0 &&
		bd.Count(piece.Rook, sd) == 0 &&
		bd.Count(piece.Queen, sd) == 0
}

/////////////////////////////////////////////
// For testing
////////////////////////////////////////////

// PrintBoard prints out the current board with pieces
func (bd *Board) PrintBoard() {
	for rank := 7; rank >= 0; rank-- {
		fmt.Println(" ")
		for file := 0; file < square.FileSize; file++ {
			sq := square.Make(file, rank)
			if bd.Square(sq) == piece.None {
				fmt.Print(". ")
			} else {
				fenPc := piece.ToFen(bd.getP12(sq))
				fmt.Printf("%v ", fenPc)
			}
		}
	}
	strturn := ""
	if bd.stm == 0 {
		strturn = "Vit"
	} else if bd.stm == 1 {
		strturn = "Svart"
	} else {
		strturn = strconv.Itoa(bd.stm) + " okänd"
	}

	fmt.Println("")
	fmt.Printf("turn: %v  flags: %04b  ep: %v\n", strturn, bd.Flags(), bd.EpSq())
}

// PrintBBInfo is a test function that prints out all bitboards associated with the board position
func (bd *Board) PrintBBInfo() {
	// bb vita
	fmt.Printf("vita  Kpos=%v", bd.king[WHITE])
	PrintBB(bd.side[WHITE])

	// bb svarta
	fmt.Printf("svarta  Kpos=%v", bd.king[BLACK])
	PrintBB(bd.side[BLACK])

	// bb alla
	fmt.Printf("alla  ")
	PrintBB(bd.all)

	// per pjäs
	for p12 := 0; p12 < piece.SideSize; p12++ {
		// pjäs och antal
		fmt.Printf("pjäs=%v antal: %v", piece.ToFen(p12), bd.count[p12]) //pjäs

		// bb
		pc := piece.Piece(p12)
		PrintBB(bd.piece[pc] & bd.side[piece.Color(p12)])
	}
}

// PrintBB is test function to print out a bitboard
func PrintBB(bb bit.BB) {

	for rank := 7; rank >= 0; rank-- {
		fmt.Println(" ")
		for file := 0; file < square.FileSize; file++ {
			sq := square.Make(file, rank)
			if bb&bit.Bit(sq) == 0 {
				fmt.Print("0 ")
			} else {
				fmt.Printf("1 ")
			}
		}
	}
	fmt.Printf("\n\n")
}

/*

func Turn_key(turn int) hash.Hash_to {
	if turn == WHITE {
		return 0
	}
	return rand_key(TURN)
}

   class Board {

   private:

      static const int SCORE_None = -10000; // HACK because "score.None" is defined later



   public:

      void operator=(const Board & bd) {

         for (int pc = 0; pc < piece.SIZE; pc++) {
            p_piece[pc] = bd.p_piece[pc];
         }

         for (int sd = 0; sd < side.SIZE; sd++) {
            p_side[sd] = bd.p_side[sd];
            p_king[sd] = bd.p_king[sd];
         }

         p_all = bd.p_all;

         for (int p12 = 0; p12 < piece.SIDE_SIZE; p12++) {
            p_count[p12] = bd.p_count[p12];
         }

         for (int sq = 0; sq < square.SIZE; sq++) {
            p_square[sq] = bd.p_square[sq];
         }

         p_turn = bd.p_turn;
         p_copy = bd.p_copy;

         p_root = bd.p_root;
         p_sp = bd.p_sp;

         for (int sp = 0; sp < bd.p_sp; sp++) {
            p_stack[sp] = bd.p_stack[sp];
         }

         // util.ASSERT(moves() == bd.moves());
      }




      int count(int pc, int sd) const {
         // util.ASSERT(pc < piece.SIZE);
         // util.ASSERT(pc != piece.None);
         // return bit.count(piece(pc, sd));
         return p_count[piece.make(pc, sd)];
      }

      bit.Bit_to side(int sd) const {
         return p_side[sd];
      }




      bit.Bit_to empty() const {
         return ~p_all;
      }






      int ep_sq() const {
         return p_copy.ep_sq;
      }

      int moves() const {
         return p_copy.moves;
      }

      int recap() const {
         return p_copy.recap;
      }



      int last_move() const {
         return (p_sp == 0) ? move.None : p_stack[p_sp - 1].move;
      }




      void clear_square(int pc, int sd, int sq, bool update_copy) {

         // util.ASSERT(pc < piece.SIZE);
         // util.ASSERT(pc != piece.None);
         // util.ASSERT(sq >= 0 && sq < square.SIZE);

         // util.ASSERT(pc == p_square[sq]);

         // util.ASSERT(bit.is_set(p_piece[pc], sq));
         bit.clear(p_piece[pc], sq);

         // util.ASSERT(bit.is_set(p_side[sd], sq));
         bit.clear(p_side[sd], sq);

         // util.ASSERT(p_square[sq] != piece.None);
         p_square[sq] = piece.None;

         int p12 = piece.make(pc, sd);

         // util.ASSERT(p_count[p12] != 0);
         p_count[p12]--;

         if (update_copy) {

            hash_to key = hash.piece_key(p12, sq);
            p_copy.key ^= key;
            if (pc == piece.PAWN) p_copy.pawn_key ^= key;

            p_copy.phase -= material.phase(pc);
         }
      }


      void move_square(int pc, int sd, int fr, int to, bool update_copy) { // TODO
         clear_square(pc, sd, fr, update_copy);
         set_square(pc, sd, to, update_copy);
      }

      void flip_turn() {
         p_turn = side.opposit(p_turn);
         p_copy.key ^= hash.turn_flip();
      }



      void move(int mv) {

         // util.ASSERT(mv != move.None);
         // util.ASSERT(mv != move.NULL_);

         int sd = p_turn;
         int xd = side.opposit(sd);

         int fr = move.from(mv);
         int to = move.to(mv);

         int pc = move.piece(mv);
         int cp = move.cap(mv);
         int pp = move.prom(mv);

         // util.ASSERT(p_square[fr] == pc);
         // util.ASSERT(square_side(fr) == sd);

         // util.ASSERT(p_sp < 1024);
         Undo & undo = p_stack[p_sp++];

         undo.copy = p_copy;
         undo.move = mv;
         undo.castling = false;

         p_copy.moves++;
         p_copy.recap = square.None;

         // capture

         // util.ASSERT(cp != piece.King);

         if (pc == piece.PAWN && to == p_copy.ep_sq) {

            int cap_sq = to - square.pawn_inc(sd);
            // util.ASSERT(p_square[cap_sq] == cp);
            // util.ASSERT(cp == piece.PAWN);

            undo.cap_sq = cap_sq;

            clear_square(cp, xd, cap_sq, true);

         } else if (cp != piece.None) {

            // util.ASSERT(p_square[to] == cp);
            // util.ASSERT(square_side(to) == xd);

            undo.cap_sq = to;

            clear_square(cp, xd, to, true);

         } else {

            // util.ASSERT(p_square[to] == cp);
         }

         // promotion

         if (pp != piece.None) {
            // util.ASSERT(pc == piece.PAWN);
            clear_square(piece.PAWN, sd, fr, true);
            set_square(pp, sd, to, true);
         } else {
            move_square(pc, sd, fr, to, true);
         }

         // castling Rook

         if (pc == piece.King && std.abs(to - fr) == square.CASTLING_DELTA) {

            undo.castling = true;

            int wg = (to > fr) ? WingKING : WingQUEEN;
            int index = castling.index(sd, wg);

            // util.ASSERT(castling.flag(p_copy.flags, index));

            // util.ASSERT(fr == castling.info[index].kf);
            // util.ASSERT(to == castling.info[index].kt);

            move_square(piece.ROOK, sd, castling.info[index].rf, castling.info[index].rt, true);
         }

         // turn

         flip_turn();

         // castling flags

         p_copy.flags &= ~castling.flags_mask[fr];
         p_copy.flags &= ~castling.flags_mask[to];

         // en-passant square

         p_copy.ep_sq = square.None;

         if (pc == piece.PAWN && std.abs(to - fr) == square.DOUBLE_PAWN_DELTA) {
            int sq = (fr + to) / 2;
            if (pawn_is_attacked(sq, xd)) {
               p_copy.ep_sq = sq;
            }
         }

         // move counter

         if (cp != piece.None || pc == piece.PAWN) {
            p_copy.moves = 0; // conversion;
         }

         // recapture

         if (cp != piece.None || pp != piece.None) {
            p_copy.recap = to;
         }

         update();
      }

      void undo() {

         // util.ASSERT(p_sp > 0);
         const Undo & undo = p_stack[--p_sp];

         int mv = undo.move;

         int fr = move.from(mv);
         int to = move.to(mv);

         int pc = move.piece(mv);
         int cp = move.cap(mv);
         int pp = move.prom(mv);

         int xd = p_turn;
         int sd = side.opposit(xd);

         // util.ASSERT(p_square[to] == pc || p_square[to] == pp);
         // util.ASSERT(square_side(to) == sd);

         // castling Rook

         if (undo.castling) {

            int wg = (to > fr) ? WingKING : WingQUEEN;
            int index = castling.index(sd, wg);

            // util.ASSERT(fr == castling.info[index].kf);
            // util.ASSERT(to == castling.info[index].kt);

            move_square(piece.ROOK, sd, castling.info[index].rt, castling.info[index].rf, false);
         }

         // promotion

         if (pp != piece.None) {
            // util.ASSERT(pc == piece.PAWN);
            clear_square(pp, sd, to, false);
            set_square(piece.PAWN, sd, fr, false);
         } else {
            move_square(pc, sd, to, fr, false);
         }

         // capture

         if (cp != piece.None) {
            set_square(cp, xd, undo.cap_sq, false);
         }

         flip_turn();
         p_copy = undo.copy;

         update();
      }


   };
}
*/
