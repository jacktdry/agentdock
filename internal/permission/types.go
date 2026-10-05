// Package permission implements Core admission policy and retry-based approval.
// Callers supply trusted facts, identities and fingerprints after validation.
// Desktop mutation authentication and dispatch constraint rechecks belong to the
// caller. This package is not an OS sandbox and never invokes a handler.
package permission

import "time"

const (
	SchemaVersion     = 1
	ReadOnly          = "readonly"
	Rules             = "rules"
	Full              = "full"
	Allow             = "allow"
	Ask               = "ask"
	Deny              = "deny"
	FileNone          = "none"
	FileRead          = "read"
	FileWrite         = "write"
	BoundaryNone      = "none"
	BoundaryWorkspace = "workspace"
	OnRequest         = "on-request"
	Never             = "never"
	Granular          = "granular"
	Defer             = "defer"
	Pending           = "pending"
	ApprovedOnce      = "approved_once"
	Consumed          = "consumed"
	ApprovedWorkspace = "approved_workspace"
	Rejected          = "rejected"
	Expired           = "expired"
	Invalidated       = "invalidated"
	NotDispatched     = "not_dispatched"
	DispatchPending   = "pending"
	Succeeded         = "succeeded"
	Failed            = "failed"
	Unknown           = "unknown"
	MaxStateBytes     = 4 << 20
	MaxHistory        = 512
	MaxPending        = 128
	MaxLiveRequests   = 128
	MaxSources        = 64
	MaxSummaryBytes   = 16 << 10
	MaxScopeBytes     = 4 << 10
	MaxReasonBytes    = 2 << 10
	DefaultExpiry     = 15 * time.Minute
)

type Profile struct {
	Filesystem      string `json:"filesystem"`
	Network         string `json:"network"`
	SandboxBoundary string `json:"sandbox_boundary"`
}
type ApprovalCategories struct {
	FileWrites bool `json:"file_writes"`
	Commands   bool `json:"commands"`
	Network    bool `json:"network"`
	MCP        bool `json:"mcp"`
	Management bool `json:"management"`
	Other      bool `json:"other"`
}
type ApprovalPolicy struct {
	Mode     string              `json:"mode"`
	Granular *ApprovalCategories `json:"granular,omitempty"`
}
type Settings struct {
	Profile  Profile        `json:"permission_profile"`
	Approval ApprovalPolicy `json:"approval_policy"`
	Reviewer string         `json:"approval_reviewer"`
}
type WorkspaceScope struct {
	ID       string    `json:"workspace_id"`
	Mode     string    `json:"mode,omitempty"`
	Settings *Settings `json:"settings,omitempty"`
}

// Rules match exact tool names; an empty action matches every action of that
// tool. Empty WorkspaceID means global. There are no implicit glob patterns.
type Rule struct {
	ID          string `json:"id"`
	Tool        string `json:"tool"`
	Action      string `json:"action,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Effect      string `json:"effect"`
	Reason      string `json:"reason"`
}
type Policy struct {
	SchemaVersion int              `json:"schema_version"`
	Revision      uint64           `json:"revision"`
	GlobalMode    string           `json:"global_mode"`
	Settings      Settings         `json:"settings"`
	Scopes        []WorkspaceScope `json:"scopes"`
	Rules         []Rule           `json:"rules"`
}

// Authenticated and Stable are assertions by trusted Core, never client hints.
type AuthorizationPrincipal struct {
	Kind          string
	ID            string
	Authenticated bool
	Stable        bool
}
type PermissionBinding struct {
	Principal        AuthorizationPrincipal
	RuntimeEpoch     string
	Source           string
	WorkspaceRoot    string
	WorkspaceID      string
	TrustedWorkspace bool
	ACPSessionID     string
	Provider         string
}
type AuditBinding struct {
	CallID             string `json:"call_id"`
	ParentCallID       string `json:"parent_call_id,omitempty"`
	OriginalApprovalID string `json:"original_approval_id,omitempty"`
	OriginalCallID     string `json:"original_call_id,omitempty"`
}

// AuditSafeBinding omits roots and hashes identity values; it cannot authorize.
type AuditSafeBinding struct {
	PrincipalKind string `json:"principal_kind"`
	PrincipalHash string `json:"principal_hash"`
	Source        string `json:"source"`
	WorkspaceHash string `json:"workspace_hash,omitempty"`
	SessionHash   string `json:"session_hash,omitempty"`
	ProviderHash  string `json:"provider_hash,omitempty"`
}
type PermissionFacts struct {
	EffectsKnown            bool
	Filesystem              string
	Network                 bool
	WorkspaceBound          bool
	ReadOnly                bool
	Management              bool
	OpaqueProviderExecution bool
	Commands                bool
	MCP                     bool
	Other                   bool
	OneShotEligible         bool
	// WorkspaceRuleEligible asserts a Core-enforceable non-opaque rule class.
	WorkspaceRuleEligible bool
	Tool                  string
	Action                string
	Reason                string
	Binding               PermissionBinding
	Constraints           []DecisionSource
}
type Effective struct {
	Mode        string   `json:"mode"`
	Settings    Settings `json:"settings"`
	WorkspaceID string   `json:"workspace_id,omitempty"`
}
type DecisionSource struct {
	Kind   string `json:"kind"`
	ID     string `json:"id,omitempty"`
	Effect string `json:"effect"`
	Reason string `json:"reason"`
}
type Decision struct {
	Effect         string           `json:"effect"`
	RuleID         string           `json:"rule_id"`
	Reason         string           `json:"reason"`
	PolicyRevision uint64           `json:"policy_revision"`
	Binding        AuditSafeBinding `json:"binding"`
	Effective      Effective        `json:"effective"`
	Sources        []DecisionSource `json:"sources"`
}

// PreparedRequest contains matching material only, never raw tool arguments.
// Its maps are copied on admission. Call IDs are deliberately absent.
type PreparedRequest struct {
	Fingerprint    string
	Binding        PermissionBinding
	Generations    map[string]string
	Tool           string
	Action         string
	PolicyRevision uint64
	RuntimeEpoch   string
}
type ApprovalRecord struct {
	SchemaVersion         int              `json:"schema_version"`
	Version               uint64           `json:"version"`
	ID                    string           `json:"approval_id"`
	Audit                 AuditBinding     `json:"audit"`
	Binding               AuditSafeBinding `json:"binding"`
	Tool                  string           `json:"tool"`
	Action                string           `json:"action,omitempty"`
	Summary               string           `json:"summary"`
	Scope                 string           `json:"scope"`
	Reason                string           `json:"reason"`
	RuleID                string           `json:"rule_id,omitempty"`
	PolicyRevision        uint64           `json:"policy_revision"`
	GrantedPolicyRevision uint64           `json:"granted_policy_revision,omitempty"`
	RuntimeEpoch          string           `json:"runtime_epoch"`
	Status                string           `json:"status"`
	CreatedAt             time.Time        `json:"created_at"`
	ExpiresAt             time.Time        `json:"expires_at"`
	DecidedAt             *time.Time       `json:"decided_at,omitempty"`
	DecidedBy             string           `json:"decided_by,omitempty"`
	GrantKind             string           `json:"grant_kind"`
	GrantedRuleID         string           `json:"granted_rule_id,omitempty"`
	RetryCallID           string           `json:"retry_call_id,omitempty"`
	DispatchOutcome       string           `json:"dispatch_outcome"`
}

type CreateApprovalInput struct {
	Audit     AuditBinding
	Binding   PermissionBinding
	Facts     PermissionFacts
	Prepared  PreparedRequest
	Summary   string
	Scope     string
	Reason    string
	RuleID    string
	ExpiresAt time.Time
}

type ConsumeInput struct {
	Prepared    PreparedRequest
	RetryCallID string
}

type WorkspaceGrantInput struct {
	Mutation Mutation
}
type State struct {
	SchemaVersion int              `json:"schema_version"`
	Revision      uint64           `json:"revision"`
	RuntimeEpoch  string           `json:"runtime_epoch"`
	Policy        Policy           `json:"policy"`
	History       []ApprovalRecord `json:"history"`
}

// Mutation must be constructed by Core after checking Desktop authority and its
// exact confirmation challenge. Actor is a server identity, never request JSON.
type Mutation struct {
	ApprovalID      string
	ApprovalVersion uint64
	PolicyRevision  uint64
	Actor           string
}

func DefaultPolicy() Policy {
	return Policy{SchemaVersion: SchemaVersion, Revision: 1, GlobalMode: Full,
		Settings: Settings{Profile: Profile{FileWrite, Allow, BoundaryNone}, Approval: ApprovalPolicy{Mode: OnRequest}, Reviewer: Defer},
		Scopes:   []WorkspaceScope{}, Rules: []Rule{}}
}
