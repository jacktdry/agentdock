//go:build !darwin

package app

type nextCodexStartupLock struct{}

func acquireNextCodexStartupLock(string) (*nextCodexStartupLock, error) {
	return nil, errNextCodexRegistryUnavailable
}
func (*nextCodexStartupLock) validate() error { return errNextCodexRegistryUnavailable }
func (*nextCodexStartupLock) close() error    { return nil }
