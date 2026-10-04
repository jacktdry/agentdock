package browser

import (
	"context"
	"errors"
	"strings"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

type ExternalWorkerOptions struct {
	Cwd   string
	Route RouteDecision
}

// The caller supplies a trusted RoutePlanner/ResolveRoute decision; company
// browser policy is resolved upstream, never inferred here.
func validateExternalStart(d RouteDecision) error {
	s := d.Start
	if (d.Route != browserpolicy.RouteExternal && d.Route != browserpolicy.RouteRequiredExternal) ||
		(d.Route == browserpolicy.RouteRequiredExternal && s.ProfileClass != ProfileAuthenticatedExternal) {
		return externalLeaseError(ErrPolicyConflict, "", "resolved route is not external or lacks required authentication", nil)
	}
	owned := ResourceOwnership{Process: OwnerExternalPersistent, Profile: OwnerExternalPersistent, Connector: OwnerAgentDockIsolated}
	if s.Engine != EngineChromeDevToolsMCP || s.EngineVersion != PreferredEngineVersion ||
		(s.Browser != BrowserEdge && s.Browser != BrowserChrome && s.Browser != BrowserChromium) ||
		(s.ProfileClass != ProfileExternal && s.ProfileClass != ProfileAuthenticatedExternal) ||
		strings.TrimSpace(s.ProfileID) == "" || strings.TrimSpace(s.ConnectorID) == "" ||
		s.Headless || !s.BackgroundPage || s.ForegroundPolicy != ForegroundForbidden ||
		s.LifecyclePolicy != LifecycleExternal || s.Ownership != owned {
		return externalLeaseError(ErrPolicyConflict, "", "resolved start is not an exclusive external route", nil)
	}
	if err := browserpolicy.ValidateCanonicalBrowserEndpoint(s.Endpoint); err != nil {
		return externalLeaseError(ErrPolicyConflict, "", "external endpoint invalid", err)
	}
	return nil
}

func externalWorkerConfig(options ExternalWorkerOptions) mcpclient.ServerConfig {
	return mcpclient.ServerConfig{Name: ManagedEngineServerName, Transport: mcpclient.TransportStdio, Command: "npx", Cwd: options.Cwd, TimeoutMS: 60000,
		Args: []string{"--yes", ManagedEnginePackageSpec, "--wsEndpoint=" + options.Route.Start.Endpoint, "--experimentalPageIdRouting", "--experimentalStructuredContent", "--no-usage-statistics", "--no-performance-crux"}}
}

// StartExternal starts only a connector, never an external browser or profile.
func (r *WorkerRegistry) StartExternal(ctx context.Context, options ExternalWorkerOptions) (WorkerInfo, error) {
	if err := validateExternalStart(options.Route); err != nil {
		return WorkerInfo{}, err
	}
	return r.startWorker(ctx, externalWorkerConfig(options))
}

// CallExternal binds calls to the immutable worker generation returned at start.
// The worker mutex serializes connector stop with in-flight calls.
func (r *WorkerRegistry) CallExternal(ctx context.Context, expected WorkerInfo, tool string, args map[string]any) (map[string]any, error) {
	w, err := r.worker(expected.WorkerID)
	if err != nil {
		return nil, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.info.State != WorkerReady || expected.State != WorkerReady || w.info.WorkerID != expected.WorkerID ||
		w.info.CreatedAt != expected.CreatedAt || w.info.Session != expected.Session || w.session == nil || w.session.Info() != expected.Session {
		return nil, externalLeaseError(ErrLeaseTargetMismatch, "", "external worker generation differs or is stale", nil)
	}
	switch tool {
	case "list_pages", "new_page", "close_page", "navigate_page", "take_snapshot", "take_screenshot", "evaluate_script", "click", "fill", "press_key":
	default:
		return nil, workerError(ErrEngineSchemaIncompatible, expected.WorkerID, errors.New("tool outside validated contract"))
	}
	result, err := w.session.Call(ctx, tool, args)
	structured, _ := result["structuredContent"].(map[string]any)
	if w.session.Info() != expected.Session || externalReconnected(structured) {
		w.info.State = WorkerFailed
		w.info.Error = "external worker reconnected or changed identity"
		return nil, externalLeaseError(ErrLeaseTargetMismatch, "", w.info.Error, err)
	}
	return result, err
}
