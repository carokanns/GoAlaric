package parms

import "fmt"

// SearchParameters contains search values that may be overridden at runtime.
type SearchParameters struct {
	Contempt                  int
	SearchRepetitionContempt  int
	LMRDivisorX100            int
	LMPMoveMultiplier         int
	AspirationInitialMarginCP int
	AspirationMinDepth        int
}

// Search contains the default search parameters.
var Search = SearchParameters{
	Contempt:                  0,
	SearchRepetitionContempt:  5,
	LMRDivisorX100:            225,
	LMPMoveMultiplier:         3,
	AspirationInitialMarginCP: 10,
	AspirationMinDepth:        5,
}

// SearchRegistryVersion identifies the named search-parameter interface.
const SearchRegistryVersion = 1

const searchRegistryName = "search-lmr-v1"
const searchLMPRegistryName = "search-lmp-v1"
const searchAspirationRegistryName = "search-aspiration-v1"
const searchAspirationDepthRegistryName = "search-aspiration-depth-v1"
const searchHPORegistryName = "search-hpo-v1"

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
		Default:     3,
		Min:         3,
		Max:         5,
		Step:        1,
		UsedIn:      "search/search.go:lateMovePrune",
		Description: "Move-count multiplier that starts late-move pruning.",
	},
}

var searchAspirationRegistry = [...]ParameterDescriptor{
	{
		Name:        "aspiration_initial_margin_cp",
		Default:     10,
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
		Default:     5,
		Min:         5,
		Max:         7,
		Step:        1,
		UsedIn:      "search/search.go:searchAsp",
		Description: "Minimum iterative-deepening depth that enables aspiration windows.",
	},
}

var searchHPORegistry = [...]ParameterDescriptor{
	searchRegistry[0],
	searchLMPRegistry[0],
	searchAspirationRegistry[0],
	searchAspirationDepthRegistry[0],
}

// SearchHPORegistry returns the stable combined search space used for
// multi-parameter optimization. The legacy one-parameter registries remain
// valid and keep their existing identities.
func SearchHPORegistry() []ParameterDescriptor {
	result := make([]ParameterDescriptor, len(searchHPORegistry))
	copy(result, searchHPORegistry[:])
	return result
}

// DefaultSearchHPOParameterFile returns all runtime-tunable search defaults in
// canonical registry order.
func DefaultSearchHPOParameterFile() ParameterFile {
	values := make([]ParameterValue, len(searchHPORegistry))
	for index, descriptor := range searchHPORegistry {
		values[index] = ParameterValue{Name: descriptor.Name, Value: descriptor.Default}
	}
	return ParameterFile{
		SchemaVersion: SearchRegistryVersion,
		Registry:      searchHPORegistryName,
		Parameters:    values,
	}
}

// DefaultSearchHPOParameterJSON exports the canonical search-hpo-v1 defaults.
func DefaultSearchHPOParameterJSON() ([]byte, error) {
	return MarshalParameterFile(DefaultSearchHPOParameterFile())
}

// DefaultSearchHPOParameterSHA256 returns the combined search baseline identity.
func DefaultSearchHPOParameterSHA256() (string, error) {
	return ParameterFileSHA256(DefaultSearchHPOParameterFile())
}

// LoadSearchHPOParameterFile reads and validates search-hpo-v1.
func LoadSearchHPOParameterFile(path string) (ParameterFile, string, error) {
	file, digest, err := LoadParameterFile(path)
	if err != nil {
		return ParameterFile{}, "", err
	}
	if file.Registry != searchHPORegistryName {
		return ParameterFile{}, "", fmt.Errorf("unsupported search HPO parameter registry %q", file.Registry)
	}
	return file, digest, nil
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
