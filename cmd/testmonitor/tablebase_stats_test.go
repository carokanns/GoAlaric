package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildTablebaseStatsAggregatesOneLinePerGame(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tablebase-game-stats.log")
	if err := os.WriteFile(path, []byte("hits=0 root_wins=0\nhits=12 root_wins=3\nhits=5 root_wins=0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := buildTablebaseStats(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Games) != 3 || report.Candidate.TotalHits != 17 || report.Candidate.GamesWithHits != 2 {
		t.Fatalf("report = %+v", report)
	}
	if report.Candidate.RootWinGames != 1 || report.Candidate.RootWinProbes != 3 {
		t.Fatalf("candidate = %+v", report.Candidate)
	}
}
