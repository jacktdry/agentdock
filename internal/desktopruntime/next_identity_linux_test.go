//go:build linux

package desktopruntime

import "testing"

func nextHelperFixture(t *testing.T, root string) (string, string) {
	t.Fatal("Darwin fixture on Linux")
	return "", ""
}
