package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAutomationInvokesCodexOnceAndStartsSPRTOnce(t *testing.T) {
	root, cfg, _ := automationFixture(t)
	configureFakeCodex(t, root, &cfg, "{\"candidate_id\":\"candidate-auto\",\"recommendation\":\"sprt\",\"next_change\":\"Run SPRT\",\"hypothesis\":\"Screening strength persists\",\"required_tests\":[\"sprt\"],\"reason\":\"Screening passed\"}")
	status := completedMatchStatus(cfg, "screening-1", "passed_screening")

	starts := 0
	oldStarter := automaticSPRTStarter
	automaticSPRTStarter = func(_ matchConfig, runDir string) error {
		starts++
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			return err
		}
		return writeJSON(filepath.Join(runDir, "status.json"), matchStatus{State: "starting", RunDir: runDir})
	}
	t.Cleanup(func() { automaticSPRTStarter = oldStarter })

	if err := processMatchCompletion(cfg, status, false); err != nil {
		t.Fatal(err)
	}
	if err := processMatchCompletion(cfg, status, false); err != nil {
		t.Fatal(err)
	}
	if got := fakeCodexCalls(t, root); got != 1 {
		t.Fatalf("Codex calls = %d, want 1", got)
	}
	if starts != 1 {
		t.Fatalf("SPRT starts = %d, want 1", starts)
	}
	var updated experimentReport
	readTestJSON(t, filepath.Join(root, "artifacts", "experiments", cfg.CandidateID, "experiment.json"), &updated)
	if updated.Status != "sprt_running" || updated.Decision == nil || updated.Decision.Recommendation != "sprt" {
		t.Fatalf("unexpected experiment state: %+v", updated)
	}
}

func TestAutomationDoesNotInvokeCodexBeforeTerminalState(t *testing.T) {
	root, cfg, _ := automationFixture(t)
	configureFakeCodex(t, root, &cfg, "{}")
	status := completedMatchStatus(cfg, "running-1", "")
	status.State = "running"
	if err := processMatchCompletion(cfg, status, false); err == nil {
		t.Fatal("non-terminal match was evaluated")
	}
	if got := fakeCodexCalls(t, root); got != 0 {
		t.Fatalf("Codex calls = %d, want 0", got)
	}
}

func TestAutomationRejectCreatesApprovalPackageWithoutSPRT(t *testing.T) {
	root, cfg, _ := automationFixture(t)
	configureFakeCodex(t, root, &cfg, "{\"candidate_id\":\"candidate-auto\",\"recommendation\":\"propose_change\",\"next_change\":\"Tune LMR\",\"hypothesis\":\"Safer reductions improve strength\",\"required_tests\":[\"go_test\",\"perft\",\"screening\"],\"reason\":\"Screening is not convincing\"}")
	oldStarter := automaticSPRTStarter
	automaticSPRTStarter = func(matchConfig, string) error { return errors.New("SPRT must not start") }
	t.Cleanup(func() { automaticSPRTStarter = oldStarter })

	if err := processMatchCompletion(cfg, completedMatchStatus(cfg, "screening-2", "rejected_below_47_percent"), false); err != nil {
		t.Fatal(err)
	}
	var pkg approvalPackage
	readTestJSON(t, filepath.Join(root, "artifacts", "experiments", cfg.CandidateID, "approval-package.json"), &pkg)
	if pkg.Status != "awaiting_approval" || pkg.BaselineRecommendation != "keep" || pkg.NextChange != "Tune LMR" {
		t.Fatalf("unexpected approval package: %+v", pkg)
	}
}

func TestAutomationInvalidDecisionRequiresExplicitRetry(t *testing.T) {
	root, cfg, _ := automationFixture(t)
	configureFakeCodex(t, root, &cfg, "not-json")
	status := completedMatchStatus(cfg, "screening-3", "passed_screening")
	if err := processMatchCompletion(cfg, status, false); err == nil {
		t.Fatal("invalid Codex output was accepted")
	}
	if got := fakeCodexCalls(t, root); got != 1 {
		t.Fatalf("Codex calls = %d, want 1", got)
	}
	if err := processMatchCompletion(cfg, status, false); err == nil || !strings.Contains(err.Error(), "retry-evaluation") {
		t.Fatalf("failed event retried without explicit retry: %v", err)
	}
	if got := fakeCodexCalls(t, root); got != 1 {
		t.Fatalf("Codex calls after implicit retry = %d, want 1", got)
	}
	t.Setenv("FAKE_CODEX_DECISION", "{\"candidate_id\":\"candidate-auto\",\"recommendation\":\"reject\",\"next_change\":\"Try LMR\",\"hypothesis\":\"LMR may help\",\"required_tests\":[\"go_test\"],\"reason\":\"Do not run SPRT\"}")
	if err := processMatchCompletion(cfg, status, true); err != nil {
		t.Fatal(err)
	}
	if got := fakeCodexCalls(t, root); got != 2 {
		t.Fatalf("Codex calls after retry = %d, want 2", got)
	}
}

func TestAutomationHardFailureBlocksAutomaticSPRT(t *testing.T) {
	root, cfg, report := automationFixture(t)
	report.HardFailures = []string{"perft mismatch"}
	writeTestJSON(t, filepath.Join(root, "artifacts", "experiments", cfg.CandidateID, "experiment.json"), report)
	configureFakeCodex(t, root, &cfg, "{\"candidate_id\":\"candidate-auto\",\"recommendation\":\"sprt\",\"next_change\":\"Run SPRT\",\"hypothesis\":\"Candidate is stronger\",\"required_tests\":[\"sprt\"],\"reason\":\"Screening passed\"}")
	oldStarter := automaticSPRTStarter
	automaticSPRTStarter = func(matchConfig, string) error {
		t.Fatal("SPRT started despite hard failure")
		return nil
	}
	t.Cleanup(func() { automaticSPRTStarter = oldStarter })
	if err := processMatchCompletion(cfg, completedMatchStatus(cfg, "screening-4", "passed_screening"), false); err == nil {
		t.Fatal("hard failure did not block automation")
	}
}

func TestPostSPRTDecisionRules(t *testing.T) {
	event := completionEvent{CandidateID: "candidate-auto", MatchType: "sprt", State: "completed", Decision: "accepted_h1"}
	promote := decision{CandidateID: event.CandidateID, Recommendation: "promote", NextChange: "Add LMR", Hypothesis: "LMR helps", RequiredTests: []string{"go_test"}, Reason: "H1 accepted"}
	if err := validateAutomationDecision(event, promote); err != nil {
		t.Fatal(err)
	}
	for _, test := range []completionEvent{
		{CandidateID: event.CandidateID, MatchType: "sprt", State: "completed", Decision: "rejected_h0"},
		{CandidateID: event.CandidateID, MatchType: "sprt", State: "completed", Decision: "inconclusive_at_game_limit"},
		{CandidateID: event.CandidateID, MatchType: "sprt", State: "stopped", Decision: "stopped_by_user"},
	} {
		if err := validateAutomationDecision(test, promote); err == nil {
			t.Fatalf("promotion accepted for %+v", test)
		}
	}
}

func TestPostSPRTPromotionCreatesApprovalPackage(t *testing.T) {
	root, cfg, report := automationFixture(t)
	cfg.SPRT = true
	configureFakeCodex(t, root, &cfg, "{\"candidate_id\":\"candidate-auto\",\"recommendation\":\"promote\",\"next_change\":\"Add LMR\",\"hypothesis\":\"LMR improves search\",\"required_tests\":[\"go_test\",\"screening\"],\"reason\":\"SPRT accepted H1\"}")
	status := completedMatchStatus(cfg, "sprt-1", "accepted_h1")
	status.SPRTLLR, status.SPRTLower, status.SPRTUpper = 2.95, -2.94, 2.94
	if err := processMatchCompletion(cfg, status, false); err != nil {
		t.Fatal(err)
	}
	var pkg approvalPackage
	readTestJSON(t, filepath.Join(root, "artifacts", "experiments", cfg.CandidateID, "approval-package.json"), &pkg)
	if pkg.BaselineRecommendation != "promote" || pkg.RecommendedBaseline.SHA256 != report.Candidate.SHA256 || pkg.NextChange != "Add LMR" {
		t.Fatalf("unexpected post-SPRT package: %+v", pkg)
	}
}

func TestRunMatchCallsCodexOnlyAfterFakeFastchessCompletes(t *testing.T) {
	root, cfg, _ := automationFixture(t)
	configureFakeCodex(t, root, &cfg, "{\"candidate_id\":\"candidate-auto\",\"recommendation\":\"reject\",\"next_change\":\"Try LMR\",\"hypothesis\":\"LMR may improve ordering\",\"required_tests\":[\"go_test\"],\"reason\":\"Screening result is insufficient\"}")
	fastchess := filepath.Join(root, "fake-fastchess")
	script := "#!/bin/sh\n" +
		"if [ -e \"$FAKE_CODEX_COUNT\" ]; then exit 91; fi\n" +
		"pgn=\"\"\n" +
		"while [ \"$#\" -gt 0 ]; do\n" +
		"  if [ \"$1\" = \"-pgnout\" ]; then shift; pgn=$(printf '%s' \"$1\" | sed 's/^file=//'); break; fi\n" +
		"  shift\n" +
		"done\n" +
		": > \"$pgn\"\n" +
		"i=1\n" +
		"while [ \"$i\" -le 200 ]; do\n" +
		"  printf '[Event \"match\"]\\n[Round \"%s.1\"]\\n[FEN \"fen-%s\"]\\n\\n1. e2e4 1/2-1/2\\n\\n' \"$i\" \"$i\" >> \"$pgn\"\n" +
		"  printf '[Event \"match\"]\\n[Round \"%s.2\"]\\n[FEN \"fen-%s\"]\\n\\n1. e2e4 1/2-1/2\\n\\n' \"$i\" \"$i\" >> \"$pgn\"\n" +
		"  i=$((i + 1))\n" +
		"done\n" +
		"echo 'Score of Candidate vs Baseline: 100 - 80 - 220  [0.525] 400'\n"
	if err := os.WriteFile(fastchess, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	book := filepath.Join(root, "book.epd")
	var lines []string
	for ix := 0; ix < 200; ix++ {
		lines = append(lines, "8/8/8/8/8/8/8/8 w - - id "+strconv.Itoa(ix))
	}
	if err := os.WriteFile(book, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(root, "match")
	args := []string{"--fastchess", fastchess, "--baseline", cfg.Baseline, "--candidate", cfg.Candidate, "--openings", book, "--games", "400", "--run-dir", runDir, "--candidate-id", cfg.CandidateID, "--auto-evaluate", "--codex", cfg.Codex, "--repo-root", root}
	if err := runMatchCommand(args); err != nil {
		t.Fatal(err)
	}
	if got := fakeCodexCalls(t, root); got != 1 {
		t.Fatalf("Codex calls = %d, want 1", got)
	}
}

func automationFixture(t *testing.T) (string, matchConfig, experimentReport) {
	t.Helper()
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	candidate := filepath.Join(root, "candidate")
	for path, data := range map[string]string{baseline: "baseline", candidate: "candidate"} {
		if err := os.WriteFile(path, []byte(data), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	baseID, err := identifyExperimentBinary(baseline)
	if err != nil {
		t.Fatal(err)
	}
	candidateID, err := identifyExperimentBinary(candidate)
	if err != nil {
		t.Fatal(err)
	}
	cfg := matchConfig{Baseline: baseline, Candidate: candidate, CandidateID: "candidate-auto", AutoEvaluate: true, RepoRoot: root, Games: 400, TC: defaultScreeningTC, RunDir: filepath.Join(root, "match")}
	report := experimentReport{
		SchemaVersion: experimentSchemaVersion, CandidateID: cfg.CandidateID, Status: "awaiting_decision",
		Baseline: baseID, Candidate: candidateID,
		Config: experimentConfig{Hypothesis: "Improve move ordering", ProposedChange: "Candidate change"},
		Stages: []experimentStage{{Name: "go_test", Status: "passed"}, {Name: "perft", Status: "passed"}, {Name: "uci", Status: "passed"}, {Name: "benchmark", Status: "passed"}, {Name: "movetime", Status: "passed"}},
	}
	dir := filepath.Join(root, "artifacts", "experiments", cfg.CandidateID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestJSON(t, filepath.Join(dir, "experiment.json"), report)
	return root, cfg, report
}

func completedMatchStatus(cfg matchConfig, runID, matchDecision string) matchStatus {
	return matchStatus{
		RunID: runID, State: "completed", Stage: "finished", StartedAt: time.Now().Add(-time.Minute), FinishedAt: time.Now(),
		Baseline: cfg.Baseline, Candidate: cfg.Candidate, Games: 400, Wins: 100, Draws: 220, Losses: 80, Score: 52.5,
		Decision: matchDecision, RunDir: cfg.RunDir, PGNAudit: &pgnAudit{Games: 400, UniqueOpenings: 200},
	}
}

func configureFakeCodex(t *testing.T, root string, cfg *matchConfig, output string) {
	t.Helper()
	path := filepath.Join(root, "fake-codex")
	script := "#!/bin/sh\n" +
		"out=\"\"\n" +
		"while [ \"$#\" -gt 0 ]; do\n" +
		"  if [ \"$1\" = \"--output-last-message\" ]; then shift; out=\"$1\"; fi\n" +
		"  shift\n" +
		"done\n" +
		"n=0\n" +
		"if [ -f \"$FAKE_CODEX_COUNT\" ]; then n=$(cat \"$FAKE_CODEX_COUNT\"); fi\n" +
		"n=$((n + 1))\n" +
		"printf '%s\\n' \"$n\" > \"$FAKE_CODEX_COUNT\"\n" +
		"printf '%s\\n' \"$FAKE_CODEX_DECISION\" > \"$out\"\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg.Codex = path
	t.Setenv("FAKE_CODEX_COUNT", filepath.Join(root, "codex-count"))
	t.Setenv("FAKE_CODEX_DECISION", output)
}

func fakeCodexCalls(t *testing.T, root string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "codex-count"))
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := writeJSON(path, value); err != nil {
		t.Fatal(err)
	}
}

func readTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatal(err)
	}
}
