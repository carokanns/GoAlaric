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

func TestCampaignFullPreScanReusesBaselineAndProfilesCandidate(t *testing.T) {
	statePath, state := depthCampaignFixture(t, "full", []string{"20+0.2"})
	writeCachedCampaignProfile(t, state, state.Config.Baseline, "baseline", "20+0.2", 9)
	oldPreScan := campaignStartPreScan
	t.Cleanup(func() { campaignStartPreScan = oldPreScan })
	var started matchConfig
	campaignStartPreScan = func(cfg matchConfig) error {
		started = cfg
		return nil
	}
	if err := advanceDepthGate(statePath, &state); err != nil {
		t.Fatal(err)
	}
	if state.Status != "prescan_candidate_running" || started.ProfileRole != "candidate" {
		t.Fatalf("unexpected state=%s profile=%+v", state.Status, started)
	}
	if started.Candidate != state.Config.CandidateBinary || started.Baseline != state.Config.Baseline {
		t.Fatalf("candidate pre-scan engines are not candidate vs baseline: %+v", started)
	}
	if state.DepthGate.Baseline == nil || state.DepthGate.Baseline.MedianDepth != 9 {
		t.Fatalf("cached baseline was not reused: %+v", state.DepthGate)
	}
}

func TestCampaignBaselineGateRaisesTimeControlAndStartsScreening(t *testing.T) {
	statePath, state := depthCampaignFixture(t, "baseline", []string{"20+0.2", "30+0.3"})
	writeCachedCampaignProfile(t, state, state.Config.Baseline, "baseline", "20+0.2", 7)
	writeCachedCampaignProfile(t, state, state.Config.Baseline, "baseline", "30+0.3", 9)
	oldStart := campaignStartScreening
	t.Cleanup(func() { campaignStartScreening = oldStart })
	started := 0
	campaignStartScreening = func(campaignState) error { started++; return nil }
	if err := advanceDepthGate(statePath, &state); err != nil {
		t.Fatal(err)
	}
	if started != 1 || state.Status != "screening_running" || state.Config.SelectedTC != "30+0.3" {
		t.Fatalf("unexpected gate result: started=%d state=%s tc=%s", started, state.Status, state.Config.SelectedTC)
	}
}

func TestCampaignCandidatePreScanCompletionStartsScreening(t *testing.T) {
	statePath, state := depthCampaignFixture(t, "full", []string{"20+0.2"})
	state.Status = "prescan_candidate_running"
	state.DepthGate.CurrentTC = "20+0.2"
	state.DepthGate.CurrentRole = "candidate"
	state.DepthGate.Baseline = &campaignDepthSummary{Role: "baseline", MedianDepth: 9}
	state.DepthGate.RunDir = filepath.Join(state.Config.RepoRoot, "candidate-prescan")
	status := matchStatus{
		State: "completed", PID: 1 << 30, RunDir: state.DepthGate.RunDir,
		DepthProfile: &depthProfileReport{
			SampleCount: 200, MedianDepth: 8, P25Depth: 7, P90Depth: 10,
			Settings: depthProfileSettings{TimeControl: "20+0.2"},
		},
	}
	if err := writeJSON(filepath.Join(state.DepthGate.RunDir, "status.json"), status); err != nil {
		t.Fatal(err)
	}
	oldStart := campaignStartScreening
	t.Cleanup(func() { campaignStartScreening = oldStart })
	started := 0
	campaignStartScreening = func(campaignState) error { started++; return nil }
	if err := advanceCampaign(statePath, &state); err != nil {
		t.Fatal(err)
	}
	if started != 1 || state.Status != "screening_running" || state.Config.SelectedTC != "20+0.2" {
		t.Fatalf("unexpected completion transition: started=%d state=%s tc=%s", started, state.Status, state.Config.SelectedTC)
	}
}

func TestCampaignDepthGateStopsWhenLadderIsInsufficient(t *testing.T) {
	statePath, state := depthCampaignFixture(t, "baseline", []string{"20+0.2"})
	writeCachedCampaignProfile(t, state, state.Config.Baseline, "baseline", "20+0.2", 7)
	if err := advanceDepthGate(statePath, &state); err != nil {
		t.Fatal(err)
	}
	if state.Status != "depth_insufficient" || !terminalCampaignStatus(state.Status) {
		t.Fatalf("unexpected terminal depth state: %+v", state)
	}
}

func depthCampaignFixture(t *testing.T, mode string, timeControls []string) (string, campaignState) {
	t.Helper()
	statePath, state := campaignFixture(t)
	root := state.Config.RepoRoot
	state.Config.Baseline = writeTestExecutable(t, root, "baseline")
	state.Config.CandidateBinary = writeTestExecutable(t, root, "candidate")
	state.Config.Fastchess = writeTestExecutable(t, root, "fastchess")
	state.Config.Openings = filepath.Join(root, "book.pgn")
	if err := os.WriteFile(state.Config.Openings, []byte("book"), 0o644); err != nil {
		t.Fatal(err)
	}
	state.Config.PreScanMode = mode
	state.Config.MinimumDepth = 8
	state.Config.PreScanGames = 40
	state.Config.PreScanTCs = timeControls
	state.Config.Concurrency = 8
	state.Config.HashMB = 128
	state.Config.Threads = 1
	state.Config.PreScanSeed = defaultPreScanSeed
	state.DepthGate = &campaignDepthGate{Mode: mode, MinimumDepth: 8, TimeControls: timeControls}
	if err := saveCampaignState(statePath, &state); err != nil {
		t.Fatal(err)
	}
	return statePath, state
}

func writeCachedCampaignProfile(t *testing.T, state campaignState, engine, role, tc string, medianDepth int) {
	t.Helper()
	cfg := campaignPreScanConfig(state, engine, role, tc)
	identity, settings, key, err := depthProfileIdentity(cfg)
	if err != nil {
		t.Fatal(err)
	}
	report := depthProfileReport{
		SchemaVersion: depthProfileSchemaVersion, CacheKey: key, Role: role,
		Engine: identity, Settings: settings, SampleCount: 100, MedianDepth: medianDepth,
	}
	if err := writeJSON(filepath.Join(cfg.DepthCacheDir, key+".json"), report); err != nil {
		t.Fatal(err)
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
