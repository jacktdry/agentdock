package desktopruntime

// NativeCodexReadiness contains only fixed, non-secret Core operator states.
// No PIDs, file names, credentials, or launch arguments can cross this contract.
type NativeCodexReadiness struct {
	OK    bool   `json:"ok"`
	State string `json:"state"`
}

func ValidNativeCodexState(state string) bool {
	switch state {
	case "disabled", "core_closing", "owner_unavailable", "registration_present", "startup_unresolved", "state_untrusted", "recovery_required", "operator_setup_required":
		return true
	}
	return false
}
