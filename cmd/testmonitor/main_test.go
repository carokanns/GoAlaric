package main

import (
	"bytes"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestScreeningProgressEveryTenGames(t *testing.T) {
	dir := t.TempDir()
	status := matchStatus{RunID: "screening", State: "running", Stage: "fastchess", TargetGames: 400, RunDir: dir}
	var output bytes.Buffer
	parser := matchOutput{status: &status, runDir: dir, dst: &output, progressEvery: 10, nextProgress: 10}
	parser.process("Score of Candidate vs Baseline: 3 - 2 - 4  [0.556] 9")
	if _, err := os.Stat(filepath.Join(dir, "progress.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("progress written before ten games: %v", err)
	}
	parser.process("Score of Candidate vs Baseline: 3 - 2 - 5  [0.550] 10")
	snapshots, err := readProgressSnapshots(filepath.Join(dir, "progress.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 || snapshots[0].Games != 10 || snapshots[0].ScorePercent != 55 {
		t.Fatalf("unexpected progress snapshots: %+v", snapshots)
	}
}

func TestSPRTProgressWaitsForLLR(t *testing.T) {
	dir := t.TempDir()
	status := matchStatus{RunID: "sprt", State: "running", Stage: "fastchess", TargetGames: 10000, RunDir: dir}
	var output bytes.Buffer
	parser := matchOutput{status: &status, runDir: dir, dst: &output, progressEvery: 50, nextProgress: 50, sprt: true}
	parser.process("Score of Candidate vs Baseline: 10 - 12 - 28  [0.480] 50")
	if _, err := os.Stat(filepath.Join(dir, "progress.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("SPRT progress written before LLR: %v", err)
	}
	parser.process("SPRT: llr -0.25 (1.0%), lbound -2.94, ubound 2.94")
	snapshots, err := readProgressSnapshots(filepath.Join(dir, "progress.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 || snapshots[0].Games != 50 || snapshots[0].SPRTLLR != -0.25 {
		t.Fatalf("unexpected SPRT progress snapshots: %+v", snapshots)
	}
}

func TestProgressIntervalDefaults(t *testing.T) {
	dir := t.TempDir()
	fastchess := filepath.Join(dir, "fastchess")
	baseline := filepath.Join(dir, "baseline")
	candidate := filepath.Join(dir, "candidate")
	for _, path := range []string{fastchess, baseline, candidate} {
		if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	book := filepath.Join(dir, "book.epd")
	lines := make([]string, 100)
	for ix := range lines {
		lines[ix] = "8/8/8/8/8/8/8/8 w - - id " + strconv.Itoa(ix)
	}
	if err := os.WriteFile(book, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	base := matchConfig{Fastchess: fastchess, Baseline: baseline, Candidate: candidate, Openings: book, Games: 100, Concurrency: 8, RunDir: filepath.Join(dir, "run")}
	screening, err := normalizeConfig(base)
	if err != nil {
		t.Fatal(err)
	}
	if screening.ProgressEvery != 10 {
		t.Fatalf("screening progress interval = %d, want 10", screening.ProgressEvery)
	}
	if screening.TC != defaultScreeningTC {
		t.Fatalf("screening time control = %q, want %q", screening.TC, defaultScreeningTC)
	}
	if screening.ProgressTime != defaultProgressInterval {
		t.Fatalf("screening progress interval = %q, want %q", screening.ProgressTime, defaultProgressInterval)
	}
	base.SPRT = true
	base.TC = ""
	sprt, err := normalizeConfig(base)
	if err != nil {
		t.Fatal(err)
	}
	if sprt.ProgressEvery != 50 {
		t.Fatalf("SPRT progress interval = %d, want 50", sprt.ProgressEvery)
	}
	if sprt.TC != defaultSPRTTC {
		t.Fatalf("SPRT time control = %q, want %q", sprt.TC, defaultSPRTTC)
	}
}

func TestSyzygyPathUsesLocalTablesByDefault(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"fastchess", "baseline", "candidate"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	book := filepath.Join(dir, "book.epd")
	var openings []string
	for ix := 0; ix < 100; ix++ {
		openings = append(openings, "8/8/8/8/8/8/8/8 w - - id "+strconv.Itoa(ix))
	}
	if err := os.WriteFile(book, []byte(strings.Join(openings, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	tables := filepath.Join(dir, defaultSyzygyPath)
	if err := os.MkdirAll(tables, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, err := normalizeConfig(matchConfig{
		Fastchess: filepath.Join(dir, "fastchess"), Baseline: filepath.Join(dir, "baseline"), Candidate: filepath.Join(dir, "candidate"),
		Openings: book, Games: 100, Concurrency: 1, RunDir: filepath.Join(dir, "run"), RepoRoot: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SyzygyPath != tables {
		t.Fatalf("Syzygy path = %q, want %q", cfg.SyzygyPath, tables)
	}
	for _, engine := range []string{cfg.Candidate, cfg.Baseline} {
		if !strings.Contains(strings.Join(fastchessEngineArgs(engine, "test", cfg), " "), "option.SyzygyPath="+tables) {
			t.Fatalf("Fastchess arguments omitted SyzygyPath for %s", engine)
		}
	}
}

func TestSyzygyPathCanBeDisabled(t *testing.T) {
	dir := t.TempDir()
	path, err := resolveSyzygyPath(dir, "off")
	if err != nil {
		t.Fatal(err)
	}
	if path != "" {
		t.Fatalf("disabled Syzygy path = %q, want empty", path)
	}
}

func TestDefaultSPRTPolicy(t *testing.T) {
	alpha, err := strconv.ParseFloat(defaultSPRTAlpha, 64)
	if err != nil {
		t.Fatal(err)
	}
	beta, err := strconv.ParseFloat(defaultSPRTBeta, 64)
	if err != nil {
		t.Fatal(err)
	}
	lower := math.Log(beta / (1 - alpha))
	upper := math.Log((1 - beta) / alpha)
	if math.Abs(lower-(-1.5686159)) > 0.0001 || math.Abs(upper-2.9957323) > 0.0001 {
		t.Fatalf("SPRT bounds = [%.6f, %.6f], want about [-1.57, 3.00]", lower, upper)
	}
	if defaultSPRTTC != defaultScreeningTC {
		t.Fatalf("SPRT time control = %q, want screening time %q", defaultSPRTTC, defaultScreeningTC)
	}
	if defaultSPRTTC != "20+0.2" {
		t.Fatalf("SPRT time control = %q, want 20+0.2", defaultSPRTTC)
	}
	if defaultResignScore != 500 {
		t.Fatalf("resign adjudication score = %d, want 500", defaultResignScore)
	}
}

func TestFollowProgressReturnsAfterCompletedMatch(t *testing.T) {
	dir := t.TempDir()
	status := matchStatus{
		RunID: "completed", State: "completed", Stage: "complete", TargetGames: 10,
		Games: 10, Wins: 4, Draws: 4, Losses: 2, Score: 60, RunDir: dir,
	}
	if err := saveStatus(dir, &status); err != nil {
		t.Fatal(err)
	}
	if err := appendProgressSnapshot(dir, status); err != nil {
		t.Fatal(err)
	}
	if err := followProgress(dir, time.Millisecond); err != nil {
		t.Fatal(err)
	}
}

func TestStopHelper(t *testing.T) {
	if os.Getenv("GOALARIC_STOP_HELPER") != "1" {
		return
	}
	for {
		time.Sleep(time.Second)
	}
}

func TestStopCommandTerminatesMonitor(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestStopHelper", "--", "run-match", "--run-dir", dir)
	cmd.Env = append(os.Environ(), "GOALARIC_STOP_HELPER=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	status := matchStatus{RunID: "stop-test", State: "running", Stage: "fastchess", PID: cmd.Process.Pid, RunDir: dir, StartedAt: time.Now()}
	if err := saveStatus(dir, &status); err != nil {
		t.Fatal(err)
	}
	if err := stopCommand([]string{"--run-dir", dir, "--timeout", "5s"}); err != nil {
		t.Fatal(err)
	}
	stopped, err := loadStatus(dir)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.State != "stopped" || stopped.Decision != "stopped_by_user" {
		t.Fatalf("unexpected stopped status: %+v", stopped)
	}
	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("helper process was not reaped")
	}
}

func TestParseScoreLine(t *testing.T) {
	line := "Score of Candidate vs Baseline: 12 - 8 - 20  [0.550] 40"
	wins, losses, draws, games, score, ok := parseScoreLine(line)
	if !ok {
		t.Fatal("score line was not parsed")
	}
	if wins != 12 || losses != 8 || draws != 20 || games != 40 || math.Abs(score-55) > 0.0001 {
		t.Fatalf("unexpected result: %d %d %d %d %.2f", wins, losses, draws, games, score)
	}
}

func TestMedian(t *testing.T) {
	if got := median([]int64{9, 1, 7, 3, 5}); got != 5 {
		t.Fatalf("median = %d, want 5", got)
	}
}

func TestMatchDecision(t *testing.T) {
	validAudit := &pgnAudit{UniqueOpenings: 100}
	if got := matchDecision(matchStatus{Score: 46.5, PGNAudit: validAudit}, false); got != "rejected_below_47_percent" {
		t.Fatalf("unexpected rejection decision %q", got)
	}
	if got := matchDecision(matchStatus{Score: 47, PGNAudit: validAudit}, false); got != "passed_screening" {
		t.Fatalf("unexpected pass decision %q", got)
	}
	if got := matchDecision(matchStatus{SPRTLLR: 2.94, SPRTLower: -2.94, SPRTUpper: 2.94, PGNAudit: validAudit}, true); got != "accepted_h1" {
		t.Fatalf("unexpected SPRT decision %q", got)
	}
	if got := matchDecision(matchStatus{Score: 55, PGNAudit: &pgnAudit{UniqueOpenings: 99}}, false); got != "invalid_insufficient_openings" {
		t.Fatalf("unexpected opening validation decision %q", got)
	}
	unpaired := &pgnAudit{UniqueOpenings: 100, OpeningGroupsWrongSize: 1}
	if got := matchDecision(matchStatus{SPRTLLR: 2.95, SPRTLower: -2.94, SPRTUpper: 2.94, PGNAudit: unpaired}, true); got != "accepted_h1" {
		t.Fatalf("SPRT rejected a valid decision with incomplete parallel pairs: %q", got)
	}
	if got := matchDecision(matchStatus{Score: 55, PGNAudit: unpaired}, false); got != "invalid_unpaired_openings" {
		t.Fatalf("screening accepted an incomplete pair: %q", got)
	}
}

func TestParseSPRTLine(t *testing.T) {
	line := "SPRT: llr -0.03 (1.0%), lbound -2.94, ubound 2.94"
	llr, lower, upper, ok := parseSPRTLine(line)
	if !ok || llr != -0.03 || lower != -2.94 || upper != 2.94 {
		t.Fatalf("unexpected SPRT result: %.2f %.2f %.2f %v", llr, lower, upper, ok)
	}
}

func TestCountOpenings(t *testing.T) {
	dir := t.TempDir()
	epd := filepath.Join(dir, "book.epd")
	lines := make([]string, 0, 100)
	for ix := 0; ix < 100; ix++ {
		lines = append(lines, "8/8/8/8/8/8/8/8 w - - id "+strconv.Itoa(ix))
	}
	if err := os.WriteFile(epd, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	count, err := countOpenings(epd, openingFormat(epd))
	if err != nil || count != 100 {
		t.Fatalf("EPD count=%d err=%v", count, err)
	}

	pgn := filepath.Join(dir, "book.pgn")
	if err := os.WriteFile(pgn, []byte("[Event \"one\"]\n\n1. e4 e5 *\n\n[Event \"two\"]\n\n1. d4 d5 *\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	count, err = countOpenings(pgn, openingFormat(pgn))
	if err != nil || count != 2 {
		t.Fatalf("PGN count=%d err=%v", count, err)
	}
}

func TestAuditPGN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "games.pgn")
	pgn := `[Event "match"]
[Round "1"]
[FEN "fen-a"]

1. e2e4 {+0.1} e7e5 2. g1f3 1-0

[Event "match"]
[Round "1"]
[FEN "fen-a"]

1. e2e4 e7e5 2. g1f3 0-1

[Event "match"]
[Round "2"]
[FEN "fen-b"]

1... c7c5 2. g1f3 1/2-1/2
`
	if err := os.WriteFile(path, []byte(pgn), 0o644); err != nil {
		t.Fatal(err)
	}
	audit, err := auditPGN(path)
	if err != nil {
		t.Fatal(err)
	}
	if audit.Games != 3 || audit.UniqueOpenings != 2 || audit.UniqueStartPositions != 2 || audit.UniqueGameSequences != 2 {
		t.Fatalf("unexpected audit: %+v", audit)
	}
	if audit.OpeningGroupsWrongSize != 1 || audit.GamesInDuplicateGroups != 2 || audit.IdenticalColorSwapPairs != 1 {
		t.Fatalf("duplicates not detected: %+v", audit)
	}
}

func TestAuditPGNBookMoves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "games.pgn")
	pgn := `[Event "match"]
[Round "1"]

1. e2e4 {book} e7e5 {book} 2. g1f3 1-0

[Event "match"]
[Round "1"]

1. e2e4 {book} e7e5 {book} 2. b1c3 0-1

[Event "match"]
[Round "2"]

1. d2d4 {book} d7d5 {book} 2. c2c4 1-0

[Event "match"]
[Round "2"]

1. d2d4 {book} d7d5 {book} 2. g1f3 0-1
`
	if err := os.WriteFile(path, []byte(pgn), 0o644); err != nil {
		t.Fatal(err)
	}
	audit, err := auditPGN(path)
	if err != nil {
		t.Fatal(err)
	}
	if audit.UniqueOpenings != 2 || audit.OpeningGroupsWrongSize != 0 {
		t.Fatalf("opening pairs not detected: %+v", audit)
	}
	if audit.MinimumBookPlies != 2 || audit.MaximumBookPlies != 2 {
		t.Fatalf("book depth not detected: %+v", audit)
	}
}
