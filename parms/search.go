package parms

import "fmt"

// SearchParameters contains search values that may be overridden at runtime.
type SearchParameters struct {
	Contempt                  int
	SearchRepetitionContempt  int
	LMRDivisorX100            int
	LMPMoveMultiplier         int
	LMPDepth4MinIteration     int
	AspirationInitialMarginCP int
	AspirationMinDepth        int
}

// Search contains the default search parameters.
var Search = SearchParameters{
	Contempt:                  0,
	SearchRepetitionContempt:  5,
	LMRDivisorX100:            225,
	LMPMoveMultiplier:         4,
	LMPDepth4MinIteration:     0,
	AspirationInitialMarginCP: 15,
	AspirationMinDepth:        6,
}

// SearchRegistryVersion identifies the named search-parameter interface.
const SearchRegistryVersion = 1

const searchRegistryName = "search-lmr-v1"
const searchLMPRegistryName = "search-lmp-v1"
const searchLMPDepthRegistryName = "search-lmp-depth-v1"
const searchAspirationRegistryName = "search-aspiration-v1"
const searchAspirationDepthRegistryName = "search-aspiration-depth-v1"

var searchRegistry = [...]ParameterDescriptor{
	{
		Name:        "lmr_divisor_x100",
		Default:     225,
		Min:         125,
		Max:         400,
		Step:        5,
		UsedIn:      "search/search.go:initLMRReductions",
		Description: "LMR divisor multiplied by 100; lower values reduce more aggressively.",
	},
}

var searchLMPRegistry = [...]ParameterDescriptor{
	{
		Name:        "lmp_move_multiplier",
		Default:     4,
		Min:         3,
		Max:         5,
		Step:        1,
		UsedIn:      "search/search.go:lateMovePrune",
		Description: "Move-count multiplier that starts late-move pruning.",
	},
}

var searchLMPDepthRegistry = [...]ParameterDescriptor{
	{
		Name:        "lmp_depth4_min_iteration",
		Default:     0,
		Min:         0,
		Max:         20,
		Step:        2,
		UsedIn:      "search/search.go:lateMovePrune",
		Description: "Minimum iterative-deepening depth that enables LMP at remaining depth 4; zero disables it.",
	},
}

var searchAspirationRegistry = [...]ParameterDescriptor{
	{
		Name:        "aspiration_initial_margin_cp",
		Default:     15,
		Min:         5,
		Max:         15,
		Step:        5,
		UsedIn:      "search/search.go:searchAsp",
		Description: "Initial aspiration-window margin in centipawns.",
	},
}

var searchAspirationDepthRegistry = [...]ParameterDescriptor{
	{
		Name:        "aspiration_min_depth",
		Default:     6,
		Min:         5,
		Max:         7,
		Step:        1,
		UsedIn:      "search/search.go:searchAsp",
		Description: "Minimum iterative-deepening depth that enables aspiration windows.",
	},
}

// SearchRegistry returns a copy of the stable search-parameter descriptors.
func SearchRegistry() []ParameterDescriptor {
	result := make([]ParameterDescriptor, len(searchRegistry))
	copy(result, searchRegistry[:])
	return result
}

// DefaultSearchParameterFile returns the engine-default search parameter set.
func DefaultSearchParameterFile() ParameterFile {
	return ParameterFile{
		SchemaVersion: SearchRegistryVersion,
		Registry:      searchRegistryName,
		Parameters: []ParameterValue{{
			Name:  searchRegistry[0].Name,
			Value: searchRegistry[0].Default,
		}},
	}
}

// DefaultSearchParameterJSON exports the canonical search-lmr-v1 defaults.
func DefaultSearchParameterJSON() ([]byte, error) {
	return MarshalParameterFile(DefaultSearchParameterFile())
}

// DefaultSearchParameterSHA256 returns the identity of the built-in search
// parameter set.
func DefaultSearchParameterSHA256() (string, error) {
	return ParameterFileSHA256(DefaultSearchParameterFile())
}

// LoadSearchParameterFile reads and validates a search-lmr-v1 parameter file.
func LoadSearchParameterFile(path string) (ParameterFile, string, error) {
	file, digest, err := LoadParameterFile(path)
	if err != nil {
		return ParameterFile{}, "", err
	}
	if file.Registry != searchRegistryName {
		return ParameterFile{}, "", fmt.Errorf("unsupported search parameter registry %q", file.Registry)
	}
	return file, digest, nil
}

// LMPRegistry returns a copy of the stable late-move-pruning descriptors.
func LMPRegistry() []ParameterDescriptor {
	result := make([]ParameterDescriptor, len(searchLMPRegistry))
	copy(result, searchLMPRegistry[:])
	return result
}

// DefaultLMPParameterFile returns the engine-default LMP parameter set.
func DefaultLMPParameterFile() ParameterFile {
	return ParameterFile{
		SchemaVersion: SearchRegistryVersion,
		Registry:      searchLMPRegistryName,
		Parameters: []ParameterValue{{
			Name:  searchLMPRegistry[0].Name,
			Value: searchLMPRegistry[0].Default,
		}},
	}
}

// DefaultLMPParameterJSON exports the canonical search-lmp-v1 defaults.
func DefaultLMPParameterJSON() ([]byte, error) {
	return MarshalParameterFile(DefaultLMPParameterFile())
}

// DefaultLMPParameterSHA256 returns the identity of the built-in LMP set.
func DefaultLMPParameterSHA256() (string, error) {
	return ParameterFileSHA256(DefaultLMPParameterFile())
}

// LoadLMPParameterFile reads and validates a search-lmp-v1 parameter file.
func LoadLMPParameterFile(path string) (ParameterFile, string, error) {
	file, digest, err := LoadParameterFile(path)
	if err != nil {
		return ParameterFile{}, "", err
	}
	if file.Registry != searchLMPRegistryName {
		return ParameterFile{}, "", fmt.Errorf("unsupported LMP parameter registry %q", file.Registry)
	}
	return file, digest, nil
}

// LMPDepthRegistry returns a copy of the dynamic late-move-pruning depth
// descriptors.
func LMPDepthRegistry() []ParameterDescriptor {
	result := make([]ParameterDescriptor, len(searchLMPDepthRegistry))
	copy(result, searchLMPDepthRegistry[:])
	return result
}

// DefaultLMPDepthParameterFile returns the engine-default dynamic LMP set.
// A zero activation depth preserves the historical depth-3 cap exactly.
func DefaultLMPDepthParameterFile() ParameterFile {
	return ParameterFile{
		SchemaVersion: SearchRegistryVersion,
		Registry:      searchLMPDepthRegistryName,
		Parameters: []ParameterValue{{
			Name:  searchLMPDepthRegistry[0].Name,
			Value: searchLMPDepthRegistry[0].Default,
		}},
	}
}

// DefaultLMPDepthParameterJSON exports the canonical dynamic LMP defaults.
func DefaultLMPDepthParameterJSON() ([]byte, error) {
	return MarshalParameterFile(DefaultLMPDepthParameterFile())
}

// DefaultLMPDepthParameterSHA256 returns the identity of the built-in dynamic
// LMP set.
func DefaultLMPDepthParameterSHA256() (string, error) {
	return ParameterFileSHA256(DefaultLMPDepthParameterFile())
}

// LoadLMPDepthParameterFile reads and validates search-lmp-depth-v1.
func LoadLMPDepthParameterFile(path string) (ParameterFile, string, error) {
	file, digest, err := LoadParameterFile(path)
	if err != nil {
		return ParameterFile{}, "", err
	}
	if file.Registry != searchLMPDepthRegistryName {
		return ParameterFile{}, "", fmt.Errorf("unsupported LMP-depth parameter registry %q", file.Registry)
	}
	return file, digest, nil
}

// AspirationRegistry returns a copy of the stable aspiration-window descriptors.
func AspirationRegistry() []ParameterDescriptor {
	result := make([]ParameterDescriptor, len(searchAspirationRegistry))
	copy(result, searchAspirationRegistry[:])
	return result
}

// DefaultAspirationParameterFile returns the engine-default aspiration set.
func DefaultAspirationParameterFile() ParameterFile {
	return ParameterFile{
		SchemaVersion: SearchRegistryVersion,
		Registry:      searchAspirationRegistryName,
		Parameters: []ParameterValue{{
			Name:  searchAspirationRegistry[0].Name,
			Value: searchAspirationRegistry[0].Default,
		}},
	}
}

// DefaultAspirationParameterJSON exports the canonical aspiration defaults.
func DefaultAspirationParameterJSON() ([]byte, error) {
	return MarshalParameterFile(DefaultAspirationParameterFile())
}

// DefaultAspirationParameterSHA256 returns the identity of the built-in
// aspiration parameter set.
func DefaultAspirationParameterSHA256() (string, error) {
	return ParameterFileSHA256(DefaultAspirationParameterFile())
}

// LoadAspirationParameterFile reads and validates search-aspiration-v1.
func LoadAspirationParameterFile(path string) (ParameterFile, string, error) {
	file, digest, err := LoadParameterFile(path)
	if err != nil {
		return ParameterFile{}, "", err
	}
	if file.Registry != searchAspirationRegistryName {
		return ParameterFile{}, "", fmt.Errorf("unsupported aspiration parameter registry %q", file.Registry)
	}
	return file, digest, nil
}

// AspirationDepthRegistry returns a copy of the stable aspiration-depth
// descriptors.
func AspirationDepthRegistry() []ParameterDescriptor {
	result := make([]ParameterDescriptor, len(searchAspirationDepthRegistry))
	copy(result, searchAspirationDepthRegistry[:])
	return result
}

// DefaultAspirationDepthParameterFile returns the engine-default aspiration
// depth set.
func DefaultAspirationDepthParameterFile() ParameterFile {
	return ParameterFile{
		SchemaVersion: SearchRegistryVersion,
		Registry:      searchAspirationDepthRegistryName,
		Parameters: []ParameterValue{{
			Name:  searchAspirationDepthRegistry[0].Name,
			Value: searchAspirationDepthRegistry[0].Default,
		}},
	}
}

// DefaultAspirationDepthParameterJSON exports the canonical aspiration-depth
// defaults.
func DefaultAspirationDepthParameterJSON() ([]byte, error) {
	return MarshalParameterFile(DefaultAspirationDepthParameterFile())
}

// DefaultAspirationDepthParameterSHA256 returns the identity of the built-in
// aspiration-depth parameter set.
func DefaultAspirationDepthParameterSHA256() (string, error) {
	return ParameterFileSHA256(DefaultAspirationDepthParameterFile())
}

// LoadAspirationDepthParameterFile reads and validates
// search-aspiration-depth-v1.
func LoadAspirationDepthParameterFile(path string) (ParameterFile, string, error) {
	file, digest, err := LoadParameterFile(path)
	if err != nil {
		return ParameterFile{}, "", err
	}
	if file.Registry != searchAspirationDepthRegistryName {
		return ParameterFile{}, "", fmt.Errorf("unsupported aspiration-depth parameter registry %q", file.Registry)
	}
	return file, digest, nil
}
