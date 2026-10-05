package permission

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	ControlMutationUpdatePolicy      = "update_policy"
	ControlMutationApproveOnce       = "approve_once"
	ControlMutationApproveWorkspace  = "approve_workspace"
	ControlMutationReject            = "reject"
	DesktopControlActor              = "desktop-control"
	MaxConfirmationChallenges        = 128
	defaultConfirmationChallengeTTL  = 2 * time.Minute
	desktopControlCredentialByteSize = 32
)

var (
	ErrControlUnauthorized  = errors.New("desktop control credential is invalid")
	ErrControlMutation      = errors.New("desktop control mutation is invalid")
	ErrConfirmationNotFound = errors.New("permission confirmation challenge not found")
	ErrConfirmationExpired  = errors.New("permission confirmation challenge expired")
	ErrConfirmationMismatch = errors.New("permission confirmation challenge does not match mutation")
	ErrConfirmationLimit    = errors.New("permission confirmation challenge limit reached")
)

// ControlMutationRequest is the normalized authorization subject for a
// permission mutation. Actor is deliberately absent; Core supplies it after
// control authorization succeeds.
type ControlMutationRequest struct {
	Kind            string  `json:"kind"`
	ApprovalID      string  `json:"approval_id,omitempty"`
	ApprovalVersion uint64  `json:"approval_version,omitempty"`
	PolicyRevision  uint64  `json:"policy_revision"`
	Policy          *Policy `json:"policy,omitempty"`
}

// ConfirmationChallenge is safe bounded metadata for the native Desktop
// client. The credential and request fingerprint never appear here.
type ConfirmationChallenge struct {
	SchemaVersion   int       `json:"schema_version"`
	ID              string    `json:"confirmation_id"`
	Kind            string    `json:"kind"`
	ApprovalID      string    `json:"approval_id,omitempty"`
	ApprovalVersion uint64    `json:"approval_version,omitempty"`
	PolicyRevision  uint64    `json:"policy_revision"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type liveConfirmation struct {
	challenge   ConfirmationChallenge
	fingerprint string
}

// ControlAuthority is process-local and intentionally non-persistent. Every
// Core start gets a fresh credential and an empty one-time challenge set.
type ControlAuthority struct {
	mu sync.Mutex

	credential string
	challenges map[string]liveConfirmation
	now        func() time.Time
	ttl        time.Duration
}

func NewControlAuthority() (*ControlAuthority, error) {
	credential, err := newDesktopControlCredential()
	if err != nil {
		return nil, err
	}
	return &ControlAuthority{
		credential: credential,
		challenges: make(map[string]liveConfirmation, MaxConfirmationChallenges),
		now:        func() time.Time { return time.Now().UTC() },
		ttl:        defaultConfirmationChallengeTTL,
	}, nil
}

// Credential returns the per-Core-start secret for trusted native bootstrap
// code only. It must never be exposed through normal MCP/Runtime read APIs.
func (a *ControlAuthority) Credential() string {
	if a == nil {
		return ""
	}
	return a.credential
}

func (a *ControlAuthority) Authenticate(credential string) bool {
	if a == nil || a.credential == "" || credential == "" {
		return false
	}
	left := []byte(a.credential)
	right := []byte(strings.TrimSpace(credential))
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare(left, right) == 1
}

func (a *ControlAuthority) BeginConfirmation(credential string, input ControlMutationRequest) (ConfirmationChallenge, error) {
	if !a.Authenticate(credential) {
		return ConfirmationChallenge{}, ErrControlUnauthorized
	}
	normalized, fingerprint, err := normalizeControlMutation(input)
	if err != nil {
		return ConfirmationChallenge{}, err
	}
	id, err := newOpaqueID("pcnf")
	if err != nil {
		return ConfirmationChallenge{}, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	a.pruneExpiredLocked(now)
	if len(a.challenges) >= MaxConfirmationChallenges {
		return ConfirmationChallenge{}, ErrConfirmationLimit
	}
	challenge := ConfirmationChallenge{
		SchemaVersion:   SchemaVersion,
		ID:              id,
		Kind:            normalized.Kind,
		ApprovalID:      normalized.ApprovalID,
		ApprovalVersion: normalized.ApprovalVersion,
		PolicyRevision:  normalized.PolicyRevision,
		ExpiresAt:       now.Add(a.ttl),
	}
	a.challenges[id] = liveConfirmation{challenge: challenge, fingerprint: fingerprint}
	return challenge, nil
}

// ConsumeConfirmation is at-most-once. Once a valid credential presents an
// existing challenge with a normalized mutation, that challenge is removed
// before match/expiry results are returned, so a failed mutation attempt cannot
// replay or broaden the original confirmation.
func (a *ControlAuthority) ConsumeConfirmation(credential, challengeID string, input ControlMutationRequest) (ControlMutationRequest, error) {
	if !a.Authenticate(credential) {
		return ControlMutationRequest{}, ErrControlUnauthorized
	}
	normalized, fingerprint, err := normalizeControlMutation(input)
	if err != nil {
		return ControlMutationRequest{}, err
	}
	challengeID = strings.TrimSpace(challengeID)
	if challengeID == "" {
		return ControlMutationRequest{}, ErrConfirmationNotFound
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	live, ok := a.challenges[challengeID]
	if !ok {
		return ControlMutationRequest{}, ErrConfirmationNotFound
	}
	delete(a.challenges, challengeID)
	now := a.now()
	if !now.Before(live.challenge.ExpiresAt) {
		return ControlMutationRequest{}, ErrConfirmationExpired
	}
	if live.challenge.Kind != normalized.Kind ||
		live.challenge.ApprovalID != normalized.ApprovalID ||
		live.challenge.ApprovalVersion != normalized.ApprovalVersion ||
		live.challenge.PolicyRevision != normalized.PolicyRevision ||
		!constantTimeStringEqual(live.fingerprint, fingerprint) {
		return ControlMutationRequest{}, ErrConfirmationMismatch
	}
	return normalized, nil
}

func (a *ControlAuthority) pruneExpiredLocked(now time.Time) {
	for id, live := range a.challenges {
		if !now.Before(live.challenge.ExpiresAt) {
			delete(a.challenges, id)
		}
	}
}

func normalizeControlMutation(input ControlMutationRequest) (ControlMutationRequest, string, error) {
	normalized := input
	normalized.Kind = strings.ToLower(strings.TrimSpace(input.Kind))
	normalized.ApprovalID = strings.TrimSpace(input.ApprovalID)
	if normalized.PolicyRevision == 0 {
		return ControlMutationRequest{}, "", fmt.Errorf("%w: policy revision is required", ErrControlMutation)
	}
	switch normalized.Kind {
	case ControlMutationUpdatePolicy:
		if normalized.ApprovalID != "" || normalized.ApprovalVersion != 0 || normalized.Policy == nil {
			return ControlMutationRequest{}, "", fmt.Errorf("%w: update_policy requires only policy_revision and policy", ErrControlMutation)
		}
		if normalized.PolicyRevision == ^uint64(0) {
			return ControlMutationRequest{}, "", fmt.Errorf("%w: policy revision overflow", ErrControlMutation)
		}
		policy, err := cloneControlPolicy(*normalized.Policy)
		if err != nil {
			return ControlMutationRequest{}, "", err
		}
		policy.SchemaVersion = SchemaVersion
		policy.Revision = normalized.PolicyRevision + 1
		if err := ValidatePolicy(policy); err != nil {
			return ControlMutationRequest{}, "", err
		}
		normalized.Policy = &policy
	case ControlMutationApproveOnce, ControlMutationApproveWorkspace, ControlMutationReject:
		if normalized.ApprovalID == "" || normalized.ApprovalVersion == 0 || normalized.Policy != nil {
			return ControlMutationRequest{}, "", fmt.Errorf("%w: approval mutation requires approval id/version and no policy payload", ErrControlMutation)
		}
	default:
		return ControlMutationRequest{}, "", fmt.Errorf("%w: unsupported mutation kind %q", ErrControlMutation, normalized.Kind)
	}

	encoded, err := json.Marshal(normalized)
	if err != nil {
		return ControlMutationRequest{}, "", fmt.Errorf("%w: encode normalized mutation: %v", ErrControlMutation, err)
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("agentdock-permission-control-mutation-v1\x00"))
	_, _ = hash.Write(encoded)
	return normalized, "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func cloneControlPolicy(policy Policy) (Policy, error) {
	encoded, err := json.Marshal(policy)
	if err != nil {
		return Policy{}, fmt.Errorf("%w: encode policy: %v", ErrControlMutation, err)
	}
	var cloned Policy
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return Policy{}, fmt.Errorf("%w: decode policy: %v", ErrControlMutation, err)
	}
	return cloned, nil
}

func newDesktopControlCredential() (string, error) {
	raw := make([]byte, desktopControlCredentialByteSize)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate desktop control credential: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func constantTimeStringEqual(left, right string) bool {
	leftBytes, rightBytes := []byte(left), []byte(right)
	if len(leftBytes) != len(rightBytes) {
		return false
	}
	return subtle.ConstantTimeCompare(leftBytes, rightBytes) == 1
}
