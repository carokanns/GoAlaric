package parms

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// RegistryVersion identifies the named evaluation-parameter interface.
const RegistryVersion = 1

const registryName = "eval-pilot-v1"

// ParameterDescriptor describes one parameter exposed to the optimizer.
// Index is the legacy position in Parms and is deliberately kept internal to
// the registry implementation; callers should use Name as the stable key.
type ParameterDescriptor struct {
	Name        string
	Index       int
	Default     int
	Min         int
	Max         int
	Step        int
	UsedIn      string
	Description string
}

// ParameterValue is a named value in an exported parameter file.
type ParameterValue struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

// ParameterFile is the stable, named parameter-file format for the pilot
// registry. Parameters are serialized in registry order.
type ParameterFile struct {
	SchemaVersion int              `json:"schema_version"`
	Registry      string           `json:"registry"`
	Parameters    []ParameterValue `json:"parameters"`
}

var pilotRegistry = [...]ParameterDescriptor{
	{
		Name:        "mobility_weight",
		Index:       31,
		Default:     18,
		Min:         0,
		Max:         64,
		Step:        1,
		UsedIn:      "eval/eval.go:mobilityScore",
		Description: "Multiplier for the mobility term.",
	},
	{
		Name:        "mobility_shift",
		Index:       32,
		Default:     9,
		Min:         1,
		Max:         16,
		Step:        1,
		UsedIn:      "eval/eval.go:mobilityScore -> mulShift",
		Description: "Right-shift scale used by the mobility term.",
	},
	{
		Name:        "activity_bias",
		Index:       33,
		Default:     5,
		Min:         0,
		Max:         32,
		Step:        1,
		UsedIn:      "eval/eval.go:attackMgScore",
		Description: "Baseline subtracted from middlegame activity.",
	},
	{
		Name:        "activity_shift",
		Index:       34,
		Default:     1,
		Min:         1,
		Max:         8,
		Step:        1,
		UsedIn:      "eval/eval.go:attackMgScore",
		Description: "Divisor controlling middlegame activity scale.",
	},
	{
		Name:        "activity_knight_weight",
		Index:       35,
		Default:     1,
		Min:         0,
		Max:         16,
		Step:        1,
		UsedIn:      "eval/eval.go:attackWeight -> attackMgScore (knight)",
		Description: "Middlegame activity weight for knights.",
	},
	{
		Name:        "activity_bishop_weight",
		Index:       36,
		Default:     3,
		Min:         0,
		Max:         16,
		Step:        1,
		UsedIn:      "eval/eval.go:attackWeight -> attackMgScore (bishop)",
		Description: "Middlegame activity weight for bishops.",
	},
	{
		Name:        "activity_rook_weight",
		Index:       37,
		Default:     5,
		Min:         0,
		Max:         16,
		Step:        1,
		UsedIn:      "eval/eval.go:attackWeight -> attackMgScore (rook)",
		Description: "Middlegame activity weight for rooks.",
	},
	{
		Name:        "activity_queen_weight",
		Index:       38,
		Default:     2,
		Min:         0,
		Max:         16,
		Step:        1,
		UsedIn:      "eval/eval.go:attackWeight -> attackMgScore (queen)",
		Description: "Middlegame activity weight for queens.",
	},
}

// Registry returns a copy of the pilot descriptors in stable order.
func Registry() []ParameterDescriptor {
	result := make([]ParameterDescriptor, len(pilotRegistry))
	copy(result, pilotRegistry[:])
	return result
}

// DefaultParameterFile returns the immutable baseline values for the pilot.
func DefaultParameterFile() ParameterFile {
	values := make([]ParameterValue, 0, len(pilotRegistry))
	for _, descriptor := range pilotRegistry {
		values = append(values, ParameterValue{
			Name:  descriptor.Name,
			Value: descriptor.Default,
		})
	}
	return ParameterFile{
		SchemaVersion: RegistryVersion,
		Registry:      registryName,
		Parameters:    values,
	}
}

// DefaultParameterJSON exports the canonical baseline parameter file.
func DefaultParameterJSON() ([]byte, error) {
	return MarshalParameterFile(DefaultParameterFile())
}

// ParameterFileSHA256 returns the SHA-256 of the canonical parameter-file
// representation. Formatting and parameter order therefore do not affect the
// identity of an otherwise identical parameter set.
func ParameterFileSHA256(file ParameterFile) (string, error) {
	data, err := MarshalParameterFile(file)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// DefaultParameterSHA256 returns the identity of the built-in baseline set.
func DefaultParameterSHA256() (string, error) {
	return ParameterFileSHA256(DefaultParameterFile())
}

// LoadParameterFile reads, validates and identifies a parameter file.
func LoadParameterFile(path string) (ParameterFile, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ParameterFile{}, "", err
	}
	file, err := ParseParameterFile(data)
	if err != nil {
		return ParameterFile{}, "", err
	}
	digest, err := ParameterFileSHA256(file)
	if err != nil {
		return ParameterFile{}, "", err
	}
	return file, digest, nil
}

// ExportDefault writes the canonical baseline parameter file to w.
func ExportDefault(w io.Writer) error {
	data, err := DefaultParameterJSON()
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// MarshalParameterFile validates and serializes a parameter file in registry
// order, making equivalent files byte-for-byte identical.
func MarshalParameterFile(file ParameterFile) ([]byte, error) {
	normalized, err := normalize(file)
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// ParseParameterFile parses and validates a named parameter file.
func ParseParameterFile(data []byte) (ParameterFile, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var file ParameterFile
	if err := decoder.Decode(&file); err != nil {
		return ParameterFile{}, fmt.Errorf("decode parameter file: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return ParameterFile{}, fmt.Errorf("decode parameter file: trailing JSON")
		}
		return ParameterFile{}, fmt.Errorf("decode parameter file: %w", err)
	}
	return normalize(file)
}

// ApplyParameterFile applies a validated parameter file to its own registry.
// Derived evaluation and search state are refreshed by their respective
// packages; parms intentionally does not import either package.
func ApplyParameterFile(file ParameterFile) error {
	normalized, err := normalize(file)
	if err != nil {
		return err
	}
	switch normalized.Registry {
	case registryName:
		for _, value := range normalized.Parameters {
			for _, descriptor := range pilotRegistry {
				if descriptor.Name == value.Name {
					Parms[descriptor.Index] = value.Value
					break
				}
			}
		}
	case searchRegistryName:
		Search.LMRDivisorX100 = normalized.Parameters[0].Value
	case searchLMPRegistryName:
		Search.LMPMoveMultiplier = normalized.Parameters[0].Value
	case searchAspirationRegistryName:
		Search.AspirationInitialMarginCP = normalized.Parameters[0].Value
	case searchAspirationDepthRegistryName:
		Search.AspirationMinDepth = normalized.Parameters[0].Value
	}
	return nil
}

func normalize(file ParameterFile) (ParameterFile, error) {
	if file.SchemaVersion != RegistryVersion {
		return ParameterFile{}, fmt.Errorf("unsupported parameter schema %d", file.SchemaVersion)
	}
	var descriptors []ParameterDescriptor
	switch file.Registry {
	case registryName:
		descriptors = pilotRegistry[:]
	case searchRegistryName:
		descriptors = searchRegistry[:]
	case searchLMPRegistryName:
		descriptors = searchLMPRegistry[:]
	case searchAspirationRegistryName:
		descriptors = searchAspirationRegistry[:]
	case searchAspirationDepthRegistryName:
		descriptors = searchAspirationDepthRegistry[:]
	default:
		return ParameterFile{}, fmt.Errorf("unsupported parameter registry %q", file.Registry)
	}
	if len(file.Parameters) != len(descriptors) {
		return ParameterFile{}, fmt.Errorf("parameter count %d, want %d", len(file.Parameters), len(descriptors))
	}

	ordered := make([]ParameterValue, len(descriptors))
	seen := make([]bool, len(descriptors))
	for _, value := range file.Parameters {
		index := -1
		for registryIndex, descriptor := range descriptors {
			if descriptor.Name == value.Name {
				index = registryIndex
				break
			}
		}
		if index < 0 {
			return ParameterFile{}, fmt.Errorf("unknown parameter %q", value.Name)
		}
		if seen[index] {
			return ParameterFile{}, fmt.Errorf("duplicate parameter %q", value.Name)
		}
		seen[index] = true
		descriptor := descriptors[index]
		if value.Value < descriptor.Min || value.Value > descriptor.Max {
			return ParameterFile{}, fmt.Errorf("parameter %q=%d outside [%d,%d]", descriptor.Name, value.Value, descriptor.Min, descriptor.Max)
		}
		if descriptor.Step <= 0 || (value.Value-descriptor.Min)%descriptor.Step != 0 {
			return ParameterFile{}, fmt.Errorf("parameter %q=%d does not match step %d from %d", descriptor.Name, value.Value, descriptor.Step, descriptor.Min)
		}
		ordered[index] = ParameterValue{Name: descriptor.Name, Value: value.Value}
	}
	for index, descriptor := range descriptors {
		if !seen[index] {
			return ParameterFile{}, fmt.Errorf("missing parameter %q", descriptor.Name)
		}
	}

	return ParameterFile{
		SchemaVersion: file.SchemaVersion,
		Registry:      file.Registry,
		Parameters:    ordered,
	}, nil
}
