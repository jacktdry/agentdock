//go:build windows

package desktopruntime

import (
	"os"
	"testing"
)

func assertTunnelPrivateMode(t *testing.T, info os.FileInfo) {
	t.Helper() /* atomicfile applies the Windows private DACL. */
}
