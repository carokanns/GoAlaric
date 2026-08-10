package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type suite struct {
	SchemaVersion int        `json:"schema_version"`
	Source        sourceInfo `json:"source"`
	Cases         []testCase `json:"cases"`
}

type sourceInfo struct {
	Oracle string `json:"oracle"`
	Seed   int64  `json:"seed"`
}

type testCase struct {
	ID                string   `json:"id"`
	FEN               string   `json:"fen"`
	Kind              string   `json:"kind"`
	OracleWDL         string   `json:"oracle_wdl"`
	ExpectedScore     string   `json:"expected_score,omitempty"`
	MinimumScoreCP    *int     `json:"minimum_score_cp,omitempty"`
	AcceptableMoves   []string `json:"acceptable_moves"`
	ForbiddenMoves    []string `json:"forbidden_moves"`
	MaterialDrawPlies int      `json:"material_draw_plies"`
}

type caseResult struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	BestMove       string `json:"bestmove,omitempty"`
	Score          string `json:"score,omitempty"`
	Status         string `json:"status"`
	Failure        string `json:"failure,omitempty"`
	DurationMillis int64  `json:"duration_ms"`
}

type report struct {
	SchemaVersion int          `json:"schema_version"`
	Engine        string       `json:"engine"`
	CasesFile     string       `json:"cases_file"`
	Depth         int          `json:"depth"`
	Passed        int          `json:"passed"`
	Failed        int          `json:"failed"`
	Status        string       `json:"status"`
	Results       []caseResult `json:"results"`
}

func main() {
	engine := flag.String("engine", "", "engine binary")
	casesPath := flag.String("cases", "scripts/material_draw_cases.json", "generated case suite")
	depth := flag.Int("depth", 6, "fixed search depth")
	output := flag.String("output", "", "optional JSON report")
	timeout := flag.Duration("timeout", 20*time.Second, "timeout per case")
	flag.Parse()

	if *engine == "" {
		fatal(errors.New("--engine is required"))
	}
	data, err := os.ReadFile(*casesPath)
	if err != nil {
		fatal(err)
	}
	var cases suite
	if err := json.Unmarshal(data, &cases); err != nil {
		fatal(err)
	}
	if cases.SchemaVersion != 1 || len(cases.Cases) == 0 {
		fatal(errors.New("unsupported or empty case suite"))
	}

	rep := report{SchemaVersion: 1, Engine: *engine, CasesFile: *casesPath, Depth: *depth, Status: "passed"}
	for _, test := range cases.Cases {
		result := runCase(*engine, test, *depth, *timeout)
		rep.Results = append(rep.Results, result)
		if result.Status == "passed" {
			rep.Passed++
		} else {
			rep.Failed++
			rep.Status = "failed"
		}
		fmt.Printf("[%s] %-24s bestmove=%-5s score=%s", result.Status, test.ID, result.BestMove, result.Score)
		if result.Failure != "" {
			fmt.Printf(" failure=%s", result.Failure)
		}
		fmt.Println()
	}

	encoded, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fatal(err)
	}
	encoded = append(encoded, '\n')
	if *output != "" {
		if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
			fatal(err)
		}
		if err := os.WriteFile(*output, encoded, 0o644); err != nil {
			fatal(err)
		}
	}
	if rep.Failed != 0 {
		os.Exit(1)
	}
}

func runCase(engine string, test testCase, depth int, timeout time.Duration) (result caseResult) {
	started := time.Now()
	result = caseResult{ID: test.ID, Kind: test.Kind, Status: "failed"}
	defer func() { result.DurationMillis = time.Since(started).Milliseconds() }()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, engine)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		result.Failure = err.Error()
		return result
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		result.Failure = err.Error()
		return result
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		result.Failure = err.Error()
		return result
	}
	scanner := bufio.NewScanner(stdout)
	send := func(command string) error {
		_, err := fmt.Fprintln(stdin, command)
		return err
	}
	waitFor := func(prefix string) error {
		for scanner.Scan() {
			if strings.HasPrefix(strings.TrimSpace(scanner.Text()), prefix) {
				return nil
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("engine exited before " + prefix)
	}

	if err := send("uci"); err != nil {
		result.Failure = err.Error()
		return result
	}
	if err := waitFor("uciok"); err != nil {
		result.Failure = err.Error()
		return result
	}
	for _, command := range []string{
		"setoption name Hash value 16",
		"setoption name Threads value 1",
		"setoption name Ponder value false",
		"setoption name Contempt value 5",
		"ucinewgame",
		"isready",
	} {
		if err := send(command); err != nil {
			result.Failure = err.Error()
			return result
		}
	}
	if err := waitFor("readyok"); err != nil {
		result.Failure = err.Error()
		return result
	}
	if err := send("position fen " + test.FEN); err != nil {
		result.Failure = err.Error()
		return result
	}
	if err := send(fmt.Sprintf("go depth %d", depth)); err != nil {
		result.Failure = err.Error()
		return result
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		fields := strings.Fields(line)
		if strings.HasPrefix(line, "info ") {
			for i := 0; i+2 < len(fields); i++ {
				if fields[i] == "score" {
					result.Score = fields[i+1] + " " + fields[i+2]
				}
			}
		}
		if len(fields) >= 2 && fields[0] == "bestmove" {
			result.BestMove = fields[1]
			break
		}
	}
	_ = send("quit")
	_ = cmd.Wait()
	if result.BestMove == "" {
		if ctx.Err() != nil {
			result.Failure = ctx.Err().Error()
		} else {
			result.Failure = "missing bestmove"
		}
		return result
	}
	if err := validateResult(test, result.BestMove, result.Score); err != nil {
		result.Failure = err.Error()
		return result
	}
	result.Status = "passed"
	return result
}

func validateResult(test testCase, bestMove, score string) error {
	if !slices.Contains(test.AcceptableMoves, bestMove) {
		return fmt.Errorf("move is not Syzygy-optimal; acceptable=%s", strings.Join(test.AcceptableMoves, ","))
	}
	if test.ExpectedScore != "" && score != test.ExpectedScore {
		return fmt.Errorf("score=%q, want %q", score, test.ExpectedScore)
	}
	if test.MinimumScoreCP != nil {
		var value int
		if _, err := fmt.Sscanf(score, "cp %d", &value); err != nil || value < *test.MinimumScoreCP {
			return fmt.Errorf("score=%q, want at least cp %d", score, *test.MinimumScoreCP)
		}
	}
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "materialdrawtest:", err)
	os.Exit(2)
}
