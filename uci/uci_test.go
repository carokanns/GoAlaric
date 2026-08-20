// uci_test.go
package uci

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goalaric/search"
)

var chSearch = make(chan int)

func TestSetoption(t *testing.T) {
	HandleInput("setoption name Hash value 256", &chSearch)
	if search.Engine.Hash != 256 {
		t.Errorf("Hash borde vara %v men är %v", 256, search.Engine.Hash)
	}
}

func TestUCIAdvertisesDefaultSyzygyPath(t *testing.T) {
	oldTellGUI := tellGUI
	defer func() { tellGUI = oldTellGUI }()

	var lines []string
	tellGUI = func(line string) { lines = append(lines, line) }
	HandleInput("uci", &chSearch)

	for _, line := range lines {
		if line == "option name SyzygyPath type string default .tools/syzygy/3-4" {
			return
		}
	}
	t.Fatalf("uci output did not advertise default SyzygyPath: %v", lines)
}

func TestSetSyzygyPathPreservesPathAndSupportsOff(t *testing.T) {
	oldPath := search.Engine.SyzygyPath
	defer func() { _, _ = search.SetSyzygyPath(oldPath) }()

	HandleInput("setoption name SyzygyPath value /tmp/Syzygy/3-4", &chSearch)
	if got, want := search.Engine.SyzygyPath, "/tmp/Syzygy/3-4"; got != want {
		t.Fatalf("SyzygyPath = %q, want %q", got, want)
	}

	HandleInput("setoption name SyzygyPath value off", &chSearch)
	if search.Engine.SyzygyPath != "" {
		t.Fatalf("SyzygyPath = %q after off, want empty", search.Engine.SyzygyPath)
	}
}

func TestTablebaseStatsFileWritesOneGameBoundary(t *testing.T) {
	oldFile := tablebaseStatsFile
	oldActive := tablebaseGameActive
	defer func() {
		tablebaseStatsFile = oldFile
		tablebaseGameActive = oldActive
		search.ResetTablebaseGameStats()
	}()

	path := filepath.Join(t.TempDir(), "tablebase-stats.log")
	HandleInput("setoption name TablebaseStatsFile value "+path, &chSearch)
	beginTablebaseGame()
	flushTablebaseGame()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != "hits=0 root_wins=0" {
		t.Fatalf("stats line = %q", got)
	}
}

func Test_GoCommand(t *testing.T) {

	HandleGo("go infinite test", &chSearch)
	if !search.Infinite {
		t.Errorf("Infinite borde vara satt till true men är false")
	}
}
