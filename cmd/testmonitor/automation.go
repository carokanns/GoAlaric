package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	automationSchemaVersion = 3
	codexEvaluationTimeout  = 5 * time.Minute
)

var automaticSPRTStarter = startAutomaticSPRT

type completionEvent struct {
	SchemaVersion int               `json:"schema_version"`
	EventID       string            `json:"event_id"`
	CandidateID   string            `json:"candidate_id"`
	MatchType     string            `json:"match_type"`
	CreatedAt     time.Time         `json:"created_at"`
	State         string            `json:"state"`
	Decision      string            `json:"match_decision,omitempty"`
	Games         int               `json:"games"`
	Wins          int               `json:"wins"`
	Draws         int               `json:"draws"`
	Losses        int               `json:"losses"`
	Score         float64           `json:"score_percent"`
	SPRTLLR       float64           `json:"sprt_llr,omitempty"`
	SPRTLower     float64           `json:"sprt_lower,omitempty"`
	SPRTUpper     float64           `json:"sprt_upper,omitempty"`
	Error         string            `json:"error,omitempty"`
	Experiment    decisionInput     `json:"experiment"`
	Artifacts     map[string]string `json:"artifacts"`
}

type automationRecord struct {
	SchemaVersion int       `json:"schema_version"`
	EventID       string    `json:"event_id"`
	Status        string    `json:"status"`
	Attempts      int       `json:"llm_attempts"`
	DecisionPath  string    `json:"decision_path,omitempty"`
	SPRTRunDir    string    `json:"sprt_run_dir,omitempty"`
	Error         string    `json:"error,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

const decisionJSONSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["candidate_id", "recommendation", "reason"],
  "properties": {
    "candidate_id": {"type": "string"},
    "recommendation": {"type": "string", "enum": ["sprt", "no_sprt"]},
    "reason": {"type": "string", "minLength": 1}
  }
}`

func resolveExecutable(name string) (string, error) {
	if strings.ContainsRune(name, filepath.Separator) {
		return existingAbs(name)
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("find executable %q: %w", name, err)
	}
	return filepath.Abs(path)
}

func retryEvaluationCommand(args []string) error {
	fs := flag.NewFlagSet("retry-evaluation", flag.ContinueOnError)
	runDir := fs.String("run-dir", "", "match run directory; defaults to latest")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir, err := resolveRunDir(*runDir)
	if err != nil {
		return err
	}
	var cfg matchConfig
	if err := readJSON(filepath.Join(dir, "monitor-config.json"), &cfg); err != nil {
		return err
	}
	if !cfg.AutoEvaluate {
		return errors.New("match was not started with --auto-evaluate")
	}
	status, err := loadStatus(dir)
	if err != nil {
		return err
	}
	if !terminalMatchState(status.State) {
		return fmt.Errorf("match is not finished: state=%s", status.State)
	}
	return processMatchCompletion(cfg, status, true)
}

func processMatchCompletion(cfg matchConfig, status matchStatus, retry bool) error {
	if !terminalMatchState(status.State) {
		return fmt.Errorf("cannot evaluate non-terminal match state %q", status.State)
	}
	report, experimentDir, err := loadAutomationExperiment(cfg)
	if err != nil {
		return err
	}
	event := newCompletionEvent(cfg, status, report, experimentDir)
	inbox := filepath.Join(cfg.RepoRoot, "artifacts", "llm-inbox")
	if err := os.MkdirAll(inbox, 0o755); err != nil {
		return err
	}
	eventPath := filepath.Join(inbox, event.EventID+".json")
	if err := writeJSON(eventPath, event); err != nil {
		return err
	}
	recordPath := filepath.Join(inbox, event.EventID+".automation.json")
	record := automationRecord{SchemaVersion: automationSchemaVersion, EventID: event.EventID, Status: "pending"}
	if err := readJSON(recordPath, &record); err == nil {
		if record.Status == "completed" {
			return nil
		}
		if record.Status == "failed" && !retry {
			return fmt.Errorf("automation event %s failed; use retry-evaluation", event.EventID)
		}
		if record.Status != "failed" && !retry {
			return fmt.Errorf("automation event %s is already %s", event.EventID, record.Status)
		}
	}
	record.Status = "running"
	record.Error = ""
	record.UpdatedAt = time.Now()
	if err := writeJSON(recordPath, record); err != nil {
		return err
	}
	if event.MatchType != "screening" || event.State != "completed" || event.Decision != "passed_screening" {
		return completeWithoutModel(recordPath, record, experimentDir, report, status)
	}
	if err := validateAutomaticSPRT(report, cfg, status); err != nil {
		return completeWithoutModel(recordPath, record, experimentDir, report, status)
	}

	decisionPath := filepath.Join(experimentDir, event.MatchType+"-decision-"+event.EventID+".json")
	var d decision
	validSavedDecision := false
	if record.DecisionPath != "" {
		if readErr := readStrictDecision(record.DecisionPath, &d); readErr == nil && validateAutomationDecision(event, d) == nil {
			validSavedDecision = true
		}
	}
	if !validSavedDecision {
		record.Attempts++
		if err := invokeCodex(cfg, event, experimentDir, decisionPath, record.Attempts); err != nil {
			return failAutomation(recordPath, record, err)
		}
		if err := readStrictDecision(decisionPath, &d); err != nil {
			return failAutomation(recordPath, record, err)
		}
		if err := validateAutomationDecision(event, d); err != nil {
			return failAutomation(recordPath, record, err)
		}
		record.DecisionPath = decisionPath
		if err := writeJSON(recordPath, record); err != nil {
			return err
		}
	}

	report.Decision = &d
	report.Match = &status
	if d.Recommendation == "sprt" {
		if err := validateAutomaticSPRT(report, cfg, status); err != nil {
			return failAutomation(recordPath, record, err)
		}
		if record.SPRTRunDir == "" {
			record.SPRTRunDir = candidateSPRTRunDir(cfg.RepoRoot, cfg.CandidateID)
			if err := writeJSON(recordPath, record); err != nil {
				return err
			}
		}
		if _, err := os.Stat(filepath.Join(record.SPRTRunDir, "status.json")); errors.Is(err, os.ErrNotExist) {
			if err := automaticSPRTStarter(cfg, record.SPRTRunDir); err != nil {
				return failAutomation(recordPath, record, err)
			}
		} else if err != nil {
			return failAutomation(recordPath, record, err)
		}
		report.Status = "sprt_running"
	} else {
		report.Status = "awaiting_decision"
	}
	if err := writeJSON(filepath.Join(experimentDir, "experiment.json"), report); err != nil {
		return failAutomation(recordPath, record, err)
	}
	record.Status = "completed"
	record.Error = ""
	record.UpdatedAt = time.Now()
	return writeJSON(recordPath, record)
}

func candidateSPRTRunDir(repoRoot, candidateID string) string {
	return filepath.Join(repoRoot, "artifacts", "matches", candidateID+"-sprt")
}

func loadAutomationExperiment(cfg matchConfig) (experimentReport, string, error) {
	dir := filepath.Join(cfg.RepoRoot, "artifacts", "experiments", cfg.CandidateID)
	var report experimentReport
	if err := readJSON(filepath.Join(dir, "experiment.json"), &report); err != nil {
		return report, dir, err
	}
	if report.CandidateID != cfg.CandidateID {
		return report, dir, errors.New("experiment candidate_id does not match match configuration")
	}
	return report, dir, nil
}

func newCompletionEvent(cfg matchConfig, status matchStatus, report experimentReport, experimentDir string) completionEvent {
	matchType := "screening"
	if cfg.SPRT {
		matchType = "sprt"
	}
	created := status.FinishedAt
	if created.IsZero() {
		created = status.UpdatedAt
	}
	eventID := digest([]byte(strings.Join([]string{cfg.CandidateID, status.RunID, matchType}, "\n")))[:20]
	return completionEvent{
		SchemaVersion: automationSchemaVersion, EventID: eventID, CandidateID: cfg.CandidateID,
		MatchType: matchType, CreatedAt: created, State: status.State, Decision: status.Decision,
		Games: status.Games, Wins: status.Wins, Draws: status.Draws, Losses: status.Losses, Score: status.Score,
		SPRTLLR: status.SPRTLLR, SPRTLower: status.SPRTLower, SPRTUpper: status.SPRTUpper, Error: status.Error,
		Experiment: compactDecisionInput(report, experimentDir),
		Artifacts: map[string]string{
			"match_status": filepath.Join(status.RunDir, "status.json"),
			"experiment":   filepath.Join(experimentDir, "experiment.json"),
			"details":      experimentDir,
		},
	}
}

func invokeCodex(cfg matchConfig, event completionEvent, experimentDir, decisionPath string, attempt int) error {
	workDir := filepath.Join(experimentDir, "llm-work")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return err
	}
	schemaPath := filepath.Join(workDir, "decision-schema.json")
	if err := os.WriteFile(schemaPath, []byte(decisionJSONSchema+"\n"), 0o644); err != nil {
		return err
	}
	tmpDecision := decisionPath + ".tmp"
	logPath := filepath.Join(workDir, fmt.Sprintf("codex-attempt-%d.log", attempt))
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer log.Close()
	prompt := evaluationPrompt(event)
	args := []string{"-a", "never", "exec", "--ephemeral", "--skip-git-repo-check", "-C", workDir, "-s", "read-only", "--output-schema", schemaPath, "--output-last-message", tmpDecision, "-"}
	cmd := exec.Command(cfg.Codex, args...)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("codex evaluation: %w", err)
		}
	case <-time.After(codexEvaluationTimeout):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		return fmt.Errorf("codex evaluation timed out after %s", codexEvaluationTimeout)
	}
	var d decision
	if err := readStrictDecision(tmpDecision, &d); err != nil {
		return err
	}
	if err := os.Rename(tmpDecision, decisionPath); err != nil {
		return err
	}
	return nil
}

func evaluationPrompt(event completionEvent) string {
	payload, _ := json.Marshal(event)
	return "Decide only whether this completed GoAlaric screening should start SPRT. Use only the supplied compact JSON; do not inspect files, run commands, propose changes, or recommend promotion. " +
		"semantic_ok is a required invariant only when semantic_preserving is true. When semantic_preserving is false, changed fixed-depth nodes, scores, or bestmoves are expected and must not be treated as correctness failures; NPS remains diagnostic only. " +
		"Use hard_failures and stage statuses for correctness. Return recommendation sprt to start it or no_sprt to leave the result for manual evaluation. Return only schema-valid JSON.\n" + string(payload) + "\n"
}

func validateAutomationDecision(event completionEvent, d decision) error {
	if err := validateDecision(event.CandidateID, d); err != nil {
		return err
	}
	allowed := map[string]bool{"no_sprt": true}
	if event.MatchType == "screening" && event.State == "completed" && event.Decision == "passed_screening" {
		allowed["sprt"] = true
	}
	if !allowed[d.Recommendation] {
		return fmt.Errorf("recommendation %q is not allowed for %s state=%s decision=%s", d.Recommendation, event.MatchType, event.State, event.Decision)
	}
	return nil
}

func validateAutomaticSPRT(report experimentReport, cfg matchConfig, status matchStatus) error {
	if status.State != "completed" || status.Decision != "passed_screening" {
		return errors.New("automatic SPRT requires a completed, passed screening")
	}
	if len(report.HardFailures) != 0 {
		return errors.New("automatic SPRT blocked by hard failures")
	}
	required := map[string]bool{"go_test": false, "perft": false, "uci": false, "benchmark": false, "movetime": false}
	for _, stage := range report.Stages {
		if _, ok := required[stage.Name]; ok && stage.Status == "passed" {
			required[stage.Name] = true
		}
	}
	for name, passed := range required {
		if !passed {
			return fmt.Errorf("automatic SPRT requires passed stage %s", name)
		}
	}
	baseline, err := identifyExperimentBinary(cfg.Baseline)
	if err != nil {
		return err
	}
	candidate, err := identifyExperimentBinary(cfg.Candidate)
	if err != nil {
		return err
	}
	if baseline.SHA256 != report.Baseline.SHA256 || candidate.SHA256 != report.Candidate.SHA256 {
		return errors.New("match binaries do not match the tested experiment identities")
	}
	return nil
}

func startAutomaticSPRT(cfg matchConfig, runDir string) error {
	tc := automaticSPRTTimeControl(cfg)
	concurrency := cfg.Concurrency
	if concurrency < 1 {
		concurrency = 8
	}
	hashMB := cfg.HashMB
	if hashMB < 16 {
		hashMB = 128
	}
	threads := cfg.Threads
	if threads < 1 {
		threads = 1
	}
	args := []string{
		"--fastchess", cfg.Fastchess,
		"--baseline", cfg.Baseline,
		"--candidate", cfg.Candidate,
		"--candidate-id", cfg.CandidateID,
		"--auto-evaluate",
		"--codex", cfg.Codex,
		"--repo-root", cfg.RepoRoot,
		"--openings", cfg.Openings,
		"--games", "10000",
		"--tc", tc,
		"--concurrency", strconv.Itoa(concurrency),
		"--hash", strconv.Itoa(hashMB),
		"--threads", strconv.Itoa(threads),
		"--run-dir", runDir,
		"--sprt",
	}
	if cfg.SyzygyPath != "" {
		args = append(args, "--syzygy-path", cfg.SyzygyPath)
	}
	return startCommand(args)
}

func automaticSPRTTimeControl(cfg matchConfig) string {
	if cfg.SPRTTC != "" {
		return cfg.SPRTTC
	}
	return defaultSPRTTC
}

func completeWithoutModel(recordPath string, record automationRecord, experimentDir string, report experimentReport, status matchStatus) error {
	report.Match = &status
	report.Decision = nil
	report.Status = "awaiting_decision"
	if err := writeJSON(filepath.Join(experimentDir, "experiment.json"), report); err != nil {
		return failAutomation(recordPath, record, err)
	}
	record.Status = "completed"
	record.Error = ""
	record.UpdatedAt = time.Now()
	return writeJSON(recordPath, record)
}

func failAutomation(path string, record automationRecord, err error) error {
	record.Status = "failed"
	record.Error = err.Error()
	record.UpdatedAt = time.Now()
	if writeErr := writeJSON(path, record); writeErr != nil {
		return errors.Join(err, writeErr)
	}
	return err
}

func terminalMatchState(state string) bool {
	return state == "completed" || state == "failed" || state == "stopped"
}

func readStrictDecision(path string, d *decision) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(d); err != nil {
		return fmt.Errorf("decode decision: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("decision contains trailing JSON data")
	}
	return nil
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}
