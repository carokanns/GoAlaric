package main

// The experiment pipeline deliberately has no LLM dependency. It produces
// deterministic, compact artifacts which an LLM can inspect later.

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const experimentSchemaVersion = 1

type experimentStage struct {
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMS int64     `json:"duration_ms"`
	Error      string    `json:"error,omitempty"`
	Log        string    `json:"log,omitempty"`
}

type perftCaseResult struct {
	Position    int    `json:"position"`
	FEN         string `json:"fen"`
	Expected    int64  `json:"expected"`
	Actual      int64  `json:"actual"`
	HasExpected bool   `json:"has_expected"`
	Status      string `json:"status"`
}

type perftReport struct {
	Depth  int               `json:"depth"`
	Cases  []perftCaseResult `json:"cases"`
	Status string            `json:"status"`
}

type uciReport struct {
	Status       string `json:"status"`
	BestMove     string `json:"bestmove,omitempty"`
	StopVerified bool   `json:"stop_verified"`
}

type benchmarkDelta struct {
	FEN             string  `json:"fen"`
	BaselineNodes   int64   `json:"baseline_nodes"`
	CandidateNodes  int64   `json:"candidate_nodes"`
	BaselineNPS     int64   `json:"baseline_nps"`
	CandidateNPS    int64   `json:"candidate_nps"`
	NPSDeltaPercent float64 `json:"nps_delta_percent"`
	SameNodes       bool    `json:"same_nodes"`
	SameScore       bool    `json:"same_score"`
	SameBestMove    bool    `json:"same_bestmove"`
	BaselineMove    string  `json:"baseline_bestmove"`
	CandidateMove   string  `json:"candidate_bestmove"`
	BaselineScore   string  `json:"baseline_score"`
	CandidateScore  string  `json:"candidate_score"`
}

type benchmarkComparison struct {
	Depth              int              `json:"depth"`
	Repetitions        int              `json:"repetitions"`
	BaselineMedianNPS  int64            `json:"baseline_median_nps"`
	CandidateMedianNPS int64            `json:"candidate_median_nps"`
	NPSDeltaPercent    float64          `json:"nps_delta_percent"`
	SemanticOK         bool             `json:"semantic_ok"`
	Cases              []benchmarkDelta `json:"cases"`
}

type movetimeReport struct {
	Milliseconds int      `json:"milliseconds"`
	Cases        int      `json:"cases"`
	Errors       int      `json:"errors"`
	ScoreDeltas  []string `json:"score_deltas,omitempty"`
	Status       string   `json:"status"`
}

type experimentIdentity struct {
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	GitRoot    string `json:"git_root,omitempty"`
	GitCommit  string `json:"git_commit,omitempty"`
	GitBranch  string `json:"git_branch,omitempty"`
	DiffSHA256 string `json:"diff_sha256,omitempty"`
}

type experimentConfig struct {
	PerftDepth       int    `json:"perft_depth"`
	PerftEPD         string `json:"perft_epd"`
	BenchDepth       int    `json:"bench_depth"`
	BenchRepetitions int    `json:"bench_repetitions"`
	MovetimeMS       int    `json:"movetime_ms"`
	MovetimeEPD      string `json:"movetime_epd"`
	SemanticPreserve bool   `json:"semantic_preserving"`
	Screening        bool   `json:"screening"`
	ScreeningGames   int    `json:"screening_games,omitempty"`
	ScreeningTC      string `json:"screening_tc,omitempty"`
	Openings         string `json:"openings,omitempty"`
	Fastchess        string `json:"fastchess,omitempty"`
	Hypothesis       string `json:"hypothesis,omitempty"`
	ProposedChange   string `json:"proposed_change,omitempty"`
}

type experimentReport struct {
	SchemaVersion int                  `json:"schema_version"`
	CandidateID   string               `json:"candidate_id"`
	Status        string               `json:"status"`
	StartedAt     time.Time            `json:"started_at"`
	FinishedAt    time.Time            `json:"finished_at,omitempty"`
	CacheKey      string               `json:"cache_key"`
	Baseline      experimentIdentity   `json:"baseline"`
	Candidate     experimentIdentity   `json:"candidate"`
	Config        experimentConfig     `json:"config"`
	Stages        []experimentStage    `json:"stages"`
	Perft         *perftReport         `json:"perft,omitempty"`
	UCI           *uciReport           `json:"uci,omitempty"`
	Benchmark     *benchmarkComparison `json:"benchmark,omitempty"`
	Movetime      *movetimeReport      `json:"movetime,omitempty"`
	Match         *matchStatus         `json:"match,omitempty"`
	HardFailures  []string             `json:"hard_failures,omitempty"`
	Decision      *decision            `json:"decision,omitempty"`
}

type compactCase struct {
	FEN             string  `json:"fen"`
	NPSDeltaPercent float64 `json:"nps_delta_percent"`
	SameNodes       bool    `json:"same_nodes"`
	SameScore       bool    `json:"same_score"`
	SameBestMove    bool    `json:"same_bestmove"`
}

type decisionInput struct {
	SchemaVersion      int                `json:"schema_version"`
	CandidateID        string             `json:"candidate_id"`
	Status             string             `json:"status"`
	Hypothesis         string             `json:"hypothesis,omitempty"`
	ProposedChange     string             `json:"proposed_change,omitempty"`
	HardFailures       []string           `json:"hard_failures,omitempty"`
	Baseline           experimentIdentity `json:"baseline"`
	Candidate          experimentIdentity `json:"candidate"`
	Stages             map[string]string  `json:"stages"`
	NPSDelta           float64            `json:"nps_delta_percent,omitempty"`
	SemanticPreserving bool               `json:"semantic_preserving"`
	SemanticOK         *bool              `json:"semantic_ok,omitempty"`
	Movetime           string             `json:"movetime_status,omitempty"`
	Match              any                `json:"match,omitempty"`
	Cases              []compactCase      `json:"changed_cases,omitempty"`
	Previous           []previousDecision `json:"previous_candidates,omitempty"`
	Artifacts          map[string]string  `json:"artifacts"`
}

type previousDecision struct {
	CandidateID    string `json:"candidate_id"`
	Status         string `json:"status"`
	Recommendation string `json:"recommendation,omitempty"`
}

type decision struct {
	CandidateID    string   `json:"candidate_id"`
	Recommendation string   `json:"recommendation"`
	NextChange     string   `json:"next_change,omitempty"`
	Hypothesis     string   `json:"hypothesis,omitempty"`
	RequiredTests  []string `json:"required_tests,omitempty"`
	Reason         string   `json:"reason"`
}

var candidateIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func pipelineCommand(args []string) error {
	fs := flag.NewFlagSet("pipeline", flag.ContinueOnError)
	var baseline, candidate, candidateID, output, repoRoot string
	var hypothesis, proposedChange string
	var perftDepth, benchDepth, repetitions, movetimeMS int
	var semanticPreserve, screening bool
	var fastchess, openings, screeningTC string
	var screeningGames int
	fs.StringVar(&baseline, "baseline", "", "baseline engine executable")
	fs.StringVar(&candidate, "candidate", "", "candidate engine executable")
	fs.StringVar(&candidateID, "candidate-id", "", "isolated candidate identifier")
	fs.StringVar(&output, "output", "", "experiment directory")
	fs.StringVar(&repoRoot, "repo-root", ".", "repository root")
	fs.StringVar(&hypothesis, "hypothesis", "", "candidate hypothesis")
	fs.StringVar(&proposedChange, "change", "", "short candidate change description")
	fs.IntVar(&perftDepth, "perft-depth", 5, "perft depth")
	fs.IntVar(&benchDepth, "bench-depth", 8, "fixed search depth")
	fs.IntVar(&repetitions, "repetitions", 7, "benchmark repetitions")
	fs.IntVar(&movetimeMS, "movetime-ms", 2000, "movetime test duration")
	fs.BoolVar(&semanticPreserve, "semantic-preserving", true, "require identical fixed-depth results")
	fs.BoolVar(&screening, "screening", false, "run the optional Fastchess screening match")
	fs.StringVar(&fastchess, "fastchess", defaultFastchess, "Fastchess executable")
	fs.StringVar(&openings, "openings", defaultOpenings, "opening book for screening")
	fs.IntVar(&screeningGames, "screening-games", 400, "screening game count")
	fs.StringVar(&screeningTC, "screening-tc", defaultScreeningTC, "screening time control")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if baseline == "" || candidate == "" || candidateID == "" {
		return errors.New("--baseline, --candidate and --candidate-id are required")
	}
	if !candidateIDPattern.MatchString(candidateID) {
		return errors.New("candidate-id contains unsafe characters")
	}
	base, err := existingAbs(baseline)
	if err != nil {
		return err
	}
	cand, err := existingAbs(candidate)
	if err != nil {
		return err
	}
	root, err := existingAbs(repoRoot)
	if err != nil {
		return err
	}
	if output == "" {
		output = filepath.Join(root, "artifacts", "experiments", candidateID)
	}
	cfg := experimentConfig{
		PerftDepth: perftDepth, PerftEPD: filepath.Join(root, "scripts", "perft_tests.txt"),
		BenchDepth: benchDepth, BenchRepetitions: repetitions,
		MovetimeMS: movetimeMS, MovetimeEPD: filepath.Join(root, "scripts", "movetime_epd"),
		SemanticPreserve: semanticPreserve, Screening: screening, ScreeningGames: screeningGames,
		ScreeningTC: screeningTC, Openings: openings, Fastchess: fastchess,
		Hypothesis: hypothesis, ProposedChange: proposedChange,
	}
	report, cached, err := runExperiment(root, output, candidateID, base, cand, cfg)
	if err != nil {
		return err
	}
	if cached {
		fmt.Printf("experiment cache hit: %s\n", output)
	} else {
		fmt.Printf("experiment saved: %s\n", output)
	}
	fmt.Printf("status=%s hard_failures=%d\n", report.Status, len(report.HardFailures))
	return nil
}

func snapshotCommand(args []string) error {
	fs := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	candidateID := fs.String("candidate-id", "", "candidate identifier")
	output := fs.String("output", "", "experiment directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *candidateID == "" {
		return errors.New("--candidate-id is required")
	}
	if !candidateIDPattern.MatchString(*candidateID) {
		return errors.New("candidate-id contains unsafe characters")
	}
	dir := *output
	if dir == "" {
		dir = filepath.Join("artifacts", "experiments", *candidateID)
	}
	var report experimentReport
	data, err := os.ReadFile(filepath.Join(dir, "experiment.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return err
	}
	input := compactDecisionInput(report, dir)
	path := filepath.Join(dir, "decision_input.json")
	if err := writeJSON(path, input); err != nil {
		return err
	}
	data, _ = json.MarshalIndent(input, "", "  ")
	fmt.Println(string(data))
	return nil
}

func recordDecisionCommand(args []string) error {
	fs := flag.NewFlagSet("record-decision", flag.ContinueOnError)
	candidateID := fs.String("candidate-id", "", "candidate identifier")
	decisionPath := fs.String("decision", "", "LLM decision JSON")
	output := fs.String("output", "", "experiment directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *candidateID == "" || *decisionPath == "" {
		return errors.New("--candidate-id and --decision are required")
	}
	if !candidateIDPattern.MatchString(*candidateID) {
		return errors.New("candidate-id contains unsafe characters")
	}
	dir := *output
	if dir == "" {
		dir = filepath.Join("artifacts", "experiments", *candidateID)
	}
	data, err := os.ReadFile(*decisionPath)
	if err != nil {
		return err
	}
	var d decision
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}
	if err := validateDecision(*candidateID, d); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "decision.json"), d); err != nil {
		return err
	}
	var report experimentReport
	data, err = os.ReadFile(filepath.Join(dir, "experiment.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return err
	}
	report.Decision = &d
	report.Status = decisionStatus(d.Recommendation)
	if err := writeJSON(filepath.Join(dir, "experiment.json"), report); err != nil {
		return err
	}
	if err := appendExperimentLog(filepath.Join("optimeringar", "experiment-log.md"), report, d); err != nil {
		return err
	}
	fmt.Printf("decision recorded: candidate=%s status=%s\n", d.CandidateID, report.Status)
	return nil
}

func validateDecision(candidateID string, d decision) error {
	allowed := map[string]bool{"reject": true, "continue": true, "propose_change": true, "screening": true, "sprt": true, "no_sprt": true, "promote": true}
	if d.CandidateID != candidateID {
		return errors.New("decision candidate_id does not match")
	}
	if !allowed[d.Recommendation] {
		return fmt.Errorf("invalid recommendation %q", d.Recommendation)
	}
	if strings.TrimSpace(d.Reason) == "" {
		return errors.New("decision reason is required")
	}
	return nil
}

func decisionStatus(recommendation string) string {
	switch recommendation {
	case "reject":
		return "rejected"
	case "screening":
		return "awaiting_approval"
	case "sprt":
		return "awaiting_approval"
	case "promote":
		return "awaiting_approval"
	case "propose_change":
		return "awaiting_approval"
	default:
		return "awaiting_decision"
	}
}

func runExperiment(root, dir, candidateID, baseline, candidate string, cfg experimentConfig) (experimentReport, bool, error) {
	baseID, err := identifyExperimentBinary(baseline)
	if err != nil {
		return experimentReport{}, false, err
	}
	candID, err := identifyExperimentBinary(candidate)
	if err != nil {
		return experimentReport{}, false, err
	}
	if baseID.SHA256 == candID.SHA256 {
		return experimentReport{}, false, fmt.Errorf("baseline and candidate binaries have identical SHA-256 %s; rebuild the candidate before running the experiment pipeline", baseID.SHA256)
	}
	cacheKey := hashJSON(struct {
		B, C   experimentIdentity
		Config experimentConfig
	}{baseID, candID, cfg})
	reportPath := filepath.Join(dir, "experiment.json")
	if data, readErr := os.ReadFile(reportPath); readErr == nil {
		var old experimentReport
		if json.Unmarshal(data, &old) == nil && old.CacheKey == cacheKey {
			_ = writeJSON(filepath.Join(dir, "decision_input.json"), compactDecisionInput(old, dir))
			return old, true, nil
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return experimentReport{}, false, err
	}
	report := experimentReport{SchemaVersion: experimentSchemaVersion, CandidateID: candidateID, Status: "running", StartedAt: time.Now(), CacheKey: cacheKey, Baseline: baseID, Candidate: candID, Config: cfg}
	addStage := func(name string, fn func(string) error) error {
		stage := experimentStage{Name: name, Status: "running", StartedAt: time.Now(), Log: filepath.Join(dir, name+".log")}
		stageErr := fn(stage.Log)
		stage.FinishedAt = time.Now()
		stage.DurationMS = stage.FinishedAt.Sub(stage.StartedAt).Milliseconds()
		if stageErr != nil {
			stage.Status = "failed"
			stage.Error = stageErr.Error()
			report.HardFailures = append(report.HardFailures, name+": "+stageErr.Error())
		} else {
			stage.Status = "passed"
		}
		report.Stages = append(report.Stages, stage)
		if writeErr := writeJSON(reportPath, report); writeErr != nil {
			return writeErr
		}
		return stageErr
	}
	if err := addStage("go_test", func(log string) error { return runGoTest(root, log) }); err != nil {
		return finishExperiment(report, reportPath), false, nil
	}
	if err := addStage("perft", func(log string) error {
		var e error
		report.Perft, e = runPerftReport(candidate, cfg.PerftEPD, cfg.PerftDepth, log)
		return e
	}); err != nil {
		return finishExperiment(report, reportPath), false, nil
	}
	if err := addStage("uci", func(log string) error { var e error; report.UCI, e = runUCISmoke(candidate, log); return e }); err != nil {
		return finishExperiment(report, reportPath), false, nil
	}
	if err := addStage("benchmark", func(log string) error {
		var e error
		report.Benchmark, e = runBenchmarkComparison(baseline, candidate, cfg)
		return e
	}); err != nil {
		return finishExperiment(report, reportPath), false, nil
	}
	if err := addStage("movetime", func(log string) error {
		var e error
		report.Movetime, e = runMovetimeReport(baseline, candidate, cfg.MovetimeEPD, cfg.MovetimeMS, log)
		return e
	}); err != nil {
		return finishExperiment(report, reportPath), false, nil
	}
	if cfg.Screening {
		if err := addStage("screening", func(log string) error {
			var e error
			report.Match, e = runScreeningMatch(baseline, candidate, cfg, filepath.Join(dir, "screening"), log)
			return e
		}); err != nil {
			return finishExperiment(report, reportPath), false, nil
		}
	}
	report.Status = "awaiting_decision"
	report.FinishedAt = time.Now()
	if err := writeJSON(reportPath, report); err != nil {
		return report, false, err
	}
	input := compactDecisionInput(report, dir)
	if err := writeJSON(filepath.Join(dir, "decision_input.json"), input); err != nil {
		return report, false, err
	}
	return report, false, nil
}

func finishExperiment(report experimentReport, path string) experimentReport {
	report.Status = "rejected"
	report.FinishedAt = time.Now()
	_ = writeJSON(path, report)
	_ = writeJSON(filepath.Join(filepath.Dir(path), "decision_input.json"), compactDecisionInput(report, filepath.Dir(path)))
	return report
}

func runGoTest(root, logPath string) error {
	return runLoggedCommand(root, logPath, 20*time.Minute, "go", "test", "./...")
}

func runLoggedCommand(dir, logPath string, timeout time.Duration, name string, args ...string) error {
	log, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return fmt.Errorf("command timed out after %s", timeout)
	}
}

func identifyExperimentBinary(path string) (experimentIdentity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return experimentIdentity{}, err
	}
	id := experimentIdentity{Path: path, SHA256: digest(data)}
	root := nearestGitRoot(filepath.Dir(path))
	if root == "" {
		return id, nil
	}
	id.GitRoot = root
	id.GitCommit = gitValue(root, "rev-parse", "HEAD")
	id.GitBranch = gitValue(root, "symbolic-ref", "--short", "HEAD")
	if diff := gitOutput(root, "diff", "--binary", "HEAD", "--"); diff != "" {
		id.DiffSHA256 = digest([]byte(diff))
	}
	return id, nil
}

func nearestGitRoot(path string) string {
	for {
		if info, err := os.Stat(filepath.Join(path, ".git")); err == nil && (info.IsDir() || info.Mode().IsRegular()) {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return ""
		}
		path = parent
	}
}

func gitValue(root string, args ...string) string { return strings.TrimSpace(gitOutput(root, args...)) }
func gitOutput(root string, args ...string) string {
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
	if err != nil {
		return ""
	}
	return string(out)
}
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func hashJSON(value any) string { data, _ := json.Marshal(value); return digest(data) }

type perftInput struct {
	FEN         string
	Expected    int64
	HasExpected bool
}

func readPerftInputs(path string, depth int) ([]perftInput, error) {
	data, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer data.Close()
	var result []perftInput
	current := -1
	scanner := bufio.NewScanner(data)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 4 && strings.Contains(fields[0], "/") && (fields[1] == "w" || fields[1] == "b") {
			result = append(result, perftInput{FEN: strings.Join(fields[:4], " ")})
			current++
			continue
		}
		if current >= 0 && len(fields) >= 2 && fields[0] == strconv.Itoa(depth) {
			n, e := strconv.ParseInt(strings.ReplaceAll(fields[1], ",", ""), 10, 64)
			if e != nil {
				return nil, e
			}
			result[current].Expected = n
			result[current].HasExpected = true
		}
	}
	return result, scanner.Err()
}

func runPerftReport(engine, epd string, depth int, logPath string) (*perftReport, error) {
	inputs, err := readPerftInputs(epd, depth)
	if err != nil {
		return nil, err
	}
	report := &perftReport{Depth: depth, Status: "passed"}
	log, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}
	defer log.Close()
	for ix, input := range inputs {
		actual, output, runErr := runPerft(engine, input.FEN, depth)
		_, _ = io.WriteString(log, output)
		if runErr != nil {
			return report, fmt.Errorf("position %d: %w", ix+1, runErr)
		}
		status := "pass"
		if !input.HasExpected {
			status = "skipped"
		} else if actual != input.Expected {
			status = "fail"
			report.Status = "failed"
		}
		report.Cases = append(report.Cases, perftCaseResult{Position: ix + 1, FEN: input.FEN, Expected: input.Expected, Actual: actual, HasExpected: input.HasExpected, Status: status})
		if status == "fail" {
			return report, fmt.Errorf("position %d: expected %d, got %d", ix+1, input.Expected, actual)
		}
	}
	return report, nil
}

func runPerft(engine, fen string, depth int) (int64, string, error) {
	cmd := exec.Command(engine)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return 0, "", err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, "", err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return 0, "", err
	}
	defer func() { _, _ = io.WriteString(stdin, "quit\n"); _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	lines := make(chan string, 128)
	go scanLines(stdout, lines)
	send := func(s string) error { _, e := io.WriteString(stdin, s+"\n"); return e }
	if err := send("uci"); err != nil {
		return 0, "", err
	}
	if err := waitLine(lines, "uciok", 5*time.Second); err != nil {
		return 0, "", err
	}
	if err := send("position fen " + fen); err != nil {
		return 0, "", err
	}
	if err := send(fmt.Sprintf("perft %d", depth)); err != nil {
		return 0, "", err
	}
	var total int64
	var output strings.Builder
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for {
		select {
		case line, open := <-lines:
			if !open {
				return 0, output.String(), errors.New("engine exited during perft")
			}
			output.WriteString(line + "\n")
			if strings.HasPrefix(line, "Total:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					total, err = strconv.ParseInt(strings.ReplaceAll(fields[1], ",", ""), 10, 64)
					if err != nil {
						return 0, output.String(), err
					}
				}
			}
			if strings.HasPrefix(line, "Time:") {
				return total, output.String(), nil
			}
		case <-timer.C:
			return 0, output.String(), errors.New("perft timeout")
		}
	}
}

func scanLines(r io.Reader, lines chan<- string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lines <- scanner.Text()
	}
	close(lines)
}
func waitLine(lines <-chan string, target string, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case line, open := <-lines:
			if !open {
				return errors.New("engine exited before " + target)
			}
			if strings.TrimSpace(line) == target {
				return nil
			}
		case <-timer.C:
			return errors.New("timeout waiting for " + target)
		}
	}
}

func runUCISmoke(engine, logPath string) (*uciReport, error) {
	sample, err := runSearch(engine, "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1", 1, 15*time.Second)
	if err != nil {
		return &uciReport{Status: "failed"}, err
	}
	if sample.BestMove == "" {
		return &uciReport{Status: "failed"}, errors.New("empty bestmove")
	}
	if err := runStopSmoke(engine); err != nil {
		return &uciReport{Status: "failed", BestMove: sample.BestMove}, err
	}
	_ = os.WriteFile(logPath, []byte(fmt.Sprintf("bestmove %s\n", sample.BestMove)), 0o644)
	return &uciReport{Status: "passed", BestMove: sample.BestMove, StopVerified: true}, nil
}

func runStopSmoke(engine string) error {
	cmd := exec.Command(engine)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		_, _ = io.WriteString(stdin, "quit\n")
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	lines := make(chan string, 256)
	go scanLines(stdout, lines)
	send := func(line string) error { _, writeErr := io.WriteString(stdin, line+"\n"); return writeErr }
	if err := send("uci"); err != nil {
		return err
	}
	if err := waitLine(lines, "uciok", 5*time.Second); err != nil {
		return err
	}
	if err := send("isready"); err != nil {
		return err
	}
	if err := waitLine(lines, "readyok", 5*time.Second); err != nil {
		return err
	}
	for _, line := range []string{"ucinewgame", "position startpos", "go infinite"} {
		if err := send(line); err != nil {
			return err
		}
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case line, open := <-lines:
			if !open {
				return errors.New("engine exited before stop test")
			}
			if strings.HasPrefix(line, "info") {
				if err := send("stop"); err != nil {
					return err
				}
				return waitBestMove(lines, 5*time.Second)
			}
		case <-timer.C:
			return errors.New("stop test timeout waiting for search info")
		}
	}
}

func waitBestMove(lines <-chan string, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case line, open := <-lines:
			if !open {
				return errors.New("engine exited before stop bestmove")
			}
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == "bestmove" && fields[1] != "0000" && fields[1] != "none" {
				return nil
			}
		case <-timer.C:
			return errors.New("stop test timeout waiting for bestmove")
		}
	}
}

func runBenchmarkComparison(baseline, candidate string, cfg experimentConfig) (*benchmarkComparison, error) {
	baseFENs, err := readFENs(cfg.MovetimeEPD)
	if err != nil {
		return nil, err
	}
	result := &benchmarkComparison{Depth: cfg.BenchDepth, Repetitions: cfg.BenchRepetitions, SemanticOK: true}
	var baseNPS, candNPS []int64
	for _, fen := range baseFENs {
		var bs, cs []searchSample
		for i := 0; i < cfg.BenchRepetitions; i++ {
			b, e := runSearch(baseline, fen, cfg.BenchDepth, 2*time.Minute)
			if e != nil {
				return nil, e
			}
			c, e := runSearch(candidate, fen, cfg.BenchDepth, 2*time.Minute)
			if e != nil {
				return nil, e
			}
			bs = append(bs, b)
			cs = append(cs, c)
		}
		b := bs[len(bs)/2]
		c := cs[len(cs)/2]
		bNodes := sampleMedian(bs, func(s searchSample) int64 { return s.Nodes })
		cNodes := sampleMedian(cs, func(s searchSample) int64 { return s.Nodes })
		bN := sampleMedian(bs, func(s searchSample) int64 { return s.NPS })
		cN := sampleMedian(cs, func(s searchSample) int64 { return s.NPS })
		d := benchmarkDelta{FEN: fen, BaselineNodes: bNodes, CandidateNodes: cNodes, BaselineNPS: bN, CandidateNPS: cN, SameNodes: bNodes == cNodes, SameScore: b.Score == c.Score, SameBestMove: b.BestMove == c.BestMove, BaselineMove: b.BestMove, CandidateMove: c.BestMove, BaselineScore: b.Score, CandidateScore: c.Score}
		if bN != 0 {
			d.NPSDeltaPercent = float64(cN-bN) * 100 / float64(bN)
		}
		result.Cases = append(result.Cases, d)
		baseNPS = append(baseNPS, bN)
		candNPS = append(candNPS, cN)
		if !d.SameNodes || !d.SameScore || !d.SameBestMove {
			result.SemanticOK = false
		}
	}
	result.BaselineMedianNPS = median(baseNPS)
	result.CandidateMedianNPS = median(candNPS)
	if result.BaselineMedianNPS != 0 {
		result.NPSDeltaPercent = float64(result.CandidateMedianNPS-result.BaselineMedianNPS) * 100 / float64(result.BaselineMedianNPS)
	}
	if cfg.SemanticPreserve && !result.SemanticOK {
		return result, errors.New("fixed-depth semantics changed")
	}
	return result, nil
}

func runMovetimeReport(baseline, candidate, epd string, ms int, logPath string) (*movetimeReport, error) {
	fens, err := readFENs(epd)
	if err != nil {
		return nil, err
	}
	report := &movetimeReport{Milliseconds: ms, Cases: len(fens), Status: "passed"}
	log, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}
	defer log.Close()
	for ix, fen := range fens {
		b, e := runTimedSearch(baseline, fen, ms)
		if e != nil {
			report.Errors++
			return report, fmt.Errorf("baseline movetime position %d: %w", ix+1, e)
		}
		c, e := runTimedSearch(candidate, fen, ms)
		if e != nil {
			report.Errors++
			return report, fmt.Errorf("candidate movetime position %d: %w", ix+1, e)
		}
		_, _ = fmt.Fprintf(log, "%d %s %s -> %s %s\n", ix+1, b.BestMove, b.Score, c.BestMove, c.Score)
		if b.BestMove == "" || c.BestMove == "" {
			report.Status = "failed"
			return report, fmt.Errorf("empty bestmove at position %d", ix+1)
		}
		if b.Score != c.Score {
			report.ScoreDeltas = append(report.ScoreDeltas, fmt.Sprintf("position %d: %s -> %s", ix+1, b.Score, c.Score))
		}
	}
	return report, nil
}

func runScreeningMatch(baseline, candidate string, cfg experimentConfig, runDir, logPath string) (*matchStatus, error) {
	if cfg.Fastchess == "" || cfg.Openings == "" {
		return nil, errors.New("screening requires fastchess and openings")
	}
	args := []string{
		"--fastchess", cfg.Fastchess,
		"--baseline", baseline,
		"--candidate", candidate,
		"--openings", cfg.Openings,
		"--games", strconv.Itoa(cfg.ScreeningGames),
		"--tc", cfg.ScreeningTC,
		"--run-dir", runDir,
	}
	if err := runMatchCommand(args); err != nil {
		return nil, err
	}
	status, err := loadStatus(runDir)
	if err != nil {
		return nil, err
	}
	log, logErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if logErr == nil {
		_, _ = fmt.Fprintf(log, "decision=%s score=%.2f games=%d\n", status.Decision, status.Score, status.Games)
		_ = log.Close()
	}
	if status.Decision != "passed_screening" {
		return &status, fmt.Errorf("screening decision: %s", status.Decision)
	}
	return &status, nil
}

func runTimedSearch(engine, fen string, ms int) (searchSample, error) {
	return runSearchCommand(engine, fen, fmt.Sprintf("go movetime %d", ms), time.Duration(ms+5000)*time.Millisecond)
}

func runSearchCommand(engine, fen, searchCommand string, timeout time.Duration) (searchSample, error) {
	cmd := exec.Command(engine)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return searchSample{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return searchSample{}, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return searchSample{}, err
	}
	defer func() { _, _ = io.WriteString(stdin, "quit\n"); _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	lines := make(chan string, 256)
	go scanLines(stdout, lines)
	send := func(s string) error { _, e := io.WriteString(stdin, s+"\n"); return e }
	if err := send("uci"); err != nil {
		return searchSample{}, err
	}
	if err := waitLine(lines, "uciok", timeout); err != nil {
		return searchSample{}, err
	}
	for _, s := range []string{"setoption name Hash value 128", "setoption name Threads value 1", "setoption name Ponder value false", "isready"} {
		if err := send(s); err != nil {
			return searchSample{}, err
		}
	}
	if err := waitLine(lines, "readyok", timeout); err != nil {
		return searchSample{}, err
	}
	if err := send("ucinewgame"); err != nil {
		return searchSample{}, err
	}
	if err := send("position fen " + fen); err != nil {
		return searchSample{}, err
	}
	if err := send(searchCommand); err != nil {
		return searchSample{}, err
	}
	sample := searchSample{}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case line, open := <-lines:
			if !open {
				return sample, errors.New("engine exited before bestmove")
			}
			fields := strings.Fields(line)
			if len(fields) > 0 && fields[0] == "info" {
				parseInfo(fields, &sample)
			}
			if len(fields) >= 2 && fields[0] == "bestmove" {
				sample.BestMove = fields[1]
				if sample.BestMove == "0000" || sample.BestMove == "none" {
					return sample, fmt.Errorf("invalid bestmove %q", sample.BestMove)
				}
				return sample, nil
			}
		case <-timer.C:
			return sample, errors.New("search timeout")
		}
	}
}

func compactDecisionInput(report experimentReport, dir string) decisionInput {
	stages := make(map[string]string)
	for _, stage := range report.Stages {
		stages[stage.Name] = stage.Status
	}
	input := decisionInput{SchemaVersion: experimentSchemaVersion, CandidateID: report.CandidateID, Status: report.Status, Hypothesis: report.Config.Hypothesis, ProposedChange: report.Config.ProposedChange, HardFailures: report.HardFailures, Baseline: report.Baseline, Candidate: report.Candidate, Stages: stages, SemanticPreserving: report.Config.SemanticPreserve, Artifacts: map[string]string{"full_report": filepath.Join(dir, "experiment.json"), "logs": dir}}
	if report.Benchmark != nil {
		input.NPSDelta = report.Benchmark.NPSDeltaPercent
		input.SemanticOK = &report.Benchmark.SemanticOK
		for _, c := range report.Benchmark.Cases {
			if !c.SameNodes || !c.SameScore || !c.SameBestMove || c.NPSDeltaPercent < 0 {
				input.Cases = append(input.Cases, compactCase{FEN: c.FEN, NPSDeltaPercent: c.NPSDeltaPercent, SameNodes: c.SameNodes, SameScore: c.SameScore, SameBestMove: c.SameBestMove})
			}
		}
	}
	if report.Movetime != nil {
		input.Movetime = report.Movetime.Status
	}
	if report.Match != nil {
		input.Match = struct {
			Decision string  `json:"decision"`
			Score    float64 `json:"score_percent"`
			Games    int     `json:"games"`
		}{report.Match.Decision, report.Match.Score, report.Match.Games}
	}
	input.Previous = previousCandidates(filepath.Dir(dir), report.CandidateID)
	return input
}

func previousCandidates(experimentsDir, current string) []previousDecision {
	entries, err := os.ReadDir(experimentsDir)
	if err != nil {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() > entries[j].Name() })
	var result []previousDecision
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == current || len(result) >= 5 {
			continue
		}
		data, err := os.ReadFile(filepath.Join(experimentsDir, entry.Name(), "experiment.json"))
		if err != nil {
			continue
		}
		var r experimentReport
		if json.Unmarshal(data, &r) != nil {
			continue
		}
		p := previousDecision{CandidateID: r.CandidateID, Status: r.Status}
		if r.Decision != nil {
			p.Recommendation = r.Decision.Recommendation
		}
		result = append(result, p)
	}
	return result
}

func appendExperimentLog(path string, report experimentReport, d decision) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintf(file, "\n## %s\n\n- Status: `%s`\n- Rekommendation: `%s`\n- Nästa ändring: %s\n- Hypotes: %s\n- Orsak: %s\n", report.CandidateID, report.Status, d.Recommendation, d.NextChange, d.Hypothesis, d.Reason)
	return err
}
