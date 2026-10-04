package browser

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestBrokerErrorContract(t *testing.T) {
	for _, code := range []string{ErrScopeRequired, ErrPolicyConflict, ErrRequiredRouteUnavailable, ErrLeaseOwnerMismatch} {
		t.Run(code, func(t *testing.T) {
			cause := errors.New("cause")
			err := browserError(code, "rejected", "routing", &ErrorDetails{WorkspaceID: "w", ConnectorID: "c", LeaseID: "l", OwnerTaskID: "t", RequiredCapabilities: []ConnectorCapability{CapabilityNoFocus}}, cause)
			if !errors.Is(err, cause) || !strings.Contains(err.Error(), code) {
				t.Fatal(err)
			}
			result := browserFailure(err)
			if result["browser_ok"] != false || result["code"] != code {
				t.Fatalf("%+v", result)
			}
			encoded, e := json.Marshal(result)
			if e != nil {
				t.Fatal(e)
			}
			for _, key := range []string{"workspace_id", "connector_id", "lease_id", "owner_task_id", "required_capabilities"} {
				if !strings.Contains(string(encoded), key) {
					t.Fatalf("missing %s: %s", key, encoded)
				}
			}
		})
	}
	old, err := json.Marshal(ErrorDetails{SessionID: "old"})
	if err != nil || string(old) != `{"session_id":"old"}` {
		t.Fatalf("old details changed: %s %v", old, err)
	}
	if ErrCDPFailed != "CDP_FAILED" || ErrSessionNotFound != "SESSION_NOT_FOUND" {
		t.Fatal("legacy codes changed")
	}
}

func TestCompatibilityBaseline(t *testing.T) {
	baseline := CompatibilityBaseline()
	var native, preferred, edge bool
	for _, r := range baseline {
		if r.Platform == "" || r.Architecture == "" || r.BrowserVersion == "" || r.Transport == "" || r.EngineVersion == "" || r.Status == "" {
			t.Fatalf("incomplete record: %+v", r)
		}
		if r.Engine == EngineNativeCDP {
			native = true
			if !r.Transitional || r.Preferred {
				t.Fatalf("native record: %+v", r)
			}
		}
		if r.Preferred {
			preferred = true
			if r.Engine != EngineChromeDevToolsMCP || r.EngineVersion != "1.7.0" || r.ProfileClass != ProfileIsolated || r.Status != CompatibilityObserved {
				t.Fatalf("preferred record: %+v", r)
			}
		}
		if r.Browser == BrowserEdge {
			edge = true
			if r.Status != CompatibilityUnqualified || len(r.Capabilities) != 0 {
				t.Fatalf("invented Edge qualification: %+v", r)
			}
		}
		if r.Status != CompatibilityUnknown && (r.TestedAt != "2026-10-03" || len(r.Evidence) == 0) {
			t.Fatalf("evidence absent: %+v", r)
		}
	}
	if !native || !preferred || !edge {
		t.Fatal("baseline routes absent")
	}
	baseline[1].Evidence[0] = "mutated"
	if CompatibilityBaseline()[1].Evidence[0] == "mutated" {
		t.Fatal("baseline mutable across callers")
	}
}
