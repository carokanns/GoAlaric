// uci_test.go
package uci

import (
	"fmt"
	"os"
	"slices"
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
	file.Parameters[0].Value = 30
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

	if parms.Search.AspirationInitialMarginCP != 30 {
		t.Fatalf("aspiration initial margin=%d, want 30", parms.Search.AspirationInitialMarginCP)
	}
	if !strings.Contains(strings.Join(output, "\n"), "registry_name=search-aspiration-v1") {
		t.Fatalf("aspiration registry was not reported: %q", output)
	}
}

func TestUCIAdvertisesAndSetsAspirationProfile(t *testing.T) {
	originalTellGUI := tellGUI
	originalEnabled := search.Engine.AspirationProfile
	t.Cleanup(func() {
		tellGUI = originalTellGUI
		search.Engine.AspirationProfile = originalEnabled
	})

	var lines []string
	tellGUI = func(line string) { lines = append(lines, line) }
	HandleInput("uci", &chSearch)
	if !slices.Contains(lines, "option name AspirationProfile type check default false") {
		t.Fatalf("uci output did not advertise AspirationProfile: %v", lines)
	}
	HandleInput("setoption name AspirationProfile value true", &chSearch)
	if !search.Engine.AspirationProfile {
		t.Fatal("AspirationProfile was not enabled")
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

func TestUCIAdvertisesDefaultSyzygyPath(t *testing.T) {
	originalTellGUI := tellGUI
	t.Cleanup(func() { tellGUI = originalTellGUI })

	var lines []string
	tellGUI = func(line string) { lines = append(lines, line) }
	HandleInput("uci", &chSearch)
	want := "option name SyzygyPath type string default " + search.DefaultSyzygyPath
	if !slices.Contains(lines, want) {
		t.Fatalf("uci output did not advertise default SyzygyPath: %v", lines)
	}
}

func TestUCIAdvertisesAndSetsContempt(t *testing.T) {
	originalTellGUI := tellGUI
	originalContempt := search.Engine.Contempt
	originalRepetitionContempt := search.Engine.SearchRepetitionContempt
	t.Cleanup(func() {
		tellGUI = originalTellGUI
		search.Engine.Contempt = originalContempt
		search.Engine.SearchRepetitionContempt = originalRepetitionContempt
		search.SG.Trans.Clear()
	})

	var lines []string
	tellGUI = func(line string) { lines = append(lines, line) }
	HandleInput("uci", &chSearch)
	if !slices.Contains(lines, "option name Contempt type spin default 0 min -100 max 100") {
		t.Fatalf("uci output did not advertise Contempt: %v", lines)
	}
	if !slices.Contains(lines, "option name SearchRepetitionContempt type spin default 5 min -100 max 100") {
		t.Fatalf("uci output did not advertise SearchRepetitionContempt: %v", lines)
	}

	HandleInput("setoption name SearchRepetitionContempt value 7", &chSearch)
	if search.Engine.SearchRepetitionContempt != 7 {
		t.Fatalf("SearchRepetitionContempt = %d, want 7", search.Engine.SearchRepetitionContempt)
	}
	HandleInput("setoption name Contempt value -9", &chSearch)
	if search.Engine.Contempt != -9 {
		t.Fatalf("Contempt = %d, want -9", search.Engine.Contempt)
	}
	HandleInput("setoption name Contempt value 101", &chSearch)
	if search.Engine.Contempt != -9 {
		t.Fatalf("invalid Contempt changed value to %d", search.Engine.Contempt)
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
	HandleInput("setoption name SyzygyPath value off", &chSearch)
	if search.Engine.SyzygyPath != "" || syzygy.Enabled() {
		t.Fatal("SyzygyPath=off should disable probing")
	}
}

func Test_GoCommand(t *testing.T) {

	HandleGo("go infinite test", &chSearch)
	if !search.IsInfinite() {
		t.Errorf("Infinite borde vara satt till true men är false")
	}
}

func TestGoReportsRootDrawClaimsAndStillStartsSearch(t *testing.T) {
	originalBoard := Bd
	originalTellGUI := tellGUI
	t.Cleanup(func() {
		Bd = originalBoard
		tellGUI = originalTellGUI
	})

	cycle := "g1f3 g8f6 f3g1 f6g8"
	tests := []struct {
		name     string
		position string
		want     string
	}{
		{
			name:     "second occurrence is not claimable",
			position: "position startpos moves " + cycle,
			want:     "",
		},
		{
			name:     "third occurrence",
			position: "position startpos moves " + cycle + " " + cycle,
			want:     "info string draw claim available: threefold repetition",
		},
		{
			name:     "fifty-move rule",
			position: "position fen 7k/8/8/8/8/8/8/R3K3 w - - 100 51",
			want:     "info string draw claim available: fifty-move rule",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output []string
			tellGUI = func(line string) { output = append(output, line) }
			SetPosition(test.position)
			commands := make(chan int, 1)
			HandleGo("go depth 1", &commands)
			select {
			case got := <-commands:
				if got != search.Simple {
					t.Fatalf("search command = %d, want %d", got, search.Simple)
				}
			default:
				t.Fatal("draw claim prevented the search command")
			}
			if test.want == "" {
				if len(output) != 0 {
					t.Fatalf("unexpected root draw claim: %v", output)
				}
			} else if !slices.Contains(output, test.want) {
				t.Fatalf("root draw claim = %v, want %q", output, test.want)
			}
		})
	}
}
