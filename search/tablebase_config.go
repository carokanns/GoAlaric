package search

import (
	"errors"

	"goalaric/syzygy"
)

// DefaultSyzygyPath is relative to the engine's working directory.
// Testmonitor resolves and supplies an absolute path for reproducible matches.
const DefaultSyzygyPath = ".tools/syzygy/3-4"

// SetSyzygyPath reloads the tablebase set. Reconfiguration is forbidden while
// a search is active because Fathom owns process-global mapped files.
func SetSyzygyPath(path string) (int, error) {
	if SearchStatus() == Running {
		return syzygy.Largest(), errors.New("SyzygyPath cannot change during an active search")
	}
	path = syzygy.NormalizePath(path)
	if err := syzygy.SetPath(path); err != nil {
		Engine.SyzygyPath = ""
		SG.Trans.Clear()
		return 0, err
	}
	Engine.SyzygyPath = path
	SG.Trans.Clear()
	return syzygy.Largest(), nil
}
