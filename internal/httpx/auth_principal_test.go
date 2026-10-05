package httpx

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/httpx/requestmeta"

	"github.com/uvwt/agentdock/internal/config"
)

func TestAuthenticateRequestStaticBearerProducesStableOpaquePrincipal(t *testing.T) {
	const token = "static-secret-token-value"
	cfg := config.Config{AuthToken: token}
	req := httptest.NewRequest("POST", "http://127.0.0.1/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	first := authenticateRequest(req, cfg, nil)
	second := authenticateRequest(req, cfg, nil)
	if !first.OK || !first.Principal.Authenticated || !first.Principal.Stable || first.Principal.Kind != "static_bearer" {
		t.Fatalf("auth = %#v", first)
	}
	if first.Principal.ID == "" || first.Principal.ID != second.Principal.ID {
		t.Fatalf("principal IDs = %q %q", first.Principal.ID, second.Principal.ID)
	}
	if strings.Contains(first.Principal.ID, token) {
		t.Fatal("principal ID leaked bearer token")
	}

	wrong := httptest.NewRequest("POST", "http://127.0.0.1/mcp", nil)
	wrong.Header.Set("Authorization", "Bearer wrong")
	if got := authenticateRequest(wrong, cfg, nil); got.OK {
		t.Fatalf("wrong token authenticated: %#v", got)
	}
}

func TestPrincipalFingerprintIsDomainSeparated(t *testing.T) {
	value := "same-value"
	if requestmeta.NewStableAuthPrincipal("static_bearer", value).ID == requestmeta.NewStableAuthPrincipal("oauth_grant", value).ID {
		t.Fatal("principal fingerprint did not domain-separate auth kinds")
	}
}
