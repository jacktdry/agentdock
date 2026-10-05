package requestmeta

import (
	"context"
	"strings"
	"testing"
)

func TestWithBaseURLRoundTrip(t *testing.T) {
	ctx := context.Background()
	wrapped := WithBaseURL(ctx, "https://agentdock.example/base")
	if got := BaseURL(wrapped); got != "https://agentdock.example/base" {
		t.Fatalf("BaseURL() = %q", got)
	}
}

func TestWithBaseURLEmptyKeepsOriginalContext(t *testing.T) {
	ctx := context.Background()
	wrapped := WithBaseURL(ctx, "")
	if wrapped != ctx {
		t.Fatal("WithBaseURL() wrapped context for empty value")
	}
	if got := BaseURL(wrapped); got != "" {
		t.Fatalf("BaseURL() = %q, want empty", got)
	}
}

func TestWithBaseURLNearestValueWins(t *testing.T) {
	ctx := WithBaseURL(context.Background(), "https://first.example")
	ctx = WithBaseURL(ctx, "https://second.example")
	if got := BaseURL(ctx); got != "https://second.example" {
		t.Fatalf("BaseURL() = %q", got)
	}
}

func TestAuthPrincipalRoundTripRequiresStableAuthenticatedIdentity(t *testing.T) {
	ctx := context.Background()
	principal := AuthPrincipal{Kind: "static_bearer", ID: "sha256:test", Authenticated: true, Stable: true}
	ctx = WithAuthPrincipal(ctx, principal)
	got, ok := AuthPrincipalFromContext(ctx)
	if !ok || got != principal {
		t.Fatalf("principal = %#v ok=%v", got, ok)
	}

	unchanged := WithAuthPrincipal(ctx, AuthPrincipal{Kind: "static_bearer", ID: "missing-flags"})
	if unchanged != ctx {
		t.Fatal("invalid principal unexpectedly wrapped context")
	}
}

func TestNewStableAuthPrincipalIsOpaqueStableAndDomainSeparated(t *testing.T) {
	first := NewStableAuthPrincipal("acp_bridge", "session-a", "codex")
	second := NewStableAuthPrincipal("acp_bridge", "session-a", "codex")
	otherSession := NewStableAuthPrincipal("acp_bridge", "session-b", "codex")
	otherKind := NewStableAuthPrincipal("nexus_device", "session-a", "codex")
	if first.ID == "" || first != second {
		t.Fatalf("principal instability: %#v %#v", first, second)
	}
	if first.ID == otherSession.ID || first.ID == otherKind.ID {
		t.Fatalf("principal collision: %#v %#v %#v", first, otherSession, otherKind)
	}
	if strings.Contains(first.ID, "session-a") || strings.Contains(first.ID, "codex") {
		t.Fatalf("principal leaked identity material: %q", first.ID)
	}
	if got := NewStableAuthPrincipal("acp_bridge", ""); got.ID != "" || got.Authenticated || got.Stable {
		t.Fatalf("empty identity produced principal: %#v", got)
	}
}
