//go:build darwin

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/browserpolicy"
)

func nextBrowserConfigFixture(t *testing.T) (string, string, []byte) {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, ".agentdock-next")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	body, err := json.Marshal(struct {
		Version           int                                 `json:"version"`
		Catalog           browserpolicy.CatalogDefinition     `json:"catalog"`
		WorkspacePolicies []browserpolicy.WorkspaceRootPolicy `json:"workspace_policies"`
	}{
		Version: 1,
		Catalog: browserpolicy.CatalogDefinition{
			Profiles:   []browserpolicy.ProfileDefinition{{ID: "company-edge", Browser: browserpolicy.BrowserEdge, Class: browserpolicy.ProfileAuthenticatedExternal, UserDataDir: "/private/owner-approved-edge"}},
			Connectors: []browserpolicy.ConnectorDefinition{{ID: "edge-cdp", ProfileID: "company-edge", Driver: browserpolicy.DriverChromeDevToolsMCPWS, Endpoint: "ws://127.0.0.1:9222/devtools/browser/test"}},
		},
		WorkspacePolicies: []browserpolicy.WorkspaceRootPolicy{{Root: workspace, Policy: browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceCompany, Route: browserpolicy.RouteRequiredExternal, RequiredConnectorID: "edge-cdp", RequiredProfileID: "company-edge", AllowedOrigins: []string{"https://cms.example.com"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AGENTDOCK_HOME", "")
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	t.Setenv("AGENTDOCK_STDIO", "")
	t.Setenv(browserCatalogEnv, "")
	t.Setenv(browserWorkspacePoliciesEnv, "")
	return root, filepath.Join(root, "browser-routes.json"), body
}

func TestNextBrowserRoutesPrivateFileLoadsOnlyInNextCore(t *testing.T) {
	_, path, body := nextBrowserConfigFixture(t)
	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.BrowserCatalog.Connectors) != 0 {
		t.Fatal("file absent should not configure external routes")
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.BrowserCatalog.Connectors) != 1 || cfg.BrowserCatalog.Connectors[0].ID != "edge-cdp" || len(cfg.BrowserWorkspacePolicies) != 1 {
		t.Fatalf("file load: %+v", cfg.BrowserCatalog)
	}
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"", "stable", "other"} {
		t.Run("variant="+variant, func(t *testing.T) {
			t.Setenv("AGENTDOCK_DESKTOP_VARIANT", variant)
			cfg, err := FromEnv()
			if err != nil {
				t.Fatal(err)
			}
			if len(cfg.BrowserCatalog.Connectors) != 0 {
				t.Fatal("non-next process inherited Next private connector")
			}
		})
	}
	for _, stdio := range []string{"true", "1", "TRUE"} {
		t.Run("stdio="+stdio, func(t *testing.T) {
			t.Setenv("AGENTDOCK_STDIO", stdio)
			cfg, err := FromEnv()
			if err != nil {
				t.Fatal(err)
			}
			if len(cfg.BrowserCatalog.Connectors) != 0 {
				t.Fatal("stdio process inherited Next private connector")
			}
		})
	}
}

func TestNextBrowserRoutesPrivateFileRejectsUnsafeConfig(t *testing.T) {
	root, path, body := nextBrowserConfigFixture(t)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), body...)
	cases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"world-readable", func(t *testing.T) {
			if err := os.Chmod(path, 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"group-readable", func(t *testing.T) {
			if err := os.Chmod(path, 0o640); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlink-config", func(t *testing.T) {
			os.Remove(path)
			target := filepath.Join(t.TempDir(), "external.json")
			if err := os.WriteFile(target, body, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
		}},
		{"world-readable-root", func(t *testing.T) {
			if err := os.Chmod(root, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{"bad-json", func(t *testing.T) {
			if err := os.WriteFile(path, []byte(`{"version":1,`), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"unknown-field", func(t *testing.T) {
			if err := os.WriteFile(path, append(append([]byte(nil), original[:len(original)-1]...), []byte(`,"healthy":true}`)...), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"trailing-data", func(t *testing.T) {
			if err := os.WriteFile(path, append(append([]byte(nil), original...), []byte(`{}`)...), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"invalid-version", func(t *testing.T) {
			changed := strings.Replace(string(original), `"version":1`, `"version":2`, 1)
			if err := os.WriteFile(path, []byte(changed), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"invalid-endpoint", func(t *testing.T) {
			changed := strings.Replace(string(original), `127.0.0.1:9222`, `example.com:9222`, 1)
			if err := os.WriteFile(path, []byte(changed), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"wrong-profile", func(t *testing.T) {
			changed := strings.Replace(string(original), `"required_profile_id":"company-edge"`, `"required_profile_id":"other"`, 1)
			if err := os.WriteFile(path, []byte(changed), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"no-workspace-policy", func(t *testing.T) {
			var modified map[string]any
			if err := json.Unmarshal(original, &modified); err != nil {
				t.Fatal(err)
			}
			modified["workspace_policies"] = []any{}
			changed, err := json.Marshal(modified)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, changed, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.Chmod(root, 0o700); err != nil {
				t.Fatal(err)
			}
			os.Remove(path)
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			tc.run(t)
			_, err := FromEnv()
			if err == nil {
				t.Fatal("unsafe Next browser config accepted")
			}
		})
	}
}

func TestNextBrowserRoutesPrivateFileRejectsEnvironmentConflict(t *testing.T) {
	_, path, body := nextBrowserConfigFixture(t)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(browserCatalogEnv, validBrowserCatalogJSON)
	if _, err := FromEnv(); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("simultaneous env/file policies must be rejected explicitly, got %v", err)
	}
}

func TestNextBrowserRoutesPrivateFileSkipsCustomHome(t *testing.T) {
	_, path, body := nextBrowserConfigFixture(t)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTDOCK_HOME", t.TempDir())
	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.BrowserCatalog.Connectors) != 0 {
		t.Fatal("custom isolated HOME got owner's live route")
	}
}

func TestNextBrowserRoutesPrivateFileSymlinkHomeRejected(t *testing.T) {
	root, path, body := nextBrowserConfigFixture(t)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	backup := root + "-real"
	if err := os.Rename(root, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(backup, root); err != nil {
		t.Fatal(err)
	}
	if _, err := FromEnv(); err == nil {
		t.Fatal("symlinked Next private home accepted")
	}
}
