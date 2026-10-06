package desktopapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	acpruntime "github.com/uvwt/agentdock/internal/acp"
	"github.com/uvwt/agentdock/internal/desktopruntime"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

const defaultSharedMemoryMCPURL = "http://127.0.0.1:8766/mcp"

type ACPProfileInfo struct {
	ID            string `json:"id"`
	DisplayName   string `json:"displayName,omitempty"`
	Kind          string `json:"kind"`
	Source        string `json:"source"`
	Command       string `json:"command,omitempty"`
	Enabled       bool   `json:"enabled"`
	Installed     *bool  `json:"installed,omitempty"`
	PackageName   string `json:"packageName,omitempty"`
	LatestVersion string `json:"latestVersion,omitempty"`
	VersionState  string `json:"versionState"`
}

type ACPLifecycleCounts struct {
	Managed             int `json:"managed"`
	Loaded              int `json:"loaded"`
	Running             int `json:"running"`
	Ready               int `json:"ready"`
	IdleManagedIdle     int `json:"idleManagedIdle"`
	IdleManagedEligible int `json:"idleManagedEligible"`
	Closed              int `json:"closed"`
	AutoCloseFailures   int `json:"autoCloseFailures"`
}

type ACPSessionLifecycle struct {
	SessionID            string     `json:"sessionId"`
	Status               string     `json:"status"`
	LifecyclePolicy      string     `json:"lifecyclePolicy"`
	Loaded               bool       `json:"loaded"`
	ActiveRunID          string     `json:"activeRunId,omitempty"`
	PendingInteractions  int        `json:"pendingInteractions"`
	SessionOperations    int        `json:"sessionOperations"`
	LastActiveAt         time.Time  `json:"lastActiveAt"`
	IdleCloseAfterMS     int64      `json:"idleCloseAfterMs"`
	IdleManagedIdle      bool       `json:"idleManagedIdle"`
	IdleManagedEligible  bool       `json:"idleManagedEligible"`
	ClosedAt             *time.Time `json:"closedAt,omitempty"`
	ClosedReason         string     `json:"closedReason,omitempty"`
	AutoCloseAttemptedAt *time.Time `json:"autoCloseAttemptedAt,omitempty"`
	AutoCloseError       string     `json:"autoCloseError,omitempty"`
}

type ACPProcessStatus struct {
	RootPID         int       `json:"rootPid"`
	Alive           *bool     `json:"alive"`
	State           string    `json:"state"`
	DescendantCount *int      `json:"descendantCount"`
	RSSBytes        *uint64   `json:"rssBytes"`
	TreeRSSBytes    *uint64   `json:"treeRssBytes"`
	ObservedAt      time.Time `json:"observedAt"`
	Error           string    `json:"error,omitempty"`
}

type ACPBrowserResources struct {
	LeaseIDs      []string `json:"leaseIds"`
	ActiveLeases  int      `json:"activeLeases"`
	CleanupIssues int      `json:"cleanupIssues"`
}

type ACPComputerResources struct {
	SessionIDs []string `json:"sessionIds"`
	Observe    int      `json:"observeSessions"`
	Act        int      `json:"actSessions"`
}

type ACPBrokerResources struct {
	SessionID string               `json:"sessionId"`
	Browser   ACPBrowserResources  `json:"browser"`
	Computer  ACPComputerResources `json:"computer"`
}

type ACPProfileStatus struct {
	Profile        ACPProfileInfo        `json:"profile"`
	ObservedAt     time.Time             `json:"observedAt,omitempty"`
	Counts         ACPLifecycleCounts    `json:"counts"`
	Sessions       []ACPSessionLifecycle `json:"sessions"`
	AdapterProcess *ACPProcessStatus     `json:"adapterProcess,omitempty"`
	Resources      []ACPBrokerResources  `json:"resources"`
	Error          *APIError             `json:"error,omitempty"`
}

type ACPMemoryStatus struct {
	Endpoint        string    `json:"endpoint"`
	Healthy         bool      `json:"healthy"`
	Version         string    `json:"version,omitempty"`
	Backend         string    `json:"backend,omitempty"`
	StoragePath     string    `json:"storagePath,omitempty"`
	Provider        string    `json:"provider,omitempty"`
	TotalMemories   int       `json:"totalMemories,omitempty"`
	ProtocolVersion string    `json:"protocolVersion"`
	Error           *APIError `json:"error,omitempty"`
}

type ACPStatusResult struct {
	Enabled        bool               `json:"enabled"`
	DefaultProfile string             `json:"defaultProfile,omitempty"`
	Profiles       []ACPProfileStatus `json:"profiles"`
	Memory         ACPMemoryStatus    `json:"memory"`
	Error          *APIError          `json:"error,omitempty"`
}

type ACPLifecycleUpdate struct {
	ProfileID        string `json:"profileId"`
	SessionID        string `json:"sessionId"`
	Policy           string `json:"policy"`
	IdleCloseAfterMS int64  `json:"idleCloseAfterMs,omitempty"`
}

type ACPMutationResult struct {
	Completed bool      `json:"completed"`
	Error     *APIError `json:"error,omitempty"`
}

type ACPService struct {
	runtimeRoot       string
	rootError         error
	readSettings      func(context.Context, string) (desktopruntime.ACPSettings, error)
	readConfiguration func(context.Context, string) (desktopruntime.ACPConfiguration, error)
	saveConfiguration func(context.Context, string, string, desktopruntime.ACPSettings) (desktopruntime.ACPConfiguration, error)
	readAccess        func(context.Context, string) (desktopruntime.LocalCoreAccess, error)
	call              func(context.Context, mcpclient.ServerConfig, string, map[string]any) (map[string]any, error)
	updateClient      *http.Client
	updateSource      desktopruntime.ACPUpdateSource
	memoryURL         string
}

func NewACPService(root string) *ACPService {
	root, err := resolveRuntimeRoot(root)
	return &ACPService{
		runtimeRoot: root, rootError: err,
		readSettings: desktopruntime.ReadACPSettings, readConfiguration: desktopruntime.ReadACPConfiguration, saveConfiguration: desktopruntime.SaveACPSettings,
		readAccess: desktopruntime.ReadLocalCoreAccess, call: mcpclient.CallRemoteTool,
		updateClient: &http.Client{Timeout: 2 * time.Minute}, updateSource: desktopruntime.DefaultACPUpdateSource(),
		memoryURL: defaultSharedMemoryMCPURL,
	}
}

func (s *ACPService) Status(ctx context.Context) ACPStatusResult {
	result := ACPStatusResult{Profiles: []ACPProfileStatus{}, Memory: s.memoryStatus(ctx)}
	if s.rootError != nil {
		result.Error = safeServiceError("acp_root_unavailable", s.rootError)
		return result
	}
	settings, err := s.readSettings(ctx, s.runtimeRoot)
	if err != nil {
		result.Error = safeContextServiceError(ctx, "acp_settings_read_failed", err)
		return result
	}
	result.Enabled, result.DefaultProfile = settings.Enabled, settings.DefaultProfile
	if !settings.Enabled {
		return result
	}
	access, err := s.readAccess(ctx, s.runtimeRoot)
	if err != nil {
		result.Error = safeContextServiceError(ctx, "acp_core_access_failed", err)
		for _, p := range settings.Profiles {
			if p.Enabled {
				result.Profiles = append(result.Profiles, ACPProfileStatus{Profile: desktopACPProfileInfo(p), Error: result.Error})
			}
		}
		return result
	}
	for _, profile := range settings.Profiles {
		if !profile.Enabled {
			continue
		}
		status := ACPProfileStatus{Profile: desktopACPProfileInfo(profile), Sessions: []ACPSessionLifecycle{}, Resources: []ACPBrokerResources{}}
		structured, callErr := s.callCore(ctx, access, "acp_session", map[string]any{"action": "status", "profile_id": profile.ID})
		if callErr != nil {
			status.Error = safeACPServiceError("acp_status_failed", callErr)
			result.Profiles = append(result.Profiles, status)
			continue
		}
		if err := decodeCoreACPStatus(structured, &status, access.AuthToken); err != nil {
			status.Error = safeACPServiceError("acp_status_decode_failed", err)
		}
		result.Profiles = append(result.Profiles, status)
	}
	return result
}

func (s *ACPService) Close(ctx context.Context, profileID, sessionID string) ACPMutationResult {
	return s.mutate(ctx, "close", map[string]any{"action": "close", "profile_id": strings.TrimSpace(profileID), "session_id": strings.TrimSpace(sessionID)})
}

func (s *ACPService) UpdateLifecycle(ctx context.Context, update ACPLifecycleUpdate) ACPMutationResult {
	policy := strings.TrimSpace(update.Policy)
	if policy == "" || update.IdleCloseAfterMS < 0 || update.IdleCloseAfterMS > acpruntime.MaxIdleCloseAfter.Milliseconds() {
		return invalidACPMutation()
	}
	if _, err := acpruntime.NormalizeSessionLifecycleOptions(acpruntime.SessionLifecycleOptions{Policy: acpruntime.SessionLifecyclePolicy(policy), IdleCloseAfter: time.Duration(update.IdleCloseAfterMS) * time.Millisecond}); err != nil {
		return invalidACPMutation()
	}
	args := map[string]any{"action": "update", "profile_id": strings.TrimSpace(update.ProfileID), "session_id": strings.TrimSpace(update.SessionID), "lifecycle_policy": strings.TrimSpace(update.Policy)}
	if update.IdleCloseAfterMS > 0 {
		args["idle_close_after_ms"] = update.IdleCloseAfterMS
	}
	return s.mutate(ctx, "update", args)
}

func (s *ACPService) mutate(ctx context.Context, operation string, args map[string]any) ACPMutationResult {
	if args["profile_id"] == "" || args["session_id"] == "" {
		return invalidACPMutation()
	}
	if s.rootError != nil {
		return ACPMutationResult{Error: safeServiceError("acp_root_unavailable", s.rootError)}
	}
	access, err := s.readAccess(ctx, s.runtimeRoot)
	if err != nil {
		return ACPMutationResult{Error: safeContextServiceError(ctx, "acp_core_access_failed", err)}
	}
	if _, err = s.callCore(ctx, access, "acp_session", args); err != nil {
		return ACPMutationResult{Error: safeACPServiceError("acp_"+operation+"_failed", err)}
	}
	return ACPMutationResult{Completed: true}
}

func invalidACPMutation() ACPMutationResult {
	return ACPMutationResult{Error: NewError("acp_mutation_invalid", "ACP profile, session and lifecycle arguments must be valid", ErrorCategoryValidation, false, nil)}
}

func (s *ACPService) callCore(ctx context.Context, access desktopruntime.LocalCoreAccess, tool string, args map[string]any) (map[string]any, error) {
	headers := map[string]string{}
	if access.AuthToken != "" {
		headers["Authorization"] = "Bearer " + access.AuthToken
	}
	result, err := s.call(ctx, mcpclient.ServerConfig{Name: "desktop-core", Description: "AgentDock Desktop local Core", Transport: mcpclient.TransportStreamableHTTP, URL: access.MCPURL, StaticHeaders: headers, Enabled: true, TimeoutMS: 15000}, tool, args)
	if err != nil {
		return nil, err
	}
	if isError, _ := result["isError"].(bool); isError {
		return nil, errors.New("AgentDock Core rejected the ACP operation")
	}
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		return nil, errors.New("AgentDock Core ACP response omitted structuredContent")
	}
	return structured, nil
}

func desktopACPProfileInfo(profile desktopruntime.ACPProfileSettings) ACPProfileInfo {
	source := "builtin"
	if profile.Kind == "custom" {
		source = "custom"
	}
	info := ACPProfileInfo{ID: profile.ID, DisplayName: profile.DisplayName, Kind: profile.Kind, Source: source, Command: profile.Command, Enabled: profile.Enabled, VersionState: "not_checked"}
	switch profile.Kind {
	case "codex":
		info.PackageName = "@agentclientprotocol/codex-acp"
	case "claude":
		info.PackageName = "@agentclientprotocol/claude-agent-acp"
	}
	if profile.Command != "" {
		installed := false
		if stat, err := os.Stat(profile.Command); err == nil && stat.Mode().IsRegular() {
			installed = true
		}
		info.Installed = &installed
	}
	if profile.Kind == "custom" && strings.Contains(strings.ToLower(filepath.Base(profile.Command)), "antigravity-acp") {
		info.Source = "custom-fork"
	}
	return info
}

// Core response structs deliberately mirror only the allowlisted desktop fields.
type coreACPStatus struct {
	ProfileID   string `json:"profile_id"`
	Diagnostics struct {
		ObservedAt time.Time `json:"observed_at"`
		Counts     struct {
			Managed             int `json:"managed"`
			Loaded              int `json:"loaded"`
			Running             int `json:"running"`
			Ready               int `json:"ready"`
			IdleManagedIdle     int `json:"idle_managed_idle"`
			IdleManagedEligible int `json:"idle_managed_eligible"`
			Closed              int `json:"closed"`
			AutoCloseFailures   int `json:"auto_close_failures"`
		} `json:"counts"`
		Sessions []struct {
			SessionID            string     `json:"session_id"`
			Status               string     `json:"status"`
			LifecyclePolicy      string     `json:"lifecycle_policy"`
			Loaded               bool       `json:"loaded"`
			ActiveRunID          string     `json:"active_run_id"`
			PendingInteractions  int        `json:"pending_interactions"`
			SessionOperations    int        `json:"session_operations"`
			LastActiveAt         time.Time  `json:"last_active_at"`
			IdleCloseAfterMS     int64      `json:"idle_close_after_ms"`
			IdleManagedIdle      bool       `json:"idle_managed_idle"`
			IdleManagedEligible  bool       `json:"idle_managed_eligible"`
			ClosedAt             *time.Time `json:"closed_at"`
			ClosedReason         string     `json:"closed_reason"`
			AutoCloseAttemptedAt *time.Time `json:"auto_close_attempted_at"`
			AutoCloseError       string     `json:"auto_close_error"`
		} `json:"sessions"`
	} `json:"diagnostics"`
	AdapterProcess *struct {
		RootPID         int       `json:"root_pid"`
		Alive           *bool     `json:"alive"`
		State           string    `json:"state"`
		DescendantCount *int      `json:"descendant_count"`
		RSSBytes        *uint64   `json:"rss_bytes"`
		TreeRSSBytes    *uint64   `json:"tree_rss_bytes"`
		ObservedAt      time.Time `json:"observed_at"`
		Error           string    `json:"error"`
	} `json:"adapter_process"`
	BrokerCorrelation struct {
		Sessions []struct {
			SessionID string `json:"session_id"`
			Browser   struct {
				LeaseIDs      []string `json:"lease_ids"`
				ActiveLeases  int      `json:"active_leases"`
				CleanupIssues int      `json:"cleanup_issues"`
			} `json:"browser"`
			Computer struct {
				SessionIDs []string `json:"computer_session_ids"`
				Observe    int      `json:"observe_sessions"`
				Act        int      `json:"act_sessions"`
			} `json:"computer"`
		} `json:"sessions"`
	} `json:"broker_correlation"`
}

func decodeCoreACPStatus(raw map[string]any, status *ACPProfileStatus, token string) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	if token != "" {
		encodedToken, _ := json.Marshal(token)
		data = []byte(strings.ReplaceAll(string(data), string(encodedToken[1:len(encodedToken)-1]), "[redacted]"))
	}
	var core coreACPStatus
	if err = json.Unmarshal(data, &core); err != nil {
		return err
	}
	// Diagnostic errors can contain Adapter output or remote payloads.
	for i := range core.Diagnostics.Sessions {
		if core.Diagnostics.Sessions[i].AutoCloseError != "" {
			core.Diagnostics.Sessions[i].AutoCloseError = "ACP automatic close failed"
		}
	}
	if core.AdapterProcess != nil && core.AdapterProcess.Error != "" {
		core.AdapterProcess.Error = "ACP adapter observation failed"
	}
	status.ObservedAt = core.Diagnostics.ObservedAt
	status.Counts = ACPLifecycleCounts{Managed: core.Diagnostics.Counts.Managed, Loaded: core.Diagnostics.Counts.Loaded, Running: core.Diagnostics.Counts.Running, Ready: core.Diagnostics.Counts.Ready, IdleManagedIdle: core.Diagnostics.Counts.IdleManagedIdle, IdleManagedEligible: core.Diagnostics.Counts.IdleManagedEligible, Closed: core.Diagnostics.Counts.Closed, AutoCloseFailures: core.Diagnostics.Counts.AutoCloseFailures}
	for _, s := range core.Diagnostics.Sessions {
		status.Sessions = append(status.Sessions, ACPSessionLifecycle{SessionID: s.SessionID, Status: s.Status, LifecyclePolicy: s.LifecyclePolicy, Loaded: s.Loaded, ActiveRunID: s.ActiveRunID, PendingInteractions: s.PendingInteractions, SessionOperations: s.SessionOperations, LastActiveAt: s.LastActiveAt, IdleCloseAfterMS: s.IdleCloseAfterMS, IdleManagedIdle: s.IdleManagedIdle, IdleManagedEligible: s.IdleManagedEligible, ClosedAt: s.ClosedAt, ClosedReason: s.ClosedReason, AutoCloseAttemptedAt: s.AutoCloseAttemptedAt, AutoCloseError: s.AutoCloseError})
	}
	if p := core.AdapterProcess; p != nil {
		status.AdapterProcess = &ACPProcessStatus{RootPID: p.RootPID, Alive: p.Alive, State: p.State, DescendantCount: p.DescendantCount, RSSBytes: p.RSSBytes, TreeRSSBytes: p.TreeRSSBytes, ObservedAt: p.ObservedAt, Error: p.Error}
	}
	for _, r := range core.BrokerCorrelation.Sessions {
		status.Resources = append(status.Resources, ACPBrokerResources{SessionID: r.SessionID, Browser: ACPBrowserResources{LeaseIDs: append([]string(nil), r.Browser.LeaseIDs...), ActiveLeases: r.Browser.ActiveLeases, CleanupIssues: r.Browser.CleanupIssues}, Computer: ACPComputerResources{SessionIDs: append([]string(nil), r.Computer.SessionIDs...), Observe: r.Computer.Observe, Act: r.Computer.Act}})
	}
	return nil
}

func (s *ACPService) memoryStatus(ctx context.Context) ACPMemoryStatus {
	status := ACPMemoryStatus{Endpoint: s.memoryURL, ProtocolVersion: "2025-11-25"}
	result, err := s.call(ctx, mcpclient.ServerConfig{Name: "desktop-memory", Description: "AgentDock shared Memory", Transport: mcpclient.TransportStreamableHTTP, ProtocolVersion: "2025-11-25", URL: s.memoryURL, Enabled: true, TimeoutMS: 5000}, "memory_health", map[string]any{})
	if err != nil {
		status.Error = safeACPServiceError("memory_health_failed", err)
		return status
	}
	if isError, _ := result["isError"].(bool); isError {
		status.Error = safeACPServiceError("memory_health_failed", nil)
		return status
	}
	content, _ := result["content"].([]any)
	for _, raw := range content {
		item, _ := raw.(map[string]any)
		text, _ := item["text"].(string)
		if text == "" {
			continue
		}
		var health map[string]any
		if json.Unmarshal([]byte(text), &health) != nil {
			continue
		}
		status.Healthy = health["status"] == "healthy"
		status.Version = stringValue(health["version"])
		status.Backend = stringValue(health["backend"])
		status.StoragePath = stringValue(health["db_path"])
		if status.StoragePath == "" {
			status.StoragePath = stringValue(health["database_path"])
		}
		status.Provider = stringValue(health["embedding_provider"])
		if n, ok := numberInt(health["total_memories"]); ok {
			status.TotalMemories = n
		}
		break
	}
	if !status.Healthy && status.Error == nil {
		status.Error = NewError("memory_unhealthy", "Shared Memory service is not healthy", ErrorCategoryUnavailable, true, nil)
	}
	return status
}

func stringValue(v any) string { s, _ := v.(string); return s }
func numberInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}
func safeACPServiceError(code string, err error) *APIError {
	return safeServiceError(code, err)
}
