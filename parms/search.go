package parms

import "fmt"

// SearchParameters contains search values that may be overridden at runtime.
type SearchParameters struct {
	Contempt       int
	LMRDivisorX100 int
}

// Search contains the default search parameters.
var Search = SearchParameters{
	Contempt:       5,
	LMRDivisorX100: 225,
}

// SearchRegistryVersion identifies the named search-parameter interface.
const SearchRegistryVersion = 1

const searchRegistryName = "search-lmr-v1"

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
