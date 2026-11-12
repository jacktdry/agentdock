//go:build darwin

package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestNextCodexOperatorIdentityOnlyReportsTransportProvenance(t *testing.T) {
	rt, _, _, _ := nextCodexAuthorizationFixture(t)
	for _, ctx := range []context.Context{
		context.Background(),
		context.WithValue(context.Background(), "acceptedPeerKey", map[string]any{"pid": 1, "uid": 0, "verified": true}),
	} {
		reply := rt.NextCodexOperatorIdentity(ctx)
		if reply["ok"] != true || reply["state"] != "desktop_peer_unverified" {
			t.Fatalf("untrusted caller appeared verified: %#v", reply)
		}
		serialized, _ := json.Marshal(reply)
		if strings.Contains(string(serialized), "pid") || strings.Contains(string(serialized), "uid") || strings.Contains(string(serialized), "token") {
			t.Fatalf("operator preflight leaked sender details: %s", serialized)
		}
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	if state := rt.NextCodexOperatorIdentity(context.Background())["state"]; state != "core_closing" {
		t.Fatalf("closed Core exposed an operator: %v", state)
	}
}
