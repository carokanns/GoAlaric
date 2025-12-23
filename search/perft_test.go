// Perft tests from perft_tests.txt.
package search

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"goalaric/board"
)

type perftCase struct {
	fen   string
	depth int
	count uint64
}

func perftMaxDepth(t *testing.T) int {
	const def = 4
	val := strings.TrimSpace(os.Getenv("GOALARIC_PERFT_MAX_DEPTH"))
	if val == "" {
		return def
	}
	parsed, err := strconv.Atoi(val)
	if err != nil || parsed < 0 {
		t.Fatalf("invalid GOALARIC_PERFT_MAX_DEPTH: %q", val)
	}
	return parsed
}

func findPerftFile() (string, error) {
	candidates := []string{
		"perft_tests.txt",
		filepath.Join("..", "perft_tests.txt"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", errors.New("perft_tests.txt not found")
}

func loadPerftCases(t *testing.T) []perftCase {
	path, err := findPerftFile()
	if err != nil {
		t.Fatal(err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var cases []perftCase
	var currentFENs []string
	seenCounts := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 4 && strings.Contains(fields[0], "/") {
			if seenCounts {
				currentFENs = nil
				seenCounts = false
			}
			currentFENs = append(currentFENs, line)
			continue
		}

		if len(fields) >= 2 {
			depth, err := strconv.Atoi(fields[0])
			if err == nil {
				countStr := strings.ReplaceAll(fields[1], ",", "")
				count, err := strconv.ParseUint(countStr, 10, 64)
				if err == nil && len(currentFENs) > 0 {
					for _, fen := range currentFENs {
						cases = append(cases, perftCase{
							fen:   fen,
							depth: depth,
							count: count,
						})
					}
					seenCounts = true
					continue
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scan perft_tests.txt: %v", err)
	}

	if len(cases) == 0 {
		t.Fatal("no perft cases parsed")
	}

	return cases
}

func TestPerft(t *testing.T) {
	cases := loadPerftCases(t)
	maxDepth := perftMaxDepth(t)

	for _, tc := range cases {
		if maxDepth > 0 && tc.depth > maxDepth {
			continue
		}
		var bd board.Board
		board.SetFen(tc.fen, &bd)
		got := perft(tc.depth, &bd)
		if got != tc.count {
			t.Fatalf("perft mismatch depth %d: got %d want %d fen %q", tc.depth, got, tc.count, tc.fen)
		}
	}
}

func BenchmarkPerftStartposDepth3(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var bd board.Board
		board.SetFen(board.StartFen, &bd)
		_ = perft(3, &bd)
	}
}
