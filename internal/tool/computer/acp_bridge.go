package computer

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"sync"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	"github.com/uvwt/agentdock/internal/permission"
)

type acpOwner struct {
	sessionID     string
	profileID     string
	token         string
	scope         OwnerScope
	workspaceRoot string
}

type ACPBridge struct {
	broker        *Broker
	mu            sync.Mutex
	byToken       map[string]*acpOwner
	bySession     map[string]*acpOwner
	admissionHook permission.HostAdmissionHook
	closed        bool
}

func NewACPBridge(broker *Broker) (*ACPBridge, error) {
	if broker == nil {
		return nil, computerError(ErrProviderUnavailable, "ACP computer bridge requires broker", "acp", nil, nil)
	}
	return &ACPBridge{broker: broker, byToken: make(map[string]*acpOwner), bySession: make(map[string]*acpOwner)}, nil
}

func (b *ACPBridge) RegisterSession(sessionID, profileID string) (string, error) {
	return b.registerSession(sessionID, profileID, "", "")
}
func (b *ACPBridge) RegisterSessionWithToken(sessionID, profileID, token string) (string, error) {
	return b.registerSession(sessionID, profileID, "", token)
}
func (b *ACPBridge) RegisterSessionWithTokenAndWorkspace(sessionID, profileID, cwd, token string) (string, error) {
	return b.registerSession(sessionID, profileID, cwd, token)
}

func (b *ACPBridge) registerSession(sessionID, profileID, cwd, token string) (string, error) {
	if b == nil {
		return "", computerError(ErrProviderUnavailable, "ACP computer bridge unavailable", "acp", nil, nil)
	}
	sessionID = strings.TrimSpace(sessionID)
	profileID = strings.TrimSpace(profileID)
	cwd = strings.TrimSpace(cwd)
	token = strings.TrimSpace(token)
	workspaceRoot := ""
	if cwd != "" {
		root, err := browserpolicy.CanonicalWorkspaceRoot(cwd)
		if err != nil {
			return "", computerError(ErrInvalidArgument, "ACP computer workspace invalid", "acp", nil, err)
		}
		workspaceRoot = root
	}
	if sessionID == "" || profileID == "" {
		return "", computerError(ErrInvalidArgument, "ACP computer owner identity required", "acp", nil, nil)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return "", computerError(ErrProviderUnavailable, "ACP computer bridge is closed", "acp", nil, nil)
	}
	if existing := b.bySession[sessionID]; existing != nil {
		if existing.profileID != profileID || existing.workspaceRoot != workspaceRoot || (token != "" && existing.token != token) {
			return "", computerError(ErrOwnerMismatch, "ACP computer owner scope changed", "acp", &ErrorDetails{OwnerACPSessionID: sessionID}, nil)
		}
		return existing.token, nil
	}
	if token == "" {
		token = rand.Text()
	}
	if existing := b.byToken[token]; existing != nil {
		return "", computerError(ErrOwnerMismatch, "ACP computer capability token already belongs to another session", "acp", &ErrorDetails{OwnerACPSessionID: existing.sessionID}, nil)
	}
	owner := &acpOwner{sessionID: sessionID, profileID: profileID, token: token, workspaceRoot: workspaceRoot, scope: OwnerScope{Kind: OwnerACP, OwnerACPSessionID: sessionID, OwnerProfileID: profileID}}
	b.bySession[sessionID] = owner
	b.byToken[token] = owner
	return token, nil
}

func (b *ACPBridge) SetAdmissionHook(hook permission.HostAdmissionHook) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.admissionHook = hook
	b.mu.Unlock()
}

func (b *ACPBridge) admissionFor(ctx context.Context, owner *acpOwner, action string, payload any) (permission.HostOperationFinish, error) {
	if b == nil || owner == nil {
		return nil, nil
	}
	b.mu.Lock()
	hook := b.admissionHook
	b.mu.Unlock()
	if hook == nil {
		return nil, nil
	}
	return hook(ctx, permission.HostOperation{
		Tool:          "acp_computer",
		Action:        action,
		SessionID:     owner.sessionID,
		ProfileID:     owner.profileID,
		WorkspaceRoot: owner.workspaceRoot,
		Payload:       payload,
	})
}

func (b *ACPBridge) owner(token string) (*acpOwner, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, computerError(ErrProviderUnavailable, "ACP computer bridge is closed", "acp", nil, nil)
	}
	owner := b.byToken[strings.TrimSpace(token)]
	if owner == nil {
		return nil, computerError(ErrOwnerMismatch, "ACP computer capability token invalid", "acp", nil, nil)
	}
	return owner, nil
}

func (b *ACPBridge) Acquire(token string, capability Capability, foreground ForegroundPolicy) (SessionMetadata, error) {
	return b.AcquireContext(context.Background(), token, capability, foreground)
}

func (b *ACPBridge) AcquireContext(ctx context.Context, token string, capability Capability, foreground ForegroundPolicy) (meta SessionMetadata, err error) {
	owner, err := b.owner(token)
	if err != nil {
		return SessionMetadata{}, err
	}
	finish, err := b.admissionFor(ctx, owner, "acquire", map[string]any{"capability": capability, "foreground": foreground})
	if err != nil {
		return SessionMetadata{}, err
	}
	if finish != nil {
		defer func() { finish(err) }()
	}
	return b.broker.Acquire(AcquireRequest{Owner: owner.scope, Capability: capability, ForegroundPolicy: foreground})
}
func (b *ACPBridge) Observe(ctx context.Context, token, sessionID string, req ObservationRequest) (result OperationResult, err error) {
	owner, err := b.owner(token)
	if err != nil {
		return OperationResult{}, err
	}
	finish, err := b.admissionFor(ctx, owner, "observe", map[string]any{"session_id": sessionID, "request": req})
	if err != nil {
		return OperationResult{}, err
	}
	if finish != nil {
		defer func() { finish(err) }()
	}
	return b.broker.ObserveOwned(ctx, sessionID, owner.scope, req)
}
func (b *ACPBridge) Act(ctx context.Context, token, sessionID string, req ActionRequest) (result OperationResult, err error) {
	owner, err := b.owner(token)
	if err != nil {
		return OperationResult{}, err
	}
	finish, err := b.admissionFor(ctx, owner, "act", map[string]any{"session_id": sessionID, "request": req})
	if err != nil {
		return OperationResult{}, err
	}
	if finish != nil {
		defer func() { finish(err) }()
	}
	return b.broker.ActOwned(ctx, sessionID, owner.scope, req)
}
func (b *ACPBridge) Release(token, sessionID string) (SessionMetadata, error) {
	owner, err := b.owner(token)
	if err != nil {
		return SessionMetadata{}, err
	}
	return b.broker.ReleaseOwned(sessionID, owner.scope)
}
func (b *ACPBridge) ReleaseSession(_ context.Context, sessionID string) error {
	b.mu.Lock()
	owner := b.bySession[sessionID]
	if owner == nil {
		b.mu.Unlock()
		return nil
	}
	b.mu.Unlock()
	if err := b.broker.ReleaseOwner(owner.scope); err != nil {
		return err
	}
	b.mu.Lock()
	if current := b.bySession[sessionID]; current == owner {
		delete(b.bySession, sessionID)
		delete(b.byToken, owner.token)
	}
	b.mu.Unlock()
	return nil
}
func (b *ACPBridge) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	sessions := make([]string, 0, len(b.bySession))
	for id := range b.bySession {
		sessions = append(sessions, id)
	}
	b.mu.Unlock()
	var failures []error
	for _, id := range sessions {
		if err := b.ReleaseSession(context.Background(), id); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
