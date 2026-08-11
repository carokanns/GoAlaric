package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const tablebaseStatsSchemaVersion = 2

type tablebaseGameStats struct {
	Sequence int   `json:"sequence"`
	Hits     int64 `json:"tbhits"`
	RootWins int   `json:"root_tb_wins"`
}

type tablebaseEngineStats struct {
	TotalHits     int64 `json:"total_hits"`
	GamesWithHits int   `json:"games_with_hits"`
	RootWinGames  int   `json:"root_tb_win_games"`
	RootWinProbes int   `json:"root_tb_win_probes"`
}

type tablebaseStatsReport struct {
	SchemaVersion int                  `json:"schema_version"`
	CreatedAt     time.Time            `json:"created_at"`
	Games         []tablebaseGameStats `json:"games"`
	Candidate     tablebaseEngineStats `json:"candidate"`
	Definition    string               `json:"definition"`
	StatsPath     string               `json:"stats_path"`
}

// buildTablebaseStats reads one compact line written by the candidate engine
// at each UCI-game boundary. This avoids Fastchess engine trace logging.
func buildTablebaseStats(path string) (tablebaseStatsReport, error) {
	file, err := os.Open(path)
	if err != nil {
		return tablebaseStatsReport{}, err
	}
	defer file.Close()

	report := tablebaseStatsReport{
		SchemaVersion: tablebaseStatsSchemaVersion,
		CreatedAt:     time.Now(),
		Definition:    "tbhits is the sum of successful Syzygy probes in each completed game. A root_tb_win is an exact root WDL=win result; one or more such probes in a game count as one tablebase-won game.",
		StatsPath:     path,
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		hits, hitsOK := fieldInt64(fields, "hits")
		rootWins, rootOK := fieldInt(fields, "root_wins")
		if !hitsOK || !rootOK || hits < 0 || rootWins < 0 {
			return tablebaseStatsReport{}, fmt.Errorf("invalid tablebase game stats line %q", scanner.Text())
		}
		game := tablebaseGameStats{Sequence: len(report.Games) + 1, Hits: hits, RootWins: rootWins}
		report.Games = append(report.Games, game)
		report.Candidate.TotalHits += hits
		if hits > 0 {
			report.Candidate.GamesWithHits++
		}
		if rootWins > 0 {
			report.Candidate.RootWinGames++
			report.Candidate.RootWinProbes += rootWins
		}
	}
	if err := scanner.Err(); err != nil {
		return tablebaseStatsReport{}, err
	}
	if len(report.Games) == 0 {
		return tablebaseStatsReport{}, fmt.Errorf("tablebase game stats contains no completed games")
	}
	return report, nil
}

func fieldInt64(fields []string, name string) (int64, bool) {
	for i := 0; i < len(fields); i++ {
		key, value, found := strings.Cut(fields[i], "=")
		if found && key == name {
			parsed, err := strconv.ParseInt(value, 10, 64)
			return parsed, err == nil
		}
	}
	return 0, false
}

func fieldInt(fields []string, name string) (int, bool) {
	value, ok := fieldInt64(fields, name)
	if !ok || value > int64(^uint(0)>>1) {
		return 0, false
	}
	return int(value), true
}
