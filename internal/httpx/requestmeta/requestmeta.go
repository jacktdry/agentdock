package requestmeta

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

type baseURLKey struct{}
type authPrincipalKey struct{}

type AuthPrincipal struct {
	Kind          string
	ID            string
	Authenticated bool
	Stable        bool
}

func WithBaseURL(ctx context.Context, baseURL string) context.Context {
	if baseURL == "" {
		return ctx
	}
	return context.WithValue(ctx, baseURLKey{}, baseURL)
}

func BaseURL(ctx context.Context) string {
	value, _ := ctx.Value(baseURLKey{}).(string)
	return value
}

func WithAuthPrincipal(ctx context.Context, principal AuthPrincipal) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if principal.Kind == "" || principal.ID == "" || !principal.Authenticated || !principal.Stable {
		return ctx
	}
	return context.WithValue(ctx, authPrincipalKey{}, principal)
}

func AuthPrincipalFromContext(ctx context.Context) (AuthPrincipal, bool) {
	if ctx == nil {
		return AuthPrincipal{}, false
	}
	principal, ok := ctx.Value(authPrincipalKey{}).(AuthPrincipal)
	if !ok || principal.Kind == "" || principal.ID == "" || !principal.Authenticated || !principal.Stable {
		return AuthPrincipal{}, false
	}
	return principal, true
}

func NewStableAuthPrincipal(kind string, identityParts ...string) AuthPrincipal {
	kind = strings.TrimSpace(kind)
	if kind == "" || len(identityParts) == 0 {
		return AuthPrincipal{}
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("agentdock-auth-principal-v1\x00"))
	_, _ = hash.Write([]byte(kind))
	for _, part := range identityParts {
		part = strings.TrimSpace(part)
		if part == "" {
			return AuthPrincipal{}
		}
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(part))
	}
	return AuthPrincipal{
		Kind:          kind,
		ID:            "sha256:" + hex.EncodeToString(hash.Sum(nil)),
		Authenticated: true,
		Stable:        true,
	}
}
