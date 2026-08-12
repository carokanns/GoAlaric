package parms

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

// ApplyParameterFile applies the validated pilot values to the legacy Parms
// vector. Derived evaluation state is refreshed by eval.ApplyParameterFile;
// this package intentionally does not import eval.
func ApplyParameterFile(file ParameterFile) error {
	normalized, err := normalize(file)
	if err != nil {
		return err
	}
	for _, value := range normalized.Parameters {
		for _, descriptor := range pilotRegistry {
			if descriptor.Name == value.Name {
				Parms[descriptor.Index] = value.Value
				break
			}
		}
	}
	return nil
}

func normalize(file ParameterFile) (ParameterFile, error) {
	if file.SchemaVersion != RegistryVersion {
		return ParameterFile{}, fmt.Errorf("unsupported parameter schema %d", file.SchemaVersion)
	}
	if file.Registry != registryName {
		return ParameterFile{}, fmt.Errorf("unsupported parameter registry %q", file.Registry)
	}
	if len(file.Parameters) != len(pilotRegistry) {
		return ParameterFile{}, fmt.Errorf("parameter count %d, want %d", len(file.Parameters), len(pilotRegistry))
	}

	ordered := make([]ParameterValue, len(pilotRegistry))
	seen := make([]bool, len(pilotRegistry))
	for _, value := range file.Parameters {
		index := -1
		for registryIndex, descriptor := range pilotRegistry {
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
		descriptor := pilotRegistry[index]
		if value.Value < descriptor.Min || value.Value > descriptor.Max {
			return ParameterFile{}, fmt.Errorf("parameter %q=%d outside [%d,%d]", descriptor.Name, value.Value, descriptor.Min, descriptor.Max)
		}
		if descriptor.Step <= 0 || (value.Value-descriptor.Min)%descriptor.Step != 0 {
			return ParameterFile{}, fmt.Errorf("parameter %q=%d does not match step %d from %d", descriptor.Name, value.Value, descriptor.Step, descriptor.Min)
		}
		ordered[index] = ParameterValue{Name: descriptor.Name, Value: value.Value}
	}
	for index, descriptor := range pilotRegistry {
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
