package search

import "testing"

func TestDefaultSyzygyPathConfigured(t *testing.T) {
	if Engine.SyzygyPath != DefaultSyzygyPath {
		t.Fatalf("default SyzygyPath = %q, want %q", Engine.SyzygyPath, DefaultSyzygyPath)
	}
}

func TestSetSyzygyPathRejectsActiveSearch(t *testing.T) {
	oldStatus := SearchStatus()
	oldPath := Engine.SyzygyPath
	t.Cleanup(func() {
		setSearchStatus(oldStatus)
		Engine.SyzygyPath = oldPath
	})

	setSearchStatus(Running)
	if _, err := SetSyzygyPath(""); err == nil {
		t.Fatal("active search accepted SyzygyPath reconfiguration")
	}
	if Engine.SyzygyPath != oldPath {
		t.Fatalf("rejected reconfiguration changed path from %q to %q", oldPath, Engine.SyzygyPath)
	}
}
