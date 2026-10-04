package computer

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"sync"
)

type acpOwner struct {
	sessionID string
	profileID string
	token     string
	scope     OwnerScope
}

type ACPBridge struct {
	broker    *Broker
	mu        sync.Mutex
	byToken   map[string]*acpOwner
	bySession map[string]*acpOwner
	closed    bool
}

func NewACPBridge(broker *Broker) (*ACPBridge, error) {
	if broker == nil {
		return nil, computerError(ErrProviderUnavailable, "ACP computer bridge requires broker", "acp", nil, nil)
	}
	return &ACPBridge{broker: broker, byToken: make(map[string]*acpOwner), bySession: make(map[string]*acpOwner)}, nil
}

func (b *ACPBridge) RegisterSession(sessionID, profileID string) (string, error) {
	return b.RegisterSessionWithToken(sessionID, profileID, "")
}
func (b *ACPBridge) RegisterSessionWithToken(sessionID, profileID, token string) (string, error) {
	if b == nil {
		return "", computerError(ErrProviderUnavailable, "ACP computer bridge unavailable", "acp", nil, nil)
	}
	sessionID = strings.TrimSpace(sessionID)
	profileID = strings.TrimSpace(profileID)
	token = strings.TrimSpace(token)
	if sessionID == "" || profileID == "" {
		return "", computerError(ErrInvalidArgument, "ACP computer owner identity required", "acp", nil, nil)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return "", computerError(ErrProviderUnavailable, "ACP computer bridge is closed", "acp", nil, nil)
	}
	if existing := b.bySession[sessionID]; existing != nil {
		if existing.profileID != profileID || (token != "" && existing.token != token) {
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
	owner := &acpOwner{sessionID: sessionID, profileID: profileID, token: token, scope: OwnerScope{Kind: OwnerACP, OwnerACPSessionID: sessionID, OwnerProfileID: profileID}}
	b.bySession[sessionID] = owner
	b.byToken[token] = owner
	return token, nil
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
	owner, err := b.owner(token)
	if err != nil {
		return SessionMetadata{}, err
	}
	return b.broker.Acquire(AcquireRequest{Owner: owner.scope, Capability: capability, ForegroundPolicy: foreground})
}
func (b *ACPBridge) Observe(ctx context.Context, token, sessionID string, req ObservationRequest) (OperationResult, error) {
	owner, err := b.owner(token)
	if err != nil {
		return OperationResult{}, err
	}
	return b.broker.ObserveOwned(ctx, sessionID, owner.scope, req)
}
func (b *ACPBridge) Act(ctx context.Context, token, sessionID string, req ActionRequest) (OperationResult, error) {
	owner, err := b.owner(token)
	if err != nil {
		return OperationResult{}, err
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
