package browserpolicy

import (
	"fmt"
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
}

func ValidateRoutePolicy(p BrowserRoutePolicy) error {
	if p.Class != WorkspaceDefault && p.Class != WorkspaceCompany {
		return fmt.Errorf("unknown workspace class")
	}
	if p.Class == WorkspaceCompany && p.Route != RouteRequiredExternal {
		return fmt.Errorf("workspace class requires authenticated external route")
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
