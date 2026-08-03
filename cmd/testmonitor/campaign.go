package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	campaignSchemaVersion = 1
	defaultCampaignState  = "artifacts/automation/active-campaign.json"
	defaultCampaignPoll   = 10 * time.Second
)

type campaignConfig struct {
	CandidateID        string `json:"candidate_id"`
	CandidateWorktree  string `json:"candidate_worktree"`
	CandidateCommit    string `json:"candidate_commit"`
	CandidateBinary    string `json:"candidate_binary"`
	Baseline           string `json:"baseline"`
	Hypothesis         string `json:"hypothesis"`
	Change             string `json:"change"`
	SemanticPreserving bool   `json:"semantic_preserving"`
	RepoRoot           string `json:"repo_root"`
	Fastchess          string `json:"fastchess"`
	Openings           string `json:"openings"`
	Codex              string `json:"codex"`
	Go                 string `json:"go"`
}

type campaignMatchSummary struct {
	State      string  `json:"state"`
	Decision   string  `json:"decision,omitempty"`
	Games      int     `json:"games"`
	Wins       int     `json:"wins"`
	Draws      int     `json:"draws"`
	Losses     int     `json:"losses"`
	Score      float64 `json:"score_percent"`
	SPRTLLR    float64 `json:"sprt_llr,omitempty"`
	SPRTLower  float64 `json:"sprt_lower,omitempty"`
	SPRTUpper  float64 `json:"sprt_upper,omitempty"`
	StatusPath string  `json:"status_path"`
}

type campaignState struct {
	SchemaVersion int                   `json:"schema_version"`
	Status        string                `json:"status"`
	StartedAt     time.Time             `json:"started_at"`
	UpdatedAt     time.Time             `json:"updated_at"`
	FinishedAt    time.Time             `json:"finished_at,omitempty"`
	Config        campaignConfig        `json:"config"`
	ExperimentDir string                `json:"experiment_dir"`
	ScreeningDir  string                `json:"screening_run_dir"`
	SPRTRunDir    string                `json:"sprt_run_dir"`
	Screening     *campaignMatchSummary `json:"screening,omitempty"`
	SPRT          *campaignMatchSummary `json:"sprt,omitempty"`
	Error         string                `json:"error,omitempty"`
}

var (
	campaignBuildCandidate = buildCampaignCandidate
	campaignRunPipeline    = runCampaignPipeline
	campaignStartScreening = startCampaignScreening
)

func campaignInitCommand(args []string) error {
	fs := flag.NewFlagSet("campaign-init", flag.ContinueOnError)
	statePath := fs.String("state", defaultCampaignState, "campaign state file")
	candidateID := fs.String("candidate-id", "", "unique candidate identifier")
	worktree := fs.String("candidate-worktree", "", "candidate Git worktree")
	baseline := fs.String("baseline", "", "promoted baseline engine")
	hypothesis := fs.String("hypothesis", "", "candidate hypothesis")
	change := fs.String("change", "", "short candidate change description")
	semantic := fs.Bool("semantic-preserving", true, "require identical fixed-depth results")
	repoRoot := fs.String("repo-root", ".", "GoAlaric repository root")
	fastchess := fs.String("fastchess", defaultFastchess, "Fastchess executable")
	openings := fs.String("openings", defaultOpenings, "opening book")
	codex := fs.String("codex", "codex", "Codex executable for the screening gate")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *candidateID == "" || *worktree == "" || *baseline == "" || strings.TrimSpace(*hypothesis) == "" || strings.TrimSpace(*change) == "" {
		return errors.New("--candidate-id, --candidate-worktree, --baseline, --hypothesis and --change are required")
	}
	if !candidateIDPattern.MatchString(*candidateID) {
		return errors.New("candidate-id contains unsafe characters")
	}
	root, err := existingAbs(*repoRoot)
	if err != nil {
		return err
	}
	worktreeAbs, err := existingAbs(*worktree)
	if err != nil {
		return err
	}
	if gitRoot := gitValue(worktreeAbs, "rev-parse", "--show-toplevel"); gitRoot == "" || filepath.Clean(gitRoot) != filepath.Clean(worktreeAbs) {
		return errors.New("candidate-worktree is not a Git worktree root")
	}
	if dirty := gitValue(worktreeAbs, "status", "--porcelain"); dirty != "" {
		return errors.New("candidate worktree has uncommitted changes")
	}
	base, err := existingAbs(*baseline)
	if err != nil {
		return err
	}
	fastchessAbs, err := existingAbs(filepath.Join(root, *fastchess))
	if filepath.IsAbs(*fastchess) {
		fastchessAbs, err = existingAbs(*fastchess)
	}
	if err != nil {
		return err
	}
	openingsAbs, err := existingAbs(filepath.Join(root, *openings))
	if filepath.IsAbs(*openings) {
		openingsAbs, err = existingAbs(*openings)
	}
	if err != nil {
		return err
	}
	codexAbs, err := resolveExecutable(*codex)
	if err != nil {
		return err
	}
	goAbs, err := resolveExecutable("go")
	if err != nil {
		return err
	}
	stateAbs := *statePath
	if !filepath.IsAbs(stateAbs) {
		stateAbs = filepath.Join(root, stateAbs)
	}
	if old, readErr := loadCampaignState(stateAbs); readErr == nil && !terminalCampaignStatus(old.Status) {
		return fmt.Errorf("campaign %s is still %s", old.Config.CandidateID, old.Status)
	}
	screeningDir := filepath.Join(root, "artifacts", "matches", *candidateID+"-screening")
	if _, statErr := os.Stat(filepath.Join(screeningDir, "status.json")); statErr == nil {
		return fmt.Errorf("screening run already exists: %s", screeningDir)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	sprtDir := filepath.Join(root, "artifacts", "matches", *candidateID+"-sprt-"+filepath.Base(screeningDir))
	now := time.Now()
	state := campaignState{
		SchemaVersion: campaignSchemaVersion,
		Status:        "queued",
		StartedAt:     now,
		UpdatedAt:     now,
		Config: campaignConfig{
			CandidateID: *candidateID, CandidateWorktree: worktreeAbs,
			CandidateCommit: gitValue(worktreeAbs, "rev-parse", "HEAD"),
			CandidateBinary: filepath.Join(worktreeAbs, "artifacts", "candidate", "goalaric-"+*candidateID),
			Baseline:        base, Hypothesis: strings.TrimSpace(*hypothesis), Change: strings.TrimSpace(*change),
			SemanticPreserving: *semantic, RepoRoot: root, Fastchess: fastchessAbs,
			Openings: openingsAbs, Codex: codexAbs, Go: goAbs,
		},
		ExperimentDir: filepath.Join(root, "artifacts", "experiments", *candidateID),
		ScreeningDir:  screeningDir,
		SPRTRunDir:    sprtDir,
	}
	if err := saveCampaignState(stateAbs, &state); err != nil {
		return err
	}
	fmt.Printf("campaign initialized: candidate=%s state=%s\n", state.Config.CandidateID, stateAbs)
	fmt.Printf("subl %s %s\n", filepath.Join(screeningDir, "monitor.log"), filepath.Join(sprtDir, "monitor.log"))
	return nil
}

func campaignTickCommand(args []string) error {
	fs := flag.NewFlagSet("campaign-tick", flag.ContinueOnError)
	statePath := fs.String("state", defaultCampaignState, "campaign state file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	state, abs, err := readCampaignStatePath(*statePath)
	if err != nil {
		return err
	}
	if terminalCampaignStatus(state.Status) {
		return nil
	}
	if err := advanceCampaign(abs, &state); err != nil {
		state.Status = "failed"
		state.Error = err.Error()
		state.FinishedAt = time.Now()
		_ = saveCampaignState(abs, &state)
		return err
	}
	return nil
}

func campaignRunCommand(args []string) error {
	fs := flag.NewFlagSet("campaign-run", flag.ContinueOnError)
	statePath := fs.String("state", defaultCampaignState, "campaign state file")
	poll := fs.Duration("poll-interval", defaultCampaignPoll, "deterministic status polling interval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *poll <= 0 {
		return errors.New("poll-interval must be positive")
	}
	for {
		state, abs, err := readCampaignStatePath(*statePath)
		if err != nil {
			return err
		}
		if terminalCampaignStatus(state.Status) {
			fmt.Printf("campaign finished: candidate=%s status=%s\n", state.Config.CandidateID, state.Status)
			return nil
		}
		if err := advanceCampaign(abs, &state); err != nil {
			state.Status = "failed"
			state.Error = err.Error()
			state.FinishedAt = time.Now()
			_ = saveCampaignState(abs, &state)
			return err
		}
		updated, _, err := readCampaignStatePath(*statePath)
		if err != nil {
			return err
		}
		if terminalCampaignStatus(updated.Status) {
			continue
		}
		time.Sleep(*poll)
	}
}

func campaignStatusCommand(args []string) error {
	fs := flag.NewFlagSet("campaign-status", flag.ContinueOnError)
	statePath := fs.String("state", defaultCampaignState, "campaign state file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	state, _, err := readCampaignStatePath(*statePath)
	if err != nil {
		return err
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	fmt.Println(string(data))
	return nil
}

func advanceCampaign(path string, state *campaignState) error {
	switch state.Status {
	case "queued", "building", "testing":
		state.Status = "building"
		state.Error = ""
		if err := saveCampaignState(path, state); err != nil {
			return err
		}
		if err := campaignBuildCandidate(*state); err != nil {
			return fmt.Errorf("build candidate: %w", err)
		}
		state.Status = "testing"
		if err := saveCampaignState(path, state); err != nil {
			return err
		}
		if err := campaignRunPipeline(*state); err != nil {
			return fmt.Errorf("pipeline: %w", err)
		}
		var report experimentReport
		if err := readJSON(filepath.Join(state.ExperimentDir, "experiment.json"), &report); err != nil {
			return err
		}
		if len(report.HardFailures) != 0 || !campaignStagesPassed(report.Stages) {
			state.Status = "tests_failed"
			state.FinishedAt = time.Now()
			return saveCampaignState(path, state)
		}
		if err := campaignStartScreening(*state); err != nil {
			return fmt.Errorf("start screening: %w", err)
		}
		state.Status = "screening_running"
		return saveCampaignState(path, state)
	case "screening_running":
		status, err := loadStatus(state.ScreeningDir)
		if err != nil {
			return err
		}
		state.Screening = campaignSummary(status)
		if !terminalMatchState(status.State) || processExists(status.PID) {
			return saveCampaignState(path, state)
		}
		if sprt, sprtErr := loadStatus(state.SPRTRunDir); sprtErr == nil {
			state.SPRT = campaignSummary(sprt)
			state.Status = "sprt_running"
			return saveCampaignState(path, state)
		} else if !errors.Is(sprtErr, os.ErrNotExist) {
			return sprtErr
		}
		state.Status = "awaiting_decision"
		state.FinishedAt = time.Now()
		return saveCampaignState(path, state)
	case "sprt_running":
		status, err := loadStatus(state.SPRTRunDir)
		if err != nil {
			return err
		}
		state.SPRT = campaignSummary(status)
		if !terminalMatchState(status.State) || processExists(status.PID) {
			return saveCampaignState(path, state)
		}
		state.Status = "awaiting_decision"
		state.FinishedAt = time.Now()
		return saveCampaignState(path, state)
	default:
		return fmt.Errorf("unknown campaign status %q", state.Status)
	}
}

func buildCampaignCandidate(state campaignState) error {
	if gitValue(state.Config.CandidateWorktree, "rev-parse", "HEAD") != state.Config.CandidateCommit {
		return errors.New("candidate worktree commit changed after campaign initialization")
	}
	if dirty := gitValue(state.Config.CandidateWorktree, "status", "--porcelain"); dirty != "" {
		return errors.New("candidate worktree became dirty")
	}
	if err := os.MkdirAll(state.ExperimentDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(state.Config.CandidateBinary), 0o755); err != nil {
		return err
	}
	return runLoggedCommand(state.Config.CandidateWorktree, filepath.Join(state.ExperimentDir, "campaign-build.log"), 10*time.Minute, state.Config.Go, "build", "-o", state.Config.CandidateBinary, "./GoAlaric.go")
}

func runCampaignPipeline(state campaignState) error {
	return pipelineCommand([]string{
		"--baseline", state.Config.Baseline,
		"--candidate", state.Config.CandidateBinary,
		"--candidate-id", state.Config.CandidateID,
		"--repo-root", state.Config.RepoRoot,
		"--hypothesis", state.Config.Hypothesis,
		"--change", state.Config.Change,
		"--semantic-preserving=" + strconv.FormatBool(state.Config.SemanticPreserving),
		"--fastchess", state.Config.Fastchess,
		"--openings", state.Config.Openings,
	})
}

func startCampaignScreening(state campaignState) error {
	return startCommand([]string{
		"--fastchess", state.Config.Fastchess,
		"--baseline", state.Config.Baseline,
		"--candidate", state.Config.CandidateBinary,
		"--candidate-id", state.Config.CandidateID,
		"--auto-evaluate",
		"--codex", state.Config.Codex,
		"--repo-root", state.Config.RepoRoot,
		"--openings", state.Config.Openings,
		"--games", "400",
		"--tc", defaultScreeningTC,
		"--run-dir", state.ScreeningDir,
	})
}

func campaignStagesPassed(stages []experimentStage) bool {
	required := map[string]bool{"go_test": false, "perft": false, "uci": false, "benchmark": false, "movetime": false}
	for _, stage := range stages {
		if _, ok := required[stage.Name]; ok && stage.Status == "passed" {
			required[stage.Name] = true
		}
	}
	for _, passed := range required {
		if !passed {
			return false
		}
	}
	return true
}

func campaignSummary(status matchStatus) *campaignMatchSummary {
	return &campaignMatchSummary{
		State: status.State, Decision: status.Decision, Games: status.Games,
		Wins: status.Wins, Draws: status.Draws, Losses: status.Losses, Score: status.Score,
		SPRTLLR: status.SPRTLLR, SPRTLower: status.SPRTLower, SPRTUpper: status.SPRTUpper,
		StatusPath: filepath.Join(status.RunDir, "status.json"),
	}
}

func terminalCampaignStatus(status string) bool {
	return status == "awaiting_decision" || status == "tests_failed" || status == "failed" || status == "cancelled"
}

func readCampaignStatePath(path string) (campaignState, string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return campaignState{}, "", err
	}
	state, err := loadCampaignState(abs)
	return state, abs, err
}

func loadCampaignState(path string) (campaignState, error) {
	var state campaignState
	err := readJSON(path, &state)
	return state, err
}

func saveCampaignState(path string, state *campaignState) error {
	state.UpdatedAt = time.Now()
	return writeJSON(path, state)
}
