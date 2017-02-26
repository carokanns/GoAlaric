package sort

import (
	"GoAlaric/bit"
	"GoAlaric/board"
	"GoAlaric/castling"
	"GoAlaric/eval"
	"GoAlaric/move"
	"GoAlaric/piece"
	"GoAlaric/square"

	"fmt"
)

//

const maxSize = 256

// ScMvList holds a list of moves with score
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
func (list *ScMvList) moveTo(frPair, toPair int) {

	// assert(pt <= pf && pf < p_size)

	pair := list.scMv[frPair]

	for i := frPair; i > toPair; i-- {
		list.scMv[i] = list.scMv[i-1]
	}

	list.scMv[toPair] = pair
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

// PawnPushes is adding pawn pushes to the Score/Move list
func PawnPushes(ml *ScMvList, sd int, bd *board.Board) {

	ts := bit.BB(0)

	if sd == board.WHITE {

		ts |= bit.Rank(square.Rank7)
		ts |= bit.Rank(square.Rank6) & ^eval.PawnAttacksFrom(board.BLACK, bd) & (^bd.Piece(piece.Pawn) >> 1) // HACK: direct access

	} else {

		ts |= bit.Rank(square.Rank2)
		ts |= bit.Rank(square.Rank3) & ^eval.PawnAttacksFrom(board.WHITE, bd) & (^bd.Piece(piece.Pawn) << 1) // HACK: direct access
	}

	addPawnQuiets(ml, sd, ts&bd.Empty(), bd)
}

// LegalMoves is generating psudomoves and selecting the legal ones
func LegalMoves(ml *ScMvList, bd *board.Board) {
	var pseudos ScMvList
	genPseudos(&pseudos, bd)
	selectLegals(ml, &pseudos, bd)
}
func canCastle(sd int, wg int, bd *board.Board) bool {

	index := castling.Index(sd, wg)

	if castling.Flag(bd.Flags(), uint(index)) {

		kf := castling.Info[index].KingFr
		// int kt = castling.info[index].kt;
		rf := castling.Info[index].RookFr
		rt := castling.Info[index].RokTo

		// assert(bd.square_is(kf, piece.King, sd))
		// assert(bd.square_is(rf, piece.Rook, sd))

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
	if bd.Square(fr) == piece.Pawn {
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
		ts := eval.PseudoAttacksTo(piece.Pawn, sd, king) & empty

		addPawnQuiets(ml, sd, ts, bd)
	}

	// direct checks, knights

	{
		pc := piece.Knight

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

	for pc := piece.Bishop; pc <= piece.Queen; pc++ {

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

func selectLegals(legals, src *ScMvList, bd *board.Board) {

	legals.Clear()

	for pos := 0; pos < src.Size(); pos++ {

		mv := src.Move(pos)

		if IsLegalMv(mv, bd) {
			legals.Add(mv)
		}
	}
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
	for pc := piece.Pawn; pc <= piece.Queen; pc++ { // skip king
		for bb := bd.PieceSd(pc, sd) & eval.AttacksTo(pc, sd, to, bd); bb != 0; bb = bit.Rest(bb) {
			fr := bit.First(bb)
			addMove(ml, fr, to, bd)
		}
	}
}
func addPieceMoves(ml *ScMvList, sd int, ts bit.BB, bd *board.Board) {

	// assert(ts != 0);
	for pc := piece.Knight; pc <= piece.King; pc++ {

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

	for pc := piece.Knight; pc <= piece.Queen; pc++ { // skip king
		for b := bd.PieceSd(pc, sd); b != 0; b = bit.Rest(b) {
			fr := bit.First(b)
			pieceMovesFr(ml, fr, ts, bd)
		}
	}
}
func addPieceCaptures(ml *ScMvList, sd int, ts bit.BB, bd *board.Board) {

	// assert(ts != 0);

	for pc := piece.Knight; pc <= piece.King; pc++ {

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
			index := castling.Index(sd, wg)
			addPieceMv(ml, castling.Info[index].KingFr, castling.Info[index].KingTo, bd)
		}
	}
}

// AddProms is adding promotion moves to the Score/Mv list
func AddProms(ml *ScMvList, sd int, ts bit.BB, bd *board.Board) {

	pawns := bd.PieceSd(piece.Pawn, sd)
	if sd == board.WHITE {
		for b := pawns & (ts >> 1) & bit.Rank(square.Rank7); b != 0; b = bit.Rest(b) {
			fr := bit.First(b)
			to := fr + 1
			//util.ASSERT(bd.Square(to) == piece.None)
			// //util.ASSERT(square.is_promotion(to));
			addPawnMv(ml, fr, to, bd)
		}

	} else {
		for b := pawns & (ts << 1) & bit.Rank(square.Rank2); b != 0; b = bit.Rest(b) {
			fr := bit.First(b)
			to := fr - 1
			//util.ASSERT(bd.Square(to) == piece.None)
			// //util.ASSERT(square.is_promotion(to));
			addPawnMv(ml, fr, to, bd)
		}
	}

}
func addPieceMv(ml *ScMvList, fr int, to int, bd *board.Board) {
	// assert(bd.square(fr) != piece.PAWN);
	ml.Add(move.Build(fr, to, bd.Square(fr), bd.Square(to), piece.None))
}
func addPawnMv(ml *ScMvList, fr, to int, bd *board.Board) {

	// assert(bd.square(fr) == piece.PAWN);

	pc := bd.Square(fr)
	cp := bd.Square(to)

	if square.IsPromotion(to) {
		ml.Add(move.Build(fr, to, pc, cp, piece.Queen))
		ml.Add(move.Build(fr, to, pc, cp, piece.Knight))
		ml.Add(move.Build(fr, to, pc, cp, piece.Rook))
		ml.Add(move.Build(fr, to, pc, cp, piece.Bishop))
	} else {
		ml.Add(move.Build(fr, to, pc, cp, piece.None))
	}
}
func addEnPassant(ml *ScMvList, sd int, bd *board.Board) {

	to := bd.EpSq()

	if to != square.None {

		fs := bd.PieceSd(piece.Pawn, sd) & eval.PawnAttacks[board.Opposit(sd)][to]

		for b := fs; b != 0; b = bit.Rest(b) {
			fr := bit.First(b)
			ml.Add(move.Build(fr, to, piece.Pawn, piece.Pawn, piece.None))
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

	pawns := bd.PieceSd(piece.Pawn, sd)
	empty := bd.Empty()

	if sd == board.WHITE {

		for b := pawns & (ts >> 1) & (empty >> 1) & ^bit.Rank(square.Rank7); b != 0; b = bit.Rest(b) { // don'to generate promotions
			fr := bit.First(b)
			to := fr + 1
			// assert(bd.square(to) == piece.None);
			// assert(!square.is_promotion(to));
			addPawnMv(ml, fr, to, bd)
		}
		// for (b := pawns & (ts >> 2) & (empty >> 1) & bit.Rank(square.Rank2); b != 0; b = bit.Rest(b)) {
		for b := pawns & (ts >> 2) & (empty >> 1) & bit.Rank(square.Rank2); b != 0; b = bit.Rest(b) {
			fr := bit.First(b)
			to := fr + 2
			// assert(bd.square(to) == piece.None);
			// assert(!square.is_promotion(to));
			addPawnMv(ml, fr, to, bd)
		}

	} else {
		for b := pawns & (ts << 1) & ^bit.Rank(square.Rank2); b != 0; b = bit.Rest(b) { // don'to generate promotions
			fr := bit.First(b)
			to := fr - 1
			// assert(bd.square(to) == piece.None);
			// assert(!square.is_promotion(to));
			addPawnMv(ml, fr, to, bd)
		}

		for b := pawns & (ts << 2) & (empty << 1) & bit.Rank(square.Rank7); b != 0; b = bit.Rest(b) {
			fr := bit.First(b)
			to := fr - 2
			// assert(bd.square(to) == piece.None);
			// assert(!square.is_promotion(to));
			addPawnMv(ml, fr, to, bd)
		}
	}
}
func addPawnCaptures(ml *ScMvList, sd int, ts bit.BB, bd *board.Board) {

	pawns := bd.PieceSd(piece.Pawn, sd)
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

// IsLegalMv returns true if the given move doesnt leave the king in chess
func IsLegalMv(mv int, bd *board.Board) bool {

	bd.Move(mv)

	sd := bd.Stm() // new new side

	answer := !eval.IsAttacked(bd.King(sd^1), sd, bd)

	bd.Undo()

	return answer
}

// IsQuiet returns true if the given move
func IsQuiet(mv int, bd *board.Board) bool {

	sd := bd.Stm()

	fr := move.From(mv)
	to := move.To(mv)

	pc := move.Piece(mv)
	// assert(move.cap(mv) == piece.None);
	// assert(move.prom(mv) == piece.None);

	if !(bd.Square(fr) == pc && bd.SquareSide(fr) == sd) {
		return false
	}

	if bd.Square(to) != piece.None {
		return false
	}

	if pc == piece.Pawn {

		inc := square.PawnInc(sd)

		if to-fr == inc && !square.IsPromotion(to) {
			return true
		} else if to-fr == inc*2 && square.RankSd(fr, sd) == square.Rank2 {
			return bd.Square(fr+inc) == piece.None
		}
		return false
	}

	return eval.PieceAttack(pc, fr, to, bd)

	// assert(false);
	//panic("shouldn'to happen")
	//return false
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

/*func  gen_moves_debug( &ml List,)  void{

       ml.clear();

 var sd int = bd.turn();

       if (eval.is_in_check(bd)) {
          add_evasions(ml, sd, bd);
       } else {
          add_captures(ml, sd, bd);
          add_promotions(ml, sd, bd);
          add_quiets(ml, sd, bd);
       }
    }

 func  filter_legals( &dst List,&bd board.Board,)  void{

       dst.clear();

       for  pos := 0; pos < src.size( ); pos++) {

 var mv int = src.move(pos);

          if (is_legal_debug(mv, bd)) {
             dst.Add(mv);
          }
       }
    }

    class List {

    private:

       static const int SIZE = 256;

 var p_size; int
 var p_pair[SIZE]; uint32





    public:

       List() {
          clear();
       }

 func  operator=( )  void{

          clear();

          for  pos := 0; pos < ml.size( ); pos++) {
 var p uint32 = ml.pair(pos);
             add_pair(p);
          }
       }

 func  clear( )  void{
          p_size = 0;
       }







 func  size( ) const  int{
          return p_size;
       }


 func  score( pos int ) const  int{
          return pair(pos) >> move.BITS;
       }


	////////// end class /////////////////

 func  add_pawn_move( &ml List,fr int,to int,)  void{

       assert(bd.square(fr) == piece.PAWN);

 var pc int = bd.square(fr);
 var cp int = bd.square(to);

       if (square.is_promotion(to)) {
          ml.Add(move.make(fr, to, pc, cp, piece.Queen));
          ml.Add(move.make(fr, to, pc, cp, piece.Knight));
          ml.Add(move.make(fr, to, pc, cp, piece.Rook));
          ml.Add(move.make(fr, to, pc, cp, piece.Bishop));
       } else {
          ml.Add(move.make(fr, to, pc, cp));
       }
    }

 func  add_piece_move( &ml List,fr int,to int,)  void{
       assert(bd.square(fr) != piece.PAWN);
       ml.Add(move.make(fr, to, bd.square(fr), bd.square(to)));
    }

 func  add_move( &ml List,fr int,to int,)  void{
       if (bd.square(fr) == piece.PAWN) {
          add_pawn_move(ml, fr, to, bd);
       } else {
          add_piece_move(ml, fr, to, bd);
       }
    }

 func  add_piece_moves_from( &ml List,fr int,ts bit_to,)  void{

 var pc int = bd.square(fr);

       for   fr bd) & ts; b != 0; b = bit.Rest(b)) {
 var to int = bit.First(b);
          add_piece_move(ml, fr, to, bd);
       }
    }

 func  add_captures_to( &ml List,sd int,to int,)  void{

       for  pc := piece.PAWN; pc <= piece.King; pc++ ) {
          for   sd) & eval.attacks_to(pc, sd, to, bd); b != 0; b = bit.Rest(b)) {
 var fr int = bit.First(b);
             add_move(ml, fr, to, bd);
          }
       }
    }

 func  add_captures_to_no_king( &ml List,sd int,to int,)  void{

       for  pc := piece.PAWN; pc <= piece.Queen; pc++ ) { // skip king
          for   sd) & eval.attacks_to(pc, sd, to, bd); b != 0; b = bit.Rest(b)) {
 var fr int = bit.First(b);
             add_move(ml, fr, to, bd);
          }
       }
    }

 func  add_pawn_captures( &ml List,sd int,ts bit_to,)  void{

 var pawns bit_to = bd.piece(piece.PAWN, sd);
       ts &= bd.side(side.opposit(sd)); // not needed

       if (sd == side.WHITE) {

          for  ) & pawns; b != 0; b = bit.Rest(b)) {
 var fr int = bit.First(b);
 var to int = fr - 7;
             add_pawn_move(ml, fr, to, bd);
          }

          for  ) & pawns; b != 0; b = bit.Rest(b)) {
 var fr int = bit.First(b);
 var to int = fr + 9;
             add_pawn_move(ml, fr, to, bd);
          }

       } else {

          for  ) & pawns; b != 0; b = bit.Rest(b)) {
 var fr int = bit.First(b);
 var to int = fr - 9;
             add_pawn_move(ml, fr, to, bd);
          }

          for  ) & pawns; b != 0; b = bit.Rest(b)) {
 var fr int = bit.First(b);
 var to int = fr + 7;
             add_pawn_move(ml, fr, to, bd);
          }
       }
    }



 func  add_promotions( &ml List,sd int,)  void{
       add_promotions(ml, sd, bd.empty(), bd);
    }



func  add_en_passant( &ml List,sd int,)  void{

 var to int = bd.ep_sq();

       if (to != square.None) {

 var fs bit_to = bd.piece(piece.PAWN, sd) & eval.Pawn_Attacks[side.opposit(sd)][to];

          for  b := fs; b != 0; b = bit.Rest(b )) {
 var fr int = bit.First(b);
             ml.Add(move.make(fr, to, piece.PAWN, piece.PAWN));
          }
       }
    }






 func  add_piece_moves_no_king( &ml List,sd int,ts bit_to,)  void{

       assert(ts != 0);

       for  pc := piece.Knight; pc <= piece.Queen; pc++ ) { // skip king
          for   sd); b != 0; b = bit.Rest(b)) {
 var fr int = bit.First(b);
             add_piece_moves_from(ml, fr, ts, bd);
          }
       }
    }

 func  add_piece_moves_rare( &ml List,sd int,ts bit_to,)  void{

       assert(ts != 0);

       for  pc := piece.Knight; pc <= piece.King; pc++ ) {

          for   sd); b != 0; b = bit.Rest(b)) {

 var fr int = bit.First(b);

             for   sd fr) & ts; bb != 0; bb = bit.Rest(bb)) {

 var to int = bit.First(bb);

                if (eval.line_is_empty(fr, to, bd)) {
                   add_piece_move(ml, fr, to, bd);
                }
             }
          }
       }
    }



 func  add_captures_mvv_lva( &ml List,sd int,)  void{

       for  pc := piece.Queen; pc >= piece.PAWN; pc-- ) {
          for   side.opposit(sd)); b != 0; b = bit.Rest(b)) {
 var to int = bit.First(b);
             add_captures_to(ml, sd, to, bd);
          }
       }

       add_en_passant(ml, sd, bd);
    }

 func  is_move( mv int, bd board.Board)  bool{  // flyttad till board

 var sd int = bd.turn();

 var fr int = move.from(mv);
 var to int = move.to(mv);

 var pc int = move.piece(mv);
 var cp int = move.cap(mv);

       if (!(bd.square(fr) == pc && bd.square_side(fr) == sd)) {
          return false;
       }

       if (bd.square(to) != piece.None && bd.square_side(to) == sd) {
          return false;
       }

       if (pc == piece.PAWN && to == bd.ep_sq()) {
          if (cp != piece.PAWN) {
             return false;
          }
       } else if (bd.square(to) != cp) {
          return false;
       }

       if (cp == piece.King) {
          return false;
       }

       if (pc == piece.PAWN) {

          // TODO

          return true;

       } else {

          // TODO: castling

          // return eval.piece_attack(pc, fr, to, bd);

          return true;
       }

       assert(false);
    }



 func  add_evasions( &ml List,sd int,) {
       eval.Attacks attacks;
       eval.init_attacks(attacks, sd, bd);
       add_evasions(ml, sd, bd, attacks);
    }




 func  gen_legal_evasions( &ml List,&bd board.Board,)  void{

 var sd int = bd.turn();

       eval.Attacks attacks;
       eval.init_attacks(attacks, sd, bd);

       if (attacks.size == 0) {
          ml.clear();
          return;
       }

       List pseudos;
       add_evasions(pseudos, sd, bd, attacks);

       filter_legals(ml, pseudos, bd);
    }
 }
*/
