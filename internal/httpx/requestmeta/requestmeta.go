package requestmeta

import "context"

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
