package browserpolicy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func catalogFixture() CatalogDefinition {
	return CatalogDefinition{Profiles: []ProfileDefinition{{ID: "user-edge", Browser: BrowserEdge, Class: ProfileAuthenticatedExternal, UserDataDir: filepath.Join(os.TempDir(), "agentdock-test-profile", "configured", "edge")}}, Connectors: []ConnectorDefinition{{ID: "edge-ws", ProfileID: "user-edge", Driver: DriverChromeDevToolsMCPWS, Endpoint: "ws://127.0.0.1:9222/devtools/browser"}}}
}

func TestNativeEdgeProfileSelectionIsExplicitAndUnambiguous(t *testing.T) {
	for _, tc := range []struct {
		name, directory string
		current, valid  bool
	}{
		{"missing", "", false, false},
		{"observed current", "", true, true},
		{"explicit Default", "Default", false, true},
		{"explicit company directory", "Profile 1", false, true},
		{"ambiguous", "Default", true, false},
		{"path traversal", "../Default", false, false},
		{"invalid Profile number", "Profile 0", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := catalogFixture()
			d.Connectors[0].Driver = DriverNextEdgeCDP
			d.Profiles[0].SelectedProfileName = "公司"
			d.Profiles[0].ProfileDirectory, d.Profiles[0].BindCurrentProfile = tc.directory, tc.current
			if _, err := NewCatalog(d); (err == nil) != tc.valid {
				t.Fatalf("selection validity=%t: %v", tc.valid, err)
			}
			d.Profiles[0].SelectedProfileName = ""
			if _, err := NewCatalog(d); err == nil {
				t.Fatal("missing owner selection accepted")
			}
		})
	}
}

func TestCatalogValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*CatalogDefinition)
	}{
		{"empty profile id", func(d *CatalogDefinition) { d.Profiles[0].ID = "" }},
		{"unstable id", func(d *CatalogDefinition) { d.Profiles[0].ID = " User Profile " }},
		{"long id", func(d *CatalogDefinition) { d.Profiles[0].ID = strings.Repeat("a", 65) }},
		{"bad connector id", func(d *CatalogDefinition) { d.Connectors[0].ID = "../edge" }},
		{"duplicate profile", func(d *CatalogDefinition) { d.Profiles = append(d.Profiles, d.Profiles[0]) }},
		{"duplicate connector", func(d *CatalogDefinition) { d.Connectors = append(d.Connectors, d.Connectors[0]) }},
		{"dangling", func(d *CatalogDefinition) { d.Connectors[0].ProfileID = "missing" }},
		{"managed user profile", func(d *CatalogDefinition) { d.Profiles[0].Class = "isolated-temporary" }},
		{"unknown browser", func(d *CatalogDefinition) { d.Profiles[0].Browser = "firefox" }},
		{"unknown driver", func(d *CatalogDefinition) { d.Connectors[0].Driver = "registry-server" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := catalogFixture()
			tc.mutate(&d)
			if _, err := NewCatalog(d); err == nil {
				t.Fatal("invalid catalog accepted")
			}
		})
	}
	for _, endpoint := range []string{"", "http://127.0.0.1:9222/devtools/browser", "/devtools/browser", "ws://example.com:9222/devtools/browser", "ws://127.0.0.1/devtools/browser", "ws://127.0.0.1:0/devtools/browser", "ws://127.0.0.1:65536/devtools/browser", "ws://user:secret@127.0.0.1:9222/devtools/browser", "ws://127.0.0.1:9222/devtools/browser?x=1", "ws://127.0.0.1:9222/devtools/browser?", "ws://127.0.0.1:9222/devtools/browser#", "ws://127.0.0.1:9222/devtools/browser#x", "ws://127.0.0.1:9222/devtools/page/123", "ws://127.0.0.1:9222/devtools/browser/../page", "ws://127.0.0.1:9222/devtools/browser/%61", "ws://[::1%25lo0]:9222/devtools/browser", "ws://127.0.0.1:9222/devtools/browser/a/b"} {
		t.Run(endpoint, func(t *testing.T) {
			d := catalogFixture()
			d.Connectors[0].Endpoint = endpoint
			if _, err := NewCatalog(d); err == nil {
				t.Fatal("unsafe endpoint accepted")
			}
		})
	}
	for _, endpoint := range []string{"ws://127.0.0.1:9222/devtools/browser", "wss://localhost:9222/devtools/browser", "ws://[::1]:9222/devtools/browser/abc-123_456"} {
		d := catalogFixture()
		d.Connectors[0].Endpoint = endpoint
		if _, err := NewCatalog(d); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCatalogCopies(t *testing.T) {
	d := catalogFixture()
	d.Connectors[0].Endpoint = "ws://LOCALHOST:09222/devtools/browser"
	c, err := NewCatalog(d)
	if err != nil {
		t.Fatal(err)
	}
	d.Profiles[0].Browser = BrowserChrome
	d.Profiles[0].UserDataDir = "changed"
	d.Connectors[0].Endpoint = "unsafe"
	copy := c.Definition()
	copy.Profiles[0].Class = ProfileExternalPersistent
	copy.Profiles[0].UserDataDir = "changed"
	copy.Connectors[0].ProfileID = "missing"
	p, _ := c.Profile("user-edge")
	p.Browser = BrowserChrome
	p.UserDataDir = "changed"
	connector, _ := c.Connector("edge-ws")
	connector.Endpoint = "unsafe"
	p, _ = c.Profile("user-edge")
	connector, _ = c.Connector("edge-ws")
	if p.UserDataDir != catalogFixture().Profiles[0].UserDataDir || c.Definition().Profiles[0].UserDataDir != p.UserDataDir || p.Browser != BrowserEdge || p.Class != ProfileAuthenticatedExternal || connector.ProfileID != "user-edge" || connector.Endpoint != "ws://localhost:9222/devtools/browser" {
		t.Fatalf("catalog mutated: %+v %+v", p, connector)
	}
}

func TestCatalogWorkspacePolicies(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*CatalogDefinition, *BrowserRoutePolicy)
		valid  bool
	}{
		{name: "company", valid: true},
		{name: "company missing path", mutate: func(d *CatalogDefinition, p *BrowserRoutePolicy) { d.Profiles[0].UserDataDir = "" }},
		{name: "unused external without path", mutate: func(d *CatalogDefinition, p *BrowserRoutePolicy) {
			d.Profiles[0].UserDataDir = ""
			*p = BrowserRoutePolicy{Class: WorkspaceDefault, Route: RouteManaged}
		}, valid: true},
		{name: "ordinary required external without path", mutate: func(d *CatalogDefinition, p *BrowserRoutePolicy) {
			d.Profiles[0].UserDataDir = ""
			p.Class = WorkspaceDefault
		}, valid: true},
		{name: "missing connector", mutate: func(d *CatalogDefinition, p *BrowserRoutePolicy) { p.RequiredConnectorID = "missing" }},
		{name: "missing profile", mutate: func(d *CatalogDefinition, p *BrowserRoutePolicy) { p.RequiredProfileID = "missing" }},
		{name: "mismatch", mutate: func(d *CatalogDefinition, p *BrowserRoutePolicy) {
			d.Profiles = append(d.Profiles, ProfileDefinition{ID: "other", Browser: BrowserEdge, Class: ProfileAuthenticatedExternal})
			p.RequiredProfileID = "other"
		}},
		{name: "wrong browser", mutate: func(d *CatalogDefinition, p *BrowserRoutePolicy) { d.Profiles[0].Browser = BrowserChrome }},
		{name: "wrong class", mutate: func(d *CatalogDefinition, p *BrowserRoutePolicy) { d.Profiles[0].Class = ProfileExternalPersistent }},
		{name: "company managed", mutate: func(d *CatalogDefinition, p *BrowserRoutePolicy) { p.Route = RouteManaged }},
		{name: "default", mutate: func(d *CatalogDefinition, p *BrowserRoutePolicy) {
			*p = BrowserRoutePolicy{Class: WorkspaceDefault, Route: RouteManaged}
		}, valid: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := catalogFixture()
			p := BrowserRoutePolicy{Class: WorkspaceCompany, Route: RouteRequiredExternal, RequiredConnectorID: "edge-ws", RequiredProfileID: "user-edge", AllowedOrigins: []string{"https://cms.example.com"}}
			if tc.mutate != nil {
				tc.mutate(&d, &p)
			}
			c, err := NewCatalog(d)
			if err != nil {
				t.Fatal(err)
			}
			err = ValidateWorkspacePolicies([]WorkspaceRootPolicy{{Policy: p}}, c)
			if (err == nil) != tc.valid {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestCatalogRejectsEquivalentCanonicalConnectorEndpoints(t *testing.T) {
	d := catalogFixture()
	d.Connectors = append(d.Connectors, ConnectorDefinition{
		ID: "edge-ws-alias", ProfileID: "user-edge", Driver: DriverChromeDevToolsMCPWS,
		Endpoint: "ws://LOCALHOST:09222/devtools/browser",
	})
	d.Connectors[0].Endpoint = "ws://localhost:9222/devtools/browser"
	if _, err := NewCatalog(d); err == nil {
		t.Fatal("equivalent canonical connector endpoint accepted")
	}
}

func TestCatalogUserDataDir(t *testing.T) {
	for _, path := range []string{"relative", "/", "/configured/../edge", "/configured/./edge", "/configured//edge", "/configured/edge/", "/configured/edge\x00"} {
		t.Run(path, func(t *testing.T) {
			d := catalogFixture()
			d.Profiles[0].UserDataDir = path
			if _, err := NewCatalog(d); err == nil {
				t.Fatal("invalid user_data_dir accepted")
			}
		})
	}
	for _, path := range []string{"", catalogFixture().Profiles[0].UserDataDir} {
		d := catalogFixture()
		d.Profiles[0].UserDataDir = path
		raw, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		if path == "" && strings.Contains(string(raw), "user_data_dir") {
			t.Fatal("optional field not omitted")
		}
		var decoded CatalogDefinition
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		c, err := NewCatalog(decoded)
		if err != nil {
			t.Fatal(err)
		}
		profile, ok := c.Profile("user-edge")
		if !ok || profile.UserDataDir != path || c.Definition().Profiles[0].UserDataDir != path {
			t.Fatal("user_data_dir lost in roundtrip")
		}
	}
}
