package eval

import (
	"GoAlaric/bit"
	"GoAlaric/board"
	"GoAlaric/move"
	"GoAlaric/piece"
	"GoAlaric/square"
	"fmt"
)

// bitboards with various attack patterns
var (
	pawnAttack  = [2][2]int{{-15, +17}, {-17, +15}}
	Blockers    [piece.Size][square.BoardSize]bit.BB
	Between     [square.BoardSize][square.BoardSize]bit.BB
	Behind      [square.BoardSize][square.BoardSize]bit.BB
	PawnAttacks [2][square.BoardSize]bit.BB
)

var pawnMove = [2]int{+1, -1}
var pawnMoves [2][square.BoardSize]bit.BB
var pieceAttacks [piece.Size][square.BoardSize]bit.BB

var knightInc = []int{-33, -31, -18, -14, +14, +18, +31, +33, 0}
var bishopInc = []int{-17, -15, +15, +17, 0}
var rookInc = []int{-16, -1, +1, +16, 0}
var queenInc = []int{-17, -16, -15, -1, +1, +15, +16, +17, 0}
var null = []int{0} //HACK
var pieceInc = [piece.Size][]int{null, knightInc, bishopInc, rookInc, queenInc, queenInc, null}

// AtkInit is run when the program starts. It sets up various tables for moves and attacks etc
func AtkInit() {
	fmt.Println("info string AtkInit startar")
	var sq int // NOTE: Will be reused
	for sd := 0; sd < 2; sd++ {
		for sq = 0; sq < square.BoardSize; sq++ {
			pawnMoves[sd][sq] = computePawnMoves(sd, sq)
			PawnAttacks[sd][sq] = computePawnAttacks(sd, sq)
		}
	}

	for pc := piece.Knight; pc <= piece.King; pc++ {
		for sq = 0; sq < square.BoardSize; sq++ {
			pieceAttacks[pc][sq] = computePieceAttacks(pc, sq)
			Blockers[pc][sq] = getBlockers(pc, sq)
		}
	}

	for sq = 0; sq < square.BoardSize; sq++ {
		for to := 0; to < square.BoardSize; to++ {
			Between[sq][to] = computeBetween(sq, to)
			Behind[sq][to] = attacksThrough(sq, to)
		}
	}
}

func deltaInc(fr, to int) int {

	for dir := 0; dir < 8; dir++ {

		inc := queenInc[dir]

		for sq := fr + inc; square.IsValid88(sq); sq += inc {
			if sq == to {
				return inc
			}
		}
	}

	return 0
}
func pawnMovesFrom(sd int, bd *board.Board) bit.BB { // for pawn mobility

	//util.ASSERT(sd < 2)

	bbFr := bd.PieceSd(piece.Pawn, sd)

	if sd == WHITE {
		return bbFr << 1
	}
	return bbFr >> 1

}

// computePawnMoves (not captures)
func computePawnMoves(sd, sq int) bit.BB {

	// assert(sd < 2);

	var bb bit.BB

	fr := square.To88(sq)
	inc := pawnMove[sd]

	to := fr + inc

	if square.IsValid88(to) {
		bit.Set(&bb, square.From88(to))
	}

	if square.RankSd(sq, sd) == square.Rank2 {
		to += inc
		// assert(square.Is_valid_88(to))
		bit.Set(&bb, square.From88(to))
	}

	return bb
}

// computePawnAttacks attacking left/right (empty board)
func computePawnAttacks(sd, sq int) bit.BB {

	// assert(sd < 2);

	var b bit.BB

	fr := square.To88(sq)

	for dir := 0; dir < 2; dir++ {
		to := fr + pawnAttack[sd][dir]
		if square.IsValid88(to) {
			bit.Set(&b, square.From88(to))
		}
	}

	return b
}

// computePieceAttacks not pawns
func computePieceAttacks(pc int, sq int) bit.BB {

	// assert(pc != piece.PAWN)

	var b bit.BB

	fr := square.To88(sq)

	for dir := 0; true; dir++ {

		inc := pieceInc[pc][dir]
		if inc == 0 {
			break
		}

		if pc >= piece.Bishop && pc <= piece.Queen { // slider

			for to := fr + inc; square.IsValid88(to); to += inc {
				bit.Set(&b, square.From88(to))
			}

		} else {

			to := fr + inc

			if square.IsValid88(to) {
				bit.Set(&b, square.From88(to))
			}
		}
	}

	return b
}

func attackBehind(fr, to, sd int, bd *board.Board) bool {

	// assert(bd.square(to) != piece.None);

	behind := Behind[fr][to]
	if behind == 0 {
		return false
	}

	for b := sliderPseudoAttacksTo(sd, to, bd) & behind; b != 0; b = bit.Rest(b) {

		sq := bit.First(b)

		if bit.Single(bd.All() & Between[sq][fr]) {
			return true
		}
	}

	return false
}
func getBlockers(pc, fr int) bit.BB {

	//assert(pc != piece.PAWN)

	var b bit.BB

	attacks := computePieceAttacks(pc, fr)

	for bb := attacks; bb != 0; bb = bit.Rest(bb) {
		sq := bit.First(bb)
		if (attacks & attacksThrough(fr, sq)) != 0 {
			bit.Set(&b, sq)
		}
	}

	return b
}

//PseudoAttacksTo returns a bitboard with all pseudoattacks to to-sq from piece=pc
func PseudoAttacksTo(pc, sd, to int) bit.BB {
	return PseudoAttacksFrom(pc, board.Opposit(sd), to) // HACK for pawns
}

func pawnAttacksTo(sd, to int) bit.BB {
	//util.ASSERT(sd < 2)
	return pawnAttacksFrom(board.Opposit(sd), to)
}

// PawnAttacksFrom returns a bitboard with all pawn attacks from side=sd
func PawnAttacksFrom(sd int, bd *board.Board) bit.BB {

	// assert(sd < 2);

	fs := bd.PieceSd(piece.Pawn, sd)

	if sd == WHITE {
		return (fs >> 7) | (fs << 9)
	}
	return (fs >> 9) | (fs << 7)

}

func pawnAttacksFrom(sd, fr int) bit.BB {
	// assert(sd < 2);
	return PawnAttacks[sd][fr]
}

// PinnedBy returns a bitboard with attackers to to-sq but with one piece in between
func PinnedBy(to, sd int, bd *board.Board) bit.BB {

	var pinned bit.BB

	for b := sliderPseudoAttacksTo(sd, to, bd); b != 0; b = bit.Rest(b) {

		fr := bit.First(b)

		bb := bd.All() & Between[fr][to]

		if bb != 0 && bit.Single(bb) {
			pinned |= bb
		}
	}

	return pinned
}

// computeBetween fr and to (no pieces on board)
func computeBetween(fr, to int) bit.BB {

	fr = square.To88(fr)
	to = square.To88(to)

	b := bit.BB(0)

	inc := deltaInc(fr, to)

	if inc != 0 {
		for sq := fr + inc; sq != to; sq += inc {
			bit.Set(&b, square.From88(sq))
		}
	}

	return b
}

func attacksThrough(fr, to int) bit.BB {

	fr = square.To88(fr)
	to = square.To88(to)

	b := bit.BB(0)

	inc := deltaInc(fr, to)

	if inc != 0 {
		for sq := to + inc; square.IsValid88(sq); sq += inc {
			bit.Set(&b, square.From88(sq))
		}
	}

	return b
}

// IsInCheck returns true if the stm is in cehck
func IsInCheck(bd *board.Board) bool {

	atk := bd.Stm()
	def := board.Opposit(atk)

	return IsAttacked(bd.King(atk), def, bd)
}

// IsLegal returns true if the one who moved is not in check
func IsLegal(bd *board.Board) bool { // Ej Flyttad från move

	atk := bd.Stm()
	def := board.Opposit(atk)

	return !IsAttacked(bd.King(def), atk, bd)
}

func isPawnAtk(sd, fr, to int) bool {
	// assert(sd < side::SIZE);
	return bit.IsOne(PawnAttacks[sd][fr], to)
}

// PieceAttack returns true if a piece on fr attacks to
func PieceAttack(pc, fr, to int, bd *board.Board) bool {
	// assert(pc != piece::PAWN);
	return bit.IsOne(pieceAttacks[pc][fr], to) && LineIsEmpty(fr, to, bd)
}

// Ray returns a bitboard with squaresd btween fr and to
func Ray(fr, to int) bit.BB {
	return Between[fr][to] | Behind[fr][to] // HACK: to should be included
}

// AttacksTo returns a bitboad with all attacks (opposite sd) to to-square
func AttacksTo(pc int, sd int, to int, bd *board.Board) bit.BB {
	return attacksFrom(pc, board.Opposit(sd), to, bd) // HACK for pawns
}

// PieceAttacksFrom returns a bitboard with all attacks from a piece (not pawn) on fr-sq
func PieceAttacksFrom(pc int, fr int, bd *board.Board) bit.BB {

	// assert(pc != piece::PAWN);

	ts := pieceAttacks[pc][fr]
	for b := bd.All() & Blockers[pc][fr]; b != 0; b = bit.Rest(b) {
		sq := bit.First(b)
		ts &= ^Behind[fr][sq]
	}
	return ts
}
func attacksFrom(pc, sd, fr int, bd *board.Board) bit.BB {
	if pc == piece.Pawn {
		return PawnAttacks[sd][fr]
	}
	return PieceAttacksFrom(pc, fr, bd)

}

// PseudoAttacksFrom square fr
func PseudoAttacksFrom(pc, sd, fr int) bit.BB {
	if pc == piece.Pawn {
		return PawnAttacks[sd][fr]
	}
	return pieceAttacks[pc][fr]
}

func attack(pc, sd, fr, to int, bd *board.Board) bool {
	// assert(sd < 2);
	if pc == piece.Pawn {
		return isPawnAtk(sd, fr, to)
	}
	return PieceAttack(pc, fr, to, bd)
}
func sliderPseudoAttacksTo(sd, to int, bd *board.Board) bit.BB {

	// assert(sd < side::SIZE);

	b := bit.BB(0)
	b |= bd.PieceSd(piece.Bishop, sd) & pieceAttacks[piece.Bishop][to]
	b |= bd.PieceSd(piece.Rook, sd) & pieceAttacks[piece.Rook][to]
	b |= bd.PieceSd(piece.Queen, sd) & pieceAttacks[piece.Queen][to]

	return b
}

// IsAttacked if a to sq is attacked by opposite color pieces
func IsAttacked(to, sd int, bd *board.Board) bool {

	// non-sliders

	if (bd.PieceSd(piece.Pawn, sd) & PawnAttacks[board.Opposit(sd)][to]) != 0 { // HACK
		return true
	}

	if (bd.PieceSd(piece.Knight, sd) & pieceAttacks[piece.Knight][to]) != 0 {
		return true
	}

	if (bd.PieceSd(piece.King, sd) & pieceAttacks[piece.King][to]) != 0 {
		return true
	}

	// sliders

	for b := sliderPseudoAttacksTo(sd, to, bd); b != 0; b = bit.Rest(b) {

		fr := bit.First(b)

		if (bd.All() & Between[fr][to]) == 0 {
			return true
		}
	}

	return false
}

// LineIsEmpty tells if there are nothing between fr and to squares
func LineIsEmpty(fr int, to int, bd *board.Board) bool {
	return (bd.All() & Between[fr][to]) == 0
}

// IsCheck returns true
func IsCheck(mv int, bd *board.Board) bool { //////flyttad från move

	if move.IsPromotion(mv) || IsEnPassant(mv, bd) || move.IsCastling(mv) {
		return inCheckAfterMove(mv, bd)
	}

	fr := move.From(mv)
	to := move.To(mv)
	pc := move.Piece(mv)
	if move.Prom(mv) != piece.None {
		pc = move.Prom(mv)
	}

	sd := bd.SquareSide(fr) // ie if fr-piece is white...:

	kingSq := bd.King(board.Opposit(sd)) // ... then look for black kingSq and viceversa

	if attack(pc, sd, to, kingSq, bd) {
		return true
	}

	if attackBehind(kingSq, fr, sd, bd) && !bit.IsOne(Ray(kingSq, fr), to) {
		return true
	}

	return false
}

func inCheckAfterMove(mv int, bd *board.Board) bool { ////////flyttad från move

	bd.Move(mv)
	b := IsInCheck(bd)
	bd.Undo()

	return b
}

// Attacks is the struct holding attacks info (Avoid and Pinned)
type Attacks struct {
	Size   int
	Square [2]int
	Avoid  bit.BB
	Pinned bit.BB
}

// InitAttacks is doing just that before starting a search node
func InitAttacks(attacks *Attacks, sd int, bd *board.Board) {

	atk := board.Opposit(sd)
	def := sd

	to := bd.King(def)

	attacks.Size = 0
	attacks.Avoid = 0
	attacks.Pinned = 0

	// non-sliders

	{
		b := bit.BB(0)
		b |= bd.PieceSd(piece.Pawn, atk) & PawnAttacks[def][to] // HACK
		b |= bd.PieceSd(piece.Knight, atk) & pieceAttacks[piece.Knight][to]

		if b != 0 {
			// assert(bit::single(b));
			// assert(attacks.Size < 2);
			attacks.Square[attacks.Size] = bit.First(b)
			attacks.Size++
		}
	}

	// sliders

	for b := sliderPseudoAttacksTo(atk, to, bd); b != 0; b = bit.Rest(b) {

		fr := bit.First(b)

		bb := bd.All() & Between[fr][to]

		if bb == 0 {
			// assert(attacks.Size < 2);
			attacks.Square[attacks.Size] = fr
			attacks.Size++
			attacks.Avoid |= Ray(fr, to)
		} else if bit.Single(bb) {
			attacks.Pinned |= bb
		}
	}
}

/*

   const int Knight_Inc[] = { -33, -31, -18, -14, +14, +18, +31, +33, 0 };
   const int Bishop_Inc[] = { -17, -15, +15, +17, 0 };
   const int Rook_Inc[]   = { -16, -1, +1, +16, 0 };
   const int Queen_Inc[]  = { -17, -16, -15, -1, +1, +15, +16, +17, 0 };

   const int * Piece_Inc[piece.SIZE] = { NULL, Knight_Inc, Bishop_Inc, Rook_Inc, Queen_Inc, Queen_Inc, NULL };


   bit.Bit_to Between[square.SIZE][square.SIZE];
   bit.Bit_to Behind[square.SIZE][square.SIZE];

   bool line_is_empty(int fr, int to, const Board & bd) {
      return (bd.all() & Between[fr][to]) == 0;
   }


   bool pawn_move(int sd, int fr, int to, const Board & bd) {
      assert(sd < 2);
      return bit.is_set(Pawn_Moves[sd][fr], to) && line_is_empty(fr, to, bd);
   }





   bit.Bit_to pawn_moves_to(int sd, bit.Bit_to ts, const Board &bd) {

      assert(sd < 2);
      assert((bd.all() & ts) == 0);

      bit.Bit_to pawns = bd.piece(piece.PAWN, sd);
      bit.Bit_to empty = bd.empty();

      bit.Bit_to fs = 0;

      if (sd == side.WHITE) {
         fs |= (ts >> 1);
         fs |= (ts >> 2) & (empty >> 1) & bit.rank(square.Rank2);
      } else {
         fs |= (ts << 1);
         fs |= (ts << 2) & (empty << 1) & bit.rank(square.Rank7);
      }

      return pawns & fs;
   }


   bit.Bit_to pawn_attacks_tos(int sd, bit.Bit_to ts) {

      assert(sd < 2);

      if (sd == side.WHITE) {
         return (ts >> 9) | (ts << 7);
      } else {
         return (ts >> 7) | (ts << 9);
      }
   }

   bit.Bit_to pawn_attacks_from(int sd, int fr) {
      assert(sd < 2);
      return Pawn_Attacks[sd][fr];
   }


   bit.Bit_to piece_attacks_from(int pc, int fr, const Board & bd) {

      assert(pc != piece.PAWN);

      bit.Bit_to ts = Piece_Attacks[pc][fr];

      for (bit.Bit_to b = bd.all() & Blockers[pc][fr]; b != 0; b = bit.rest(b)) {
         int sq = bit.first(b);
         ts &= ~Behind[fr][sq];
      }

      return ts;
   }

   bit.Bit_to piece_attacks_to(int pc, int to, const Board & bd) {
      assert(pc != piece.PAWN);
      return piece_attacks_from(pc, to, bd);
   }

   bit.Bit_to piece_moves_from(int pc, int sd, int fr, const Board & bd) {
      if (pc == piece.PAWN) {
         assert(false); // TODO: blockers
         return Pawn_Moves[sd][fr];
      } else {
         return piece_attacks_from(pc, fr, bd);
      }
   }

   bit.Bit_to attacks_from(int pc, int sd, int fr, const Board & bd) {
      if (pc == piece.PAWN) {
         return Pawn_Attacks[sd][fr];
      } else {
         return piece_attacks_from(pc, fr, bd);
      }
   }

   bit.Bit_to attacks_to(int pc, int sd, int to, const Board & bd) {
      return attacks_from(pc, side.opposit(sd), to, bd); // HACK for pawns
   }

   bit.Bit_to pseudo_attacks_from(int pc, int sd, int fr) {
      if (pc == piece.PAWN) {
         return Pawn_Attacks[sd][fr];
      } else {
         return Piece_Attacks[pc][fr];
      }
   }


   bit.Bit_to slider_pseudo_attacks_to(int sd, int to, const Board & bd) {

      assert(sd < 2);

      bit.Bit_to b = 0;
      b |= bd.piece(piece.Bishop, sd) & Piece_Attacks[piece.Bishop][to];
      b |= bd.piece(piece.Rook,   sd) & Piece_Attacks[piece.Rook][to];
      b |= bd.piece(piece.Queen,  sd) & Piece_Attacks[piece.Queen][to];

      return b;
   }







   void init_attacks(Attacks & attacks, const Board & bd) { //meningslös - använd 3 parms direkt
      init_attacks(attacks, bd.turn(), bd);
   }



   bit.Bit_to piece_attacks_debug(int pc, int sq) {

      assert(pc != piece.PAWN);

      bit.Bit_to b = 0;

      int fr = square.to_88(sq);

      for (int dir = 0; true; dir++) {

         int inc = Piece_Inc[pc][dir];
         if (inc == 0) break;

         if (piece.is_slider(pc)) {

            for (int to = fr + inc; square.is_valid_88(to); to += inc) {
               bit.Set(b, square.from_88(to));
            }

         } else {

            int to = fr + inc;

            if (square.is_valid_88(to)) {
               bit.Set(b, square.from_88(to));
            }
         }
      }

      return b;
   }

   int delta_inc(int fr, int to) {

      for (int dir = 0; dir < 8; dir++) {

         int inc = Queen_Inc[dir];

         for (int sq = fr + inc; square.is_valid_88(sq); sq += inc) {
            if (sq == to) {
               return inc;
            }
         }
      }

      return 0;
   }

   bit.Bit_to between_debug(int fr, int to) {

      fr = square.to_88(fr);
      to = square.to_88(to);

      bit.Bit_to b = 0;

      int inc = delta_inc(fr, to);

      if (inc != 0) {
         for (int sq = fr + inc; sq != to; sq += inc) {
            bit.Set(b, square.from_88(sq));
         }
      }

      return b;
   }

   bit.Bit_to behind_debug(int fr, int to) {

      fr = square.to_88(fr);
      to = square.to_88(to);

      bit.Bit_to b = 0;

      int inc = delta_inc(fr, to);

      if (inc != 0) {
         for (int sq = to + inc; square.is_valid_88(sq); sq += inc) {
            bit.Set(b, square.from_88(sq));
         }
      }

      return b;
   }

   bit.Bit_to blockers_debug(int pc, int fr) {

      assert(pc != piece.PAWN);

      bit.Bit_to b = 0;

      bit.Bit_to attacks = piece_attacks_debug(pc, fr);

      for (bit.Bit_to bb = attacks; bb != 0; bb = bit.rest(bb)) {
         int sq = bit.first(bb);
         if ((attacks & behind_debug(fr, sq)) != 0) {
            bit.Set(b, sq);
         }
      }

      return b;
   }


}
*/
