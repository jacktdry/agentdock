//go:build darwin || linux

package desktopruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestACPProtectedEnvironmentReadBoundary(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "agentdock.env")
	data := []byte("AGENTDOCK_HOST='[::1]'\nAGENTDOCK_PORT=28765\nAGENTDOCK_AUTH_TOKEN='private-token'\nUNRELATED_SECRET='private-env'\nAGENTDOCK_ACP_ENABLED=true\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	access, err := ReadLocalCoreAccess(context.Background(), root)
	if err != nil || access.MCPURL != "http://[::1]:28765/mcp" || access.AuthToken != "private-token" {
		t.Fatalf("access=%+v err=%v", access, err)
	}
	settings, err := ReadACPSettings(context.Background(), root)
	encoded, _ := json.Marshal(settings)
	if err != nil || strings.Contains(string(encoded), "private-") {
		t.Fatal("settings leaks environment")
	}
	for _, state := range []string{"public", "symlink", "parse", "port", "host", "enabled"} {
		t.Run(state, func(t *testing.T) {
			caseRoot := t.TempDir()
			casePath := filepath.Join(caseRoot, "agentdock.env")
			contents := string(data)
			switch state {
			case "parse":
				contents = "private-env='unterminated"
			case "port":
				contents += "AGENTDOCK_PORT='private-env'\n"
			case "host":
				contents += "AGENTDOCK_HOST='private-env'\n"
			case "enabled":
				contents += "AGENTDOCK_ACP_ENABLED='private-env'\n"
			}
			if err := os.WriteFile(casePath, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if state == "public" {
				if err := os.Chmod(casePath, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if state == "symlink" {
				if err := os.Rename(casePath, casePath+".real"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(casePath+".real", casePath); err != nil {
					t.Fatal(err)
				}
			}
			_, coreErr := ReadLocalCoreAccess(context.Background(), caseRoot)
			_, settingsErr := ReadACPSettings(context.Background(), caseRoot)
			if state != "enabled" && coreErr == nil {
				t.Fatal("invalid Core environment accepted")
			}
			if state != "host" && state != "port" && settingsErr == nil {
				t.Fatal("invalid ACP environment accepted")
			}
			if strings.Contains(fmt.Sprint(coreErr, settingsErr), "private-env") {
				t.Fatal("environment error leaks secret")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadLocalCoreAccess(ctx, root); err != context.Canceled {
		t.Fatal(err)
	}
	if _, err := ReadACPSettings(ctx, root); err != context.Canceled {
		t.Fatal(err)
	}
}
