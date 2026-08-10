package search

import (
	"testing"

	"goalaric/parms"
	"goalaric/syzygy"
)

func TestDefaultContemptComesFromParameters(t *testing.T) {
	if Engine.Contempt != parms.Search.Contempt {
		t.Fatalf("default contempt = %d, parameter = %d", Engine.Contempt, parms.Search.Contempt)
	}
}

func TestTablebaseScore(t *testing.T) {
	original := Engine.Contempt
	Engine.Contempt = 5
	t.Cleanup(func() { Engine.Contempt = original })

	if got := tablebaseScore(syzygy.Win, 7); got != tablebaseWinScore {
		t.Fatalf("win score = %d", got)
	}
	if got := tablebaseScore(syzygy.Loss, 7); got != -tablebaseWinScore {
		t.Fatalf("loss score = %d", got)
	}
	if got := tablebaseScore(syzygy.CursedWin, 7); got != 1 {
		t.Fatalf("cursed win score = %d", got)
	}
	if got := tablebaseScore(syzygy.BlessedLoss, 7); got != -1 {
		t.Fatalf("blessed loss score = %d", got)
	}
	if got := tablebaseScore(syzygy.Draw, 1); got != Engine.Contempt {
		t.Fatalf("draw score = %d", got)
	}
}

func TestDrawContemptIsRootRelative(t *testing.T) {
	original := Engine.Contempt
	Engine.Contempt = 7
	t.Cleanup(func() { Engine.Contempt = original })

	for _, test := range []struct {
		ply  int
		want int
	}{
		{ply: 0, want: 0},
		{ply: 1, want: 7},
		{ply: 2, want: -7},
		{ply: 3, want: 7},
	} {
		if got := drawScore(test.ply); got != test.want {
			t.Errorf("ply %d draw score = %d, want %d", test.ply, got, test.want)
		}
	}
}
