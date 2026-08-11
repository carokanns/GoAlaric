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
	campaignSchemaVersion = 2
	defaultCampaignState  = "artifacts/automation/active-campaign.json"
	defaultCampaignPoll   = 10 * time.Second
	defaultPreScanTCs     = "10+0.1,20+0.2,30+0.3,45+0.45,60+0.6"
)

type campaignConfig struct {
	CandidateID        string   `json:"candidate_id"`
	CandidateWorktree  string   `json:"candidate_worktree"`
	CandidateCommit    string   `json:"candidate_commit"`
	CandidateBinary    string   `json:"candidate_binary"`
	Baseline           string   `json:"baseline"`
	Hypothesis         string   `json:"hypothesis"`
	Change             string   `json:"change"`
	ChangeClass        string   `json:"change_class"`
	ComparisonPolicy   string   `json:"comparison_policy"`
	SemanticPreserving bool     `json:"semantic_preserving"`
	RepoRoot           string   `json:"repo_root"`
	Fastchess          string   `json:"fastchess"`
	Openings           string   `json:"openings"`
	Codex              string   `json:"codex"`
	Go                 string   `json:"go"`
	PreScanMode        string   `json:"prescan_mode"`
	PreScanSkipReason  string   `json:"prescan_skip_reason,omitempty"`
	MinimumDepth       int      `json:"minimum_depth,omitempty"`
	PreScanGames       int      `json:"prescan_games,omitempty"`
	PreScanTCs         []string `json:"prescan_time_controls,omitempty"`
	Concurrency        int      `json:"concurrency"`
	HashMB             int      `json:"hash_mb"`
	Threads            int      `json:"threads"`
	PreScanSeed        int64    `json:"prescan_seed"`
	SelectedTC         string   `json:"selected_time_control,omitempty"`
}

type campaignDepthSummary struct {
	Role        string  `json:"role"`
	TimeControl string  `json:"time_control"`
	SampleCount int     `json:"sample_count"`
	MeanDepth   float64 `json:"mean_depth"`
	MedianDepth int     `json:"median_depth"`
	P25Depth    int     `json:"p25_depth"`
	P90Depth    int     `json:"p90_depth"`
	Decision    string  `json:"decision"`
	ProfilePath string  `json:"profile_path"`
}

type campaignDepthGate struct {
	Mode         string                `json:"mode"`
	MinimumDepth int                   `json:"minimum_depth,omitempty"`
	TimeControls []string              `json:"time_controls,omitempty"`
	CurrentIndex int                   `json:"current_index"`
	CurrentTC    string                `json:"current_time_control,omitempty"`
	CurrentRole  string                `json:"current_role,omitempty"`
	RunDir       string                `json:"run_dir,omitempty"`
	Decision     string                `json:"decision,omitempty"`
	Baseline     *campaignDepthSummary `json:"baseline,omitempty"`
	Candidate    *campaignDepthSummary `json:"candidate,omitempty"`
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
	DepthGate     *campaignDepthGate    `json:"depth_gate,omitempty"`
	Error         string                `json:"error,omitempty"`
}

var (
	campaignBuildCandidate = buildCampaignCandidate
	campaignRunPipeline    = runCampaignPipeline
	campaignStartScreening = startCampaignScreening
	campaignStartPreScan   = startCampaignPreScan
)

func campaignInitCommand(args []string) error {
	fs := flag.NewFlagSet("campaign-init", flag.ContinueOnError)
	statePath := fs.String("state", defaultCampaignState, "campaign state file")
	candidateID := fs.String("candidate-id", "", "unique candidate identifier")
	worktree := fs.String("candidate-worktree", "", "candidate Git worktree")
	baseline := fs.String("baseline", "", "promoted baseline engine")
	hypothesis := fs.String("hypothesis", "", "candidate hypothesis")
	change := fs.String("change", "", "short candidate change description")
	changeClass := fs.String("change-class", "", "candidate change class: implementation, eval, search, correctness or mixed")
	semantic := fs.Bool("semantic-preserving", true, "require identical fixed-depth results")
	repoRoot := fs.String("repo-root", ".", "GoAlaric repository root")
	fastchess := fs.String("fastchess", defaultFastchess, "Fastchess executable")
	openings := fs.String("openings", defaultOpenings, "opening book")
	codex := fs.String("codex", "codex", "Codex executable for the screening gate")
	preScanMode := fs.String("prescan", "", "required depth pre-scan mode: full, baseline or skip")
	preScanSkipReason := fs.String("prescan-skip-reason", "", "document why full pre-scan is unnecessary")
	minimumDepth := fs.Int("minimum-depth", 0, "minimum accepted median search depth")
	preScanGames := fs.Int("prescan-games", defaultPreScanGames, "self-play games per depth profile")
	preScanTCs := fs.String("prescan-time-controls", defaultPreScanTCs, "comma-separated time-control ladder")
	concurrency := fs.Int("concurrency", 8, "concurrent pre-scan and match games")
	hashMB := fs.Int("hash", 128, "engine hash in MB")
	threads := fs.Int("threads", 1, "threads per engine")
	preScanSeed := fs.Int64("prescan-seed", defaultPreScanSeed, "fixed opening seed for comparable depth profiles")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *candidateID == "" || *worktree == "" || *baseline == "" || strings.TrimSpace(*hypothesis) == "" || strings.TrimSpace(*change) == "" {
		return errors.New("--candidate-id, --candidate-worktree, --baseline, --hypothesis and --change are required")
	}
	if !candidateIDPattern.MatchString(*candidateID) {
		return errors.New("candidate-id contains unsafe characters")
	}
	definition, err := campaignCandidateDefinition(*changeClass, boolFlagOverride(fs, "semantic-preserving", *semantic))
	if err != nil {
		return err
	}
	if *preScanMode != "full" && *preScanMode != "baseline" && *preScanMode != "skip" {
		return errors.New("--prescan must be full, baseline or skip")
	}
	if *preScanMode == "skip" && strings.TrimSpace(*preScanSkipReason) == "" {
		return errors.New("--prescan-skip-reason is required when pre-scan is skipped")
	}
	if (*preScanMode == "full" || *preScanMode == "baseline") && *minimumDepth < 1 {
		return errors.New("--minimum-depth must be positive when pre-scan is full or baseline")
	}
	if *preScanGames < 2 || *preScanGames%2 != 0 {
		return errors.New("--prescan-games must be a positive even number")
	}
	if *concurrency < 1 || *hashMB < 16 || *threads < 1 {
		return errors.New("invalid concurrency, hash or thread configuration")
	}
	timeControls, err := parseTimeControlLadder(*preScanTCs)
	if err != nil {
		return err
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
	sprtDir := candidateSPRTRunDir(root, *candidateID)
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
			ChangeClass: definition.ChangeClass, ComparisonPolicy: definition.ComparisonPolicy,
			SemanticPreserving: policyRequiresExactEquivalence(definition.ComparisonPolicy), RepoRoot: root, Fastchess: fastchessAbs,
			Openings: openingsAbs, Codex: codexAbs, Go: goAbs,
			PreScanMode: *preScanMode, PreScanSkipReason: strings.TrimSpace(*preScanSkipReason),
			MinimumDepth: *minimumDepth, PreScanGames: *preScanGames, PreScanTCs: timeControls,
			Concurrency: *concurrency, HashMB: *hashMB, Threads: *threads,
			PreScanSeed: *preScanSeed,
		},
		ExperimentDir: filepath.Join(root, "artifacts", "experiments", *candidateID),
		ScreeningDir:  screeningDir,
		SPRTRunDir:    sprtDir,
		DepthGate: &campaignDepthGate{
			Mode: *preScanMode, MinimumDepth: *minimumDepth, TimeControls: timeControls,
		},
	}
	if err := saveCampaignState(stateAbs, &state); err != nil {
		return err
	}
	fmt.Printf("campaign initialized: candidate=%s state=%s\n", state.Config.CandidateID, stateAbs)
	logs := make([]string, 0, 2+len(timeControls)*2)
	if *preScanMode != "skip" {
		for _, tc := range timeControls {
			logs = append(logs, filepath.Join(root, "artifacts", "prescans", *candidateID+"-prescan-baseline-"+sanitizeTimeControl(tc), "monitor.log"))
			if *preScanMode == "full" {
				logs = append(logs, filepath.Join(root, "artifacts", "prescans", *candidateID+"-prescan-candidate-"+sanitizeTimeControl(tc), "monitor.log"))
			}
		}
	}
	logs = append(logs, filepath.Join(screeningDir, "monitor.log"), filepath.Join(sprtDir, "monitor.log"))
	fmt.Printf("subl %s\n", strings.Join(logs, " "))
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
		return advanceDepthGate(path, state)
	case "prescan_baseline_running", "prescan_candidate_running":
		status, err := loadStatus(state.DepthGate.RunDir)
		if err != nil {
			return err
		}
		if !terminalMatchState(status.State) || processExists(status.PID) {
			return saveCampaignState(path, state)
		}
		if status.State != "completed" || status.DepthProfile == nil {
			state.Status = "depth_measurement_invalid"
			state.Error = status.Error
			if state.Error == "" {
				state.Error = "pre-scan finished without a depth profile"
			}
			state.FinishedAt = time.Now()
			return saveCampaignState(path, state)
		}
		summary := campaignDepthProfileSummary(state.DepthGate.CurrentRole, *status.DepthProfile, filepath.Join(status.RunDir, "depth-profile.json"), state.Config.MinimumDepth)
		if state.DepthGate.CurrentRole == "baseline" {
			state.DepthGate.Baseline = summary
		} else {
			state.DepthGate.Candidate = summary
		}
		state.DepthGate.RunDir = ""
		state.DepthGate.CurrentRole = ""
		return advanceDepthGate(path, state)
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
	args := []string{
		"--baseline", state.Config.Baseline,
		"--candidate", state.Config.CandidateBinary,
		"--change-class", state.Config.ChangeClass,
		"--validation-policy", state.Config.ComparisonPolicy,
		"--candidate-id", state.Config.CandidateID,
		"--repo-root", state.Config.RepoRoot,
		"--hypothesis", state.Config.Hypothesis,
		"--change", state.Config.Change,
		"--fastchess", state.Config.Fastchess,
		"--openings", state.Config.Openings,
	}
	if state.Config.ChangeClass == "" {
		// Campaign states written before change_class retain their explicit flag.
		args = append(args, "--semantic-preserving="+strconv.FormatBool(state.Config.SemanticPreserving))
	} else {
		args = append(args, "--change-class", state.Config.ChangeClass)
	}
	return pipelineCommand(args)
}

func startCampaignScreening(state campaignState) error {
	tc := state.Config.SelectedTC
	if tc == "" {
		tc = defaultScreeningTC
	}
	return startMatchCommand([]string{
		"--fastchess", state.Config.Fastchess,
		"--baseline", state.Config.Baseline,
		"--candidate", state.Config.CandidateBinary,
		"--change-class", state.Config.ChangeClass,
		"--validation-policy", state.Config.ComparisonPolicy,
		"--candidate-id", state.Config.CandidateID,
		"--auto-evaluate",
		"--codex", state.Config.Codex,
		"--repo-root", state.Config.RepoRoot,
		"--openings", state.Config.Openings,
		"--games", "400",
		"--tc", tc,
		"--sprt-tc", defaultSPRTTC,
		"--concurrency", strconv.Itoa(state.Config.Concurrency),
		"--hash", strconv.Itoa(state.Config.HashMB),
		"--threads", strconv.Itoa(state.Config.Threads),
		"--run-dir", state.ScreeningDir,
	})
}

func advanceDepthGate(path string, state *campaignState) error {
	gate := state.DepthGate
	if gate == nil || gate.Mode == "skip" {
		if gate != nil {
			gate.Decision = "skipped"
		}
		state.Config.SelectedTC = defaultScreeningTC
		if len(state.Config.PreScanTCs) > 0 {
			state.Config.SelectedTC = state.Config.PreScanTCs[0]
		}
		return launchCampaignScreening(path, state)
	}
	for gate.CurrentIndex < len(gate.TimeControls) {
		tc := gate.TimeControls[gate.CurrentIndex]
		gate.CurrentTC = tc
		if gate.Baseline == nil {
			cfg := campaignPreScanConfig(*state, state.Config.Baseline, "baseline", tc)
			if report, profilePath, found, err := loadCachedDepthProfile(cfg); err != nil {
				return err
			} else if found {
				gate.Baseline = campaignDepthProfileSummary("baseline", report, profilePath, state.Config.MinimumDepth)
			} else {
				return launchCampaignPreScan(path, state, cfg)
			}
		}
		if gate.Baseline.MedianDepth < state.Config.MinimumDepth {
			advanceDepthTimeControl(gate)
			continue
		}
		if gate.Mode == "baseline" {
			gate.Decision = "depth_adequate"
			state.Config.SelectedTC = tc
			return launchCampaignScreening(path, state)
		}
		if gate.Candidate == nil {
			cfg := campaignPreScanConfig(*state, state.Config.CandidateBinary, "candidate", tc)
			if report, profilePath, found, err := loadCachedDepthProfile(cfg); err != nil {
				return err
			} else if found {
				gate.Candidate = campaignDepthProfileSummary("candidate", report, profilePath, state.Config.MinimumDepth)
			} else {
				return launchCampaignPreScan(path, state, cfg)
			}
		}
		if gate.Candidate.MedianDepth < state.Config.MinimumDepth {
			advanceDepthTimeControl(gate)
			continue
		}
		gate.Decision = "depth_adequate"
		state.Config.SelectedTC = tc
		return launchCampaignScreening(path, state)
	}
	gate.Decision = "depth_insufficient"
	state.Status = "depth_insufficient"
	state.FinishedAt = time.Now()
	return saveCampaignState(path, state)
}

func launchCampaignScreening(path string, state *campaignState) error {
	if err := campaignStartScreening(*state); err != nil {
		return fmt.Errorf("start screening: %w", err)
	}
	state.Status = "screening_running"
	return saveCampaignState(path, state)
}

func launchCampaignPreScan(path string, state *campaignState, cfg matchConfig) error {
	if err := campaignStartPreScan(cfg); err != nil {
		return fmt.Errorf("start %s pre-scan: %w", cfg.ProfileRole, err)
	}
	state.DepthGate.CurrentRole = cfg.ProfileRole
	state.DepthGate.RunDir = cfg.RunDir
	state.Status = "prescan_" + cfg.ProfileRole + "_running"
	return saveCampaignState(path, state)
}

func advanceDepthTimeControl(gate *campaignDepthGate) {
	gate.CurrentIndex++
	gate.CurrentTC = ""
	gate.CurrentRole = ""
	gate.RunDir = ""
	gate.Baseline = nil
	gate.Candidate = nil
}

func campaignPreScanConfig(state campaignState, engine, role, tc string) matchConfig {
	runName := state.Config.CandidateID + "-prescan-" + role + "-" + sanitizeTimeControl(tc)
	baselineEngine := engine
	if role == "candidate" {
		baselineEngine = state.Config.Baseline
	}
	return matchConfig{
		Fastchess: state.Config.Fastchess, Baseline: baselineEngine, Candidate: engine,
		Openings: state.Config.Openings, Games: state.Config.PreScanGames, TC: tc,
		Concurrency: state.Config.Concurrency, RunDir: filepath.Join(state.Config.RepoRoot, "artifacts", "prescans", runName),
		RepoRoot: state.Config.RepoRoot, DepthProfile: true, ProfileRole: role,
		MinimumDepth: state.Config.MinimumDepth, HashMB: state.Config.HashMB, Threads: state.Config.Threads,
		DepthCacheDir: filepath.Join(state.Config.RepoRoot, "artifacts", "depth-profiles", "cache"),
		ProgressEvery: 10, ProgressTime: defaultProgressInterval, Seed: state.Config.PreScanSeed,
	}
}

func startCampaignPreScan(cfg matchConfig) error {
	return startCommand([]string{
		"--fastchess", cfg.Fastchess, "--baseline", cfg.Baseline, "--candidate", cfg.Candidate,
		"--openings", cfg.Openings, "--games", strconv.Itoa(cfg.Games), "--tc", cfg.TC,
		"--concurrency", strconv.Itoa(cfg.Concurrency), "--hash", strconv.Itoa(cfg.HashMB),
		"--threads", strconv.Itoa(cfg.Threads), "--run-dir", cfg.RunDir,
		"--repo-root", cfg.RepoRoot, "--depth-profile", "--profile-role", cfg.ProfileRole,
		"--allow-identical-binaries",
		"--minimum-depth", strconv.Itoa(cfg.MinimumDepth), "--depth-cache-dir", cfg.DepthCacheDir,
		"--seed", strconv.FormatInt(cfg.Seed, 10),
	})
}

func campaignDepthProfileSummary(role string, report depthProfileReport, path string, minimumDepth int) *campaignDepthSummary {
	decision := "depth_adequate"
	if report.MedianDepth < minimumDepth {
		decision = "increase_time_control"
	}
	return &campaignDepthSummary{
		Role: role, TimeControl: report.Settings.TimeControl, SampleCount: report.SampleCount,
		MeanDepth: report.MeanDepth, MedianDepth: report.MedianDepth, P25Depth: report.P25Depth,
		P90Depth: report.P90Depth, Decision: decision, ProfilePath: path,
	}
}

func parseTimeControlLadder(input string) ([]string, error) {
	var result []string
	for _, value := range strings.Split(input, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, errors.New("pre-scan time-control ladder contains an empty value")
		}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil, errors.New("pre-scan time-control ladder is empty")
	}
	return result, nil
}

func sanitizeTimeControl(tc string) string {
	replacer := strings.NewReplacer("+", "p", ".", "_", ":", "_", "/", "_")
	return replacer.Replace(tc)
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
	return status == "awaiting_decision" || status == "tests_failed" || status == "failed" || status == "cancelled" || status == "depth_insufficient" || status == "depth_measurement_invalid"
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
