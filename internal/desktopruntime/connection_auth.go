package desktopruntime

import "context"

type OAuthPasswordState string

const (
	OAuthPasswordStored      OAuthPasswordState = "stored"
	OAuthPasswordMissing     OAuthPasswordState = "missing"
	OAuthPasswordUnreadable  OAuthPasswordState = "unreadable"
	OAuthPasswordUnavailable OAuthPasswordState = "unavailable"
)

// ConnectionConfig is a safe projection, never an environment or secret map.
// TunnelGeneration is supplied by the integrated Tunnel adapter.
type ConnectionConfig struct {
	CoreEndpoint     string
	Port             int
	PublicOrigin     string
	Mode             string
	OAuthEnabled     bool
	TunnelGeneration string
}

func ReadConnectionConfig(ctx context.Context, root string) (ConnectionConfig, error) {
	return platformReadConnectionConfig(ctx, root)
}

// ReadOAuthPassword never creates, repairs, rotates or persists a credential.
// No underlying file/parse/decryption error is returned across this boundary.
func ReadOAuthPassword(ctx context.Context, root string) (string, OAuthPasswordState) {
	if ctx.Err() != nil {
		return "", OAuthPasswordUnavailable
	}
	return platformReadOAuthPassword(root)
}

// Availability is separate from reveal so ordinary callers never receive
// credential bytes. Unix reads the shared private env file; no secret leaves
// the platform adapter. Unsupported platforms do not decrypt or create files.
func ReadOAuthPasswordState(ctx context.Context, root string) OAuthPasswordState {
	if ctx.Err() != nil {
		return OAuthPasswordUnavailable
	}
	return platformReadOAuthPasswordState(root)
}
