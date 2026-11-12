//go:build !darwin

package selfupdate

import "os"

func validateUpdateCaller(string) error        { return nil }
func validateUpdateRequest(applyRequest) error { return nil }

func updateHelperEnvironment() ([]string, error) { return os.Environ(), nil }
