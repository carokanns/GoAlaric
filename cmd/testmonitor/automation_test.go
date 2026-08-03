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
	configureFakeCodex(t, root, &cfg, "{\"candidate_id\":\"candidate-auto\",\"recommendation\":\"sprt\",\"reason\":\"Screening passed\"}")
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

func TestAutomationRejectAwaitsManualDecisionWithoutSPRT(t *testing.T) {
	root, cfg, _ := automationFixture(t)
	configureFakeCodex(t, root, &cfg, "{\"candidate_id\":\"candidate-auto\",\"recommendation\":\"no_sprt\",\"reason\":\"Screening is not convincing\"}")
	oldStarter := automaticSPRTStarter
	automaticSPRTStarter = func(matchConfig, string) error { return errors.New("SPRT must not start") }
	t.Cleanup(func() { automaticSPRTStarter = oldStarter })

	if err := processMatchCompletion(cfg, completedMatchStatus(cfg, "screening-2", "passed_screening"), false); err != nil {
		t.Fatal(err)
	}
	var report experimentReport
	readTestJSON(t, filepath.Join(root, "artifacts", "experiments", cfg.CandidateID, "experiment.json"), &report)
	if report.Status != "awaiting_decision" || report.Decision == nil || report.Decision.Recommendation != "no_sprt" {
		t.Fatalf("unexpected experiment state: %+v", report)
	}
	if _, err := os.Stat(filepath.Join(root, "artifacts", "experiments", cfg.CandidateID, "approval-package.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected automatic approval package: %v", err)
	}
}

func TestFailedScreeningSkipsCodexAndAwaitsManualDecision(t *testing.T) {
	root, cfg, _ := automationFixture(t)
	configureFakeCodex(t, root, &cfg, "must-not-be-used")
	status := completedMatchStatus(cfg, "screening-failed", "rejected_below_47_percent")
	if err := processMatchCompletion(cfg, status, false); err != nil {
		t.Fatal(err)
	}
	if got := fakeCodexCalls(t, root); got != 0 {
		t.Fatalf("Codex calls = %d, want 0", got)
	}
	var report experimentReport
	readTestJSON(t, filepath.Join(root, "artifacts", "experiments", cfg.CandidateID, "experiment.json"), &report)
	if report.Status != "awaiting_decision" || report.Match == nil || report.Match.Decision != "rejected_below_47_percent" {
		t.Fatalf("unexpected failed-screening state: %+v", report)
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
	t.Setenv("FAKE_CODEX_DECISION", "{\"candidate_id\":\"candidate-auto\",\"recommendation\":\"no_sprt\",\"reason\":\"Do not run SPRT\"}")
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
	configureFakeCodex(t, root, &cfg, "{\"candidate_id\":\"candidate-auto\",\"recommendation\":\"sprt\",\"reason\":\"Screening passed\"}")
	oldStarter := automaticSPRTStarter
	automaticSPRTStarter = func(matchConfig, string) error {
		t.Fatal("SPRT started despite hard failure")
		return nil
	}
	t.Cleanup(func() { automaticSPRTStarter = oldStarter })
	if err := processMatchCompletion(cfg, completedMatchStatus(cfg, "screening-4", "passed_screening"), false); err != nil {
		t.Fatal(err)
	}
	if got := fakeCodexCalls(t, root); got != 0 {
		t.Fatalf("Codex calls = %d, want 0 for a hard failure", got)
	}
}

func TestEvaluationPromptExplainsNonSemanticCandidate(t *testing.T) {
	event := completionEvent{
		CandidateID: "candidate-lmr",
		MatchType:   "screening",
		Experiment: decisionInput{
			SemanticPreserving: false,
		},
	}
	prompt := evaluationPrompt(event)
	for _, required := range []string{
		`"semantic_preserving":false`,
		"Decide only whether this completed GoAlaric screening should start SPRT",
		"semantic_ok is a required invariant only when semantic_preserving is true",
		"must not be treated as correctness failures",
		"do not inspect files, run commands, propose changes, or recommend promotion",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("prompt omitted %q: %s", required, prompt)
		}
	}
}

func TestPostSPRTSkipsCodexAndAwaitsManualDecision(t *testing.T) {
	root, cfg, _ := automationFixture(t)
	cfg.SPRT = true
	configureFakeCodex(t, root, &cfg, "must-not-be-used")
	status := completedMatchStatus(cfg, "sprt-1", "accepted_h1")
	status.SPRTLLR, status.SPRTLower, status.SPRTUpper = 2.95, -2.94, 2.94
	if err := processMatchCompletion(cfg, status, false); err != nil {
		t.Fatal(err)
	}
	if got := fakeCodexCalls(t, root); got != 0 {
		t.Fatalf("Codex calls = %d, want 0 after SPRT", got)
	}
	var report experimentReport
	readTestJSON(t, filepath.Join(root, "artifacts", "experiments", cfg.CandidateID, "experiment.json"), &report)
	if report.Status != "awaiting_decision" || report.Match == nil || report.Match.Decision != "accepted_h1" {
		t.Fatalf("unexpected post-SPRT state: %+v", report)
	}
}

func TestRunMatchCallsCodexOnlyAfterFakeFastchessCompletes(t *testing.T) {
	root, cfg, _ := automationFixture(t)
	configureFakeCodex(t, root, &cfg, "{\"candidate_id\":\"candidate-auto\",\"recommendation\":\"no_sprt\",\"reason\":\"Screening result is insufficient\"}")
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
