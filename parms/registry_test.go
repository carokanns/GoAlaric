package parms

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPilotRegistryIsStableAndComplete(t *testing.T) {
	registry := Registry()
	if len(registry) != 8 {
		t.Fatalf("pilot registry has %d parameters, want 8", len(registry))
	}

	want := []struct {
		name, usedIn                 string
		index, value, min, max, step int
	}{
		{"mobility_weight", "eval/eval.go:mobilityScore", 31, 18, 0, 64, 1},
		{"mobility_shift", "eval/eval.go:mobilityScore -> mulShift", 32, 9, 1, 16, 1},
		{"activity_bias", "eval/eval.go:attackMgScore", 33, 5, 0, 32, 1},
		{"activity_shift", "eval/eval.go:attackMgScore", 34, 1, 1, 8, 1},
		{"activity_knight_weight", "eval/eval.go:attackWeight -> attackMgScore (knight)", 35, 1, 0, 16, 1},
		{"activity_bishop_weight", "eval/eval.go:attackWeight -> attackMgScore (bishop)", 36, 3, 0, 16, 1},
		{"activity_rook_weight", "eval/eval.go:attackWeight -> attackMgScore (rook)", 37, 5, 0, 16, 1},
		{"activity_queen_weight", "eval/eval.go:attackWeight -> attackMgScore (queen)", 38, 2, 0, 16, 1},
	}
	for index, want := range want {
		got := registry[index]
		if Parms[got.Index] != got.Default {
			t.Errorf("registry[%d] default=%d but Parms[%d]=%d", index, got.Default, got.Index, Parms[got.Index])
		}
		if got.Name != want.name || got.UsedIn != want.usedIn || got.Index != want.index ||
			got.Default != want.value || got.Min != want.min || got.Max != want.max || got.Step != want.step {
			t.Errorf("registry[%d] = %+v, want name=%q index=%d default=%d range=[%d,%d] step=%d usedIn=%q", index, got, want.name, want.index, want.value, want.min, want.max, want.step, want.usedIn)
		}
	}
}

func TestDefaultParameterFileRoundTripsCanonically(t *testing.T) {
	want, err := DefaultParameterJSON()
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseParameterFile(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := MarshalParameterFile(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("canonical parameter JSON changed after round trip:\n%s\nwant:\n%s", got, want)
	}

	var exported bytes.Buffer
	if err := ExportDefault(&exported); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(exported.Bytes(), want) {
		t.Fatal("ExportDefault differs from DefaultParameterJSON")
	}
}

func TestParameterFileRejectsOutOfRangeValue(t *testing.T) {
	file := DefaultParameterFile()
	file.Parameters[0].Value = 65
	if _, err := MarshalParameterFile(file); err == nil {
		t.Fatal("out-of-range parameter was accepted")
	}
}

func TestCheckedInDefaultParameterFileMatchesExporter(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(source), "..", "optimizer", "registries", "eval-pilot-v1-default.json")
	checkedIn, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := DefaultParameterJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(checkedIn, exported) {
		t.Fatalf("checked-in standard file differs from exporter:\n%s\nwant:\n%s", checkedIn, exported)
	}
}

func TestSearchRegistryIsStableAndComplete(t *testing.T) {
	registry := SearchRegistry()
	if len(registry) != 1 {
		t.Fatalf("search registry has %d parameters, want 1", len(registry))
	}
	got := registry[0]
	if got.Name != "lmr_divisor_x100" || got.Default != 225 || got.Min != 125 || got.Max != 400 || got.Step != 5 {
		t.Fatalf("search registry descriptor = %+v", got)
	}
	if Search.LMRDivisorX100 != got.Default {
		t.Fatalf("search default=%d, want %d", Search.LMRDivisorX100, got.Default)
	}
}

func TestSearchParameterFileRejectsInvalidRangeAndStep(t *testing.T) {
	file := DefaultSearchParameterFile()

	file.Parameters[0].Value = 124
	if _, err := MarshalParameterFile(file); err == nil {
		t.Fatal("search parameter below the allowed range was accepted")
	}

	file.Parameters[0].Value = 126
	if _, err := MarshalParameterFile(file); err == nil {
		t.Fatal("search parameter with an invalid step was accepted")
	}

	file.Parameters[0].Value = 401
	if _, err := MarshalParameterFile(file); err == nil {
		t.Fatal("search parameter above the allowed range was accepted")
	}
}

func TestCheckedInDefaultSearchParameterFileMatchesExporter(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(source), "..", "optimizer", "registries", "search-lmr-v1-default.json")
	checkedIn, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := DefaultSearchParameterJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(checkedIn, exported) {
		t.Fatalf("checked-in search file differs from exporter:\n%s\nwant:\n%s", checkedIn, exported)
	}
}

func TestSearchParameterFileLeavesEvaluationParametersUntouched(t *testing.T) {
	originalParms := Parms
	originalSearch := Search
	t.Cleanup(func() {
		Parms = originalParms
		Search = originalSearch
	})

	file := DefaultSearchParameterFile()
	file.Parameters[0].Value = 175
	if err := ApplyParameterFile(file); err != nil {
		t.Fatal(err)
	}
	if Search.LMRDivisorX100 != 175 {
		t.Fatalf("LMR divisor = %d, want 175", Search.LMRDivisorX100)
	}
	if Parms != originalParms {
		t.Fatal("search-lmr-v1 changed the legacy evaluation parameter vector")
	}
}

func TestLMPRegistryIsStableAndComplete(t *testing.T) {
	registry := LMPRegistry()
	if len(registry) != 1 {
		t.Fatalf("LMP registry has %d parameters, want 1", len(registry))
	}
	got := registry[0]
	if got.Name != "lmp_move_multiplier" || got.Default != 4 || got.Min != 3 || got.Max != 5 || got.Step != 1 {
		t.Fatalf("LMP registry descriptor = %+v", got)
	}
	if Search.LMPMoveMultiplier != got.Default {
		t.Fatalf("LMP default=%d, want %d", Search.LMPMoveMultiplier, got.Default)
	}
}

func TestLMPParameterFileRoundTripsAndApplies(t *testing.T) {
	original := Search
	t.Cleanup(func() { Search = original })

	want, err := DefaultLMPParameterJSON()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseParameterFile(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := MarshalParameterFile(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("LMP parameter JSON changed after round trip:\n%s\nwant:\n%s", got, want)
	}

	parsed.Parameters[0].Value = 3
	if err := ApplyParameterFile(parsed); err != nil {
		t.Fatal(err)
	}
	if Search.LMPMoveMultiplier != 3 {
		t.Fatalf("LMP multiplier=%d, want 3", Search.LMPMoveMultiplier)
	}
}

func TestCheckedInDefaultLMPParameterFileMatchesExporter(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(source), "..", "optimizer", "registries", "search-lmp-v1-default.json")
	checkedIn, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := DefaultLMPParameterJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(checkedIn, exported) {
		t.Fatalf("checked-in standard LMP file differs from exporter:\n%s\nwant:\n%s", checkedIn, exported)
	}
}

func TestLMPParameterFileRejectsInvalidRangeAndStep(t *testing.T) {
	file := DefaultLMPParameterFile()

	file.Parameters[0].Value = 2
	if _, err := MarshalParameterFile(file); err == nil {
		t.Fatal("LMP parameter below the allowed range was accepted")
	}

	file.Parameters[0].Value = 6
	if _, err := MarshalParameterFile(file); err == nil {
		t.Fatal("LMP parameter above the allowed range was accepted")
	}
}

func TestLMPParameterFileLeavesOtherSearchParametersUntouched(t *testing.T) {
	original := Search
	t.Cleanup(func() { Search = original })

	file := DefaultLMPParameterFile()
	file.Parameters[0].Value = 5
	if err := ApplyParameterFile(file); err != nil {
		t.Fatal(err)
	}
	if Search.LMPMoveMultiplier != 5 {
		t.Fatalf("LMP multiplier=%d, want 5", Search.LMPMoveMultiplier)
	}
	if Search.LMRDivisorX100 != original.LMRDivisorX100 || Search.Contempt != original.Contempt {
		t.Fatal("search-lmp-v1 changed unrelated search parameters")
	}
}

func TestAspirationRegistryIsStableAndComplete(t *testing.T) {
	registry := AspirationRegistry()
	if len(registry) != 1 {
		t.Fatalf("aspiration registry has %d parameters, want 1", len(registry))
	}
	got := registry[0]
	if got.Name != "aspiration_initial_margin_cp" || got.Default != 10 || got.Min != 5 || got.Max != 15 || got.Step != 5 {
		t.Fatalf("aspiration registry descriptor = %+v", got)
	}
	if Search.AspirationInitialMarginCP != got.Default {
		t.Fatalf("aspiration default=%d, want %d", Search.AspirationInitialMarginCP, got.Default)
	}
}

func TestAspirationParameterFileRoundTripsAndApplies(t *testing.T) {
	original := Search
	t.Cleanup(func() { Search = original })

	want, err := DefaultAspirationParameterJSON()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseParameterFile(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := MarshalParameterFile(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("aspiration parameter JSON changed after round trip:\n%s\nwant:\n%s", got, want)
	}

	parsed.Parameters[0].Value = 15
	if err := ApplyParameterFile(parsed); err != nil {
		t.Fatal(err)
	}
	if Search.AspirationInitialMarginCP != 15 {
		t.Fatalf("aspiration initial margin=%d, want 15", Search.AspirationInitialMarginCP)
	}
	if Search.LMRDivisorX100 != original.LMRDivisorX100 || Search.LMPMoveMultiplier != original.LMPMoveMultiplier {
		t.Fatal("search-aspiration-v1 changed unrelated search parameters")
	}
}

func TestAspirationParameterFileRejectsInvalidRangeAndStep(t *testing.T) {
	file := DefaultAspirationParameterFile()

	file.Parameters[0].Value = 4
	if _, err := MarshalParameterFile(file); err == nil {
		t.Fatal("aspiration parameter below the allowed range was accepted")
	}

	file.Parameters[0].Value = 6
	if _, err := MarshalParameterFile(file); err == nil {
		t.Fatal("aspiration parameter with an invalid step was accepted")
	}

	file.Parameters[0].Value = 16
	if _, err := MarshalParameterFile(file); err == nil {
		t.Fatal("aspiration parameter above the allowed range was accepted")
	}
}

func TestCheckedInDefaultAspirationParameterFileMatchesExporter(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(source), "..", "optimizer", "registries", "search-aspiration-v1-default.json")
	checkedIn, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := DefaultAspirationParameterJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(checkedIn, exported) {
		t.Fatalf("checked-in standard aspiration file differs from exporter:\n%s\nwant:\n%s", checkedIn, exported)
	}
}
