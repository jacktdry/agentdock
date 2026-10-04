package browser

type CompatibilityStatus string

const (
	CompatibilityObserved    CompatibilityStatus = "observed-partial"
	CompatibilityUnknown     CompatibilityStatus = "unknown"
	CompatibilityUnqualified CompatibilityStatus = "unqualified"
)

type CompatibilityRecord struct {
	Platform       string
	Architecture   string
	Browser        Kind
	BrowserVersion string
	ProfileClass   ProfileClass
	Transport      string
	Engine         EngineKind
	EngineVersion  string
	Preferred      bool
	Transitional   bool
	Capabilities   []ConnectorCapability
	TestedAt       string // Date or empty when not tested; unknown versions stay explicit.
	Evidence       []string
	Status         CompatibilityStatus
	Notes          string
}

// CompatibilityBaseline records local documentary evidence, not qualification.
// Unknown architecture/browser versions were not recorded in the bake-off.
// Background-page evidence alone does not establish the full no-focus route.
func CompatibilityBaseline() []CompatibilityRecord {
	evidence := []string{"docs/custom/browser-cdp-lifecycle.md#2026-10-03-browser-backend-bake-off", "docs/custom/roadmap.md#m6--browser-broker"}
	return []CompatibilityRecord{
		{Platform: "unknown", Architecture: "unknown", Browser: BrowserAuto, BrowserVersion: "unknown", ProfileClass: ProfileIsolated, Transport: "native-cdp", Engine: EngineNativeCDP, EngineVersion: "repository-pinned", Transitional: true, Status: CompatibilityUnknown, Notes: "Existing native implementation preserved; no full M6 qualification."},
		{Platform: "darwin", Architecture: "unknown", Browser: BrowserChrome, BrowserVersion: "unknown", ProfileClass: ProfileIsolated, Transport: "managed", Engine: EngineChromeDevToolsMCP, EngineVersion: PreferredEngineVersion, Preferred: true, TestedAt: "2026-10-03", Evidence: append([]string(nil), evidence...), Status: CompatibilityObserved, Notes: "Isolated headless operation and owned process reclamation observed; lease concurrency/no-focus qualification pending."},
		{Platform: "darwin", Architecture: "unknown", Browser: BrowserChrome, BrowserVersion: "unknown", ProfileClass: ProfileExternal, Transport: "remote-debugging", Engine: EngineChromeDevToolsMCP, EngineVersion: PreferredEngineVersion, TestedAt: "2026-10-03", Evidence: append([]string(nil), evidence...), Status: CompatibilityObserved, Notes: "Attach and external process preservation observed; exact default-profile route not qualified."},
		{Platform: "darwin", Architecture: "unknown", Browser: BrowserEdge, BrowserVersion: "unknown", ProfileClass: ProfileAuthenticatedExternal, Transport: "websocket", Engine: EngineChromeDevToolsMCP, EngineVersion: PreferredEngineVersion, TestedAt: "2026-10-03", Evidence: append([]string(nil), evidence...), Status: CompatibilityUnqualified, Notes: "Attach observed; background page observed separately. Full authenticated default-profile/no-focus/lease-target workflow not proven."},
	}
}
