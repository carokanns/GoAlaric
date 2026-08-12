package eval

import "goalaric/parms"

// ApplyParameterFile applies named evaluation parameters and refreshes the
// derived evaluator weights used by search.
func ApplyParameterFile(file parms.ParameterFile) error {
	if err := parms.ApplyParameterFile(file); err != nil {
		return err
	}
	Update()
	return nil
}
