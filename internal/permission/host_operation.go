package permission

import "context"

// HostOperation is a trusted Core-host capability operation submitted by a
// broker boundary such as ACP Browser or Computer Control. The broker owns the
// session/profile/workspace provenance; policy evaluation remains in Core.
type HostOperation struct {
	Tool          string
	Action        string
	Source        string
	SessionID     string
	ProfileID     string
	WorkspaceRoot string
	Payload       any
	// PreparedFingerprint is an optional Core-only request identity. It is used
	// when the display/audit payload is intentionally redacted and therefore
	// cannot safely bind approval identity on its own.
	PreparedFingerprint string
}

// HostOperationFinish settles the M5 execution/admission record after the
// broker operation actually dispatches. It must be safe to call at most once.
type HostOperationFinish func(error)

// HostAdmissionHook is injected by AgentDock Core into host capability brokers.
type HostAdmissionHook func(context.Context, HostOperation) (HostOperationFinish, error)
