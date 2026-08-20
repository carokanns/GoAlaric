// uci_test.go
package uci

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"goalaric/eval"
	"goalaric/parms"
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

func TestSetoptionParameterFileLoadsAndReportsIdentity(t *testing.T) {
	originalParms := parms.Parms
	originalTellGUI := tellGUI
	originalPath := parameterFilePath
	originalSHA := parameterFileSHA
	t.Cleanup(func() {
		parms.Parms = originalParms
		eval.Update()
		parameterFilePath = originalPath
		parameterFileSHA = originalSHA
		tellGUI = originalTellGUI
	})

	data, err := parms.DefaultParameterJSON()
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/parameters.json"
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	wantSHA, err := parms.DefaultParameterSHA256()
	if err != nil {
		t.Fatal(err)
	}

	var output []string
	tellGUI = func(line string) { output = append(output, line) }
	HandleInput("setoption name ParameterFile value "+path, &chSearch)

	if parameterFilePath != path || parameterFileSHA != wantSHA {
		t.Fatalf("parameter state path=%q sha=%q, want path=%q sha=%q", parameterFilePath, parameterFileSHA, path, wantSHA)
	}
	joined := strings.Join(output, "\n")
	if !strings.Contains(joined, fmt.Sprintf("registry=%d", parms.RegistryVersion)) || !strings.Contains(joined, "sha256="+wantSHA) {
		t.Fatalf("load report = %q", joined)
	}
}

func TestSetoptionSearchParameterFileRefreshesLMR(t *testing.T) {
	originalSearch := parms.Search
	originalTellGUI := tellGUI
	originalPath := parameterFilePath
	originalSHA := parameterFileSHA
	t.Cleanup(func() {
		parms.Search = originalSearch
		search.RefreshRuntimeParameters()
		parameterFilePath = originalPath
		parameterFileSHA = originalSHA
		tellGUI = originalTellGUI
	})

	parms.Search.LMRDivisorX100 = 225
	search.RefreshRuntimeParameters()
	baseline := search.LMRReduction(12, 30)
	file := parms.DefaultSearchParameterFile()
	file.Parameters[0].Value = 175
	data, err := parms.MarshalParameterFile(file)
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/search-lmr.json"
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	var output []string
	tellGUI = func(line string) { output = append(output, line) }
	HandleInput("setoption name ParameterFile value "+path, &chSearch)

	if parms.Search.LMRDivisorX100 != 175 {
		t.Fatalf("LMR divisor = %d, want 175", parms.Search.LMRDivisorX100)
	}
	if got := search.LMRReduction(12, 30); got <= baseline {
		t.Fatalf("refreshed LMR reduction = %d, want greater than %d", got, baseline)
	}
	joined := strings.Join(output, "\n")
	if !strings.Contains(joined, "registry_name=search-lmr-v1") {
		t.Fatalf("search registry was not reported: %q", joined)
	}
}

func TestSetoptionLMPParameterFileChangesMultiplier(t *testing.T) {
	originalSearch := parms.Search
	originalTellGUI := tellGUI
	originalPath := parameterFilePath
	originalSHA := parameterFileSHA
	t.Cleanup(func() {
		parms.Search = originalSearch
		search.RefreshRuntimeParameters()
		parameterFilePath = originalPath
		parameterFileSHA = originalSHA
		tellGUI = originalTellGUI
	})

	file := parms.DefaultLMPParameterFile()
	file.Parameters[0].Value = 3
	data, err := parms.MarshalParameterFile(file)
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/search-lmp.json"
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	var output []string
	tellGUI = func(line string) { output = append(output, line) }
	HandleInput("setoption name ParameterFile value "+path, &chSearch)

	if parms.Search.LMPMoveMultiplier != 3 {
		t.Fatalf("LMP multiplier=%d, want 3", parms.Search.LMPMoveMultiplier)
	}
	if !strings.Contains(strings.Join(output, "\n"), "registry_name=search-lmp-v1") {
		t.Fatalf("LMP registry was not reported: %q", output)
	}
}

func TestSetoptionAspirationParameterFileChangesInitialMargin(t *testing.T) {
	originalSearch := parms.Search
	originalPath := parameterFilePath
	originalSHA := parameterFileSHA
	t.Cleanup(func() {
		parms.Search = originalSearch
		parameterFilePath = originalPath
		parameterFileSHA = originalSHA
	})

	file := parms.DefaultAspirationParameterFile()
	file.Parameters[0].Value = 15
	data, err := parms.MarshalParameterFile(file)
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/search-aspiration.json"
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	var output []string
	originalTellGUI := tellGUI
	t.Cleanup(func() { tellGUI = originalTellGUI })
	tellGUI = func(line string) { output = append(output, line) }
	HandleInput("setoption name ParameterFile value "+path, &chSearch)

	if parms.Search.AspirationInitialMarginCP != 15 {
		t.Fatalf("aspiration initial margin=%d, want 15", parms.Search.AspirationInitialMarginCP)
	}
	if !strings.Contains(strings.Join(output, "\n"), "registry_name=search-aspiration-v1") {
		t.Fatalf("aspiration registry was not reported: %q", output)
	}
}

func TestSetoptionAspirationDepthParameterFileChangesMinimumDepth(t *testing.T) {
	originalSearch := parms.Search
	originalPath := parameterFilePath
	originalSHA := parameterFileSHA
	t.Cleanup(func() {
		parms.Search = originalSearch
		parameterFilePath = originalPath
		parameterFileSHA = originalSHA
	})

	file := parms.DefaultAspirationDepthParameterFile()
	file.Parameters[0].Value = 7
	data, err := parms.MarshalParameterFile(file)
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/search-aspiration-depth.json"
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	var output []string
	originalTellGUI := tellGUI
	t.Cleanup(func() { tellGUI = originalTellGUI })
	tellGUI = func(line string) { output = append(output, line) }
	HandleInput("setoption name ParameterFile value "+path, &chSearch)

	if parms.Search.AspirationMinDepth != 7 {
		t.Fatalf("aspiration minimum depth=%d, want 7", parms.Search.AspirationMinDepth)
	}
	if !strings.Contains(strings.Join(output, "\n"), "registry_name=search-aspiration-depth-v1") {
		t.Fatalf("aspiration-depth registry was not reported: %q", output)
	}
}

func TestSetoptionParameterFileRejectsInvalidFile(t *testing.T) {
	original := parms.Parms
	originalTellGUI := tellGUI
	t.Cleanup(func() {
		parms.Parms = original
		eval.Update()
		tellGUI = originalTellGUI
	})

	path := t.TempDir() + "/invalid.json"
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"registry":"eval-pilot-v1","parameters":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var output string
	tellGUI = func(line string) { output = line }
	HandleInput("setoption name ParameterFile value "+path, &chSearch)
	if parms.Parms != original {
		t.Fatal("invalid parameter file changed parameters")
	}
	if !strings.Contains(output, "ParameterFile rejected") {
		t.Fatalf("invalid-file report = %q", output)
	}
}

func TestSetoptionParameterFileRejectsActiveSearch(t *testing.T) {
	originalParms := parms.Parms
	originalTellGUI := tellGUI
	t.Cleanup(func() {
		search.SetStop(true)
		parms.Parms = originalParms
		eval.Update()
		tellGUI = originalTellGUI
		search.SetInfinite(false)
	})

	path := t.TempDir() + "/parameters.json"
	data, err := parms.DefaultParameterJSON()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	search.SetInfinite(true)
	search.SetStop(false)
	SetPosition("position startpos")
	searchType := make(chan int)
	bestmove := make(chan string, 1)
	go search.StartSearch(searchType, bestmove, &Bd)
	searchType <- search.Simple
	deadline := time.Now().Add(2 * time.Second)
	for search.SearchStatus() != search.Running && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if search.SearchStatus() != search.Running {
		t.Fatal("search did not become active")
	}

	var output string
	tellGUI = func(line string) { output = line }
	before := parms.Parms
	HandleInput("setoption name ParameterFile value "+path, &chSearch)
	if parms.Parms != before {
		t.Fatal("active search accepted a parameter change")
	}
	if !strings.Contains(output, "ParameterFile rejected: search active") {
		t.Fatalf("active-search report = %q", output)
	}

	search.SetStop(true)
	select {
	case <-bestmove:
	case <-time.After(2 * time.Second):
		t.Fatal("search did not stop")
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
