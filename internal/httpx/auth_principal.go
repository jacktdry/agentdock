package httpx

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	gooauth2 "github.com/go-oauth2/oauth2/v4"
	"github.com/uvwt/agentdock/internal/auth"
	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/httpx/requestmeta"
)

type requestAuthentication struct {
	OK        bool
	Principal requestmeta.AuthPrincipal
}

func authenticateRequest(r *http.Request, cfg config.Config, store *auth.OAuthStore) requestAuthentication {
	if r == nil {
		return requestAuthentication{}
	}
	if cfg.AuthToken != "" && (auth.Bearer{Token: cfg.AuthToken}).Authorized(r) {
		return requestAuthentication{
			OK: true,
			Principal: requestmeta.AuthPrincipal{
				Kind:          "static_bearer",
				ID:            principalFingerprint("static-bearer", cfg.AuthToken),
				Authenticated: true,
				Stable:        true,
			},
		}
	}
	info, ok := validatedOAuthTokenInfo(r, cfg, store)
	if !ok {
		return requestAuthentication{}
	}
	clientID := strings.TrimSpace(info.GetClientID())
	grantID := strings.TrimSpace(info.GetUserID())
	principal := requestmeta.AuthPrincipal{}
	if clientID != "" && grantID != "" {
		principal = requestmeta.AuthPrincipal{
			Kind:          "oauth_grant",
			ID:            principalFingerprint("oauth-grant", clientID+"\x00"+grantID),
			Authenticated: true,
			Stable:        true,
		}
	}
	return requestAuthentication{OK: true, Principal: principal}
}

func validatedOAuthTokenInfo(r *http.Request, cfg config.Config, store *auth.OAuthStore) (gooauth2.TokenInfo, bool) {
	if r == nil || !cfg.OAuthEnabled || store == nil {
		return nil, false
	}
	issuer := issuerFor(cfg, r)
	resource := issuer + "/mcp"
	ctx := auth.WithOAuthRequest(r.Context(), issuer, resource, "")
	info, err := newOAuthProtocolServer(cfg, store).ValidationBearerToken(r.WithContext(ctx))
	return info, err == nil && info != nil
}

func principalFingerprint(domain, value string) string {
	sum := sha256.Sum256([]byte("agentdock-auth-principal-v1\x00" + domain + "\x00" + value))
	return "sha256:" + hex.EncodeToString(sum[:])
}
