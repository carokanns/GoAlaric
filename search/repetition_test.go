package search

import (
	"testing"

	"goalaric/hash"
	"goalaric/parms"
	"goalaric/syzygy"
)

func preserveContempt(t *testing.T) {
	t.Helper()
	oldContempt := Engine.Contempt
	oldRepetitionContempt := Engine.SearchRepetitionContempt
	oldStatus := SearchStatus()
	t.Cleanup(func() {
		Engine.Contempt = oldContempt
		Engine.SearchRepetitionContempt = oldRepetitionContempt
		setSearchStatus(oldStatus)
		SG.Trans.Clear()
	})
}

func TestDefaultContemptComesFromParameters(t *testing.T) {
	if Engine.Contempt != parms.Search.Contempt || Engine.Contempt != DefaultContempt {
		t.Fatalf("default contempt = %d, parameter = %d", Engine.Contempt, parms.Search.Contempt)
	}
	if Engine.SearchRepetitionContempt != parms.Search.SearchRepetitionContempt || Engine.SearchRepetitionContempt != DefaultSearchRepetitionContempt {
		t.Fatalf("default repetition contempt = %d, parameter = %d", Engine.SearchRepetitionContempt, parms.Search.SearchRepetitionContempt)
	}
}

func TestSearchRepetitionContemptIsRootRelative(t *testing.T) {
	preserveContempt(t)
	Engine.Contempt = DefaultContempt
	Engine.SearchRepetitionContempt = DefaultSearchRepetitionContempt

	for _, test := range []struct {
		ply  int
		want int
	}{
		{ply: 1, want: DefaultSearchRepetitionContempt},
		{ply: 2, want: -DefaultSearchRepetitionContempt},
		{ply: 3, want: DefaultSearchRepetitionContempt},
	} {
		if got := repetitionScore(test.ply); got != test.want {
			t.Errorf("ply %d repetition score = %d, want %d", test.ply, got, test.want)
		}
	}
}

func TestGeneralContemptAppliesToAllDrawsAndOverridesRepetition(t *testing.T) {
	preserveContempt(t)
	Engine.SearchRepetitionContempt = 7
	Engine.Contempt = DefaultContempt
	if got := drawScore(1); got != 0 {
		t.Fatalf("draw score with neutral Contempt = %d, want 0", got)
	}
	if got := repetitionScore(1); got != 7 {
		t.Fatalf("delegated repetition score = %d, want 7", got)
	}

	Engine.Contempt = 10
	for _, test := range []struct {
		ply  int
		want int
	}{
		{ply: 0, want: -10},
		{ply: 1, want: 10},
		{ply: 2, want: -10},
	} {
		if got := drawScore(test.ply); got != test.want {
			t.Errorf("ply %d draw score = %d, want %d", test.ply, got, test.want)
		}
		if got := repetitionScore(test.ply); got != test.want {
			t.Errorf("ply %d repetition score = %d, want override %d", test.ply, got, test.want)
		}
	}
}

func TestTablebaseScoreUsesGeneralContempt(t *testing.T) {
	preserveContempt(t)
	Engine.Contempt = 10

	if got := tablebaseScore(syzygy.Win, 7); got != tablebaseWinScore {
		t.Fatalf("win score = %d", got)
	}
	if got := tablebaseScore(syzygy.Loss, 7); got != -tablebaseWinScore {
		t.Fatalf("loss score = %d", got)
	}
	if got := tablebaseScore(syzygy.CursedWin, 7); got != 11 {
		t.Fatalf("cursed win score = %d, want 11", got)
	}
	if got := tablebaseScore(syzygy.BlessedLoss, 7); got != 9 {
		t.Fatalf("blessed loss score = %d, want 9", got)
	}
	if got := tablebaseScore(syzygy.Draw, 1); got != 10 {
		t.Fatalf("draw score = %d, want 10", got)
	}
}

func TestContemptSettersValidateStateRangeAndInvalidateTT(t *testing.T) {
	preserveContempt(t)
	setSearchStatus(idle)
	Engine.Contempt = DefaultContempt
	Engine.SearchRepetitionContempt = DefaultSearchRepetitionContempt

	key := hash.Key(0x456789abcdef0123)
	SG.Trans.Store(key, 1, 0, 20, 45, scoreTypeBetween)
	if err := SetContempt(9); err != nil {
		t.Fatal(err)
	}
	var mv, sc, flags int
	if SG.Trans.Retrieve(key, 1, 0, &mv, &sc, &flags) {
		t.Fatal("Contempt change left stale TT entry available")
	}

	if err := SetContempt(MaxContempt + 1); err == nil || Engine.Contempt != 9 {
		t.Fatalf("out-of-range Contempt changed value to %d, err=%v", Engine.Contempt, err)
	}
	if err := SetSearchRepetitionContempt(MinContempt - 1); err == nil || Engine.SearchRepetitionContempt != DefaultSearchRepetitionContempt {
		t.Fatalf("out-of-range repetition contempt changed value to %d, err=%v", Engine.SearchRepetitionContempt, err)
	}

	setSearchStatus(Running)
	if err := SetContempt(8); err == nil || Engine.Contempt != 9 {
		t.Fatalf("active search accepted Contempt: value=%d err=%v", Engine.Contempt, err)
	}
	if err := SetSearchRepetitionContempt(8); err == nil || Engine.SearchRepetitionContempt != DefaultSearchRepetitionContempt {
		t.Fatalf("active search accepted repetition contempt: value=%d err=%v", Engine.SearchRepetitionContempt, err)
	}
}
