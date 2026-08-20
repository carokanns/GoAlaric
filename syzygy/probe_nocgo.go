//go:build !cgo

package syzygy

// Available reports that this build has no native Fathom prober.
func Available() bool { return false }

// Load keeps non-cgo builds functional with tablebase probing disabled.
func Load(string) (int, error) { return 0, nil }

// Largest reports that no local prober is available in non-cgo builds.
func Largest() int { return 0 }

// ProbeWDL is unavailable without cgo.
func ProbeWDL(Position) (WDL, bool) { return Draw, false }

// ProbeRoot is unavailable without cgo.
func ProbeRoot(Position) (RootResult, bool) { return RootResult{}, false }
