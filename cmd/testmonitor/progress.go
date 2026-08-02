package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type progressSnapshot struct {
	Timestamp         time.Time `json:"timestamp"`
	RunID             string    `json:"run_id"`
	State             string    `json:"state"`
	Stage             string    `json:"stage"`
	Games             int       `json:"games"`
	TargetGames       int       `json:"target_games"`
	CompletionPercent float64   `json:"completion_percent"`
	Wins              int       `json:"wins"`
	Losses            int       `json:"losses"`
	Draws             int       `json:"draws"`
	ScorePercent      float64   `json:"score_percent"`
	SPRTLLR           float64   `json:"sprt_llr,omitempty"`
	SPRTLower         float64   `json:"sprt_lower,omitempty"`
	SPRTUpper         float64   `json:"sprt_upper,omitempty"`
	Decision          string    `json:"decision,omitempty"`
}

func snapshotFromStatus(status matchStatus) progressSnapshot {
	completion := 0.0
	if status.TargetGames > 0 {
		completion = float64(status.Games) * 100 / float64(status.TargetGames)
	}
	return progressSnapshot{
		Timestamp: time.Now(), RunID: status.RunID, State: status.State, Stage: status.Stage,
		Games: status.Games, TargetGames: status.TargetGames, CompletionPercent: completion,
		Wins: status.Wins, Losses: status.Losses, Draws: status.Draws, ScorePercent: status.Score,
		SPRTLLR: status.SPRTLLR, SPRTLower: status.SPRTLower, SPRTUpper: status.SPRTUpper,
		Decision: status.Decision,
	}
}

func appendProgressSnapshot(runDir string, status matchStatus) error {
	snapshot := snapshotFromStatus(status)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(runDir, "progress.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if _, err = file.Write(append(data, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return writeJSON(filepath.Join(runDir, "progress.json"), snapshot)
}

func formatProgress(status matchStatus) string {
	completion := 0.0
	if status.TargetGames > 0 {
		completion = float64(status.Games) * 100 / float64(status.TargetGames)
	}
	line := fmt.Sprintf("[progress] games=%d/%d (%.1f%%) W-D-L=%d-%d-%d score=%.1f%% state=%s",
		status.Games, status.TargetGames, completion, status.Wins, status.Draws, status.Losses, status.Score, status.State)
	if status.SPRTLower != 0 || status.SPRTUpper != 0 {
		line += fmt.Sprintf(" SPRT=%.2f [%.2f, %.2f]", status.SPRTLLR, status.SPRTLower, status.SPRTUpper)
	}
	if status.Decision != "" {
		line += " decision=" + status.Decision
	}
	return line
}

func progressCommand(args []string) error {
	fs := flag.NewFlagSet("progress", flag.ContinueOnError)
	runDir := fs.String("run-dir", "", "match run directory; defaults to latest")
	tail := fs.Int("tail", 10, "number of progress snapshots to show")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *tail < 1 {
		return errors.New("--tail must be positive")
	}
	dir, err := resolveRunDir(*runDir)
	if err != nil {
		return err
	}
	snapshots, err := readProgressSnapshots(filepath.Join(dir, "progress.jsonl"))
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		status, loadErr := loadStatus(dir)
		if loadErr != nil {
			return loadErr
		}
		fmt.Println(formatProgress(status))
		return nil
	}
	start := len(snapshots) - *tail
	if start < 0 {
		start = 0
	}
	for _, snapshot := range snapshots[start:] {
		fmt.Println(formatProgressSnapshot(snapshot))
	}
	return nil
}

func followCommand(args []string) error {
	fs := flag.NewFlagSet("follow", flag.ContinueOnError)
	runDir := fs.String("run-dir", "", "match run directory; defaults to latest")
	interval := fs.Duration("interval", 500*time.Millisecond, "progress polling interval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *interval <= 0 {
		return errors.New("--interval must be positive")
	}
	dir, err := resolveRunDir(*runDir)
	if err != nil {
		return err
	}
	return followProgress(dir, *interval)
}

func followProgress(runDir string, interval time.Duration) error {
	seen := 0
	for {
		snapshots, err := readProgressSnapshots(filepath.Join(runDir, "progress.jsonl"))
		if err == nil {
			for _, snapshot := range snapshots[seen:] {
				fmt.Println(formatProgressSnapshot(snapshot))
			}
			seen = len(snapshots)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}

		status, err := loadStatus(runDir)
		if err != nil {
			return err
		}
		if status.State == "completed" || status.State == "failed" || status.State == "stopped" {
			if seen == 0 || snapshots[seen-1].State != status.State {
				fmt.Printf("%s %s\n", time.Now().Format(time.RFC3339), strings.TrimPrefix(formatProgress(status), "[progress] "))
			}
			return nil
		}
		time.Sleep(interval)
	}
}

func formatProgressSnapshot(snapshot progressSnapshot) string {
	status := matchStatus{
		State: snapshot.State, Games: snapshot.Games, TargetGames: snapshot.TargetGames,
		Wins: snapshot.Wins, Losses: snapshot.Losses, Draws: snapshot.Draws, Score: snapshot.ScorePercent,
		SPRTLLR: snapshot.SPRTLLR, SPRTLower: snapshot.SPRTLower, SPRTUpper: snapshot.SPRTUpper,
		Decision: snapshot.Decision,
	}
	return fmt.Sprintf("%s %s", snapshot.Timestamp.Format(time.RFC3339), strings.TrimPrefix(formatProgress(status), "[progress] "))
}

func readProgressSnapshots(path string) ([]progressSnapshot, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var snapshots []progressSnapshot
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var snapshot progressSnapshot
		if err := json.Unmarshal(scanner.Bytes(), &snapshot); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, scanner.Err()
}
