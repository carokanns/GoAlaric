package main

import (
	"path/filepath"
	"testing"
)

func TestRootFixCorrectnessPolicyFlowsToScreeningAndSPRT(t *testing.T) {
	definition, err := resolveCandidateDefinition(changeClassCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if definition.ComparisonPolicy != comparisonPolicyBehavioral {
		t.Fatalf("rootfix policy=%q, want behavioral", definition.ComparisonPolicy)
	}

	root := t.TempDir()
	experiment := experimentConfig{
		ChangeClass: definition.ChangeClass, ComparisonPolicy: definition.ComparisonPolicy,
		Fastchess: "fake-fastchess", Openings: "fake-openings", ScreeningGames: 400, ScreeningTC: defaultScreeningTC,
	}
	runDir := filepath.Join(root, "pipeline-screening")
	oldScreeningRunner := runScreeningMatchCommand
	runScreeningMatchCommand = func(args []string) error {
		cfg, err := parseMatchConfig("screening-capture", args)
		if err != nil {
			return err
		}
		status := initialStatus(cfg)
		status.State = "completed"
		status.Decision = "passed_screening"
		return writeJSON(filepath.Join(cfg.RunDir, "status.json"), status)
	}
	t.Cleanup(func() { runScreeningMatchCommand = oldScreeningRunner })

	screening, err := runScreeningMatch("baseline", "candidate", experiment, runDir, filepath.Join(root, "screening.log"))
	if err != nil {
		t.Fatal(err)
	}
	assertBehavioralRootFixStatus(t, *screening)

	oldStarter := startMatchCommand
	var started []matchConfig
	startMatchCommand = func(args []string) error {
		cfg, err := parseMatchConfig("match-capture", args)
		if err != nil {
			return err
		}
		started = append(started, cfg)
		return nil
	}
	t.Cleanup(func() { startMatchCommand = oldStarter })

	state := campaignState{Config: campaignConfig{
		CandidateID: "rootfix", Baseline: "baseline", CandidateBinary: "candidate",
		ChangeClass: definition.ChangeClass, ComparisonPolicy: definition.ComparisonPolicy,
		Fastchess: "fake-fastchess", Openings: "fake-openings", RepoRoot: root,
		Codex: "fake-codex", Concurrency: 1, HashMB: 128, Threads: 1,
	}, ScreeningDir: filepath.Join(root, "campaign-screening")}
	if err := startCampaignScreening(state); err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 {
		t.Fatalf("campaign screening starts=%d, want 1", len(started))
	}
	assertBehavioralRootFixStatus(t, initialStatus(started[0]))

	if err := startAutomaticSPRT(matchConfig{
		Fastchess: "fake-fastchess", Baseline: "baseline", Candidate: "candidate", CandidateID: "rootfix",
		ChangeClass: definition.ChangeClass, ValidationPolicy: definition.ComparisonPolicy,
		Openings: "fake-openings", RepoRoot: root, Codex: "fake-codex", Concurrency: 1, HashMB: 128, Threads: 1,
		DrawMoveNumber: defaultDrawMoveNumber,
	}, filepath.Join(root, "sprt")); err != nil {
		t.Fatal(err)
	}
	if len(started) != 2 || !started[1].SPRT {
		t.Fatalf("automatic SPRT start=%+v", started)
	}
	assertBehavioralRootFixStatus(t, initialStatus(started[1]))
}

func assertBehavioralRootFixStatus(t *testing.T, status matchStatus) {
	t.Helper()
	if status.ChangeClass != changeClassCorrectness || status.ValidationPolicy != comparisonPolicyBehavioral {
		t.Fatalf("status class=%q policy=%q, want correctness/behavioral", status.ChangeClass, status.ValidationPolicy)
	}
}
