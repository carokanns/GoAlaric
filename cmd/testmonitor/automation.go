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
	"strings"
	"syscall"
	"time"
)

const (
	automationSchemaVersion = 1
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

type approvalPackage struct {
	SchemaVersion          int                `json:"schema_version"`
	CandidateID            string             `json:"candidate_id"`
	SourceEvent            string             `json:"source_event"`
	Status                 string             `json:"status"`
	BaselineRecommendation string             `json:"baseline_recommendation"`
	RecommendedBaseline    experimentIdentity `json:"recommended_baseline"`
	NextChange             string             `json:"next_change"`
	Hypothesis             string             `json:"hypothesis"`
	RequiredTests          []string           `json:"required_tests"`
	Reason                 string             `json:"reason"`
}

const decisionJSONSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["candidate_id", "recommendation", "next_change", "hypothesis", "required_tests", "reason"],
  "properties": {
    "candidate_id": {"type": "string"},
    "recommendation": {"type": "string", "enum": ["sprt", "promote", "reject", "propose_change"]},
    "next_change": {"type": "string", "minLength": 1},
    "hypothesis": {"type": "string", "minLength": 1},
    "required_tests": {"type": "array", "minItems": 1, "items": {"type": "string", "minLength": 1}},
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
	if event.MatchType == "screening" && d.Recommendation == "sprt" {
		if err := validateAutomaticSPRT(report, cfg, status); err != nil {
			return failAutomation(recordPath, record, err)
		}
		if record.SPRTRunDir == "" {
			record.SPRTRunDir = filepath.Join(cfg.RepoRoot, "artifacts", "matches", cfg.CandidateID+"-sprt-"+status.RunID)
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
		report.Status = "awaiting_approval"
		if err := writeApprovalPackage(experimentDir, event, report, d); err != nil {
			return failAutomation(recordPath, record, err)
		}
	}
	if err := writeJSON(filepath.Join(experimentDir, "experiment.json"), report); err != nil {
		return failAutomation(recordPath, record, err)
	}
	record.Status = "completed"
	record.Error = ""
	record.UpdatedAt = time.Now()
	return writeJSON(recordPath, record)
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
	payload, _ := json.Marshal(event)
	prompt := "Evaluate this completed GoAlaric match. Use only the supplied compact JSON; do not inspect files or run commands. " +
		"For screening choose sprt only when justified, otherwise reject or propose_change. For SPRT choose promote only for accepted_h1; always provide the next proposed change. Return only schema-valid JSON.\n" + string(payload) + "\n"
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

func validateAutomationDecision(event completionEvent, d decision) error {
	if err := validateDecision(event.CandidateID, d); err != nil {
		return err
	}
	if strings.TrimSpace(d.NextChange) == "" || strings.TrimSpace(d.Hypothesis) == "" || len(d.RequiredTests) == 0 {
		return errors.New("automatic decision requires next_change, hypothesis and required_tests")
	}
	allowed := map[string]bool{"reject": true, "propose_change": true}
	if event.MatchType == "screening" && event.State == "completed" && event.Decision == "passed_screening" {
		allowed["sprt"] = true
	}
	if event.MatchType == "sprt" {
		if event.State == "completed" && event.Decision == "accepted_h1" {
			allowed["promote"] = true
		}
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
		"--tc", defaultSPRTTC,
		"--run-dir", runDir,
		"--sprt",
	}
	return startCommand(args)
}

func writeApprovalPackage(dir string, event completionEvent, report experimentReport, d decision) error {
	recommended := report.Baseline
	baselineRecommendation := "keep"
	if d.Recommendation == "promote" {
		recommended = report.Candidate
		baselineRecommendation = "promote"
	}
	pkg := approvalPackage{
		SchemaVersion: automationSchemaVersion, CandidateID: event.CandidateID, SourceEvent: event.EventID,
		Status: "awaiting_approval", BaselineRecommendation: baselineRecommendation, RecommendedBaseline: recommended,
		NextChange: d.NextChange, Hypothesis: d.Hypothesis, RequiredTests: d.RequiredTests, Reason: d.Reason,
	}
	if err := writeJSON(filepath.Join(dir, "approval-package-"+event.EventID+".json"), pkg); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, "approval-package.json"), pkg)
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
