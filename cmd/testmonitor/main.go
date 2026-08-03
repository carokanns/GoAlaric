// Command testmonitor runs reproducible GoAlaric benchmarks and monitored
// Fastchess matches. Long matches can run independently of the caller while
// progress is persisted as JSON under artifacts/.
package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultFastchess         = ".tools/fastchess/bin/fastchess"
	defaultOpenings          = ".tools/books/8moves_v3.pgn"
	minimumOpenings          = 100
	defaultScreeningTC       = "20+0.2"
	defaultSPRTTC            = "30+0.3"
	defaultProgressInterval  = "1m"
	defaultScreeningProgress = 10
	defaultSPRTProgress      = 50
)

type matchStatus struct {
	RunID         string    `json:"run_id"`
	State         string    `json:"state"`
	Stage         string    `json:"stage"`
	PID           int       `json:"pid,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	FinishedAt    time.Time `json:"finished_at,omitempty"`
	Baseline      string    `json:"baseline"`
	Candidate     string    `json:"candidate"`
	TimeControl   string    `json:"time_control"`
	TargetGames   int       `json:"target_games"`
	ProgressEvery int       `json:"progress_every_games"`
	ProgressTime  string    `json:"progress_interval"`
	OpeningFile   string    `json:"opening_file"`
	OpeningCount  int       `json:"opening_count"`
	RandomSeed    int64     `json:"random_seed"`
	Games         int       `json:"games"`
	Wins          int       `json:"wins"`
	Losses        int       `json:"losses"`
	Draws         int       `json:"draws"`
	Score         float64   `json:"score_percent"`
	SPRTLLR       float64   `json:"sprt_llr,omitempty"`
	SPRTLower     float64   `json:"sprt_lower,omitempty"`
	SPRTUpper     float64   `json:"sprt_upper,omitempty"`
	Decision      string    `json:"decision,omitempty"`
	PGNAudit      *pgnAudit `json:"pgn_audit,omitempty"`
	ExitCode      int       `json:"exit_code,omitempty"`
	Error         string    `json:"error,omitempty"`
	RunDir        string    `json:"run_dir"`
}

type pgnAudit struct {
	Games                   int `json:"games"`
	UniqueOpenings          int `json:"unique_openings"`
	UniqueStartPositions    int `json:"unique_fen_start_positions"`
	OpeningGroupsWrongSize  int `json:"opening_groups_with_wrong_size"`
	SingleGameOpeningGroups int `json:"single_game_opening_groups"`
	OversizedOpeningGroups  int `json:"oversized_opening_groups"`
	MinimumBookPlies        int `json:"minimum_book_plies"`
	MaximumBookPlies        int `json:"maximum_book_plies"`
	UniqueGameSequences     int `json:"unique_game_sequences"`
	GamesInDuplicateGroups  int `json:"games_in_duplicate_groups"`
	IdenticalColorSwapPairs int `json:"identical_color_swap_pairs"`
}

type pgnGame struct {
	round        string
	fen          string
	bookSequence string
	sequence     string
}

type matchConfig struct {
	Fastchess     string `json:"fastchess"`
	Baseline      string `json:"baseline"`
	Candidate     string `json:"candidate"`
	Openings      string `json:"openings"`
	BookFormat    string `json:"book_format"`
	BookCount     int    `json:"book_count"`
	Seed          int64  `json:"random_seed"`
	Games         int    `json:"games"`
	TC            string `json:"time_control"`
	Concurrency   int    `json:"concurrency"`
	RunDir        string `json:"run_dir"`
	SPRT          bool   `json:"sprt"`
	ProgressEvery int    `json:"progress_every_games"`
	ProgressTime  string `json:"progress_interval"`
	Follow        bool   `json:"-"`
}

type searchSample struct {
	Depth     int    `json:"depth"`
	Nodes     int64  `json:"nodes"`
	NPS       int64  `json:"nps"`
	ElapsedMS int64  `json:"elapsed_ms"`
	Score     string `json:"score"`
	BestMove  string `json:"bestmove"`
}

type benchCase struct {
	FEN             string         `json:"fen"`
	Samples         []searchSample `json:"samples"`
	MedianNodes     int64          `json:"median_nodes"`
	MedianNPS       int64          `json:"median_nps"`
	MedianElapsedMS int64          `json:"median_elapsed_ms"`
}

type benchReport struct {
	Engine          string      `json:"engine"`
	Depth           int         `json:"depth"`
	Repetitions     int         `json:"repetitions"`
	StartedAt       time.Time   `json:"started_at"`
	FinishedAt      time.Time   `json:"finished_at"`
	Cases           []benchCase `json:"cases"`
	MedianNPS       int64       `json:"median_nps"`
	MedianElapsedMS int64       `json:"median_elapsed_ms"`
}

var (
	scorePattern = regexp.MustCompile(`Score of Candidate vs Baseline:\s*(\d+)\s*-\s*(\d+)\s*-\s*(\d+)\s*\[([0-9.]+)\]\s*(\d+)`)
	sprtPattern  = regexp.MustCompile(`SPRT: llr\s+(-?[0-9.]+).*lbound\s+(-?[0-9.]+),\s+ubound\s+(-?[0-9.]+)`)
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "bench":
		err = benchCommand(os.Args[2:])
	case "audit-pgn":
		err = auditPGNCommand(os.Args[2:])
	case "validate":
		err = validateCommand(os.Args[2:])
	case "start":
		err = startCommand(os.Args[2:])
	case "run-match":
		err = runMatchCommand(os.Args[2:])
	case "status":
		err = statusCommand(os.Args[2:])
	case "progress":
		err = progressCommand(os.Args[2:])
	case "follow":
		err = followCommand(os.Args[2:])
	case "wait":
		err = waitCommand(os.Args[2:])
	case "stop":
		err = stopCommand(os.Args[2:])
	case "pipeline":
		err = pipelineCommand(os.Args[2:])
	case "snapshot":
		err = snapshotCommand(os.Args[2:])
	case "record-decision":
		err = recordDecisionCommand(os.Args[2:])
	default:
		usage()
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "testmonitor:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: testmonitor <audit-pgn|bench|validate|start|status|progress|follow|wait|stop|pipeline|snapshot|record-decision> [options]")
}

func auditPGNCommand(args []string) error {
	fs := flag.NewFlagSet("audit-pgn", flag.ContinueOnError)
	pgn := fs.String("pgn", "", "match PGN to audit")
	output := fs.String("output", "", "optional JSON report path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pgn == "" {
		return errors.New("--pgn is required")
	}
	audit, err := auditPGN(*pgn)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(audit, "", "  ")
	if err != nil {
		return err
	}
	if *output != "" {
		if err := writeJSON(*output, audit); err != nil {
			return err
		}
	}
	fmt.Println(string(data))
	return nil
}

func validateCommand(args []string) error {
	cfg, err := parseMatchConfig("validate", args)
	if err != nil {
		return err
	}
	if cfg.Openings == "" {
		cfg.Openings = defaultOpenings
	}
	cfg, err = normalizeConfig(cfg)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func benchCommand(args []string) error {
	fs := flag.NewFlagSet("bench", flag.ContinueOnError)
	engine := fs.String("engine", "", "engine executable")
	epd := fs.String("epd", "scripts/movetime_epd", "EPD input")
	depth := fs.Int("depth", 8, "fixed search depth")
	repetitions := fs.Int("repetitions", 7, "runs per position")
	output := fs.String("output", "", "JSON report path")
	timeout := fs.Duration("timeout", 2*time.Minute, "timeout per position")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *engine == "" {
		return errors.New("--engine is required")
	}
	if *depth < 1 || *repetitions < 1 {
		return errors.New("depth and repetitions must be positive")
	}

	enginePath, err := existingAbs(*engine)
	if err != nil {
		return err
	}
	epdPath, err := existingAbs(*epd)
	if err != nil {
		return err
	}
	fens, err := readFENs(epdPath)
	if err != nil {
		return err
	}
	if len(fens) == 0 {
		return fmt.Errorf("no positions found in %s", epdPath)
	}

	report := benchReport{Engine: enginePath, Depth: *depth, Repetitions: *repetitions, StartedAt: time.Now()}
	var allNPS, allElapsed []int64
	for ix, fen := range fens {
		bc := benchCase{FEN: fen}
		for rep := 0; rep < *repetitions; rep++ {
			sample, runErr := runSearch(enginePath, fen, *depth, *timeout)
			if runErr != nil {
				return fmt.Errorf("position %d repetition %d: %w", ix+1, rep+1, runErr)
			}
			bc.Samples = append(bc.Samples, sample)
			allNPS = append(allNPS, sample.NPS)
			allElapsed = append(allElapsed, sample.ElapsedMS)
		}
		bc.MedianNodes = sampleMedian(bc.Samples, func(s searchSample) int64 { return s.Nodes })
		bc.MedianNPS = sampleMedian(bc.Samples, func(s searchSample) int64 { return s.NPS })
		bc.MedianElapsedMS = sampleMedian(bc.Samples, func(s searchSample) int64 { return s.ElapsedMS })
		report.Cases = append(report.Cases, bc)
		fmt.Printf("position %d/%d median nps=%d ms=%d\n", ix+1, len(fens), bc.MedianNPS, bc.MedianElapsedMS)
	}
	report.FinishedAt = time.Now()
	report.MedianNPS = median(allNPS)
	report.MedianElapsedMS = median(allElapsed)

	if *output == "" {
		*output = filepath.Join("artifacts", "bench", time.Now().Format("20060102-150405")+".json")
	}
	outputPath, err := filepath.Abs(*output)
	if err != nil {
		return err
	}
	if err := writeJSON(outputPath, report); err != nil {
		return err
	}
	fmt.Printf("benchmark saved: %s\nmedian nps=%d median ms=%d\n", outputPath, report.MedianNPS, report.MedianElapsedMS)
	return nil
}

func startCommand(args []string) error {
	cfg, err := parseMatchConfig("start", args)
	if err != nil {
		return err
	}
	if cfg.RunDir == "" {
		cfg.RunDir = filepath.Join("artifacts", "matches", time.Now().Format("20060102-150405"))
	}
	if cfg.Openings == "" {
		cfg.Openings = defaultOpenings
	}
	cfg, err = normalizeConfig(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.RunDir, 0o755); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(cfg.RunDir, "config.json"), cfg); err != nil {
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(cfg.RunDir, "monitor.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	childArgs := []string{"run-match", "--fastchess", cfg.Fastchess, "--baseline", cfg.Baseline, "--candidate", cfg.Candidate, "--openings", cfg.Openings, "--seed", strconv.FormatInt(cfg.Seed, 10), "--games", strconv.Itoa(cfg.Games), "--tc", cfg.TC, "--concurrency", strconv.Itoa(cfg.Concurrency), "--progress-games", strconv.Itoa(cfg.ProgressEvery), "--progress-interval", cfg.ProgressTime, "--run-dir", cfg.RunDir}
	if cfg.SPRT {
		childArgs = append(childArgs, "--sprt")
	}
	cmd := exec.Command(exe, childArgs...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return err
	}
	_ = logFile.Close()

	status := initialStatus(cfg)
	status.PID = cmd.Process.Pid
	if err := saveStatus(cfg.RunDir, &status); err != nil {
		return err
	}
	fmt.Printf("match started: pid=%d run_dir=%s\n", cmd.Process.Pid, cfg.RunDir)
	fmt.Printf("status: go run ./cmd/testmonitor status --run-dir %s\n", cfg.RunDir)
	if cfg.Follow {
		return followProgress(cfg.RunDir, 500*time.Millisecond)
	}
	return nil
}

func runMatchCommand(args []string) error {
	cfg, err := parseMatchConfig("run-match", args)
	if err != nil {
		return err
	}
	cfg, err = normalizeConfig(cfg)
	if err != nil {
		return err
	}
	if cfg.RunDir == "" {
		return errors.New("--run-dir is required")
	}
	if cfg.Openings == "" {
		return errors.New("--openings is required")
	}

	status := initialStatus(cfg)
	status.PID = os.Getpid()
	status.State = "running"
	status.Stage = "fastchess"
	if err := saveStatus(cfg.RunDir, &status); err != nil {
		return err
	}
	_ = appendProgressSnapshot(cfg.RunDir, status)

	rounds := cfg.Games / 2
	fcArgs := []string{
		"-engine", "cmd=" + cfg.Candidate, "name=Candidate", "option.Hash=128", "option.Threads=1", "option.Ponder=false",
		"-engine", "cmd=" + cfg.Baseline, "name=Baseline", "option.Hash=128", "option.Threads=1", "option.Ponder=false",
		"-each", "tc=" + cfg.TC,
		"-openings", "file=" + cfg.Openings, "format=" + cfg.BookFormat, "order=random",
		"-srand", strconv.FormatInt(cfg.Seed, 10), "-rounds", strconv.Itoa(rounds), "-repeat", "-concurrency", strconv.Itoa(cfg.Concurrency),
		"-resign", "movecount=3", "score=800", "twosided=true",
		"-draw", "movenumber=40", "movecount=8", "score=10", "-maxmoves", "200",
		"-recover", "-autosaveinterval", "10", "-strict",
		"-pgnout", "file=" + filepath.Join(cfg.RunDir, "games.pgn"), "append=false", "notation=uci", "nodes=true", "nps=true",
		"-log", "file=" + filepath.Join(cfg.RunDir, "fastchess.log"), "level=info", "append=false", "realtime=true",
		"-output", "format=cutechess", "-scoreinterval", "1",
	}
	if cfg.SPRT {
		fcArgs = append(fcArgs, "-sprt", "elo0=0", "elo1=5", "alpha=0.05", "beta=0.05", "model=logistic")
	}

	logFile, err := os.OpenFile(filepath.Join(cfg.RunDir, "match.out"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()
	parser := &matchOutput{status: &status, runDir: cfg.RunDir, dst: logFile, progressEvery: cfg.ProgressEvery, nextProgress: cfg.ProgressEvery, sprt: cfg.SPRT}
	progressInterval, _ := time.ParseDuration(cfg.ProgressTime)
	stopPeriodicProgress := parser.startPeriodicProgress(progressInterval)
	defer stopPeriodicProgress()
	cmd := exec.Command(cfg.Fastchess, fcArgs...)
	cmd.Dir = cfg.RunDir
	cmd.Stdout = parser
	cmd.Stderr = parser
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signalCh)
	stopped := false
	if err = cmd.Start(); err == nil {
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err = <-done:
		case received := <-signalCh:
			stopPeriodicProgress()
			stopped = true
			status.State = "stopping"
			status.Error = "stop requested: " + received.String()
			_ = saveStatus(cfg.RunDir, &status)
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
			select {
			case err = <-done:
			case <-time.After(10 * time.Second):
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				err = <-done
			}
		}
	}
	stopPeriodicProgress()
	parser.flush()
	audit, auditErr := auditPGN(filepath.Join(cfg.RunDir, "games.pgn"))
	if auditErr == nil {
		status.PGNAudit = &audit
		if writeErr := writeJSON(filepath.Join(cfg.RunDir, "pgn-audit.json"), audit); writeErr != nil && err == nil {
			err = writeErr
		}
	} else if err == nil && !stopped {
		err = fmt.Errorf("audit completed PGN: %w", auditErr)
	}
	status.UpdatedAt = time.Now()
	status.FinishedAt = status.UpdatedAt
	if stopped {
		status.State = "stopped"
		status.Stage = "finished"
		status.Error = "stopped by user"
		status.Decision = "stopped_by_user"
	} else if err != nil {
		status.State = "failed"
		status.Stage = "finished"
		status.Error = err.Error()
		if exitErr := new(exec.ExitError); errors.As(err, &exitErr) {
			status.ExitCode = exitErr.ExitCode()
		}
	} else {
		status.State = "completed"
		status.Stage = "finished"
		status.Decision = matchDecision(status, cfg.SPRT)
	}
	if saveErr := saveStatus(cfg.RunDir, &status); saveErr != nil {
		return saveErr
	}
	parser.finalProgress()
	if stopped {
		return nil
	}
	return err
}

func statusCommand(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	runDir := fs.String("run-dir", "", "match run directory; defaults to latest")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir, err := resolveRunDir(*runDir)
	if err != nil {
		return err
	}
	status, err := loadStatus(dir)
	if err != nil {
		return err
	}
	data, _ := json.MarshalIndent(status, "", "  ")
	fmt.Println(string(data))
	return nil
}

func waitCommand(args []string) error {
	fs := flag.NewFlagSet("wait", flag.ContinueOnError)
	runDir := fs.String("run-dir", "", "match run directory; defaults to latest")
	interval := fs.Duration("interval", 5*time.Second, "status polling interval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir, err := resolveRunDir(*runDir)
	if err != nil {
		return err
	}
	for {
		status, loadErr := loadStatus(dir)
		if loadErr != nil {
			return loadErr
		}
		fmt.Printf("%s state=%s games=%d/%d W-D-L=%d-%d-%d score=%.2f%%\n", time.Now().Format(time.RFC3339), status.State, status.Games, status.TargetGames, status.Wins, status.Draws, status.Losses, status.Score)
		if status.State == "completed" || status.State == "failed" || status.State == "stopped" {
			if status.State == "failed" {
				return fmt.Errorf("match failed: %s", status.Error)
			}
			return nil
		}
		time.Sleep(*interval)
	}
}

func stopCommand(args []string) error {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	runDir := fs.String("run-dir", "", "match run directory; defaults to latest")
	timeout := fs.Duration("timeout", 15*time.Second, "time allowed for graceful shutdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir, err := resolveRunDir(*runDir)
	if err != nil {
		return err
	}
	status, err := loadStatus(dir)
	if err != nil {
		return err
	}
	if status.State == "completed" || status.State == "failed" || status.State == "stopped" {
		fmt.Printf("match already finished: state=%s run_dir=%s\n", status.State, dir)
		return nil
	}
	if status.PID <= 0 {
		return errors.New("match status has no monitor PID")
	}
	if !processExists(status.PID) {
		status.State = "stopped"
		status.Stage = "finished"
		status.FinishedAt = time.Now()
		status.Error = "monitor process was already gone"
		status.Decision = "stopped_by_user"
		if err := saveStatus(dir, &status); err != nil {
			return err
		}
		_ = appendProgressSnapshot(dir, status)
		fmt.Printf("match stopped: monitor already exited run_dir=%s\n", dir)
		return nil
	}
	expected, err := expectedMonitorProcess(status.PID, dir)
	if err != nil {
		return err
	}
	if !expected {
		return fmt.Errorf("refusing to signal PID %d: it is not the monitor for %s", status.PID, dir)
	}
	status.State = "stopping"
	status.Error = "stop requested by user"
	if err := saveStatus(dir, &status); err != nil {
		return err
	}
	process, err := os.FindProcess(status.PID)
	if err != nil {
		return err
	}
	if err := process.Signal(syscall.SIGTERM); err != nil && processExists(status.PID) {
		return err
	}
	deadline := time.NewTimer(*timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			latest, loadErr := loadStatus(dir)
			if loadErr != nil {
				return loadErr
			}
			if latest.State == "stopped" || latest.State == "completed" || latest.State == "failed" {
				fmt.Printf("match stopped: state=%s games=%d run_dir=%s\n", latest.State, latest.Games, dir)
				return nil
			}
			if !processExists(status.PID) {
				latest.State = "stopped"
				latest.Stage = "finished"
				latest.FinishedAt = time.Now()
				latest.Error = "stopped by user"
				latest.Decision = "stopped_by_user"
				if err := saveStatus(dir, &latest); err != nil {
					return err
				}
				_ = appendProgressSnapshot(dir, latest)
				fmt.Printf("match stopped: games=%d run_dir=%s\n", latest.Games, dir)
				return nil
			}
		case <-deadline.C:
			return fmt.Errorf("monitor PID %d did not stop within %s", status.PID, *timeout)
		}
	}
}

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func expectedMonitorProcess(pid int, runDir string) (bool, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	commandLine := strings.ReplaceAll(string(data), "\x00", " ")
	return strings.Contains(commandLine, "run-match") && strings.Contains(commandLine, runDir), nil
}

func parseMatchConfig(name string, args []string) (matchConfig, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	var cfg matchConfig
	fs.StringVar(&cfg.Fastchess, "fastchess", defaultFastchess, "fastchess executable")
	fs.StringVar(&cfg.Baseline, "baseline", "", "baseline engine")
	fs.StringVar(&cfg.Candidate, "candidate", "", "candidate engine")
	fs.StringVar(&cfg.Openings, "openings", "", "PGN or EPD opening book")
	fs.Int64Var(&cfg.Seed, "seed", 0, "opening randomization seed; random and persisted when zero")
	fs.IntVar(&cfg.Games, "games", 400, "even number of games")
	fs.StringVar(&cfg.TC, "tc", "", "Fastchess time control; defaults to 20+0.2 for screening and 30+0.3 for SPRT")
	fs.IntVar(&cfg.Concurrency, "concurrency", 8, "concurrent games")
	fs.IntVar(&cfg.ProgressEvery, "progress-games", 0, "games between progress snapshots; defaults to 10 for screening and 50 for SPRT")
	fs.StringVar(&cfg.ProgressTime, "progress-interval", defaultProgressInterval, "time between progress snapshots; use 0 to disable")
	fs.BoolVar(&cfg.Follow, "follow", false, "print progress live until the match finishes")
	fs.StringVar(&cfg.RunDir, "run-dir", "", "artifact directory")
	fs.BoolVar(&cfg.SPRT, "sprt", false, "enable SPRT 0/5 Elo")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if cfg.Baseline == "" || cfg.Candidate == "" {
		return cfg, errors.New("--baseline and --candidate are required")
	}
	if cfg.Games < 2 || cfg.Games%2 != 0 {
		return cfg, errors.New("--games must be a positive even number")
	}
	if cfg.Concurrency < 1 {
		return cfg, errors.New("--concurrency must be positive")
	}
	if cfg.ProgressEvery < 0 {
		return cfg, errors.New("--progress-games cannot be negative")
	}
	if cfg.ProgressTime != "0" {
		interval, err := time.ParseDuration(cfg.ProgressTime)
		if err != nil || interval <= 0 {
			return cfg, errors.New("--progress-interval must be a positive duration or 0")
		}
	}
	return cfg, nil
}

func normalizeConfig(cfg matchConfig) (matchConfig, error) {
	var err error
	if cfg.TC == "" {
		cfg.TC = defaultScreeningTC
		if cfg.SPRT {
			cfg.TC = defaultSPRTTC
		}
	}
	if cfg.ProgressTime == "" {
		cfg.ProgressTime = defaultProgressInterval
	}
	if cfg.ProgressEvery == 0 {
		cfg.ProgressEvery = defaultScreeningProgress
		if cfg.SPRT {
			cfg.ProgressEvery = defaultSPRTProgress
		}
	}
	if cfg.Fastchess, err = existingAbs(cfg.Fastchess); err != nil {
		return cfg, err
	}
	if cfg.Baseline, err = existingAbs(cfg.Baseline); err != nil {
		return cfg, err
	}
	if cfg.Candidate, err = existingAbs(cfg.Candidate); err != nil {
		return cfg, err
	}
	if cfg.Openings != "" {
		if cfg.Openings, err = existingAbs(cfg.Openings); err != nil {
			return cfg, err
		}
		cfg.BookFormat = openingFormat(cfg.Openings)
		cfg.BookCount, err = countOpenings(cfg.Openings, cfg.BookFormat)
		if err != nil {
			return cfg, err
		}
		if cfg.BookCount < minimumOpenings {
			return cfg, fmt.Errorf("opening book has %d positions; at least %d are required", cfg.BookCount, minimumOpenings)
		}
		if cfg.Games/2 > cfg.BookCount {
			return cfg, fmt.Errorf("%d paired openings requested, but the book only has %d", cfg.Games/2, cfg.BookCount)
		}
	}
	if cfg.Seed == 0 {
		cfg.Seed = randomSeed()
	}
	if cfg.RunDir != "" {
		cfg.RunDir, err = filepath.Abs(cfg.RunDir)
	}
	return cfg, err
}

func initialStatus(cfg matchConfig) matchStatus {
	now := time.Now()
	return matchStatus{
		RunID: filepath.Base(cfg.RunDir), State: "starting", Stage: "setup", StartedAt: now, UpdatedAt: now,
		Baseline: cfg.Baseline, Candidate: cfg.Candidate, TimeControl: cfg.TC, TargetGames: cfg.Games,
		ProgressEvery: cfg.ProgressEvery, ProgressTime: cfg.ProgressTime, OpeningFile: cfg.Openings, OpeningCount: cfg.BookCount, RandomSeed: cfg.Seed, RunDir: cfg.RunDir,
	}
}

func openingFormat(path string) string {
	if strings.EqualFold(filepath.Ext(path), ".pgn") {
		return "pgn"
	}
	return "epd"
}

func countOpenings(path, format string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	unique := make(map[string]struct{})
	var moves strings.Builder
	flushMoves := func() {
		text := strings.Join(strings.Fields(moves.String()), " ")
		if text != "" {
			unique[text] = struct{}{}
		}
		moves.Reset()
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if format == "pgn" {
			if strings.HasPrefix(line, "[Event ") {
				flushMoves()
			}
			if line != "" && !strings.HasPrefix(line, "[") {
				moves.WriteString(" ")
				moves.WriteString(line)
			}
			continue
		}
		if line != "" && !strings.HasPrefix(line, "#") {
			unique[line] = struct{}{}
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	if format == "pgn" {
		flushMoves()
	}
	return len(unique), nil
}

func randomSeed() int64 {
	var data [4]byte
	if _, err := rand.Read(data[:]); err == nil {
		if seed := int64(binary.LittleEndian.Uint32(data[:])); seed != 0 {
			return seed
		}
	}
	return time.Now().UnixNano() & 0x7fffffff
}

type matchOutput struct {
	mu              sync.Mutex
	buf             bytes.Buffer
	status          *matchStatus
	runDir          string
	dst             io.Writer
	progressEvery   int
	nextProgress    int
	sprt            bool
	pendingProgress bool
}

func (m *matchOutput) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, err := m.dst.Write(p)
	_, _ = m.buf.Write(p)
	for {
		line, readErr := m.buf.ReadString('\n')
		if readErr != nil {
			_, _ = m.buf.WriteString(line)
			break
		}
		m.process(strings.TrimSpace(line))
	}
	return n, err
}

func (m *matchOutput) flush() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.buf.Len() > 0 {
		m.process(strings.TrimSpace(m.buf.String()))
		m.buf.Reset()
	}
}

func (m *matchOutput) process(line string) {
	if llr, lower, upper, ok := parseSPRTLine(line); ok {
		m.status.SPRTLLR = llr
		m.status.SPRTLower = lower
		m.status.SPRTUpper = upper
		m.status.UpdatedAt = time.Now()
		_ = saveStatus(m.runDir, m.status)
		if m.pendingProgress {
			m.emitProgressLocked()
			m.pendingProgress = false
		}
		return
	}
	wins, losses, draws, games, score, ok := parseScoreLine(line)
	if !ok {
		return
	}
	m.status.Wins = wins
	m.status.Losses = losses
	m.status.Draws = draws
	m.status.Games = games
	m.status.Score = score
	m.status.UpdatedAt = time.Now()
	_ = saveStatus(m.runDir, m.status)
	if m.progressEvery > 0 && games >= m.nextProgress {
		for m.nextProgress <= games {
			m.nextProgress += m.progressEvery
		}
		if m.sprt {
			m.pendingProgress = true
		} else {
			m.emitProgressLocked()
		}
	}
}

func (m *matchOutput) emitProgressLocked() {
	if err := appendProgressSnapshot(m.runDir, *m.status); err != nil {
		_, _ = fmt.Fprintf(m.dst, "[progress-error] %v\n", err)
		fmt.Printf("[progress-error] %v\n", err)
		return
	}
	line := formatProgress(*m.status)
	_, _ = fmt.Fprintln(m.dst, line)
	fmt.Println(line)
}

func (m *matchOutput) startPeriodicProgress(interval time.Duration) func() {
	if interval <= 0 {
		return func() {}
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.mu.Lock()
				m.emitProgressLocked()
				m.mu.Unlock()
			case <-stop:
				return
			}
		}
	}()
	return func() {
		once.Do(func() {
			close(stop)
			<-done
		})
	}
}

func (m *matchOutput) finalProgress() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pendingProgress = false
	m.emitProgressLocked()
}

func parseScoreLine(line string) (wins, losses, draws, games int, score float64, ok bool) {
	match := scorePattern.FindStringSubmatch(line)
	if len(match) != 6 {
		return 0, 0, 0, 0, 0, false
	}
	values := []*int{&wins, &losses, &draws, &games}
	for ix, raw := range []string{match[1], match[2], match[3], match[5]} {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return 0, 0, 0, 0, 0, false
		}
		*values[ix] = value
	}
	fraction, err := strconv.ParseFloat(match[4], 64)
	if err != nil {
		return 0, 0, 0, 0, 0, false
	}
	return wins, losses, draws, games, math.Round(fraction*1000) / 10, true
}

func parseSPRTLine(line string) (llr, lower, upper float64, ok bool) {
	match := sprtPattern.FindStringSubmatch(line)
	if len(match) != 4 {
		return 0, 0, 0, false
	}
	values := []*float64{&llr, &lower, &upper}
	for ix, raw := range match[1:] {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return 0, 0, 0, false
		}
		*values[ix] = value
	}
	return llr, lower, upper, true
}

func matchDecision(status matchStatus, sprt bool) string {
	if status.PGNAudit == nil {
		return "invalid_missing_pgn_audit"
	}
	if status.PGNAudit.UniqueOpenings < minimumOpenings {
		return "invalid_insufficient_openings"
	}
	if !validOpeningPairing(*status.PGNAudit, sprt) {
		return "invalid_unpaired_openings"
	}
	if sprt {
		if status.SPRTUpper != 0 && status.SPRTLLR >= status.SPRTUpper {
			return "accepted_h1"
		}
		if status.SPRTLower != 0 && status.SPRTLLR <= status.SPRTLower {
			return "rejected_h0"
		}
		return "inconclusive_at_game_limit"
	}
	if status.Score >= 47 {
		return "passed_screening"
	}
	return "rejected_below_47_percent"
}

func validOpeningPairing(audit pgnAudit, sprt bool) bool {
	if audit.OpeningGroupsWrongSize == 0 {
		return true
	}
	// Fastchess evaluates SPRT after every completed game and can therefore
	// stop after the first game of the final color-swapped opening pair.
	return sprt && audit.OpeningGroupsWrongSize == 1 && audit.SingleGameOpeningGroups == 1 && audit.OversizedOpeningGroups == 0
}

var (
	pgnCommentPattern    = regexp.MustCompile(`(?s)\{.*?\}`)
	pgnBookMovePattern   = regexp.MustCompile(`([a-h][1-8][a-h][1-8][qrbn]?)\s+\{book(?:[^}]*)\}`)
	pgnMoveNumberPattern = regexp.MustCompile(`^\d+\.(?:\.\.)?`)
)

// auditPGN checks the actual match output rather than assuming that the book
// configuration produced varied games. A game identity includes its starting
// position because identical continuations from different positions are not
// identical chess games.
func auditPGN(path string) (pgnAudit, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return pgnAudit{}, err
	}

	games, err := parsePGNGames(string(data))
	if err != nil {
		return pgnAudit{}, err
	}

	audit := pgnAudit{Games: len(games)}
	starts := make(map[string]struct{})
	openings := make(map[string]int)
	identities := make(map[string]int)
	rounds := make(map[string][]string)
	for ix, game := range games {
		if game.fen != "" {
			starts[game.fen] = struct{}{}
		}
		opening := game.bookSequence
		if opening == "" {
			opening = game.fen
		}
		if opening == "" {
			opening = "standard-start-without-book-moves"
		}
		openings[opening]++
		bookPlies := len(strings.Fields(game.bookSequence))
		if ix == 0 || bookPlies < audit.MinimumBookPlies {
			audit.MinimumBookPlies = bookPlies
		}
		if bookPlies > audit.MaximumBookPlies {
			audit.MaximumBookPlies = bookPlies
		}
		identity := game.fen + "\n" + game.sequence
		identities[identity]++
		rounds[game.round] = append(rounds[game.round], identity)
	}

	audit.UniqueOpenings = len(openings)
	audit.UniqueStartPositions = len(starts)
	audit.UniqueGameSequences = len(identities)
	for _, count := range openings {
		if count != 2 {
			audit.OpeningGroupsWrongSize++
		}
		if count == 1 {
			audit.SingleGameOpeningGroups++
		}
		if count > 2 {
			audit.OversizedOpeningGroups++
		}
	}
	for _, count := range identities {
		if count > 1 {
			audit.GamesInDuplicateGroups += count
		}
	}
	for _, pair := range rounds {
		if len(pair) == 2 && pair[0] == pair[1] {
			audit.IdenticalColorSwapPairs++
		}
	}
	return audit, nil
}

func parsePGNGames(input string) ([]pgnGame, error) {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	starts := regexp.MustCompile(`(?m)^\[Event `).FindAllStringIndex(input, -1)
	if len(starts) == 0 {
		return nil, errors.New("PGN contains no games")
	}
	header := regexp.MustCompile(`(?m)^\[([A-Za-z0-9_]+) "([^"]*)"\]$`)
	resultToken := map[string]bool{"1-0": true, "0-1": true, "1/2-1/2": true, "*": true}
	games := make([]pgnGame, 0, len(starts))
	for ix, start := range starts {
		end := len(input)
		if ix+1 < len(starts) {
			end = starts[ix+1][0]
		}
		block := input[start[0]:end]
		tags := make(map[string]string)
		for _, match := range header.FindAllStringSubmatch(block, -1) {
			tags[match[1]] = match[2]
		}
		body := header.ReplaceAllString(block, "")
		bookMoves := make([]string, 0)
		for _, match := range pgnBookMovePattern.FindAllStringSubmatch(body, -1) {
			bookMoves = append(bookMoves, match[1])
		}
		body = pgnCommentPattern.ReplaceAllString(body, " ")
		moves := make([]string, 0)
		for _, token := range strings.Fields(body) {
			token = pgnMoveNumberPattern.ReplaceAllString(token, "")
			if token == "" || resultToken[token] || strings.HasPrefix(token, "$") {
				continue
			}
			moves = append(moves, token)
		}
		if len(moves) == 0 {
			return nil, fmt.Errorf("game %d has no moves", ix+1)
		}
		games = append(games, pgnGame{
			round: tags["Round"], fen: tags["FEN"], bookSequence: strings.Join(bookMoves, " "),
			sequence: strings.Join(moves, " "),
		})
	}
	return games, nil
}

func runSearch(engine, fen string, depth int, timeout time.Duration) (searchSample, error) {
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
	defer func() {
		_, _ = io.WriteString(stdin, "quit\n")
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	lines := make(chan string, 128)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	send := func(line string) error { _, writeErr := io.WriteString(stdin, line+"\n"); return writeErr }
	waitFor := func(target string) error {
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
	if err := send("uci"); err != nil {
		return searchSample{}, err
	}
	if err := waitFor("uciok"); err != nil {
		return searchSample{}, err
	}
	for _, line := range []string{"setoption name Hash value 128", "setoption name Threads value 1", "setoption name Ponder value false", "isready"} {
		if err := send(line); err != nil {
			return searchSample{}, err
		}
	}
	if err := waitFor("readyok"); err != nil {
		return searchSample{}, err
	}
	if err := send("ucinewgame"); err != nil {
		return searchSample{}, err
	}
	if err := send("position fen " + fen); err != nil {
		return searchSample{}, err
	}
	started := time.Now()
	if err := send(fmt.Sprintf("go depth %d", depth)); err != nil {
		return searchSample{}, err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var sample searchSample
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
				sample.ElapsedMS = time.Since(started).Milliseconds()
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

func parseInfo(fields []string, sample *searchSample) {
	for ix := 0; ix+1 < len(fields); ix++ {
		switch fields[ix] {
		case "depth":
			sample.Depth, _ = strconv.Atoi(fields[ix+1])
		case "nodes":
			sample.Nodes, _ = strconv.ParseInt(fields[ix+1], 10, 64)
		case "nps":
			sample.NPS, _ = strconv.ParseInt(fields[ix+1], 10, 64)
		case "cp", "mate":
			if ix > 0 && fields[ix-1] == "score" {
				sample.Score = fields[ix] + " " + fields[ix+1]
			}
		}
	}
}

func readFENs(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var fens []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 4 && strings.Contains(fields[0], "/") && (fields[1] == "w" || fields[1] == "b") {
			fens = append(fens, strings.Join(fields[:4], " "))
		}
	}
	return fens, scanner.Err()
}

func sampleMedian(samples []searchSample, value func(searchSample) int64) int64 {
	values := make([]int64, len(samples))
	for ix, sample := range samples {
		values[ix] = value(sample)
	}
	return median(values)
}

func median(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]int64(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i] < copyValues[j] })
	return copyValues[len(copyValues)/2]
}

func existingAbs(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(abs); err != nil {
		return "", err
	}
	return abs, nil
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func saveStatus(runDir string, status *matchStatus) error {
	status.UpdatedAt = time.Now()
	return writeJSON(filepath.Join(runDir, "status.json"), status)
}

func loadStatus(runDir string) (matchStatus, error) {
	var status matchStatus
	data, err := os.ReadFile(filepath.Join(runDir, "status.json"))
	if err != nil {
		return status, err
	}
	err = json.Unmarshal(data, &status)
	return status, err
}

func resolveRunDir(runDir string) (string, error) {
	if runDir != "" {
		return filepath.Abs(runDir)
	}
	base := filepath.Join("artifacts", "matches")
	entries, err := os.ReadDir(base)
	if err != nil {
		return "", err
	}
	latestDir := ""
	var latestMod time.Time
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, statErr := os.Stat(filepath.Join(base, entry.Name(), "status.json"))
		if statErr == nil && (latestDir == "" || info.ModTime().After(latestMod)) {
			latestDir = entry.Name()
			latestMod = info.ModTime()
		}
	}
	if latestDir == "" {
		return "", errors.New("no match runs found")
	}
	return filepath.Abs(filepath.Join(base, latestDir))
}
