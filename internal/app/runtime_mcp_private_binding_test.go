package app

import (
	"encoding/json"
	"strings"
	"testing"

	toolmcp "github.com/uvwt/agentdock/internal/tool/mcp"
)

func TestDesktopPrivateFingerprintBindsRedactedMutationContent(t *testing.T) {
	rt := newPermissionRuntime(t)
	first := "PRIVATE_BINDING_CANARY_ONE"
	second := "PRIVATE_BINDING_CANARY_TWO"
	request := toolmcp.DesktopManageRequest{
		Action: "desktop_env_set", RequestID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Name: "demo", Key: "TOKEN", Value: &first,
		ExpectedRegistryRevision: "rev-1", ExpectedGeneration: "gen-1", ExpectedEnvRevision: "env-1",
	}
	firstDescriptor := desktopAdmissionDescriptor(request)
	firstFingerprint, err := rt.desktopMCPRequestFingerprint(request)
	if err != nil {
		t.Fatal(err)
	}
	request.Value = &second
	secondDescriptor := desktopAdmissionDescriptor(request)
	secondFingerprint, err := rt.desktopMCPRequestFingerprint(request)
	if err != nil {
		t.Fatal(err)
	}
	firstDisplay, _ := json.Marshal(firstDescriptor)
	secondDisplay, _ := json.Marshal(secondDescriptor)
	if string(firstDisplay) != string(secondDisplay) {
		t.Fatalf("redacted descriptors should be display-equivalent: %s != %s", firstDisplay, secondDisplay)
	}
	if firstFingerprint == secondFingerprint {
		t.Fatal("private request fingerprint did not bind the write-only value")
	}
	encoded := string(firstDisplay) + firstFingerprint + secondFingerprint
	if strings.Contains(encoded, first) || strings.Contains(encoded, second) {
		t.Fatal("private request binding exposed write-only values")
	}
}

func TestDesktopAdmissionDescriptorRedactsProtectedConfiguration(t *testing.T) {
	const canary = "PROTECTED_DESCRIPTOR_CANARY"
	request := toolmcp.DesktopManageRequest{
		Action:  "desktop_create",
		URL:     "https://" + canary + ".example/mcp?token=" + canary,
		Command: "/tmp/" + canary + "/bin/server",
		CWD:     "/tmp/" + canary,
		Args:    []string{"--token", canary},
	}
	descriptor := desktopAdmissionDescriptor(request)
	data, _ := json.Marshal(descriptor)
	if strings.Contains(string(data), canary) || strings.Contains(string(data), "example") {
		t.Fatalf("protected configuration reached admission descriptor: %s", data)
	}
	if descriptor["command_configured"] != true || descriptor["cwd_configured"] != true ||
		descriptor["endpoint_configured"] != true || descriptor["endpoint_scheme"] != "https" {
		t.Fatalf("descriptor lost safe configuration facts: %#v", descriptor)
	}
	if _, exists := descriptor["command"]; exists {
		t.Fatal("raw command field remained in admission descriptor")
	}
	if _, exists := descriptor["cwd"]; exists {
		t.Fatal("raw cwd field remained in admission descriptor")
	}
	if _, exists := descriptor["endpoint_origin"]; exists {
		t.Fatal("raw endpoint origin remained in admission descriptor")
	}
}

func TestDesktopPrivateFingerprintDistinguishesOmittedAndExplicitEmptyArgs(t *testing.T) {
	rt := newPermissionRuntime(t)
	omitted, err := toolmcp.DecodeDesktopRequest([]byte(`{"action":"desktop_update","name":"demo","expected_registry_revision":"rev-1","expected_generation":"gen-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	explicitEmpty, err := toolmcp.DecodeDesktopRequest([]byte(`{"action":"desktop_update","name":"demo","args":[],"expected_registry_revision":"rev-1","expected_generation":"gen-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if omitted.ArgsProvided() || !explicitEmpty.ArgsProvided() {
		t.Fatalf("args presence omitted=%v explicit=%v", omitted.ArgsProvided(), explicitEmpty.ArgsProvided())
	}
	omittedFingerprint, err := rt.desktopMCPRequestFingerprint(omitted)
	if err != nil {
		t.Fatal(err)
	}
	explicitFingerprint, err := rt.desktopMCPRequestFingerprint(explicitEmpty)
	if err != nil {
		t.Fatal(err)
	}
	if omittedFingerprint == explicitFingerprint {
		t.Fatal("omitted args (preserve) collided with explicit empty args (clear)")
	}
}
