package main

import (
	"fmt"
	"strings"

	"goalaric/parms"
)

// identifyExperimentInstance combines the executable identity with the
// canonical parameter-file identity used by that engine instance.
func identifyExperimentInstance(binary, parameterFile string) (experimentIdentity, error) {
	identity, err := identifyExperimentBinary(binary)
	if err != nil {
		return experimentIdentity{}, err
	}
	sha, version, err := identifyParameterFile(parameterFile)
	if err != nil {
		return experimentIdentity{}, err
	}
	identity.ParameterSHA256 = sha
	identity.ParameterRegisterVersion = version
	return identity, nil
}

func identifyParameterFile(path string) (string, int, error) {
	if strings.TrimSpace(path) == "" {
		sha, err := parms.DefaultParameterSHA256()
		if err != nil {
			return "", 0, fmt.Errorf("identify built-in parameter defaults: %w", err)
		}
		return sha, parms.RegistryVersion, nil
	}
	_, sha, err := parms.LoadParameterFile(path)
	if err != nil {
		return "", 0, fmt.Errorf("load parameter file %q: %w", path, err)
	}
	return sha, parms.RegistryVersion, nil
}
