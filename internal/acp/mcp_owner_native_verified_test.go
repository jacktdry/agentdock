package acp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
)

func makeNativeSignedFixture(t *testing.T, key []byte, nonce string, mutate func(*nextNativePayload)) nextNativeSigned {
	t.Helper()
	payload := nextNativePayload{
		Version: 1, Nonce: nonce, AdapterPID: 41000, AppServerPID: 41001, AppServerEpoch: 1,
		ActiveACPThreadIDs: []string{"thread-1"}, LoadedThreadIDs: []string{"thread-1", "background-thread"},
		MCPByThread:             []nextNativeThreadStatus{{ThreadID: "thread-1", Servers: []nextNativeStatus{{Name: "shared-memory", Status: "connected"}}}},
		MCPChildPIDAndTransport: "unavailable", ReclaimPermission: false,
	}
	if mutate != nil {
		mutate(&payload)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return nextNativeSigned{Payload: string(raw), MAC: hex.EncodeToString(nextNativeHMAC(key, "agentdock-next-mcp-owner-response-v1", string(raw)))}
}
func TestNextNativeAuthenticatedDiagnosticStrictAndNoReclaim(t *testing.T) {
	key := make([]byte, 32)
	nonceRaw := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(nonceRaw); err != nil {
		t.Fatal(err)
	}
	nonce := hex.EncodeToString(nonceRaw)
	valid := makeNativeSignedFixture(t, key, nonce, nil)
	decoded, err := decodeNextNativeSigned(key, nonce, valid)
	if err != nil || decoded.MCPChildPIDAndTransport != "unavailable" || decoded.ReclaimPermission || decoded.LoadedThreadIDs[1] != "background-thread" {
		t.Fatalf("legitimate native evidence denied: %+v %v", decoded, err)
	}
	invalid := []struct {
		name   string
		mutate func(*nextNativePayload)
	}{
		{"missing native child identity", func(p *nextNativePayload) { p.AppServerPID = 0 }},
		{"claimed reclaim authority", func(p *nextNativePayload) { p.ReclaimPermission = true }},
		{"fabricated native ownership", func(p *nextNativePayload) { p.MCPChildPIDAndTransport = "available" }},
		{"duplicate native thread", func(p *nextNativePayload) { p.LoadedThreadIDs = append(p.LoadedThreadIDs, "thread-1") }},
		{"duplicate ACP thread", func(p *nextNativePayload) { p.ActiveACPThreadIDs = append(p.ActiveACPThreadIDs, "thread-1") }},
		{"incomplete ACP mapping", func(p *nextNativePayload) { p.MCPByThread = nil }},
		{"unknown native thread", func(p *nextNativePayload) { p.LoadedThreadIDs = []string{"other"} }},
		{"unexpected server status", func(p *nextNativePayload) { p.MCPByThread[0].Servers[0].Status = "connected/localhost" }},
		{"bad nonce", func(p *nextNativePayload) { p.Nonce = "not-the-request" }},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			bad := makeNativeSignedFixture(t, key, nonce, tc.mutate)
			if _, err := decodeNextNativeSigned(key, nonce, bad); !errors.Is(err, errMCPDiagnosticNotQualified) {
				t.Fatalf("malformed signed report accepted: %v", err)
			}
		})
	}
	corrupted := valid
	corrupted.Payload += " "
	if _, err := decodeNextNativeSigned(key, nonce, corrupted); !errors.Is(err, errMCPDiagnosticNotQualified) {
		t.Fatal("changed payload accepted")
	}
	if _, err := decodeNextNativeSigned(key, "d"+nonce[1:], valid); !errors.Is(err, errMCPDiagnosticNotQualified) {
		t.Fatal("wrong nonce accepted")
	}
	if _, err := decodeNextNativeSigned(key[:16], nonce, valid); !errors.Is(err, errMCPDiagnosticNotQualified) {
		t.Fatal("wrong key size accepted")
	}
	alternate := make([]byte, 32)
	copy(alternate, key)
	alternate[0] ^= 1
	if _, err := decodeNextNativeSigned(alternate, nonce, valid); !errors.Is(err, errMCPDiagnosticNotQualified) {
		t.Fatal("forged HMAC accepted")
	}
	if v, err := (*Manager)(nil).readNextNativeMCPPartialEvidence(context.Background(), key); !errors.Is(err, errMCPDiagnosticNotQualified) || v.ReclaimAllowed {
		t.Fatal("nil process unexpectedly authorized")
	}
}
