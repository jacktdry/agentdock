package desktopcontrol

import (
	"os"
	"strings"
	"testing"
)

func TestWindowsPermissionBootstrapRejectsRemotePipeClients(t *testing.T) {
	data, err := os.ReadFile("control_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS") {
		t.Fatal("current-user ACL alone does not make credential bootstrap local-only")
	}
}
