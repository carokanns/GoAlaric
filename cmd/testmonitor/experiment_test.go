package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateDecision(t *testing.T) {
	d := decision{CandidateID: "candidate-001", Recommendation: "propose_change", Reason: "fixed-depth gate passed"}
	if err := validateDecision("candidate-001", d); err != nil {
		t.Fatal(err)
	}
	if err := validateDecision("candidate-002", d); err == nil {
		t.Fatal("expected candidate mismatch")
	}
	if err := validateDecision("candidate-001", decision{CandidateID: "candidate-001", Recommendation: "unknown", Reason: "x"}); err == nil {
		t.Fatal("expected invalid recommendation")
	}
}

func TestDecisionStatusNeverPromotesAutomatically(t *testing.T) {
	if got := decisionStatus("promote"); got != "awaiting_approval" {
		t.Fatalf("status=%q, want awaiting_approval", got)
	}
	if got := decisionStatus("reject"); got != "rejected" {
		t.Fatalf("status=%q, want rejected", got)
	}
}

func TestReadPerftInputs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "perft.epd")
	content := "Position 1\nrnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1\n 4 197281\nPosition 2\nr3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq -\n4 4085603\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readPerftInputs(path, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Expected != 197281 || got[1].Expected != 4085603 {
		t.Fatalf("unexpected perft inputs: %+v", got)
	}
}

func TestCompactDecisionInputOmitsStableCases(t *testing.T) {
	semanticOK := true
	report := experimentReport{
		CandidateID: "candidate-001",
		Status:      "awaiting_decision",
		Stages:      []experimentStage{{Name: "go_test", Status: "passed"}},
		Benchmark: &benchmarkComparison{
			SemanticOK:      true,
			NPSDeltaPercent: 5,
			Cases: []benchmarkDelta{
				{FEN: "stable", SameNodes: true, SameScore: true, SameBestMove: true, NPSDeltaPercent: 5},
				{FEN: "changed", SameNodes: false, SameScore: true, SameBestMove: true, NPSDeltaPercent: -2},
			},
		},
	}
	report.Benchmark.SemanticOK = semanticOK
	input := compactDecisionInput(report, "/tmp/experiment")
	if len(input.Cases) != 1 || input.Cases[0].FEN != "changed" {
		t.Fatalf("unexpected compact cases: %+v", input.Cases)
	}
	data, err := json.Marshal(input)
	if err != nil || len(data) == 0 {
		t.Fatal("compact input did not marshal")
	}
	if !strings.Contains(string(data), `"semantic_preserving":false`) {
		t.Fatalf("compact input omitted semantic-preserving policy: %s", data)
	}
}

func TestCompactDecisionInputIncludesSemanticPreservingPolicy(t *testing.T) {
	report := experimentReport{
		CandidateID: "candidate-semantic",
		Config:      experimentConfig{SemanticPreserve: true},
		Benchmark:   &benchmarkComparison{SemanticOK: true},
	}
	input := compactDecisionInput(report, "/tmp/experiment")
	if !input.SemanticPreserving || input.SemanticOK == nil || !*input.SemanticOK {
		t.Fatalf("unexpected semantic policy: %+v", input)
	}
}

func TestRunExperimentRejectsIdenticalBinariesBeforeTests(t *testing.T) {
	dir := t.TempDir()
	baseline := filepath.Join(dir, "baseline")
	candidate := filepath.Join(dir, "candidate")
	for _, path := range []string{baseline, candidate} {
		if err := os.WriteFile(path, []byte("same-engine"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	_, _, err := runExperiment(dir, filepath.Join(dir, "experiment"), "candidate-identical", baseline, candidate, experimentConfig{})
	if err == nil || !strings.Contains(err.Error(), "identical SHA-256") {
		t.Fatalf("identical binaries reached the experiment pipeline: %v", err)
	}
}
