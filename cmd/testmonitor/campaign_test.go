package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCampaignStagesPassed(t *testing.T) {
	if defaultCampaignPoll != 10*time.Second {
		t.Fatalf("campaign poll interval = %s, want 10s", defaultCampaignPoll)
	}
	passed := []experimentStage{
		{Name: "go_test", Status: "passed"},
		{Name: "perft", Status: "passed"},
		{Name: "uci", Status: "passed"},
		{Name: "benchmark", Status: "passed"},
		{Name: "movetime", Status: "passed"},
	}
	if !campaignStagesPassed(passed) {
		t.Fatal("complete passing pipeline was rejected")
	}
	passed[1].Status = "failed"
	if campaignStagesPassed(passed) {
		t.Fatal("failed perft was accepted")
	}
}

func TestCampaignBuildsTestsAndStartsScreening(t *testing.T) {
	statePath, state := campaignFixture(t)
	oldBuild, oldPipeline, oldStart := campaignBuildCandidate, campaignRunPipeline, campaignStartScreening
	t.Cleanup(func() {
		campaignBuildCandidate, campaignRunPipeline, campaignStartScreening = oldBuild, oldPipeline, oldStart
	})
	built, tested, started := 0, 0, 0
	campaignBuildCandidate = func(campaignState) error { built++; return nil }
	campaignRunPipeline = func(s campaignState) error {
		tested++
		report := experimentReport{Stages: []experimentStage{
			{Name: "go_test", Status: "passed"}, {Name: "perft", Status: "passed"},
			{Name: "uci", Status: "passed"}, {Name: "benchmark", Status: "passed"},
			{Name: "movetime", Status: "passed"},
		}}
		return writeJSON(filepath.Join(s.ExperimentDir, "experiment.json"), report)
	}
	campaignStartScreening = func(s campaignState) error {
		started++
		return writeJSON(filepath.Join(s.ScreeningDir, "status.json"), matchStatus{State: "running", PID: os.Getpid(), RunDir: s.ScreeningDir})
	}
	if err := advanceCampaign(statePath, &state); err != nil {
		t.Fatal(err)
	}
	if built != 1 || tested != 1 || started != 1 || state.Status != "screening_running" {
		t.Fatalf("unexpected campaign transition: built=%d tested=%d started=%d state=%s", built, tested, started, state.Status)
	}
}

func TestCampaignStopsForManualDecisionWithoutSPRT(t *testing.T) {
	statePath, state := campaignFixture(t)
	state.Status = "screening_running"
	status := matchStatus{
		State: "completed", Decision: "passed_screening", PID: 1 << 30,
		Games: 400, Wins: 90, Draws: 220, Losses: 90, Score: 50,
		RunDir: state.ScreeningDir,
	}
	if err := writeJSON(filepath.Join(state.ScreeningDir, "status.json"), status); err != nil {
		t.Fatal(err)
	}
	if err := advanceCampaign(statePath, &state); err != nil {
		t.Fatal(err)
	}
	if state.Status != "awaiting_decision" || state.Screening == nil || state.Screening.Games != 400 {
		t.Fatalf("unexpected terminal screening state: %+v", state)
	}
}

func TestCampaignTracksAutomaticSPRTToCompletion(t *testing.T) {
	statePath, state := campaignFixture(t)
	state.Status = "screening_running"
	if err := writeJSON(filepath.Join(state.ScreeningDir, "status.json"), matchStatus{State: "completed", PID: 1 << 30, RunDir: state.ScreeningDir}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(state.SPRTRunDir, "status.json"), matchStatus{State: "running", PID: os.Getpid(), RunDir: state.SPRTRunDir}); err != nil {
		t.Fatal(err)
	}
	if err := advanceCampaign(statePath, &state); err != nil {
		t.Fatal(err)
	}
	if state.Status != "sprt_running" {
		t.Fatalf("state=%s, want sprt_running", state.Status)
	}
	if err := writeJSON(filepath.Join(state.SPRTRunDir, "status.json"), matchStatus{
		State: "completed", Decision: "accepted_h1", PID: 1 << 30, RunDir: state.SPRTRunDir,
		Games: 1200, Wins: 300, Draws: 650, Losses: 250, Score: 52.1, SPRTLLR: 3,
	}); err != nil {
		t.Fatal(err)
	}
	if err := advanceCampaign(statePath, &state); err != nil {
		t.Fatal(err)
	}
	if state.Status != "awaiting_decision" || state.SPRT == nil || state.SPRT.Decision != "accepted_h1" {
		t.Fatalf("unexpected terminal SPRT state: %+v", state)
	}
}

func campaignFixture(t *testing.T) (string, campaignState) {
	t.Helper()
	root := t.TempDir()
	state := campaignState{
		SchemaVersion: campaignSchemaVersion,
		Status:        "queued",
		StartedAt:     time.Now(),
		Config:        campaignConfig{CandidateID: "candidate-test", RepoRoot: root},
		ExperimentDir: filepath.Join(root, "artifacts", "experiments", "candidate-test"),
		ScreeningDir:  filepath.Join(root, "artifacts", "matches", "candidate-test-screening"),
		SPRTRunDir:    filepath.Join(root, "artifacts", "matches", "candidate-test-sprt-candidate-test-screening"),
	}
	statePath := filepath.Join(root, "artifacts", "automation", "active-campaign.json")
	if err := saveCampaignState(statePath, &state); err != nil {
		t.Fatal(err)
	}
	return statePath, state
}
