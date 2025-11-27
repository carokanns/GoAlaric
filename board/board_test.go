// board_test.go
package board

import (
	"strings"
	"testing"

	"goalaric/bit"
	"goalaric/material"
	"goalaric/square"
)

func TestFenMove(t *testing.T) {
	//castling.Init()

	var fenTest = [...][7]string{
		{StartFen, "b", "KQkq", "-", "e2e4", "e2 e3", "P"},
		{"rnb1kb1r/1pqp1ppp/p3pn2/8/2PNP3/2NB4/PP1B1PPP/R2QK2R w KQ - 7 9", "b", "KQ", "-", "e4e5", "e4 d5 f5", "P"},                                             // ett bondedrag
		{"r1b1k1nr/2qp1p1p/p1n1p3/1pb3p1/2PNPP2/2NB2PP/PP6/R1BQK2R b kq - 0 12", "w", "kq", "-", "b5c4", "b5 b4 a4", "p"},                                        // bondeslag (båda hållen)
		{"r1b1k1nr/2qp1p1p/p1n1p3/1pb3p1/2PNPP2/2NB2PP/PP6/R1BQK2R b kq - 0 12", "w", "kq", "-", "g5f4", "g5 g4 h4", "p"},                                        // bondeslag (båda hållen)
		{"3K4/PPP1PPPP/8/8/8/8/ppp1pppp/3k4 w - - 1 58", "b", "-", "-", "a7a8R", "a7 b8", "R"},                                                                   // bondeförvandling (s/v slag båda hållen och ickeslag, underförvandling)
		{"3K4/PPP1PPPP/8/8/8/8/ppp1pppp/3k4 w - - 1 58", "b", "-", "-", "b7b8N", "b7 a8 c8", "N"},                                                                // bondeförvandling (s/v slag båda hållen och ickeslag, underförvandling)
		{"3K4/PPP1PPPP/8/8/8/8/ppp1pppp/3k4 w - - 1 58", "b", "-", "-", "c7c8b", "c7 b8", "B"},                                                                   // bondeförvandling (s/v slag båda hållen och ickeslag, underförvandling)
		{"3K4/PPP1PPPP/8/8/8/8/ppp1pppp/3k4 w - - 1 58", "b", "-", "-", "e7e8Q", "e7 f8", "Q"},                                                                   // bondeförvandling (s/v slag båda hållen och ickeslag, underförvandling)
		{"3K4/PPP1PPPP/8/8/8/8/ppp1pppp/3k4 w - - 1 58", "b", "-", "-", "f7f8b", "f7 e8 g8", "B"},                                                                // bondeförvandling (s/v slag båda hållen och ickeslag, underförvandling)
		{"3K4/PPP1PPPP/8/8/8/8/ppp1pppp/3k4 w - - 1 58", "b", "-", "-", "g7g8n", "g7 f8 h8", "N"},                                                                // bondeförvandling (s/v slag båda hållen och ickeslag, underförvandling)
		{"3K4/PPP1PPPP/8/8/8/8/ppp1pppp/3k4 w - - 1 58", "b", "-", "-", "h7h8r", "h7 g8", "R"},                                                                   // bondeförvandling (s/v slag båda hållen och ickeslag, underförvandling)
		{"3K4/PPP1PPPP/8/8/8/8/ppp1pppp/3k4 w - - 1 58", "w", "-", "-", "d8d7 a2a1r", "a2 b1", "r"},                                                              // bondeförvandling (s/v slag båda hållen och ickeslag, underförvandling)
		{"3K4/PPP1PPPP/8/8/8/8/ppp1pppp/3k4 w - - 1 58", "w", "-", "-", "d8d7 b2b1N", "b2 a1 c1", "n"},                                                           // bondeförvandling (s/v slag båda hållen och ickeslag, underförvandling)
		{"3K4/PPP1PPPP/8/8/8/8/ppp1pppp/3k4 w - - 1 58", "w", "-", "-", "d8d7 c2c1b", "c2 b1", "b"},                                                              // bondeförvandling (s/v slag båda hållen och ickeslag, underförvandling)
		{"3K4/PPP1PPPP/8/8/8/8/ppp1pppp/3k4 w - - 1 58", "w", "-", "-", "d8d7 e2e1q", "e2 f1", "q"},                                                              // bondeförvandling (s/v slag båda hållen och ickeslag, underförvandling)
		{"3K4/PPP1PPPP/8/8/8/8/ppp1pppp/3k4 w - - 1 58", "w", "-", "-", "d8d7 f2f1b", "f2 e1 g1", "b"},                                                           // bondeförvandling (s/v slag båda hållen och ickeslag, underförvandling)
		{"3K4/PPP1PPPP/8/8/8/8/ppp1pppp/3k4 w - - 1 58", "w", "-", "-", "d8d7 g2g1n", "g2 f1 h1", "n"},                                                           // bondeförvandling (s/v slag båda hållen och ickeslag, underförvandling)
		{"3K4/PPP1PPPP/8/8/8/8/ppp1pppp/3k4 w - - 1 58", "w", "-", "-", "d8d7 h2h1r", "h2 g1", "r"},                                                              // bondeförvandling (s/v slag båda hållen och ickeslag, underförvandling)
		{"rnbqk2r/1p1p1ppp/p3pn2/8/1bPNP3/2NB4/PP3PPP/R1BQK2R w Qkq - 9 10", "w", "Q", "-", "a2a3 e8f8", "a2 e8 g8", "k"},                                        // kungen drar (kolla rockad-status)
		{"rnbqk2r/1p1p1ppp/p3pn2/2b5/2PNP3/2NB4/PPQ2PPP/R1B1K2R w Qq - 11 11", "b", "q", "-", "e1d2", "e1 e2 d1 f1", "K"},                                        // kungen drar (kolla rockad-status)
		{"rnbqk2r/1p1p1ppp/p3pn2/2b5/2PNP3/2NB4/PPQ2PPP/R1B1K2R w KQkq - 11 11", "w", "", "-", "e1g1 e8g8", "e1 h1 e8 h8", "k"},                                  // Kort rockad (s/v) (kolla rockad-status)
		{"r3k2r/1pqb1ppp/p1n1pn2/2bp4/2PNP2P/2NB4/PPQB1PPR/R3K3 w Qkq - 2 15", "w", "", "-", "e1c1 e8c8", "e1 b1 a1 e8 b8 a8", "k"},                              // Lång rockad (s/v) (kolla rockad-status)
		{"r1b1k2r/1pqp1ppp/p1n1pn2/2b5/2PNP2P/2NB4/PPQB1PP1/R3K2R w KQkq - 1 13", "b", "Qkq", "-", "h1h3", "h1 h2", "R"},                                         // tornet drar (a-torn,) (kolla rockad-status)
		{"r1b1k2r/1pqp1ppp/p1n1pn2/2b5/2PNP2P/2NB4/PPQB1PP1/R3K2R w KQkq - 1 13", "b", "Kkq", "-", "a1c1", "a1 b1", "R"},                                         // tornet drar (h-torn,) (kolla rockad-status)
		{"r3k2r/1pqb1ppp/p1n1pn2/2bp4/2PNP2P/2NB3R/PPQB1PP1/R3K3 b Qkq - 3 15", "w", "Qk", "-", "a8a7", "a8 b8", "r"},                                            // tornet drar sv (a-torn,) (kolla rockad-status)
		{"4k2r/rpqb1ppp/p1n1pn2/2bp4/2PNP2P/2NB3R/PPQB1PP1/R3K3 w Qk - 4 16", "b", "Qk", "-", "c2a4 c6d4 a4d7 c7d7 d2g5 c5b4 d3c2", "d1 c6 a4 c7 d2 c5 d3", "B"}, // pjäser drar i serie
	}

	var bd Board

	for ix, toTest := range fenTest {
		moves := strings.Split(toTest[4], " ")
		saveMove := ""
		if len(moves) == 0 {
			continue
		}

		saveMove = moves[len(moves)-1]
		SetFen(toTest[0], &bd)
		FenMoves(moves, &bd)
		KollaBoard(t, ix, "testFen", toTest[1], toTest[2], toTest[3], &bd)

		//tomma rutor
		tomma := strings.Split(toTest[5], " ")
		for _, tom := range tomma {

			if len(tom) == 0 {
				continue
			}
			sq := square.FromString(tom)
			pc := bd.Square(sq)
			if pc != material.None {
				t.Errorf("Test=%v: Ruta %v borde innehålla %v (tom) men innehåller %v", ix, tom, material.None, pc)
			}
		}

		// ifylld to-ruta (sista draget)
		if saveMove != "" {
			sq := square.FromString(saveMove[2:4])
			p12 := material.MakeP12(bd.Square(sq), Opposite(bd.Stm()))
			strPiece := material.ToFen(p12)
			if strPiece != toTest[6] {
				t.Errorf("Test=%v: Ruta %v borde innehålla %v men innehåller %v (kan bero på att p_turn är felaktig)", ix, sq, toTest[6], strPiece)
			}
		}
	}

	// bonde drar två steg (kolla e.p)
	// utför e.p (s/v båda hållen)

}
func TestBB(t *testing.T) {
	//castling.Init()
	var fenTest = [...][4]string{
		{StartFen, "w", "KQkq", "-"},
		{"rnb1kb1r/1pqp1ppp/p3pn2/8/2PNP3/2NB4/PP1B1PPP/R2QK2R w KQ - 7 9", "w", "KQ", "-"},
		{"r1b1k2r/2qp1ppp/p1n1pn2/1pb5/2PNP3/2NB4/PP3PPP/R1BQK2R w kq - 0 10", "w", "kq", "-"},
		{"rnbqk2r/1p1p1ppp/p3pn2/8/1bPNP3/2NB4/PP3PPP/R1BQK2R w Qkq - 9 10", "w", "Qkq", "-"},
		{"rnbqk2r/1p1p1ppp/p3pn2/2b5/2PNP3/2NB4/PPQ2PPP/R1B1K2R w Qq - 11 11", "w", "Qq", "-"},
		{"1nbqk2r/rp1p1ppp/p3pn2/2b5/2PNP3/2NB4/PPQ2PPP/1RB1K2R w - - 13 12", "w", "", "-"},
		{"1nbqk2r/rp3ppp/p3pn2/2bpP3/2PN4/2NB4/PPQ2PPP/1RB1K2R w - d6 0 13", "w", "", "d6"},
		{"1nbqk2r/rp3ppp/4pn2/2bpP3/pPP5/2NB1N2/P1Q2PPP/1RB2K1R b - b3 0 15", "b", "", "b3"},
	}
	var bd Board
	for ix, toTest := range fenTest {
		SetFen(toTest[0], &bd)
		KollaBoard(t, ix, "Testbb", toTest[1], toTest[2], toTest[3], &bd)
	}

}

func KollaBoard(t *testing.T, ix int, caller string, turn string, rockader string, epSq string, bd *Board) {

	color := WHITE
	if turn == "b" {
		color = BLACK
	}

	// bball = bbwhite|bbblack
	if bd.all != bd.side[WHITE]|bd.side[BLACK] {
		t.Errorf("Test=%v %v: bball är inte samma som bbwhite | bbblack", caller, ix)
	}

	// bball == bbpawn|bbKNight|bbbishop...etc
	bbpieces := bd.piece[material.Pawn] |
		bd.piece[material.Knight] |
		bd.piece[material.Bishop] |
		bd.piece[material.Rook] |
		bd.piece[material.Queen] |
		bd.piece[material.King]

	if bd.All() != bbpieces {
		t.Errorf("Test=%v %v: bball är inte summan av alla bbpieces", caller, ix)
	}

	// turn is ok
	if bd.stm != color {
		t.Errorf("Test=%v %v: Borde vara %v vid draget men är %v", caller, ix, color, bd.stm)
	}

	// kungpos ok
	sq := bd.king[WHITE]
	if bd.getP12(sq) != material.WhiteKing {
		t.Errorf("Test=%v %v: Sparad vit Kungpos %v stämmer inte med pjäs från getP12(sq) = %v", caller, ix, sq, bd.getP12(sq))
	}
	sq = bd.king[BLACK]
	if bd.getP12(sq) != material.BlackKing {
		t.Errorf("Test=%v %v: Sparad svart Kungpos %v stämmer inte med pjäs från getP12(sq) = %v", caller, ix, sq, bd.getP12(sq))
	}

	// board och bbAll och bbPieces och count stämmer
	var egenCount [material.SideSize]int
	for rank := 7; rank >= 0; rank-- {
		for file := 0; file < square.FileSize; file++ {
			sq := square.Make(file, rank)
			pc := bd.Square(sq)
			p12 := bd.getP12(sq)
			if pc == material.None {
				if bit.Bit(sq)&bd.all != bit.BB(0) {
					t.Errorf("Test=%v %v: Tom ruta på board f= %v, r=%v stämmer inte med Ball ", caller, ix, file, rank)
				}
			} else {
				egenCount[p12]++
				if bit.Bit(sq)&bd.all != bit.Bit(sq) {
					t.Errorf("Test=%v %v:Icke tom ruta på board f= %v, r=%v stämmer inte med Ball ", caller, ix, file, rank)
				}
			}
		}
	}
	// count
	for p12 := material.WhitePawn; p12 <= material.BlackKing; p12++ {
		if bd.count[p12] != egenCount[p12] {
			fenP12 := material.ToFen(p12)
			t.Errorf("Test=%v %v: material_count=%v skall vara %v för piece=%v %v", caller, ix, bd.count[p12], egenCount[p12], p12, fenP12)
		}
	}

	// castling rights
	for i, c := range "KQkq" {
		if strings.Contains(rockader, string(c)) {
			if !CastleFlag(bd.copyStr.flags, uint(i)) {
				t.Errorf("Test=%v %v: rockad %v ej satt ", caller, ix, string(c))
			}

		} else {
			if CastleFlag(bd.copyStr.flags, uint(i)) {
				t.Errorf("Test=%v %v: rockad %v felaktigt satt ", caller, ix, string(c))
			}
		}

	}

	// ep
	if epSq == "-" {
		if bd.copyStr.epSq != square.None {
			t.Errorf("Test=%v %v: e.p. på ruta %v felaktigt satt", caller, ix, bd.copyStr.epSq)
		}
	} else {
		if bd.copyStr.epSq != square.FromString(epSq) {
			t.Errorf("Test=%v %v: e.p. på ruta %v borde vara satt", caller, ix, epSq)
		}
	}
}

func TestMakeFenMveUndoRestoresState(t *testing.T) {
	var bd Board
	SetFen(StartFen, &bd)

	origKey := bd.Key()
	origPawnKey := bd.PawnKey()
	origStm := bd.Stm()
	origPly := bd.Ply()
	origCount := bd.count
	origStackIx := bd.stackIx

	mv := FromString("e2e4", &bd)

	bd.MakeFenMve(mv)
	bd.Undo()

	if bd.Key() != origKey || bd.PawnKey() != origPawnKey {
		t.Fatalf("hash mismatch after undo: key %v/%v pawn %v/%v", bd.Key(), origKey, bd.PawnKey(), origPawnKey)
	}
	if bd.Stm() != origStm || bd.Ply() != origPly {
		t.Fatalf("state mismatch after undo: stm %v/%v ply %v/%v", bd.Stm(), origStm, bd.Ply(), origPly)
	}
	if bd.count != origCount {
		t.Fatalf("piece counts changed after undo: got %v want %v", bd.count, origCount)
	}
	if bd.stackIx != origStackIx {
		t.Fatalf("stack index not restored after undo: got %v want %v", bd.stackIx, origStackIx)
	}
}
