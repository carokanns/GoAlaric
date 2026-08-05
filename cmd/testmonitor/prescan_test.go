package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestParseCandidateDepthTraceUsesOnlyTargetEngine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fastchess.log")
	trace := `[Engine] [10:00:00.000001] < 101 > Candidate ---> info depth 7 seldepth 11 nodes 700 time 10 score cp 1 pv e2e4
[Engine] [10:00:00.000002] < 101 > Candidate ---> bestmove e2e4
[Engine] [10:00:00.000003] < 101 > Baseline ---> info depth 20 seldepth 30 nodes 9000 time 10 score cp 0 pv e7e5
[Engine] [10:00:00.000004] < 101 > Baseline ---> bestmove e7e5
[Engine] [10:00:00.000005] < 202 > Candidate ---> info depth 8 seldepth 12 nodes 800 time 10 nps 80000 score cp 2 pv g1f3
[Engine] [10:00:00.000006] < 202 > Candidate ---> info depth 9 seldepth 14 nodes 900 time 12 score cp 3 pv g1f3
[Engine] [10:00:00.000007] < 202 > Candidate ---> bestmove g1f3
`
	if err := os.WriteFile(path, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	samples, err := parseDepthTrace(path, "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 {
		t.Fatalf("samples=%d, want 2", len(samples))
	}
	if samples[0].Depth != 7 || samples[0].NPS != 70000 {
		t.Fatalf("first sample=%+v", samples[0])
	}
	if samples[1].Depth != 9 || samples[1].SelDepth != 14 || samples[1].NPS != 75000 {
		t.Fatalf("second sample=%+v", samples[1])
	}
}

func TestDepthProfileDecisionUsesMedian(t *testing.T) {
	dir := t.TempDir()
	engine := writeTestExecutable(t, dir, "engine")
	fastchess := writeTestExecutable(t, dir, "fastchess")
	openings := filepath.Join(dir, "book.pgn")
	if err := os.WriteFile(openings, []byte("book"), 0o644); err != nil {
		t.Fatal(err)
	}
	trace := filepath.Join(dir, "trace.log")
	lines := ""
	for ix, depth := range []int{5, 6, 8, 9, 10} {
		context := strconv.Itoa(ix + 1)
		lines += "[Engine] [10:00:00.000001] < " + context + " > Candidate ---> info depth " + strconv.Itoa(depth) + " seldepth 12 nodes 100 time 10\n"
		lines += "[Engine] [10:00:00.000002] < " + context + " > Candidate ---> bestmove e2e4\n"
	}
	if err := os.WriteFile(trace, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := matchConfig{
		Candidate: engine, Baseline: engine, Fastchess: fastchess, Openings: openings,
		TC: "20+0.2", Games: 40, Concurrency: 8, HashMB: 128, Threads: 1,
		ProfileRole: "candidate", MinimumDepth: 9, Seed: defaultPreScanSeed,
	}
	report, err := buildDepthProfile(cfg, trace)
	if err != nil {
		t.Fatal(err)
	}
	if report.MedianDepth != 8 || report.Decision != "increase_time_control" {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func writeTestExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(name), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
