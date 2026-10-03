package desktopapi

import (
	"fmt"
	"slices"
	"strings"
)

const ProtocolVersion = 1

type Domain string

const (
	DomainRuntime     Domain = "runtime"
	DomainSettings    Domain = "settings"
	DomainConnection  Domain = "connection"
	DomainACP         Domain = "acp"
	DomainBrowser     Domain = "browser"
	DomainActivity    Domain = "activity"
	DomainPermission  Domain = "permission"
	DomainMCP         Domain = "mcp"
	DomainPlugin      Domain = "plugin"
	DomainUpdate      Domain = "update"
	DomainDiagnostics Domain = "diagnostics"
)

var domainOrder = []Domain{
	DomainRuntime,
	DomainConnection,
	DomainSettings,
	DomainACP,
	DomainBrowser,
	DomainActivity,
	DomainPermission,
	DomainMCP,
	DomainPlugin,
	DomainUpdate,
	DomainDiagnostics,
}

type Availability string

const (
	AvailabilityAvailable    Availability = "available"
	AvailabilityExperimental Availability = "experimental"
	AvailabilityUnavailable  Availability = "unavailable"
)

type AccessLevel string

const (
	AccessRead       AccessLevel = "read"
	AccessMutating   AccessLevel = "mutating"
	AccessPrivileged AccessLevel = "privileged"
)

type OperationCapability struct {
	Name                 string      `json:"name"`
	Access               AccessLevel `json:"access"`
	RequiresConfirmation bool        `json:"requiresConfirmation"`
}

type StreamCapability struct {
	Name            string `json:"name"`
	Version         int    `json:"version"`
	Transport       string `json:"transport"`
	Backpressure    string `json:"backpressure"`
	MaxPayloadBytes int    `json:"maxPayloadBytes,omitempty"`
}

type DomainCapability struct {
	Domain       Domain                `json:"domain"`
	Version      int                   `json:"version"`
	Availability Availability          `json:"availability"`
	Operations   []OperationCapability `json:"operations"`
	Streams      []StreamCapability    `json:"streams"`
	Reason       string                `json:"reason,omitempty"`
}

type Manifest struct {
	ProtocolVersion  int                `json:"protocolVersion"`
	MinClientVersion int                `json:"minClientVersion"`
	MaxClientVersion int                `json:"maxClientVersion"`
	Capabilities     []DomainCapability `json:"capabilities"`
}

type NegotiationRequest struct {
	ClientVersion int      `json:"clientVersion"`
	Domains       []Domain `json:"domains,omitempty"`
}

type NegotiationResult struct {
	Accepted      bool               `json:"accepted"`
	ServerVersion int                `json:"serverVersion"`
	Capabilities  []DomainCapability `json:"capabilities"`
	Error         *APIError          `json:"error,omitempty"`
}

type ContractService struct{}

func NewContractService() *ContractService {
	return &ContractService{}
}

func (s *ContractService) Manifest() Manifest {
	return DefaultManifest()
}

func (s *ContractService) Negotiate(request NegotiationRequest) NegotiationResult {
	if request.ClientVersion != ProtocolVersion {
		return NegotiationResult{
			ServerVersion: ProtocolVersion,
			Error: NewError(
				"desktop_api_version_unsupported",
				fmt.Sprintf("desktop API version %d is not supported", request.ClientVersion),
				ErrorCategoryCompatibility,
				false,
				map[string]string{"supportedVersion": fmt.Sprint(ProtocolVersion)},
			),
		}
	}

	manifest := DefaultManifest()
	if len(request.Domains) == 0 {
		return NegotiationResult{
			Accepted:      true,
			ServerVersion: ProtocolVersion,
			Capabilities:  manifest.Capabilities,
		}
	}

	requested := make(map[Domain]struct{}, len(request.Domains))
	for _, domain := range request.Domains {
		domain = Domain(strings.TrimSpace(string(domain)))
		if !slices.Contains(domainOrder, domain) {
			return NegotiationResult{
				ServerVersion: ProtocolVersion,
				Error: NewError(
					"desktop_api_domain_unknown",
					fmt.Sprintf("desktop API domain %q is unknown", domain),
					ErrorCategoryValidation,
					false,
					nil,
				),
			}
		}
		requested[domain] = struct{}{}
	}

	capabilities := make([]DomainCapability, 0, len(requested))
	for _, capability := range manifest.Capabilities {
		if _, ok := requested[capability.Domain]; ok {
			capabilities = append(capabilities, capability)
		}
	}
	return NegotiationResult{
		Accepted:      true,
		ServerVersion: ProtocolVersion,
		Capabilities:  capabilities,
	}
}

func DefaultManifest() Manifest {
	return Manifest{
		ProtocolVersion:  ProtocolVersion,
		MinClientVersion: ProtocolVersion,
		MaxClientVersion: ProtocolVersion,
		Capabilities: []DomainCapability{
			{
				Domain:       DomainRuntime,
				Version:      1,
				Availability: AvailabilityAvailable,
				Operations: []OperationCapability{
					{Name: "status", Access: AccessRead},
					{Name: "start", Access: AccessMutating, RequiresConfirmation: true},
					{Name: "stop", Access: AccessMutating, RequiresConfirmation: true},
					{Name: "restart", Access: AccessMutating, RequiresConfirmation: true},
				},
				Streams: []StreamCapability{},
			},
			availableCapability(DomainConnection, "Tunnel configuration remains native-only; regenerate requires quick mode", "status", "start", "stop", "restart", "regenerate"),
			availableCapability(DomainSettings, "Basic settings only; macOS autostart changes require native SMAppService", "read", "save"),
			unavailableCapability(DomainACP, "ACP desktop contract is scheduled after the activity execution slice"),
			unavailableCapability(DomainBrowser, "browser routing contract is scheduled for the browser-routing milestone"),
			{
				Domain:       DomainActivity,
				Version:      ActivitySchemaVersion,
				Availability: AvailabilityExperimental,
				Operations:   []OperationCapability{},
				Streams: []StreamCapability{
					{
						Name:            "activity",
						Version:         ActivitySchemaVersion,
						Transport:       "bounded_stream",
						Backpressure:    "drop_when_full",
						MaxPayloadBytes: MaxActivityPayloadBytes,
					},
				},
				Reason: "wire contract is defined; production execution source is scheduled for M5",
			},
			unavailableCapability(DomainPermission, "permission parity is not implemented in the shared shell yet"),
			unavailableCapability(DomainMCP, "MCP management has not been adapted to the shared desktop API yet"),
			unavailableCapability(DomainPlugin, "Plugin management has not been adapted to the shared desktop API yet"),
			availableCapability(DomainUpdate, "Apply and recovery remain native-only", "check"),
			availableCapability(DomainDiagnostics, "Local file availability only; logs, content and environment are omitted", "snapshot"),
		},
	}
}

func unavailableCapability(domain Domain, reason string) DomainCapability {
	return DomainCapability{
		Domain:       domain,
		Version:      1,
		Availability: AvailabilityUnavailable,
		Operations:   []OperationCapability{},
		Streams:      []StreamCapability{},
		Reason:       reason,
	}
}

func availableCapability(domain Domain, reason string, names ...string) DomainCapability {
	operations := make([]OperationCapability, 0, len(names))
	for _, name := range names {
		access := AccessRead
		switch name {
		case "start", "stop", "restart", "regenerate", "save":
			access = AccessMutating
		}
		operations = append(operations, OperationCapability{Name: name, Access: access, RequiresConfirmation: access != AccessRead})
	}
	return DomainCapability{Domain: domain, Version: 1, Availability: AvailabilityAvailable, Operations: operations, Streams: []StreamCapability{}, Reason: reason}
}
