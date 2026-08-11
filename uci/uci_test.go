// uci_test.go
package uci

import (
	"os"
	"testing"

	"goalaric/search"
	"goalaric/syzygy"
)

var chSearch = make(chan int)

func TestSetoption(t *testing.T) {
	HandleInput("setoption name Hash value 256", &chSearch)
	if search.Engine.Hash != 256 {
		t.Errorf("Hash borde vara %v men är %v", 256, search.Engine.Hash)
	}
}

func TestSetoptionSyzygyWithTables(t *testing.T) {
	path := os.Getenv("GOALARIC_SYZYGY_PATH")
	if path == "" {
		t.Skip("GOALARIC_SYZYGY_PATH is not set")
	}
	t.Cleanup(func() {
		_ = syzygy.SetPath("")
		search.Engine.SyzygyPath = ""
	})
	HandleInput("setoption name SyzygyPath value "+path, &chSearch)
	if search.Engine.SyzygyPath != path || !syzygy.Enabled() || syzygy.Largest() < 3 {
		t.Fatalf("path=%q enabled=%v largest=%d", search.Engine.SyzygyPath, syzygy.Enabled(), syzygy.Largest())
	}
}

func TestSetoptionContempt(t *testing.T) {
	original := search.Engine.Contempt
	t.Cleanup(func() { search.Engine.Contempt = original })

	HandleInput("setoption name Contempt value 9", &chSearch)
	if search.Engine.Contempt != 9 {
		t.Errorf("Contempt borde vara %v men är %v", 9, search.Engine.Contempt)
	}

	HandleInput("setoption name Contempt value 101", &chSearch)
	if search.Engine.Contempt != 9 {
		t.Errorf("Ogiltigt Contempt-värde borde ignoreras, fick %v", search.Engine.Contempt)
	}
}

func TestSetoptionSyzygy(t *testing.T) {
	originalPath := search.Engine.SyzygyPath
	originalDepth := search.Engine.SyzygyProbeDepth
	t.Cleanup(func() {
		_ = syzygy.SetPath(originalPath)
		search.Engine.SyzygyPath = originalPath
		search.Engine.SyzygyProbeDepth = originalDepth
	})

	HandleInput("setoption name SyzygyProbeDepth value 4", &chSearch)
	if search.Engine.SyzygyProbeDepth != 4 {
		t.Fatalf("SyzygyProbeDepth = %d, want 4", search.Engine.SyzygyProbeDepth)
	}
	HandleInput("setoption name SyzygyPath value", &chSearch)
	if search.Engine.SyzygyPath != "" || syzygy.Enabled() {
		t.Fatal("empty SyzygyPath should disable probing")
	}
}

func Test_GoCommand(t *testing.T) {

	HandleGo("go infinite test", &chSearch)
	if !search.IsInfinite() {
		t.Errorf("Infinite borde vara satt till true men är false")
	}
}
