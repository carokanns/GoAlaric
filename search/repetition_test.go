package search

import (
	"testing"

	"goalaric/parms"
)

func TestDefaultContemptComesFromParameters(t *testing.T) {
	if Engine.Contempt != parms.Search.Contempt {
		t.Fatalf("default contempt = %d, parameter = %d", Engine.Contempt, parms.Search.Contempt)
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
