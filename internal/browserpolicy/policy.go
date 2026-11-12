package browserpolicy

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// WorkspaceClass is assigned only through canonical-root mappings from trusted
// configuration, never inferred from names, URLs, substrings or model flags.
type WorkspaceClass string

const (
	WorkspaceDefault WorkspaceClass = "default"
	WorkspaceCompany WorkspaceClass = "company"
)

type RouteKind string

const (
	RouteManaged          RouteKind = "managed-isolated"
	RouteRequiredExternal RouteKind = "required-authenticated-external"
	RouteExternal         RouteKind = "explicit-external"
)

// BrowserRoutePolicy comes from trusted configuration, never tool arguments.
type BrowserRoutePolicy struct {
	Class                 WorkspaceClass `json:"class"`
	Route                 RouteKind      `json:"route"`
	RequiredConnectorID   string         `json:"required_connector_id,omitempty"`
	RequiredProfileID     string         `json:"required_profile_id,omitempty"`
	AllowExplicitExternal bool           `json:"allow_explicit_external,omitempty"`
	AllowedOrigins        []string       `json:"allowed_origins,omitempty"`
}

func ValidateRoutePolicy(p BrowserRoutePolicy) error {
	if p.Class != WorkspaceDefault && p.Class != WorkspaceCompany {
		return fmt.Errorf("unknown workspace class")
	}
	if p.Class == WorkspaceCompany && p.Route != RouteRequiredExternal {
		return fmt.Errorf("workspace class requires authenticated external route")
	}
	if p.Class == WorkspaceCompany && len(p.AllowedOrigins) == 0 {
		return fmt.Errorf("company workspace requires at least one allowed origin")
	}
	if _, err := NewOriginScope(p.AllowedOrigins); err != nil {
		return err
	}
	switch p.Route {
	case RouteManaged:
		if p.RequiredConnectorID != "" || p.RequiredProfileID != "" {
			return fmt.Errorf("managed route conflicts with required connector/profile")
		}
	case RouteRequiredExternal:
		if strings.TrimSpace(p.RequiredConnectorID) == "" || strings.TrimSpace(p.RequiredProfileID) == "" {
			return fmt.Errorf("required connector and profile must be specified")
		}
	default:
		return fmt.Errorf("unknown policy route")
	}
	return nil
}

// OriginScope is a comparable, immutable encoding of trusted configuration.
// It is authorization scope only: it does not prove browser login or consent.
type OriginScope string

func canonicalHTTPSOrigin(raw string) (string, error) {
	if raw == "" || strings.TrimSpace(raw) != raw || strings.ContainsRune(raw, '\x00') {
		return "", fmt.Errorf("allowed origin must be canonical HTTPS origin")
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil || strings.ToLower(u.Scheme) != "https" || u.Opaque != "" || u.User != nil ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Path != "" || u.RawPath != "" || u.Host == "" {
		return "", fmt.Errorf("allowed origin must be canonical HTTPS origin")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("allowed origin must be canonical HTTPS origin")
	}
	port := u.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 || n == 443 {
			return "", fmt.Errorf("allowed origin must use canonical port")
		}
		port = strconv.Itoa(n)
	}
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	}
	canonicalHost := host
	if strings.Contains(host, ":") {
		canonicalHost = "[" + host + "]"
	}
	if port != "" {
		canonicalHost = net.JoinHostPort(host, port)
	}
	canonical := "https://" + canonicalHost
	if canonical != raw {
		return "", fmt.Errorf("allowed origin must be canonical HTTPS origin")
	}
	return canonical, nil
}

// NewOriginScope validates exact trusted origins and emits a deterministic
// comparable scope. Input order is irrelevant; duplicates fail closed.
func NewOriginScope(origins []string) (OriginScope, error) {
	if len(origins) == 0 {
		return "", nil
	}
	values := make([]string, len(origins))
	seen := make(map[string]struct{}, len(origins))
	for i, raw := range origins {
		canonical, err := canonicalHTTPSOrigin(raw)
		if err != nil {
			return "", err
		}
		if _, exists := seen[canonical]; exists {
			return "", fmt.Errorf("duplicate allowed origin")
		}
		seen[canonical] = struct{}{}
		values[i] = canonical
	}
	sort.Strings(values)
	return OriginScope(strings.Join(values, "\n")), nil
}

func OriginScopeForPolicy(p BrowserRoutePolicy) OriginScope {
	scope, _ := NewOriginScope(p.AllowedOrigins)
	return scope
}

func (s OriginScope) Allows(rawURL string) bool {
	if s == "" || rawURL == "" || strings.TrimSpace(rawURL) != rawURL || strings.ContainsRune(rawURL, '\x00') {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil || u == nil || strings.ToLower(u.Scheme) != "https" || u.User != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port == "443" {
		port = ""
	} else if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
		port = strconv.Itoa(n)
	}
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	}
	originHost := host
	if strings.Contains(host, ":") {
		originHost = "[" + host + "]"
	}
	if port != "" {
		originHost = net.JoinHostPort(host, port)
	}
	origin := "https://" + originHost
	for _, allowed := range strings.Split(string(s), "\n") {
		if origin == allowed {
			return true
		}
	}
	return false
}

func routePoliciesEqual(a, b BrowserRoutePolicy) bool {
	if a.Class != b.Class || a.Route != b.Route || a.RequiredConnectorID != b.RequiredConnectorID ||
		a.RequiredProfileID != b.RequiredProfileID || a.AllowExplicitExternal != b.AllowExplicitExternal {
		return false
	}
	return OriginScopeForPolicy(a) == OriginScopeForPolicy(b)
}
