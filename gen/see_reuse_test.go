package gen

import (
	"testing"

	"goalaric/board"
	"goalaric/eval"
	"goalaric/move"
)

func TestListReusesEquivalentSEEClassification(t *testing.T) {
	var bd board.Board
	board.SetFen("r3k2r/pp1n1ppp/2p1bn2/q2p4/3P4/2N1PN2/PPQ1BPPP/R3K2R w KQkq - 2 10", &bd)
	var attacks eval.Attacks
	eval.InitAttacks(&attacks, bd.Stm(), &bd)
	var killer Killer
	var history HistoryTab
	history.Clear()

	var list List
	list.Init(6, &bd, &attacks, move.None, &killer, &history, false)
	checked := 0
	for mv := list.Next(); mv != move.None; mv = list.Next() {
		if !list.seeKnown {
			continue
		}
		checked++
		if got, want := list.NoSacrifice(mv), NoSacrifice(mv, &bd); got != want {
			t.Errorf("NoSacrifice(%d) = %v, want %v", mv, got, want)
		}
		if got, want := list.IsWin(mv), IsWin(mv, &bd); got != want {
			t.Errorf("IsWin(%d) = %v, want %v", mv, got, want)
		}
	}
	if checked == 0 {
		t.Fatal("generator produced no reusable SEE classifications")
	}
}
