package oauthclient

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	CallbackLocal     = "local"
	CallbackAgentDock = "agentdock"
	CallbackNexus     = "nexus"

	StatusUnauthorized = "unauthorized"
	StatusAuthRequired = "auth_required"
	StatusAuthorizing  = "authorizing"
	StatusAuthorized   = "authorized"
)

type CallbackOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	RedirectURL string `json:"-"`
}

type BeginResult struct {
	FlowID           string           `json:"flow_id,omitempty"`
	AuthorizationURL string           `json:"authorization_url,omitempty"`
	CallbackID       string           `json:"callback_id,omitempty"`
	ExpiresAt        string           `json:"expires_at,omitempty"`
	CallbackOptions  []CallbackOption `json:"callback_options,omitempty"`
}

type FlowStatus struct {
	FlowID    string `json:"flow_id"`
	Status    string `json:"status"`
	ExpiresAt string `json:"expires_at,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

type CallbackResult struct {
	State            string `json:"state"`
	Code             string `json:"code,omitempty"`
	Issuer           string `json:"iss,omitempty"`
	Error            string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
}

type AuthRequiredError struct {
	Server string
}

func (e *AuthRequiredError) Error() string {
	if strings.TrimSpace(e.Server) == "" {
		return "Remote MCP requires OAuth authorization"
	}
	return fmt.Sprintf("Remote MCP %s requires OAuth authorization", e.Server)
}

type FlowError struct {
	Code    string
	Message string
	Cause   error
}

func (e *FlowError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *FlowError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func newFlowError(code, message string, cause error) *FlowError {
	return &FlowError{Code: code, Message: message, Cause: sanitizeOAuthErrorCause(cause)}
}

func sanitizeOAuthErrorCause(cause error) error {
	if errors.Is(cause, errOAuthRedirectRejected) {
		return errOAuthRedirectRejected
	}
	var urlErr *url.Error
	if errors.As(cause, &urlErr) {
		return errOAuthHTTPFailed
	}
	return cause
}

func formatExpiry(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
