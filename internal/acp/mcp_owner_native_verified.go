package acp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"sort"

	"github.com/uvwt/agentdock/internal/process"
)

const nextMCPNativeMethod = "_agentdock_next/mcp_owner/diagnostic_v1"
const nextMCPNativeMaxPayload = 96 << 10

var nextMCPNativeNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
var nextMCPNativeStatusPattern = regexp.MustCompile(`^[A-Za-z]{1,32}$`)

// nativeMCPPartialEvidence is a strictly diagnostic/private output, NOT a
// child-process control capability or a complete ownership mapping.
type nativeMCPPartialEvidence struct {
	Adapter           process.BirthIdentity
	Native            process.BirthIdentity
	ActiveThreads     int
	LoadedThreads     int
	MCPStatusRows     int
	ChildProcessProof string
	ReclaimAllowed    bool
}
type nextNativeStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}
type nextNativeThreadStatus struct {
	ThreadID string             `json:"threadId"`
	Servers  []nextNativeStatus `json:"servers"`
}
type nextNativePayload struct {
	Version                 int                      `json:"version"`
	Nonce                   string                   `json:"nonce"`
	AdapterPID              int                      `json:"adapterPid"`
	AppServerPID            int                      `json:"appServerPid"`
	AppServerEpoch          int                      `json:"appServerEpoch"`
	ActiveACPThreadIDs      []string                 `json:"activeACPThreadIds"`
	LoadedThreadIDs         []string                 `json:"loadedThreadIds"`
	MCPByThread             []nextNativeThreadStatus `json:"mcpByThread"`
	MCPChildPIDAndTransport string                   `json:"mcpChildPIDAndTransport"`
	ReclaimPermission       bool                     `json:"reclaimPermission"`
}
type nextNativeSigned struct {
	Payload string `json:"payload"`
	MAC     string `json:"mac"`
}

func nextNativeHMAC(key []byte, domain, message string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(domain))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(message))
	return h.Sum(nil)
}
func decodeNextNativeSigned(key []byte, nonce string, signed nextNativeSigned) (nextNativePayload, error) {
	deny := nextNativePayload{}
	if len(key) != 32 || len(nonce) != 64 || len(signed.Payload) > nextMCPNativeMaxPayload || len(signed.MAC) != 64 {
		return deny, errMCPDiagnosticNotQualified
	}
	decoded, err := hex.DecodeString(signed.MAC)
	if err != nil || !hmac.Equal(decoded, nextNativeHMAC(key, "agentdock-next-mcp-owner-response-v1", signed.Payload)) {
		return deny, errMCPDiagnosticNotQualified
	}
	d := json.NewDecoder(bytes.NewReader([]byte(signed.Payload)))
	d.DisallowUnknownFields()
	var payload nextNativePayload
	if err := d.Decode(&payload); err != nil {
		return deny, errMCPDiagnosticNotQualified
	}
	var trailing any
	if err := d.Decode(&trailing); !errors.Is(err, io.EOF) {
		return deny, errMCPDiagnosticNotQualified
	}
	if payload.Version != 1 || payload.Nonce != nonce || payload.AdapterPID <= 0 || payload.AppServerPID <= 0 || payload.AdapterPID == payload.AppServerPID || payload.AppServerEpoch < 0 || payload.MCPChildPIDAndTransport != "unavailable" || payload.ReclaimPermission || len(payload.ActiveACPThreadIDs) > 128 || len(payload.LoadedThreadIDs) > 128 || len(payload.MCPByThread) > 128 {
		return deny, errMCPDiagnosticNotQualified
	}
	nativeIDs := map[string]bool{}
	activeIDs := map[string]bool{}
	mapped := map[string]bool{}
	for _, id := range payload.LoadedThreadIDs {
		if !nextMCPNativeNamePattern.MatchString(id) || nativeIDs[id] {
			return deny, errMCPDiagnosticNotQualified
		}
		nativeIDs[id] = true
	}
	for _, id := range payload.ActiveACPThreadIDs {
		if !nextMCPNativeNamePattern.MatchString(id) || activeIDs[id] || !nativeIDs[id] {
			return deny, errMCPDiagnosticNotQualified
		}
		activeIDs[id] = true
	}
	for _, t := range payload.MCPByThread {
		if !activeIDs[t.ThreadID] || mapped[t.ThreadID] || len(t.Servers) > 64 {
			return deny, errMCPDiagnosticNotQualified
		}
		mapped[t.ThreadID] = true
		serverIDs := map[string]bool{}
		for _, s := range t.Servers {
			if !nextMCPNativeNamePattern.MatchString(s.Name) || !nextMCPNativeStatusPattern.MatchString(s.Status) || serverIDs[s.Name] {
				return deny, errMCPDiagnosticNotQualified
			}
			serverIDs[s.Name] = true
		}
	}
	if len(mapped) != len(activeIDs) {
		return deny, errMCPDiagnosticNotQualified
	}
	return payload, nil
}

// readNextNativeMCPPartialEvidence is a SOURCE-ONLY private bridge. The caller
// supplies the per-launch 32-byte Adapter secret held by trusted Next Core;
// it MUST match the Adapter's pinned AgentSpec environment. The key is never
// serialized to ACP requests/logs and never grants a process kill capability.
// Even a valid MAC + OS adapter→native birth does NOT prove native Thread→MCP
// stdio pipe→child PID. We *always* return ChildProcessProof=unavailable.
func (m *Manager) readNextNativeMCPPartialEvidence(ctx context.Context, key []byte) (nativeMCPPartialEvidence, error) {
	return m.readNextNativeMCPPartialEvidenceQualified(ctx, key, false)
}

func (m *Manager) readNextNativeMCPPartialEvidenceQualified(ctx context.Context, key []byte, allowEmpty bool) (nativeMCPPartialEvidence, error) {
	fail := nativeMCPPartialEvidence{}
	if len(key) != 32 || m == nil {
		return fail, errMCPDiagnosticNotQualified
	}
	var report nativeMCPPartialEvidence
	err := m.withMCPDiagnosticScopePolicy(ctx, allowEmpty, func(ctx context.Context, scope diagnosticManagerScope) error {
		// This is a read-only ACP diagnostic on the locally pinned Adapter. Do not
		// call back into Manager methods requiring mu.Lock while the scope is held.
		stored := m.process.spec.Environment["AGENTDOCK_NEXT_MCP_OWNER_DIAGNOSTIC_KEY"]
		supplied, decodeErr := hex.DecodeString(stored)
		if decodeErr != nil || len(supplied) != 32 || subtle.ConstantTimeCompare(supplied, key) != 1 {
			return errMCPDiagnosticNotQualified
		}
		var challenge [32]byte
		if _, err := rand.Read(challenge[:]); err != nil {
			return errMCPDiagnosticNotQualified
		}
		nonce := hex.EncodeToString(challenge[:])
		request := map[string]string{"nonce": nonce, "mac": hex.EncodeToString(nextNativeHMAC(key, "agentdock-next-mcp-owner-request-v1", nonce))}
		var signed nextNativeSigned
		if err := m.process.connection.Request(ctx, nextMCPNativeMethod, request, &signed); err != nil {
			return errMCPDiagnosticNotQualified
		}
		payload, err := decodeNextNativeSigned(key, nonce, signed)
		if err != nil || payload.AdapterPID != scope.adapter.PID {
			return errMCPDiagnosticNotQualified
		}
		actual := make([]string, 0, len(scope.loaded))
		for _, loaded := range scope.loaded {
			actual = append(actual, loaded.adapterSessionID)
		}
		sort.Strings(actual)
		expected := append([]string(nil), payload.ActiveACPThreadIDs...)
		sort.Strings(expected)
		if len(actual) != len(expected) {
			return errMCPDiagnosticNotQualified
		}
		for i := range actual {
			if actual[i] != expected[i] {
				return errMCPDiagnosticNotQualified
			}
		}
		// A Codex app-server spawned by Node need not create its OWN process
		// group. Do not impose the managed Go stdio child PGID=PID contract
		// here: verify its direct PPID, UID, exact kernel birth and re-sample.
		// No native Thread/transport/MCP-child ownership follows from this.
		native, err := process.ObserveBirthIdentity(ctx, payload.AppServerPID)
		if err != nil || native.ParentPID != scope.adapter.PID || native.EffectiveUID != scope.adapter.EffectiveUID || native.PID == scope.adapter.PID {
			return errMCPDiagnosticNotQualified
		}
		native2, err := process.ObserveBirthIdentity(ctx, payload.AppServerPID)
		if err != nil || native2 != native {
			return errMCPDiagnosticNotQualified
		}
		rows := 0
		for _, t := range payload.MCPByThread {
			rows += len(t.Servers)
		}
		report = nativeMCPPartialEvidence{Adapter: scope.adapter, Native: native, ActiveThreads: len(actual), LoadedThreads: len(payload.LoadedThreadIDs), MCPStatusRows: rows, ChildProcessProof: "unavailable", ReclaimAllowed: false}
		return nil
	})
	if err != nil {
		return fail, errMCPDiagnosticNotQualified
	}
	return report, nil
}

// NextCodexAppServerBirths exposes a strict read-only slice of the existing
// authenticated Adapter diagnostic to the private Next Core registrar. It
// must not be used as native per-thread proof or process-control authority.
type NextCodexAppServerBirths struct {
	Adapter   process.BirthIdentity
	AppServer process.BirthIdentity
}

func (m *Manager) NextCodexAppServerBirths(ctx context.Context, key []byte) (NextCodexAppServerBirths, error) {
	report, err := m.readNextNativeMCPPartialEvidenceQualified(ctx, key, true)
	if err != nil || report.ChildProcessProof != "unavailable" || report.ReclaimAllowed {
		return NextCodexAppServerBirths{}, errMCPDiagnosticNotQualified
	}
	return NextCodexAppServerBirths{Adapter: report.Adapter, AppServer: report.Native}, nil
}
