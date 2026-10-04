package computer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

type session struct {
	mu       sync.Mutex
	metadata SessionMetadata
	closed   bool
}

type Broker struct {
	provider Provider

	mu       sync.RWMutex
	sessions map[string]*session
	released map[string]SessionMetadata
	closed   bool

	// Native GUI state is process-global from the user's point of view. Keep
	// provider operations serialized until a provider proves stronger isolation.
	providerMu sync.Mutex

	diagMu     sync.Mutex
	diagEvents []DiagnosticEvent
}

func NewBroker(provider Provider) (*Broker, error) {
	if provider == nil {
		return nil, computerError(ErrProviderUnavailable, "computer control provider is required", "init", nil, nil)
	}
	return &Broker{provider: provider, sessions: make(map[string]*session), released: make(map[string]SessionMetadata)}, nil
}

func (b *Broker) Acquire(req AcquireRequest) (SessionMetadata, error) {
	if b == nil || b.provider == nil {
		return SessionMetadata{}, computerError(ErrProviderUnavailable, "computer control broker unavailable", "acquire", nil, nil)
	}
	if req.Capability == "" {
		req.Capability = CapabilityObserve
	}
	if req.Capability != CapabilityObserve && req.Capability != CapabilityAct {
		return SessionMetadata{}, invalidArgument("unsupported computer capability", string(req.Capability))
	}
	if req.ForegroundPolicy == "" {
		req.ForegroundPolicy = ForegroundForbidden
	}
	if req.ForegroundPolicy != ForegroundForbidden && req.ForegroundPolicy != ForegroundAllowed {
		return SessionMetadata{}, invalidArgument("unsupported foreground policy", string(req.ForegroundPolicy))
	}
	if req.Owner.Kind == "" {
		req.Owner.Kind = OwnerDirect
	}
	if req.Owner.Kind != OwnerDirect && req.Owner.Kind != OwnerACP {
		return SessionMetadata{}, invalidArgument("unsupported computer owner kind", string(req.Owner.Kind))
	}
	if req.Owner.Kind == OwnerACP && (strings.TrimSpace(req.Owner.OwnerACPSessionID) == "" || strings.TrimSpace(req.Owner.OwnerProfileID) == "") {
		return SessionMetadata{}, computerError(ErrInvalidArgument, "ACP computer owner requires session and profile identity", "acquire", &ErrorDetails{OwnerACPSessionID: req.Owner.OwnerACPSessionID}, nil)
	}
	id, err := newComputerID()
	if err != nil {
		return SessionMetadata{}, computerError(ErrProviderFailed, "generate computer session id", "acquire", nil, err)
	}
	now := time.Now().UTC()
	meta := SessionMetadata{
		SessionID: id, Provider: b.provider.ID(), Owner: req.Owner, Capability: req.Capability,
		ForegroundPolicy: req.ForegroundPolicy, CreatedAt: now, LastActiveAt: now,
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return SessionMetadata{}, computerError(ErrProviderUnavailable, "computer control broker is closed", "acquire", nil, nil)
	}
	b.sessions[id] = &session{metadata: meta}
	return meta, nil
}

func (b *Broker) ObserveDirect(ctx context.Context, sessionID string, req ObservationRequest) (OperationResult, error) {
	return b.observe(ctx, sessionID, OwnerScope{Kind: OwnerDirect}, false, req)
}

func (b *Broker) ObserveOwned(ctx context.Context, sessionID string, owner OwnerScope, req ObservationRequest) (OperationResult, error) {
	return b.observe(ctx, sessionID, owner, true, req)
}

func (b *Broker) observe(ctx context.Context, sessionID string, owner OwnerScope, exactOwner bool, req ObservationRequest) (OperationResult, error) {
	s, err := b.sessionFor(sessionID, owner, exactOwner)
	if err != nil {
		return OperationResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return OperationResult{}, sessionNotFound(sessionID)
	}
	foregroundRequired := req.RestoreWindow || req.Action == "permissions"
	if foregroundRequired && s.metadata.ForegroundPolicy != ForegroundAllowed {
		err := foregroundRequiredError(s.metadata, req.Action)
		b.recordDiagnostic(DiagnosticEvent{Kind: "foreground_denied", SessionID: sessionID, Action: req.Action, Message: err.Error()})
		return OperationResult{}, err
	}
	opCtx, cancel := computerOperationContext(ctx, req.Timeout)
	defer cancel()
	b.providerMu.Lock()
	defer b.providerMu.Unlock()
	before, _ := b.provider.FrontmostApp(opCtx)
	providerResult, err := b.provider.Observe(opCtx, req)
	if err != nil {
		b.recordDiagnostic(DiagnosticEvent{Kind: "provider_failure", SessionID: sessionID, Action: req.Action, Message: err.Error(), ActiveAppBefore: before})
		return OperationResult{}, err
	}
	after, _ := b.provider.FrontmostApp(opCtx)
	focusChanged := appIdentityChanged(before, after)
	s.metadata.LastActiveAt = time.Now().UTC()
	if focusChanged && !foregroundRequired {
		err := computerError(ErrFocusViolation, "background computer observation changed the active app", "focus", &ErrorDetails{SessionID: sessionID, Provider: b.provider.ID(), Action: req.Action, ForegroundPolicy: s.metadata.ForegroundPolicy}, nil)
		b.recordDiagnostic(DiagnosticEvent{Kind: "focus_violation", SessionID: sessionID, Action: req.Action, Message: err.Error(), ActiveAppBefore: before, ActiveAppAfter: after})
		return OperationResult{}, err
	}
	return OperationResult{
		Provider: b.provider.ID(), SessionID: sessionID, Action: req.Action,
		ForegroundPolicy: s.metadata.ForegroundPolicy, ForegroundRequired: foregroundRequired,
		ActiveAppBefore: before, ActiveAppAfter: after, FocusChanged: focusChanged, ProviderResult: providerResult,
	}, nil
}

func (b *Broker) ActDirect(ctx context.Context, sessionID string, req ActionRequest) (OperationResult, error) {
	return b.act(ctx, sessionID, OwnerScope{Kind: OwnerDirect}, false, req)
}

func (b *Broker) ActOwned(ctx context.Context, sessionID string, owner OwnerScope, req ActionRequest) (OperationResult, error) {
	return b.act(ctx, sessionID, owner, true, req)
}

func (b *Broker) act(ctx context.Context, sessionID string, owner OwnerScope, exactOwner bool, req ActionRequest) (OperationResult, error) {
	s, err := b.sessionFor(sessionID, owner, exactOwner)
	if err != nil {
		return OperationResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return OperationResult{}, sessionNotFound(sessionID)
	}
	if s.metadata.Capability != CapabilityAct {
		return OperationResult{}, computerError(ErrCapabilityDenied, "computer session was not acquired for actions", "policy", &ErrorDetails{SessionID: sessionID, Action: req.Action}, nil)
	}
	if s.metadata.ForegroundPolicy != ForegroundAllowed {
		err := foregroundRequiredError(s.metadata, req.Action)
		b.recordDiagnostic(DiagnosticEvent{Kind: "foreground_denied", SessionID: sessionID, Action: req.Action, Message: err.Error()})
		return OperationResult{}, err
	}
	opCtx, cancel := computerOperationContext(ctx, req.Timeout)
	defer cancel()
	b.providerMu.Lock()
	defer b.providerMu.Unlock()
	before, _ := b.provider.FrontmostApp(opCtx)
	providerResult, err := b.provider.Act(opCtx, req)
	if err != nil {
		b.recordDiagnostic(DiagnosticEvent{Kind: "provider_failure", SessionID: sessionID, Action: req.Action, Message: err.Error(), ActiveAppBefore: before})
		return OperationResult{}, err
	}
	after, _ := b.provider.FrontmostApp(opCtx)
	s.metadata.LastActiveAt = time.Now().UTC()
	return OperationResult{
		Provider: b.provider.ID(), SessionID: sessionID, Action: req.Action,
		ForegroundPolicy: s.metadata.ForegroundPolicy, ForegroundRequired: true,
		ActiveAppBefore: before, ActiveAppAfter: after, FocusChanged: appIdentityChanged(before, after),
		ActionVerification: verificationSummary(providerResult), ProviderResult: providerResult,
	}, nil
}

func (b *Broker) ReleaseDirect(sessionID string) (SessionMetadata, error) {
	return b.release(sessionID, OwnerScope{Kind: OwnerDirect}, false)
}

func (b *Broker) ReleaseOwned(sessionID string, owner OwnerScope) (SessionMetadata, error) {
	return b.release(sessionID, owner, true)
}

func (b *Broker) release(sessionID string, owner OwnerScope, exactOwner bool) (SessionMetadata, error) {
	b.mu.Lock()
	if released, ok := b.released[sessionID]; ok {
		b.mu.Unlock()
		if exactOwner {
			if !released.Owner.equal(owner) {
				return SessionMetadata{}, computerError(ErrOwnerMismatch, "computer session owner mismatch", "ownership", &ErrorDetails{SessionID: sessionID, OwnerACPSessionID: owner.OwnerACPSessionID, OwnerTaskID: owner.OwnerTaskID}, nil)
			}
		} else if released.Owner.Kind != OwnerDirect {
			return SessionMetadata{}, computerError(ErrOwnerMismatch, "computer session is not owned by a direct caller", "ownership", &ErrorDetails{SessionID: sessionID}, nil)
		}
		return released, nil
	}
	s := b.sessions[sessionID]
	if s == nil {
		b.mu.Unlock()
		return SessionMetadata{}, sessionNotFound(sessionID)
	}
	s.mu.Lock()
	if exactOwner {
		if !s.metadata.Owner.equal(owner) {
			s.mu.Unlock()
			b.mu.Unlock()
			return SessionMetadata{}, computerError(ErrOwnerMismatch, "computer session owner mismatch", "ownership", &ErrorDetails{SessionID: sessionID, OwnerACPSessionID: owner.OwnerACPSessionID, OwnerTaskID: owner.OwnerTaskID}, nil)
		}
	} else if s.metadata.Owner.Kind != OwnerDirect {
		s.mu.Unlock()
		b.mu.Unlock()
		return SessionMetadata{}, computerError(ErrOwnerMismatch, "computer session is not owned by a direct caller", "ownership", &ErrorDetails{SessionID: sessionID}, nil)
	}
	s.closed = true
	s.metadata.CleanupState = "complete"
	meta := s.metadata
	delete(b.sessions, sessionID)
	b.released[sessionID] = meta
	s.mu.Unlock()
	b.mu.Unlock()
	return meta, nil
}

func (b *Broker) ReleaseOwner(owner OwnerScope) error {
	b.mu.RLock()
	ids := make([]string, 0)
	for id, s := range b.sessions {
		s.mu.Lock()
		matches := s.metadata.Owner.equal(owner)
		s.mu.Unlock()
		if matches {
			ids = append(ids, id)
		}
	}
	b.mu.RUnlock()
	for _, id := range ids {
		if _, err := b.ReleaseOwned(id, owner); err != nil {
			return err
		}
	}
	return nil
}

func (b *Broker) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	for id, s := range b.sessions {
		s.mu.Lock()
		s.closed = true
		s.metadata.CleanupState = "complete"
		b.released[id] = s.metadata
		s.mu.Unlock()
		delete(b.sessions, id)
	}
	b.mu.Unlock()
	return nil
}

func (b *Broker) sessionFor(sessionID string, owner OwnerScope, exactOwner bool) (*session, error) {
	b.mu.RLock()
	s := b.sessions[sessionID]
	b.mu.RUnlock()
	if s == nil {
		return nil, sessionNotFound(sessionID)
	}
	if exactOwner {
		if !s.metadata.Owner.equal(owner) {
			return nil, computerError(ErrOwnerMismatch, "computer session owner mismatch", "ownership", &ErrorDetails{SessionID: sessionID, OwnerACPSessionID: owner.OwnerACPSessionID, OwnerTaskID: owner.OwnerTaskID}, nil)
		}
	} else if s.metadata.Owner.Kind != OwnerDirect {
		return nil, computerError(ErrOwnerMismatch, "computer session is not owned by a direct caller", "ownership", &ErrorDetails{SessionID: sessionID}, nil)
	}
	return s, nil
}

func computerOperationContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = defaultProviderTimeout
	}
	if timeout > 5*time.Minute {
		timeout = 5 * time.Minute
	}
	return context.WithTimeout(parent, timeout)
}

func appIdentityChanged(before, after *AppIdentity) bool {
	if before == nil || after == nil {
		return false
	}
	return before.Name != after.Name || before.BundleID != after.BundleID || before.PID != after.PID
}

func newComputerID() (string, error) {
	var bytes [12]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return "computer_" + hex.EncodeToString(bytes[:]), nil
}

func sessionNotFound(sessionID string) *Error {
	return computerError(ErrSessionNotFound, "computer session not found", "session", &ErrorDetails{SessionID: sessionID}, nil)
}

func foregroundRequiredError(meta SessionMetadata, action string) *Error {
	return computerError(ErrForegroundRequired, "computer action requires foreground control but policy forbids it", "policy", &ErrorDetails{SessionID: meta.SessionID, Provider: meta.Provider, Action: action, ForegroundPolicy: meta.ForegroundPolicy}, nil)
}

func verificationSummary(result map[string]any) string {
	if result == nil {
		return "unverified"
	}
	if value, ok := result["verification"].(string); ok && strings.TrimSpace(value) != "" {
		return value
	}
	if value, ok := result["verification"].(map[string]any); ok {
		for _, key := range []string{"status", "state", "result"} {
			if text, ok := value[key].(string); ok && strings.TrimSpace(text) != "" {
				return text
			}
		}
	}
	return "unverified"
}
