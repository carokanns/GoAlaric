package main

import (
	"os"
	"path/filepath"
	"testing"

	"goalaric/parms"
)

func TestIdentifySearchParameterFile(t *testing.T) {
	data, err := parms.DefaultSearchParameterJSON()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "search-lmr-v1.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	wantSHA, err := parms.DefaultSearchParameterSHA256()
	if err != nil {
		t.Fatal(err)
	}
	gotSHA, gotVersion, err := identifyParameterFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if gotSHA != wantSHA || gotVersion != parms.SearchRegistryVersion {
		t.Fatalf("search parameter identity sha=%q version=%d, want sha=%q version=%d", gotSHA, gotVersion, wantSHA, parms.SearchRegistryVersion)
	}
}
