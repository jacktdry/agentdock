//go:build !darwin

package app

type nextPermissionOwner struct{}

func acquireNextPermissionOwner(string) (*nextPermissionOwner, error) {
	return nil, errNextCodexRegistryUnavailable
}
func (*nextPermissionOwner) close() error { return nil }
