package search

import "testing"

func TestSearchRepetitionContemptIsRootRelative(t *testing.T) {
	for _, test := range []struct {
		ply  int
		want int
	}{
		{ply: 1, want: searchRepetitionContempt},
		{ply: 2, want: -searchRepetitionContempt},
		{ply: 3, want: searchRepetitionContempt},
	} {
		if got := repetitionScore(test.ply); got != test.want {
			t.Errorf("ply %d repetition score = %d, want %d", test.ply, got, test.want)
		}
	}
}
