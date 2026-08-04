package search

import "testing"

func TestNullMoveReductionGrowsConservativelyWithDepth(t *testing.T) {
	tests := []struct {
		depth int
		want  int
	}{
		{depth: 4, want: 3},
		{depth: 5, want: 3},
		{depth: 6, want: 4},
		{depth: 11, want: 4},
		{depth: 12, want: 5},
		{depth: maxDepth, want: 5},
	}

	for _, test := range tests {
		if got := nullMoveReduction(test.depth); got != test.want {
			t.Errorf("depth %d reduction = %d, want %d", test.depth, got, test.want)
		}
	}
}

func TestNullMoveDynamicSearchDepthIsNonNegative(t *testing.T) {
	for depth := 4; depth <= maxDepth; depth++ {
		if childDepth := depth - nullMoveReduction(depth) - 1; childDepth < 0 {
			t.Fatalf("depth %d gives negative child depth %d", depth, childDepth)
		}
	}
}
