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

func TestAspirationMinimumDepthIsRuntimeTunable(t *testing.T) {
	original := parms.Search.AspirationMinDepth
	t.Cleanup(func() { parms.Search.AspirationMinDepth = original })

	parms.Search.AspirationMinDepth = 5
	if got := aspirationMinDepth(); got != 5 {
		t.Fatalf("minimum aspiration depth=%d, want 5", got)
	}

	parms.Search.AspirationMinDepth = 7
	if got := aspirationMinDepth(); got != 7 {
		t.Fatalf("minimum aspiration depth=%d, want 7", got)
	}
}

func TestAspirationMinimumDepthFallsBackSafely(t *testing.T) {
	original := parms.Search.AspirationMinDepth
	t.Cleanup(func() { parms.Search.AspirationMinDepth = original })

	parms.Search.AspirationMinDepth = 0
	if got := aspirationMinDepth(); got != 6 {
		t.Fatalf("invalid minimum aspiration depth=%d, want fallback 6", got)
	}
}
