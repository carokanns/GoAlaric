package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	depthProfileSchemaVersion       = 1
	defaultPreScanGames             = 40
	defaultPreScanSeed        int64 = 20260805
)

type depthProfileSettings struct {
	TimeControl  string `json:"time_control"`
	Games        int    `json:"games"`
	Concurrency  int    `json:"concurrency"`
	HashMB       int    `json:"hash_mb"`
	Threads      int    `json:"threads"`
	Openings     string `json:"openings"`
	OpeningsSHA  string `json:"openings_sha256"`
	Fastchess    string `json:"fastchess"`
	FastchessSHA string `json:"fastchess_sha256"`
	OpponentSHA  string `json:"opponent_sha256"`
	Machine      string `json:"machine"`
	RandomSeed   int64  `json:"random_seed"`
}

type depthProfileReport struct {
	SchemaVersion  int                  `json:"schema_version"`
	CacheKey       string               `json:"cache_key"`
	Role           string               `json:"role"`
	CreatedAt      time.Time            `json:"created_at"`
	Engine         experimentIdentity   `json:"engine"`
	Settings       depthProfileSettings `json:"settings"`
	MinimumDepth   int                  `json:"minimum_depth"`
	SampleCount    int                  `json:"sample_count"`
	MeanDepth      float64              `json:"mean_depth"`
	MedianDepth    int                  `json:"median_depth"`
	P25Depth       int                  `json:"p25_depth"`
	P90Depth       int                  `json:"p90_depth"`
	MeanSelDepth   float64              `json:"mean_seldepth"`
	MedianSelDepth int                  `json:"median_seldepth"`
	MedianNodes    int64                `json:"median_nodes"`
	MedianNPS      int64                `json:"median_nps"`
	Decision       string               `json:"decision"`
	TracePath      string               `json:"trace_path"`
}

type depthTraceSample struct {
	Depth    int
	SelDepth int
	Nodes    int64
	TimeMS   int64
	NPS      int64
}

func preScanCommand(args []string) error {
	fs := flag.NewFlagSet("prescan", flag.ContinueOnError)
	engine := fs.String("engine", "", "engine executable to profile in self-play")
	role := fs.String("role", "standalone", "profile role: baseline, candidate or standalone")
	minimumDepth := fs.Int("minimum-depth", 0, "minimum accepted median depth")
	games := fs.Int("games", defaultPreScanGames, "even self-play game count")
	tc := fs.String("tc", defaultScreeningTC, "Fastchess time control")
	concurrency := fs.Int("concurrency", 8, "concurrent games")
	hashMB := fs.Int("hash", 128, "engine hash in MB")
	threads := fs.Int("threads", 1, "threads per engine")
	fastchess := fs.String("fastchess", defaultFastchess, "Fastchess executable")
	openings := fs.String("openings", defaultOpenings, "opening book")
	repoRoot := fs.String("repo-root", ".", "GoAlaric repository root")
	runDir := fs.String("run-dir", "", "artifact directory")
	seed := fs.Int64("seed", defaultPreScanSeed, "opening randomization seed")
	follow := fs.Bool("follow", false, "print progress until profiling finishes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *engine == "" {
		return errors.New("--engine is required")
	}
	root, err := existingAbs(*repoRoot)
	if err != nil {
		return err
	}
	if *runDir == "" {
		*runDir = filepath.Join(root, "artifacts", "prescans", *role+"-"+time.Now().Format("20060102-150405"))
	}
	resolveFromRoot := func(path string) (string, error) {
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		return existingAbs(path)
	}
	enginePath, err := resolveFromRoot(*engine)
	if err != nil {
		return err
	}
	fastchessPath, err := resolveFromRoot(*fastchess)
	if err != nil {
		return err
	}
	openingsPath, err := resolveFromRoot(*openings)
	if err != nil {
		return err
	}
	startArgs := []string{
		"--baseline", enginePath, "--candidate", enginePath,
		"--fastchess", fastchessPath, "--openings", openingsPath,
		"--games", strconv.Itoa(*games), "--tc", *tc,
		"--concurrency", strconv.Itoa(*concurrency),
		"--hash", strconv.Itoa(*hashMB), "--threads", strconv.Itoa(*threads),
		"--run-dir", *runDir, "--repo-root", root,
		"--allow-identical-binaries",
		"--depth-profile", "--profile-role", *role,
		"--minimum-depth", strconv.Itoa(*minimumDepth),
		"--seed", strconv.FormatInt(*seed, 10),
	}
	if *follow {
		startArgs = append(startArgs, "--follow")
	}
	if err := startCommand(startArgs); err != nil {
		return err
	}
	fmt.Printf("subl %s\n", filepath.Join(*runDir, "monitor.log"))
	return nil
}

func buildDepthProfile(cfg matchConfig, tracePath string) (depthProfileReport, error) {
	samples, err := parseDepthTrace(tracePath, cfg.ProfileRole)
	if err != nil {
		return depthProfileReport{}, err
	}
	if len(samples) == 0 {
		return depthProfileReport{}, errors.New("depth trace contains no completed searches")
	}
	engine, settings, cacheKey, err := depthProfileIdentity(cfg)
	if err != nil {
		return depthProfileReport{}, err
	}
	depths := make([]int, 0, len(samples))
	selDepths := make([]int, 0, len(samples))
	nodes := make([]int64, 0, len(samples))
	nps := make([]int64, 0, len(samples))
	var depthSum, selDepthSum int64
	for _, sample := range samples {
		depths = append(depths, sample.Depth)
		selDepths = append(selDepths, sample.SelDepth)
		nodes = append(nodes, sample.Nodes)
		if sample.NPS > 0 {
			nps = append(nps, sample.NPS)
		}
		depthSum += int64(sample.Depth)
		selDepthSum += int64(sample.SelDepth)
	}
	report := depthProfileReport{
		SchemaVersion: depthProfileSchemaVersion, CacheKey: cacheKey, Role: cfg.ProfileRole,
		CreatedAt: time.Now(), Engine: engine, Settings: settings, MinimumDepth: cfg.MinimumDepth,
		SampleCount: len(samples), MeanDepth: float64(depthSum) / float64(len(samples)),
		MedianDepth: percentileInt(depths, 50), P25Depth: percentileInt(depths, 25),
		P90Depth: percentileInt(depths, 90), MeanSelDepth: float64(selDepthSum) / float64(len(samples)),
		MedianSelDepth: percentileInt(selDepths, 50), MedianNodes: median(nodes), MedianNPS: median(nps),
		TracePath: tracePath,
	}
	report.Decision = "depth_adequate"
	if cfg.MinimumDepth > 0 && report.MedianDepth < cfg.MinimumDepth {
		report.Decision = "increase_time_control"
	}
	return report, nil
}

func parseDepthTrace(path, role string) ([]depthTraceSample, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	latest := make(map[string]depthTraceSample)
	var samples []depthTraceSample
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		arrow := strings.Index(line, "--->")
		if !strings.HasPrefix(line, "[Engine]") || arrow < 0 {
			continue
		}
		left := strings.TrimSpace(line[:arrow])
		message := strings.TrimSpace(line[arrow+4:])
		contextStart := strings.LastIndex(left, "<")
		contextEnd := strings.LastIndex(left, ">")
		if contextStart < 0 || contextEnd <= contextStart {
			continue
		}
		context := strings.TrimSpace(left[contextStart+1 : contextEnd])
		engine := strings.TrimSpace(left[contextEnd+1:])
		if role == "candidate" && engine != "Candidate" {
			continue
		}
		key := context + "\x00" + engine
		fields := strings.Fields(message)
		if len(fields) >= 3 && fields[0] == "info" && fields[1] == "depth" {
			var sample depthTraceSample
			parseDepthInfo(fields, &sample)
			if sample.Depth > 0 {
				latest[key] = sample
			}
			continue
		}
		if len(fields) >= 2 && fields[0] == "bestmove" {
			if sample, ok := latest[key]; ok {
				if sample.NPS == 0 && sample.TimeMS > 0 {
					sample.NPS = sample.Nodes * 1000 / sample.TimeMS
				}
				samples = append(samples, sample)
				delete(latest, key)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return samples, nil
}

func parseDepthInfo(fields []string, sample *depthTraceSample) {
	for ix := 0; ix+1 < len(fields); ix++ {
		switch fields[ix] {
		case "depth":
			sample.Depth, _ = strconv.Atoi(fields[ix+1])
		case "seldepth":
			sample.SelDepth, _ = strconv.Atoi(fields[ix+1])
		case "nodes":
			sample.Nodes, _ = strconv.ParseInt(fields[ix+1], 10, 64)
		case "time":
			sample.TimeMS, _ = strconv.ParseInt(fields[ix+1], 10, 64)
		case "nps":
			sample.NPS, _ = strconv.ParseInt(fields[ix+1], 10, 64)
		}
	}
}

func percentileInt(values []int, percent int) int {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	index := (len(sorted) - 1) * percent / 100
	return sorted[index]
}

func depthProfileIdentity(cfg matchConfig) (experimentIdentity, depthProfileSettings, string, error) {
	engine, err := identifyExperimentBinary(cfg.Candidate)
	if err != nil {
		return experimentIdentity{}, depthProfileSettings{}, "", err
	}
	opponent, err := identifyExperimentBinary(cfg.Baseline)
	if err != nil {
		return experimentIdentity{}, depthProfileSettings{}, "", err
	}
	openingsData, err := os.ReadFile(cfg.Openings)
	if err != nil {
		return experimentIdentity{}, depthProfileSettings{}, "", err
	}
	fastchessData, err := os.ReadFile(cfg.Fastchess)
	if err != nil {
		return experimentIdentity{}, depthProfileSettings{}, "", err
	}
	settings := depthProfileSettings{
		TimeControl: cfg.TC, Games: cfg.Games, Concurrency: cfg.Concurrency,
		HashMB: cfg.HashMB, Threads: cfg.Threads, Openings: cfg.Openings,
		OpeningsSHA: digest(openingsData), Fastchess: cfg.Fastchess,
		FastchessSHA: digest(fastchessData), Machine: depthMachineIdentity(),
		RandomSeed: cfg.Seed, OpponentSHA: opponent.SHA256,
	}
	key := hashJSON(struct {
		SchemaVersion int                  `json:"schema_version"`
		EngineSHA     string               `json:"engine_sha256"`
		Settings      depthProfileSettings `json:"settings"`
	}{depthProfileSchemaVersion, engine.SHA256, settings})
	return engine, settings, key, nil
}

func depthMachineIdentity() string {
	model := "unknown-cpu"
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.ToLower(line), "model name") {
				if _, value, found := strings.Cut(line, ":"); found {
					model = strings.TrimSpace(value)
				}
				break
			}
		}
	}
	return fmt.Sprintf("%s/%s cpu=%s logical=%d", runtime.GOOS, runtime.GOARCH, model, runtime.NumCPU())
}

func persistDepthProfile(cfg matchConfig, report depthProfileReport) error {
	if err := writeJSON(filepath.Join(cfg.RunDir, "depth-profile.json"), report); err != nil {
		return err
	}
	return writeJSON(filepath.Join(cfg.DepthCacheDir, report.CacheKey+".json"), report)
}

func loadCachedDepthProfile(cfg matchConfig) (depthProfileReport, string, bool, error) {
	_, _, cacheKey, err := depthProfileIdentity(cfg)
	if err != nil {
		return depthProfileReport{}, "", false, err
	}
	path := filepath.Join(cfg.DepthCacheDir, cacheKey+".json")
	var report depthProfileReport
	if err := readJSON(path, &report); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return depthProfileReport{}, path, false, nil
		}
		return depthProfileReport{}, path, false, err
	}
	if report.CacheKey != cacheKey || report.SchemaVersion != depthProfileSchemaVersion {
		return depthProfileReport{}, path, false, errors.New("cached depth profile identity mismatch")
	}
	return report, path, true, nil
}
