package search

import (
	"testing"

	"goalaric/parms"
)

func TestAspirationInitialMarginIsRuntimeTunable(t *testing.T) {
	original := parms.Search.AspirationInitialMarginCP
	t.Cleanup(func() { parms.Search.AspirationInitialMarginCP = original })

	parms.Search.AspirationInitialMarginCP = 5
	if got := aspirationInitialMargin(); got != 5 {
		t.Fatalf("initial aspiration margin=%d, want 5", got)
	}

	parms.Search.AspirationInitialMarginCP = 15
	if got := aspirationInitialMargin(); got != 15 {
		t.Fatalf("initial aspiration margin=%d, want 15", got)
	}
}

func TestAspirationInitialMarginFallsBackSafely(t *testing.T) {
	original := parms.Search.AspirationInitialMarginCP
	t.Cleanup(func() { parms.Search.AspirationInitialMarginCP = original })

	parms.Search.AspirationInitialMarginCP = 0
	if got := aspirationInitialMargin(); got != 10 {
		t.Fatalf("invalid initial aspiration margin=%d, want fallback 10", got)
	}
}
