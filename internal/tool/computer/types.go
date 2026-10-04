package computer

import "time"

type ProviderID string

type ForegroundPolicy string

type Capability string

type OwnerKind string

const (
	ProviderOrca ProviderID = "orca"

	ForegroundForbidden ForegroundPolicy = "forbidden"
	ForegroundAllowed   ForegroundPolicy = "allowed"

	CapabilityObserve Capability = "observe"
	CapabilityAct     Capability = "act"

	OwnerDirect OwnerKind = "direct"
	OwnerACP    OwnerKind = "acp"
)

type OwnerScope struct {
	Kind              OwnerKind `json:"owner_kind"`
	OwnerTaskID       string    `json:"owner_task_id,omitempty"`
	OwnerACPSessionID string    `json:"owner_acp_session_id,omitempty"`
	OwnerProfileID    string    `json:"owner_profile_id,omitempty"`
}

func (s OwnerScope) equal(other OwnerScope) bool {
	return s.Kind == other.Kind &&
		s.OwnerTaskID == other.OwnerTaskID &&
		s.OwnerACPSessionID == other.OwnerACPSessionID &&
		s.OwnerProfileID == other.OwnerProfileID
}

type SessionMetadata struct {
	SessionID        string           `json:"computer_session_id"`
	Provider         ProviderID       `json:"computer_provider"`
	Owner            OwnerScope       `json:"owner"`
	Capability       Capability       `json:"capability"`
	ForegroundPolicy ForegroundPolicy `json:"foreground_policy"`
	CreatedAt        time.Time        `json:"created_at"`
	LastActiveAt     time.Time        `json:"last_active_at"`
	CleanupState     string           `json:"cleanup_state,omitempty"`
}

type AcquireRequest struct {
	Owner            OwnerScope
	Capability       Capability
	ForegroundPolicy ForegroundPolicy
}

type ObservationRequest struct {
	Action        string
	App           string
	WindowID      *int64
	WindowIndex   *int
	RestoreWindow bool
	NoScreenshot  bool
	Timeout       time.Duration
}

type ActionRequest struct {
	Action           string
	App              string
	WindowID         *int64
	WindowIndex      *int
	ElementIndex     *int
	X                *int
	Y                *int
	FromElementIndex *int
	ToElementIndex   *int
	FromX            *int
	FromY            *int
	ToX              *int
	ToY              *int
	Value            string
	Text             string
	Key              string
	Direction        string
	SecondaryAction  string
	Modifiers        string
	MouseButton      string
	RestoreWindow    bool
	NoScreenshot     bool
	Timeout          time.Duration
}

type AppIdentity struct {
	Name     string `json:"name,omitempty"`
	BundleID string `json:"bundle_id,omitempty"`
	PID      int    `json:"pid,omitempty"`
}

type OperationResult struct {
	Provider           ProviderID       `json:"computer_provider"`
	SessionID          string           `json:"computer_session_id"`
	Action             string           `json:"action_kind"`
	ForegroundPolicy   ForegroundPolicy `json:"foreground_policy"`
	ForegroundRequired bool             `json:"foreground_required"`
	ActiveAppBefore    *AppIdentity     `json:"active_app_before,omitempty"`
	ActiveAppAfter     *AppIdentity     `json:"active_app_after,omitempty"`
	ActionVerification string           `json:"action_verification,omitempty"`
	FocusChanged       bool             `json:"focus_changed"`
	ProviderResult     map[string]any   `json:"provider_result,omitempty"`
}
