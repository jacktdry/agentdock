package desktopapi

import "testing"

func TestDefaultManifestCoversEveryDomainOnce(t *testing.T) {
	manifest := DefaultManifest()
	if manifest.ProtocolVersion != ProtocolVersion ||
		manifest.MinClientVersion != ProtocolVersion ||
		manifest.MaxClientVersion != ProtocolVersion {
		t.Fatalf("unexpected protocol range: %#v", manifest)
	}

	seen := make(map[Domain]DomainCapability, len(manifest.Capabilities))
	for _, capability := range manifest.Capabilities {
		if _, exists := seen[capability.Domain]; exists {
			t.Fatalf("duplicate domain capability %q", capability.Domain)
		}
		seen[capability.Domain] = capability
	}
	if len(seen) != len(domainOrder) {
		t.Fatalf("capability count = %d, want %d", len(seen), len(domainOrder))
	}
	for _, domain := range domainOrder {
		if _, ok := seen[domain]; !ok {
			t.Fatalf("missing domain capability %q", domain)
		}
	}

	runtimeCapability := seen[DomainRuntime]
	if runtimeCapability.Availability != AvailabilityAvailable {
		t.Fatalf("runtime availability = %q", runtimeCapability.Availability)
	}
	if len(runtimeCapability.Operations) != 4 {
		t.Fatalf("runtime operations = %#v", runtimeCapability.Operations)
	}

	activityCapability := seen[DomainActivity]
	if activityCapability.Availability != AvailabilityExperimental {
		t.Fatalf("activity availability = %q", activityCapability.Availability)
	}
	if len(activityCapability.Streams) != 1 ||
		activityCapability.Streams[0].Transport != "bounded_stream" ||
		activityCapability.Streams[0].Backpressure != "drop_when_full" ||
		activityCapability.Streams[0].MaxPayloadBytes != MaxActivityPayloadBytes {
		t.Fatalf("activity stream capability = %#v", activityCapability.Streams)
	}

	acpCapability := seen[DomainACP]
	if acpCapability.Availability != AvailabilityAvailable || len(acpCapability.Operations) != 8 {
		t.Fatalf("ACP capability = %#v", acpCapability)
	}
	for _, operation := range acpCapability.Operations {
		switch operation.Name {
		case "status", "settings", "probeProfile", "checkProfileUpdate":
			if operation.Access != AccessRead || operation.RequiresConfirmation {
				t.Fatal("ACP status must be read-only")
			}
		case "close", "updateLifecycle", "updateProfileAdapter":
			if operation.Access != AccessMutating || !operation.RequiresConfirmation {
				t.Fatal("ACP mutation requires confirmation")
			}
		case "saveSettings":
			if operation.Access != AccessMutating || operation.RequiresConfirmation {
				t.Fatal("ACP settings save must preserve sessions without a restart confirmation")
			}
		default:
			t.Fatalf("unexpected ACP operation: %q", operation.Name)
		}
	}
	mcpCapability := seen[DomainMCP]
	if mcpCapability.Availability != AvailabilityAvailable || len(mcpCapability.Operations) != 16 {
		t.Fatalf("MCP capability = %#v", mcpCapability)
	}
	for _, operation := range mcpCapability.Operations {
		switch operation.Name {
		case "snapshot", "inspect", "environment", "authorizationStatus", "authorizationFlowStatus", "operationStatus":
			if operation.Access != AccessRead || operation.RequiresConfirmation {
				t.Fatalf("MCP read operation = %#v", operation)
			}
		default:
			if operation.Access == AccessRead || !operation.RequiresConfirmation {
				t.Fatalf("MCP mutation operation = %#v", operation)
			}
		}
	}
	pluginCapability := seen[DomainPlugin]
	if pluginCapability.Availability != AvailabilityAvailable || len(pluginCapability.Operations) != 13 {
		t.Fatalf("Plugin capability = %#v", pluginCapability)
	}
	for _, operation := range pluginCapability.Operations {
		switch operation.Name {
		case "snapshot", "inspect", "environment", "operationStatus":
			if operation.Access != AccessRead || operation.RequiresConfirmation {
				t.Fatalf("Plugin read operation = %#v", operation)
			}
		case "chooseCandidate":
			if operation.Access != AccessPrivileged || operation.RequiresConfirmation || !operation.NativeRequired {
				t.Fatalf("Plugin candidate picker operation = %#v", operation)
			}
		case "discardCandidate":
			if operation.Access != AccessMutating || operation.RequiresConfirmation {
				t.Fatalf("Plugin discard candidate operation = %#v", operation)
			}
		default:
			if operation.Access != AccessMutating || !operation.RequiresConfirmation {
				t.Fatalf("Plugin mutation operation = %#v", operation)
			}
		}
	}
	if seen[DomainBrowser].Availability != AvailabilityUnavailable {
		t.Fatalf("browser should remain unavailable, got %#v", seen[DomainBrowser])
	}
}

func TestContractNegotiationFiltersRequestedDomains(t *testing.T) {
	service := NewContractService()
	result := service.Negotiate(NegotiationRequest{
		ClientVersion: ProtocolVersion,
		Domains:       []Domain{DomainActivity, DomainRuntime, DomainActivity},
	})
	if !result.Accepted || result.Error != nil {
		t.Fatalf("negotiation failed: %#v", result)
	}
	if len(result.Capabilities) != 2 {
		t.Fatalf("capabilities = %#v", result.Capabilities)
	}
	if result.Capabilities[0].Domain != DomainRuntime || result.Capabilities[1].Domain != DomainActivity {
		t.Fatalf("capability ordering = %#v", result.Capabilities)
	}
}

func TestContractNegotiationRejectsUnknownDomainAndVersion(t *testing.T) {
	service := NewContractService()

	version := service.Negotiate(NegotiationRequest{ClientVersion: ProtocolVersion + 1})
	if version.Accepted || version.Error == nil ||
		version.Error.Code != "desktop_api_version_unsupported" ||
		version.Error.Category != ErrorCategoryCompatibility {
		t.Fatalf("version negotiation = %#v", version)
	}

	domain := service.Negotiate(NegotiationRequest{
		ClientVersion: ProtocolVersion,
		Domains:       []Domain{"unknown"},
	})
	if domain.Accepted || domain.Error == nil ||
		domain.Error.Code != "desktop_api_domain_unknown" ||
		domain.Error.Category != ErrorCategoryValidation {
		t.Fatalf("domain negotiation = %#v", domain)
	}
}
