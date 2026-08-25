package search

import (
	"testing"

	"goalaric/parms"
)

func TestLateMovePruningUsesMultiplierAndKeepsDepthCap(t *testing.T) {
	original := parms.Search
	t.Cleanup(func() { parms.Search = original })

	parms.Search.LMPMoveMultiplier = 4
	for _, test := range []struct {
		name         string
		depth        int
		searchedSize int
		want         bool
	}{
		{name: "depth one below threshold", depth: 1, searchedSize: 3, want: false},
		{name: "depth one at threshold", depth: 1, searchedSize: 4, want: true},
		{name: "depth two below threshold", depth: 2, searchedSize: 7, want: false},
		{name: "depth two at threshold", depth: 2, searchedSize: 8, want: true},
		{name: "depth three at threshold", depth: 3, searchedSize: 12, want: true},
		{name: "depth four remains protected", depth: 4, searchedSize: 100, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := lateMovePrune(false, test.depth, 0, test.searchedSize, false); got != test.want {
				t.Fatalf("lateMovePrune()=%v, want %v", got, test.want)
			}
		})
	}

	parms.Search.LMPMoveMultiplier = 3
	if !lateMovePrune(false, 1, 0, 3, false) {
		t.Fatal("multiplier 3 did not prune at three searched moves")
	}
	parms.Search.LMPMoveMultiplier = 5
	if lateMovePrune(false, 1, 0, 4, false) {
		t.Fatal("multiplier 5 pruned before its threshold")
	}
	if !lateMovePrune(false, 1, 0, 5, false) {
		t.Fatal("multiplier 5 did not prune at its threshold")
	}
}

func TestLateMovePruningKeepsSafetyGuards(t *testing.T) {
	original := parms.Search
	t.Cleanup(func() { parms.Search = original })
	parms.Search.LMPMoveMultiplier = 4

	for name, got := range map[string]bool{
		"PV node":        lateMovePrune(true, 1, 0, 100, false),
		"dangerous move": lateMovePrune(false, 1, 0, 100, true),
		"mate score":     lateMovePrune(false, 1, mateScore, 100, false),
		"zero depth":     lateMovePrune(false, 0, 0, 100, false),
	} {
		if got {
			t.Errorf("%s was pruned", name)
		}
	}
}
