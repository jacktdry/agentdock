package browserpolicy

import (
	"strings"
	"testing"
)

func catalogFixture() CatalogDefinition {
	return CatalogDefinition{Profiles: []ProfileDefinition{{ID: "user-edge", Browser: BrowserEdge, Class: ProfileAuthenticatedExternal}}, Connectors: []ConnectorDefinition{{ID: "edge-ws", ProfileID: "user-edge", Driver: DriverChromeDevToolsMCPWS, Endpoint: "ws://127.0.0.1:9222/devtools/browser"}}}
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
	d.Connectors[0].Endpoint = "unsafe"
	copy := c.Definition()
	copy.Profiles[0].Class = ProfileExternalPersistent
	copy.Connectors[0].ProfileID = "missing"
	p, _ := c.Profile("user-edge")
	p.Browser = BrowserChrome
	connector, _ := c.Connector("edge-ws")
	connector.Endpoint = "unsafe"
	p, _ = c.Profile("user-edge")
	connector, _ = c.Connector("edge-ws")
	if p.Browser != BrowserEdge || p.Class != ProfileAuthenticatedExternal || connector.ProfileID != "user-edge" || connector.Endpoint != "ws://localhost:9222/devtools/browser" {
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
			p := BrowserRoutePolicy{Class: WorkspaceCompany, Route: RouteRequiredExternal, RequiredConnectorID: "edge-ws", RequiredProfileID: "user-edge"}
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
