package browserpolicy

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type BrowserKind string

const (
	BrowserChrome   BrowserKind = "chrome"
	BrowserChromium BrowserKind = "chromium"
	BrowserEdge     BrowserKind = "edge"
)

type ExternalProfileClass string

const (
	ProfileExternalPersistent    ExternalProfileClass = "external-persistent"
	ProfileAuthenticatedExternal ExternalProfileClass = "authenticated-external"
)

type ConnectorDriver string

const DriverChromeDevToolsMCPWS ConnectorDriver = "chrome-devtools-mcp-ws"

// Definitions describe intent only. Runtime observations belong to the broker.
type ProfileDefinition struct {
	ID      string               `json:"id"`
	Browser BrowserKind          `json:"browser"`
	Class   ExternalProfileClass `json:"class"`
}

type ConnectorDefinition struct {
	ID        string          `json:"id"`
	ProfileID string          `json:"profile_id"`
	Driver    ConnectorDriver `json:"driver"`
	Endpoint  string          `json:"endpoint"`
}

type CatalogDefinition struct {
	Profiles   []ProfileDefinition   `json:"profiles"`
	Connectors []ConnectorDefinition `json:"connectors"`
}

// Catalog owns its data; lookups return values and Definition returns fresh slices.
type Catalog struct {
	definition CatalogDefinition
	profiles   map[string]ProfileDefinition
	connectors map[string]ConnectorDefinition
}

var stableID = regexp.MustCompile(`^[a-z][a-z0-9]*([._-][a-z0-9]+)*$`)
var browserEndpointPath = regexp.MustCompile(`^/devtools/browser(/[A-Za-z0-9_-]{1,128})?$`)

func validID(id string) bool { return len(id) <= 64 && stableID.MatchString(id) }

func normalizeEndpoint(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u == nil || (u.Scheme != "ws" && u.Scheme != "wss") || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(endpoint, "#") || u.RawPath != "" || !browserEndpointPath.MatchString(u.Path) {
		return "", fmt.Errorf("endpoint must be an absolute loopback ws/wss browser endpoint without credentials, query or fragment")
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		return "", fmt.Errorf("endpoint requires host and port")
	}
	ip := net.ParseIP(host)
	if strings.ToLower(host) != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return "", fmt.Errorf("endpoint host must be loopback")
	}
	for _, c := range port {
		if c < '0' || c > '9' {
			return "", fmt.Errorf("endpoint port must be numeric")
		}
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("endpoint port out of range")
	}
	if ip != nil {
		host = ip.String()
	} else {
		host = strings.ToLower(host)
	}
	u.Host = net.JoinHostPort(host, strconv.Itoa(n))
	return u.String(), nil
}

func NewCatalog(def CatalogDefinition) (Catalog, error) {
	c := Catalog{profiles: make(map[string]ProfileDefinition), connectors: make(map[string]ConnectorDefinition)}
	for _, p := range def.Profiles {
		if !validID(p.ID) || (p.Browser != BrowserChrome && p.Browser != BrowserChromium && p.Browser != BrowserEdge) || (p.Class != ProfileExternalPersistent && p.Class != ProfileAuthenticatedExternal) {
			return Catalog{}, fmt.Errorf("invalid profile %q", p.ID)
		}
		if _, exists := c.profiles[p.ID]; exists {
			return Catalog{}, fmt.Errorf("duplicate profile %q", p.ID)
		}
		c.profiles[p.ID] = p
		c.definition.Profiles = append(c.definition.Profiles, p)
	}
	for _, connector := range def.Connectors {
		if !validID(connector.ID) || connector.Driver != DriverChromeDevToolsMCPWS {
			return Catalog{}, fmt.Errorf("invalid connector %q", connector.ID)
		}
		if _, exists := c.connectors[connector.ID]; exists {
			return Catalog{}, fmt.Errorf("duplicate connector %q", connector.ID)
		}
		if _, exists := c.profiles[connector.ProfileID]; !exists {
			return Catalog{}, fmt.Errorf("connector %q references missing profile", connector.ID)
		}
		endpoint, err := normalizeEndpoint(connector.Endpoint)
		if err != nil {
			return Catalog{}, fmt.Errorf("connector %q: %w", connector.ID, err)
		}
		connector.Endpoint = endpoint
		c.connectors[connector.ID] = connector
		c.definition.Connectors = append(c.definition.Connectors, connector)
	}
	return c, nil
}

func (c Catalog) Profile(id string) (ProfileDefinition, bool) { p, ok := c.profiles[id]; return p, ok }
func (c Catalog) Connector(id string) (ConnectorDefinition, bool) {
	p, ok := c.connectors[id]
	return p, ok
}
func (c Catalog) Definition() CatalogDefinition {
	return CatalogDefinition{Profiles: append([]ProfileDefinition(nil), c.definition.Profiles...), Connectors: append([]ConnectorDefinition(nil), c.definition.Connectors...)}
}

func ValidateWorkspacePolicies(policies []WorkspaceRootPolicy, catalog Catalog) error {
	for i, mapping := range policies {
		p := mapping.Policy
		if err := ValidateRoutePolicy(p); err != nil {
			return fmt.Errorf("workspace policy %d: %w", i, err)
		}
		if p.Route != RouteRequiredExternal {
			continue
		}
		connector, connectorOK := catalog.Connector(p.RequiredConnectorID)
		profile, profileOK := catalog.Profile(p.RequiredProfileID)
		if !connectorOK || !profileOK || connector.ProfileID != profile.ID {
			return fmt.Errorf("workspace policy %d: required connector/profile missing or mismatched", i)
		}
		if p.Class == WorkspaceCompany && (profile.Browser != BrowserEdge || profile.Class != ProfileAuthenticatedExternal) {
			return fmt.Errorf("workspace policy %d: company requires authenticated external Edge", i)
		}
	}
	return nil
}
