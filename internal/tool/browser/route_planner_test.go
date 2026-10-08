package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
)

type testConnectorStatus func(context.Context, string) (ConnectorRuntimeStatus, error)

func (f testConnectorStatus) ConnectorStatus(ctx context.Context, id string) (ConnectorRuntimeStatus, error) {
	return f(ctx, id)
}

func plannerFixture(t *testing.T, provider ConnectorStatusProvider) (*RoutePlanner, RequestScope) {
	t.Helper()
	catalog, err := browserpolicy.NewCatalog(browserpolicy.CatalogDefinition{
		Profiles:   []browserpolicy.ProfileDefinition{{ID: "user-edge", Browser: browserpolicy.BrowserEdge, Class: browserpolicy.ProfileAuthenticatedExternal}},
		Connectors: []browserpolicy.ConnectorDefinition{{ID: "edge-ws", ProfileID: "user-edge", Driver: browserpolicy.DriverChromeDevToolsMCPWS, Endpoint: "ws://127.0.0.1:9222/devtools/browser"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	scope := RequestScope{WorkspaceID: "workspace", CanonicalWorkspaceRoot: t.TempDir(), OwnerTaskID: "task", Provenance: ScopeACP}
	policies := []browserpolicy.WorkspaceRootPolicy{{Root: scope.CanonicalWorkspaceRoot, Policy: browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceCompany, Route: browserpolicy.RouteRequiredExternal, RequiredConnectorID: "edge-ws", RequiredProfileID: "user-edge"}}}
	planner, err := NewRoutePlanner(policies, catalog, provider)
	if err != nil {
		t.Fatal(err)
	}
	// Caller mutations cannot weaken the planner's trusted policy snapshot.
	policies[0].Policy = browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceDefault, Route: browserpolicy.RouteManaged}
	return planner, scope
}

func assertRouteCode(t *testing.T, err error, code string) {
	t.Helper()
	var be *Error
	if !errors.As(err, &be) || be.Code != code {
		t.Fatalf("error=%v, want %s", err, code)
	}
}

func TestPlannerCompanyIntentAndCanonicalScope(t *testing.T) {
	planner, scope := plannerFixture(t, nil)
	canonical, err := browserpolicy.CanonicalWorkspaceRoot(scope.CanonicalWorkspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "workspace-alias")
	if err := os.Symlink(scope.CanonicalWorkspaceRoot, alias); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{scope.CanonicalWorkspaceRoot, alias, alias + string(filepath.Separator) + "."} {
		scope.CanonicalWorkspaceRoot = root
		plan, err := planner.Plan(scope, nil)
		if err != nil {
			t.Fatal(err)
		}
		if plan.Scope.CanonicalWorkspaceRoot != canonical || plan.Route != browserpolicy.RouteRequiredExternal || plan.ProfileID != "user-edge" || plan.ConnectorID != "edge-ws" || plan.Browser != BrowserEdge || plan.ProfileClass != ProfileAuthenticatedExternal || plan.Endpoint != "ws://127.0.0.1:9222/devtools/browser" || plan.ProfileTemplate != "" || plan.Engine != EngineChromeDevToolsMCP || plan.EngineVersion != PreferredEngineVersion || plan.Headless || len(plan.RequiredCapabilities) != 4 {
			t.Fatalf("plan=%+v", plan)
		}
		if plan.Ownership != (ResourceOwnership{Process: OwnerExternalPersistent, Profile: OwnerExternalPersistent, Connector: OwnerAgentDockIsolated}) {
			t.Fatalf("ownership=%+v", plan.Ownership)
		}
		decision, err := planner.Resolve(context.Background(), scope, nil)
		assertRouteCode(t, err, ErrRequiredRouteUnavailable)
		if decision != (RouteDecision{}) {
			t.Fatalf("fail-open decision=%+v", decision)
		}
		plan.RequiredCapabilities[0] = "caller-mutation"
		again, err := planner.Plan(scope, nil)
		if err != nil || again.RequiredCapabilities[0] != CapabilityBackgroundPage {
			t.Fatalf("plan mutation: %+v %v", again, err)
		}
	}
	scope.CanonicalWorkspaceRoot = filepath.Join(alias, "missing")
	_, err = planner.Plan(scope, nil)
	assertRouteCode(t, err, ErrScopeRequired)
}

func validRuntimeStatus() ConnectorRuntimeStatus {
	now := time.Now().UTC()
	return ConnectorRuntimeStatus{ConnectorID: "edge-ws", ProfileID: "user-edge", Endpoint: "ws://127.0.0.1:9222/devtools/browser", ObservedAt: now.Add(-time.Second), ExpiresAt: now.Add(3 * time.Second), Healthy: true, Verified: true, Authenticated: true, Engine: EngineChromeDevToolsMCP, EngineVersion: PreferredEngineVersion, Transport: "websocket", Capabilities: requiredRouteCapabilities()}
}

func TestPlannerRuntimeFailClosed(t *testing.T) {
	cases := []struct {
		name        string
		mutate      func(*ConnectorRuntimeStatus)
		providerErr error
	}{
		{name: "valid"},
		{name: "missing status", mutate: func(s *ConnectorRuntimeStatus) { *s = ConnectorRuntimeStatus{} }},
		{name: "wrong connector", mutate: func(s *ConnectorRuntimeStatus) { s.ConnectorID = "other" }},
		{name: "missing connector", mutate: func(s *ConnectorRuntimeStatus) { s.ConnectorID = "" }},
		{name: "wrong profile", mutate: func(s *ConnectorRuntimeStatus) { s.ProfileID = "other" }},
		{name: "missing profile", mutate: func(s *ConnectorRuntimeStatus) { s.ProfileID = "" }},
		{name: "wrong endpoint", mutate: func(s *ConnectorRuntimeStatus) { s.Endpoint += "/other" }},
		{name: "wrong port", mutate: func(s *ConnectorRuntimeStatus) { s.Endpoint = "ws://127.0.0.1:9333/devtools/browser" }},
		{name: "noncanonical endpoint", mutate: func(s *ConnectorRuntimeStatus) { s.Endpoint = "ws://127.0.0.1:09222/devtools/browser" }},
		{name: "missing endpoint", mutate: func(s *ConnectorRuntimeStatus) { s.Endpoint = "" }},
		{name: "zero observation", mutate: func(s *ConnectorRuntimeStatus) { s.ObservedAt = time.Time{} }},
		{name: "zero expiration", mutate: func(s *ConnectorRuntimeStatus) { s.ExpiresAt = time.Time{} }},
		{name: "stale", mutate: func(s *ConnectorRuntimeStatus) { s.ObservedAt = time.Now().UTC().Add(-6 * time.Second) }},
		{name: "future", mutate: func(s *ConnectorRuntimeStatus) { s.ObservedAt = time.Now().UTC().Add(time.Second) }},
		{name: "expired", mutate: func(s *ConnectorRuntimeStatus) { s.ExpiresAt = time.Now().UTC().Add(-time.Millisecond) }},
		{name: "long TTL", mutate: func(s *ConnectorRuntimeStatus) { s.ExpiresAt = s.ObservedAt.Add(5*time.Second + time.Nanosecond) }},
		{name: "reversed times", mutate: func(s *ConnectorRuntimeStatus) { s.ExpiresAt = s.ObservedAt.Add(-time.Second) }},
		{name: "equal times", mutate: func(s *ConnectorRuntimeStatus) { s.ExpiresAt = s.ObservedAt }},
		{name: "non UTC observation", mutate: func(s *ConnectorRuntimeStatus) { s.ObservedAt = s.ObservedAt.In(time.FixedZone("offset", 3600)) }},
		{name: "non UTC expiration", mutate: func(s *ConnectorRuntimeStatus) { s.ExpiresAt = s.ExpiresAt.In(time.FixedZone("offset", 3600)) }},
		{name: "unhealthy", mutate: func(s *ConnectorRuntimeStatus) { s.Healthy = false }},
		{name: "unverified", mutate: func(s *ConnectorRuntimeStatus) { s.Verified = false }},
		{name: "unauthenticated", mutate: func(s *ConnectorRuntimeStatus) { s.Authenticated = false }},
		{name: "native engine", mutate: func(s *ConnectorRuntimeStatus) { s.Engine = EngineNativeCDP }},
		{name: "unknown engine", mutate: func(s *ConnectorRuntimeStatus) { s.Engine = "other" }},
		{name: "wrong version", mutate: func(s *ConnectorRuntimeStatus) { s.EngineVersion = "1.8.0" }},
		{name: "missing version", mutate: func(s *ConnectorRuntimeStatus) { s.EngineVersion = "" }},
		{name: "wrong transport", mutate: func(s *ConnectorRuntimeStatus) { s.Transport = "stdio" }},
		{name: "missing transport", mutate: func(s *ConnectorRuntimeStatus) { s.Transport = "" }},
		{name: "provider failure", providerErr: errors.New("ws://secret-canary:9222/devtools/browser?token=error-canary")},
	}
	for _, capability := range requiredRouteCapabilities() {
		want := capability
		cases = append(cases, struct {
			name        string
			mutate      func(*ConnectorRuntimeStatus)
			providerErr error
		}{name: "missing " + string(want), mutate: func(s *ConnectorRuntimeStatus) {
			s.Capabilities = nil
			for _, c := range requiredRouteCapabilities() {
				if c != want {
					s.Capabilities = append(s.Capabilities, c)
				}
			}
		}})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status := validRuntimeStatus()
			if tc.mutate != nil {
				tc.mutate(&status)
			}
			calls := 0
			provider := testConnectorStatus(func(ctx context.Context, id string) (ConnectorRuntimeStatus, error) {
				calls++
				if id != "edge-ws" || ctx.Value("test-context") != "passed" {
					t.Fatalf("provider args: %s %v", id, ctx)
				}
				return status, tc.providerErr
			})
			planner, scope := plannerFixture(t, provider)
			if _, err := planner.Plan(scope, nil); err != nil || calls != 0 {
				t.Fatalf("Plan queried runtime: %v calls=%d", err, calls)
			}
			decision, err := planner.Resolve(context.WithValue(context.Background(), "test-context", "passed"), scope, nil)
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
			if tc.name != "valid" {
				assertRouteCode(t, err, ErrRequiredRouteUnavailable)
				if decision != (RouteDecision{}) {
					t.Fatalf("fail-open decision=%+v", decision)
				}
				var be *Error
				errors.As(err, &be)
				if be.Cause != nil || strings.Contains(fmt.Sprintf("%+v %+v", err, be.Details), "ws://") || strings.Contains(fmt.Sprintf("%+v %+v", err, be.Details), "error-canary") {
					t.Fatalf("unsanitized error: %+v %+v", err, be.Details)
				}
				return
			}
			if err != nil || decision.Route != browserpolicy.RouteRequiredExternal || decision.Start.Browser != BrowserEdge || decision.Start.ProfileID != "user-edge" || decision.Start.ConnectorID != "edge-ws" || decision.Start.EngineVersion != PreferredEngineVersion || decision.Start.LifecyclePolicy != LifecycleExternal || decision.Start.ProfilePath != "" || decision.Start.ForegroundPolicy != ForegroundForbidden || !decision.Start.BackgroundPage || decision.Start.Ownership != (ResourceOwnership{Process: OwnerExternalPersistent, Profile: OwnerExternalPersistent, Connector: OwnerAgentDockIsolated}) {
				t.Fatalf("decision=%+v error=%v", decision, err)
			}
		})
	}
}

func TestPlannerOverridesAndDefault(t *testing.T) {
	provider := testConnectorStatus(func(context.Context, string) (ConnectorRuntimeStatus, error) { return validRuntimeStatus(), nil })
	planner, scope := plannerFixture(t, provider)
	for _, override := range []*RouteOverride{
		{Route: browserpolicy.RouteManaged, ExplicitUserInstruction: true},
		{Route: browserpolicy.RouteExternal, ConnectorID: "other", ExplicitUserInstruction: true},
		{Route: browserpolicy.RouteExternal, ConnectorID: "edge-ws", ProfileID: "other", ExplicitUserInstruction: true},
		{Route: browserpolicy.RouteExternal, ConnectorID: "edge-ws", Browser: BrowserChrome, ExplicitUserInstruction: true},
		{Route: browserpolicy.RouteExternal, ConnectorID: "edge-ws"},
	} {
		_, err := planner.Plan(scope, override)
		assertRouteCode(t, err, ErrPolicyConflict)
		_, err = planner.Resolve(context.Background(), scope, override)
		assertRouteCode(t, err, ErrPolicyConflict)
	}
	matching := &RouteOverride{Route: browserpolicy.RouteExternal, ConnectorID: "edge-ws", Browser: BrowserEdge, ExplicitUserInstruction: true}
	decision, err := planner.Resolve(context.Background(), scope, matching)
	if err != nil || decision.Route != browserpolicy.RouteRequiredExternal {
		t.Fatalf("decision=%+v error=%v", decision, err)
	}
	scope.CanonicalWorkspaceRoot = t.TempDir()
	planner.status = testConnectorStatus(func(context.Context, string) (ConnectorRuntimeStatus, error) {
		t.Fatal("default queried Edge status")
		return ConnectorRuntimeStatus{}, nil
	})
	plan, err := planner.Plan(scope, nil)
	if err != nil || plan.Route != browserpolicy.RouteManaged || plan.ProfileTemplate != ManagedIsolatedChrome || plan.ProfileID != "" || plan.ConnectorID != "" || plan.Endpoint != "" || plan.Browser != BrowserChrome || plan.ProfileClass != ProfileIsolated || plan.EngineVersion != PreferredEngineVersion || !plan.Headless {
		t.Fatalf("plan=%+v error=%v", plan, err)
	}
	decision, err = planner.Resolve(context.Background(), scope, nil)
	if err != nil || decision.Route != browserpolicy.RouteManaged || !decision.Start.Headless {
		t.Fatalf("decision=%+v error=%v", decision, err)
	}
	_, err = planner.Plan(scope, matching)
	assertRouteCode(t, err, ErrPolicyConflict)
	allowed, err := NewRoutePlanner([]browserpolicy.WorkspaceRootPolicy{{Root: scope.CanonicalWorkspaceRoot, Policy: browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceDefault, Route: browserpolicy.RouteManaged, AllowExplicitExternal: true}}}, planner.catalog, provider)
	if err != nil {
		t.Fatal(err)
	}
	decision, err = allowed.Resolve(context.Background(), scope, matching)
	if err != nil || decision.Route != browserpolicy.RouteExternal || decision.Start.Browser != BrowserEdge {
		t.Fatalf("decision=%+v error=%v", decision, err)
	}
}

func TestPlannerRejectsInvalidPolicyCatalog(t *testing.T) {
	planner, scope := plannerFixture(t, nil)
	bad := []browserpolicy.WorkspaceRootPolicy{{Root: scope.CanonicalWorkspaceRoot, Policy: browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceCompany, Route: browserpolicy.RouteRequiredExternal, RequiredConnectorID: "missing", RequiredProfileID: "user-edge"}}}
	if _, err := NewRoutePlanner(bad, planner.catalog, nil); err == nil {
		t.Fatal("dangling required route accepted")
	}
}

func TestPlannerRuntimeCancellation(t *testing.T) {
	for _, mode := range []string{"before", "deadline before", "during", "derived deadline"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "before" {
				cancel()
			}
			if mode == "deadline before" {
				var stop context.CancelFunc
				ctx, stop = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer stop()
			}
			var calls atomic.Int32
			returned := make(chan struct{}, 1)
			provider := testConnectorStatus(func(providerCtx context.Context, _ string) (ConnectorRuntimeStatus, error) {
				defer func() { returned <- struct{}{} }()
				calls.Add(1)
				deadline, ok := providerCtx.Deadline()
				if !ok || time.Until(deadline) > 2*time.Second {
					t.Fatal("provider context must have a bounded deadline")
				}
				if mode == "derived deadline" {
					// Deliberately return healthy data after the caller deadline.
					<-providerCtx.Done()
				} else {
					cancel()
				}
				return validRuntimeStatus(), nil
			})
			planner, scope := plannerFixture(t, provider)
			if mode == "derived deadline" {
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, 30*time.Millisecond)
				defer stop()
			}
			decision, err := planner.Resolve(ctx, scope, nil)
			assertRouteCode(t, err, ErrRequiredRouteUnavailable)
			wantCalls := 1
			if mode == "before" || mode == "deadline before" {
				wantCalls = 0
			}
			if wantCalls == 1 {
				select {
				case <-returned:
				case <-time.After(time.Second):
					t.Fatal("provider did not finish after cancellation")
				}
			}
			if calls.Load() != int32(wantCalls) || decision != (RouteDecision{}) {
				t.Fatalf("calls=%d decision=%+v", calls.Load(), decision)
			}
		})
	}
}

func TestPlannerNonCooperativeProviderIsolation(t *testing.T) {
	block := make(chan struct{})
	released := false
	returned := make(chan struct{})
	var calls atomic.Int32
	provider := testConnectorStatus(func(context.Context, string) (ConnectorRuntimeStatus, error) {
		if calls.Add(1) == 1 {
			<-block // Malicious source ignores cancellation.
			defer close(returned)
		}
		return validRuntimeStatus(), nil
	})
	planner, scope := plannerFixture(t, provider)
	defer func() {
		if !released {
			close(block)
		}
		if calls.Load() > 0 {
			select {
			case <-returned:
			case <-time.After(time.Second):
				t.Error("blocked provider did not finish during cleanup")
			}
		}
	}()
	for attempt := 0; attempt < 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		type outcome struct {
			decision RouteDecision
			err      error
		}
		done := make(chan outcome, 1)
		go func() {
			d, err := planner.Resolve(ctx, scope, nil)
			done <- outcome{d, err}
		}()
		select {
		case result := <-done:
			cancel()
			assertRouteCode(t, result.err, ErrRequiredRouteUnavailable)
			if result.decision != (RouteDecision{}) || calls.Load() != 1 {
				t.Fatalf("attempt=%d decision=%+v calls=%d", attempt, result.decision, calls.Load())
			}
		case <-time.After(time.Second):
			cancel()
			t.Fatal("Resolve blocked behind non-cooperative provider")
		}
		select {
		case <-returned:
			t.Fatal("provider unexpectedly returned before release")
		default:
		}
	}
	close(block)
	released = true
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	decision, err := planner.Resolve(ctx, scope, nil)
	if err != nil || calls.Load() != 2 || decision.Route != browserpolicy.RouteRequiredExternal || decision.Start.Browser != BrowserEdge {
		t.Fatalf("slot not recovered: decision=%+v calls=%d err=%v", decision, calls.Load(), err)
	}
}

func TestPlannerProviderPanicRecovery(t *testing.T) {
	for _, panicValue := range []any{errors.New("panic-canary ws://secret-endpoint user-canary"), nil} {
		t.Run(fmt.Sprintf("nil=%t", panicValue == nil), func(t *testing.T) {
			var calls atomic.Int32
			provider := testConnectorStatus(func(context.Context, string) (ConnectorRuntimeStatus, error) {
				if calls.Add(1) == 1 {
					panic(panicValue)
				}
				return validRuntimeStatus(), nil
			})
			planner, scope := plannerFixture(t, provider)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			decision, err := planner.Resolve(ctx, scope, nil)
			assertRouteCode(t, err, ErrRequiredRouteUnavailable)
			var be *Error
			errors.As(err, &be)
			if decision != (RouteDecision{}) || be.Cause != nil || be.Details.Reason != "runtime connector verification unavailable" {
				t.Fatalf("panic failed open or escaped: decision=%+v err=%+v", decision, be)
			}
			exposed := fmt.Sprintf("%+v %+v", err, be.Details)
			for _, canary := range []string{"panic-canary", "ws://", "secret-endpoint", "user-canary", "goroutine"} {
				if strings.Contains(exposed, canary) {
					t.Fatalf("panic information escaped: %s", exposed)
				}
			}
			decision, err = planner.Resolve(ctx, scope, nil)
			if err != nil || calls.Load() != 2 || decision.Route != browserpolicy.RouteRequiredExternal || decision.Start.Browser != BrowserEdge {
				t.Fatalf("panic stranded slot: decision=%+v calls=%d err=%v", decision, calls.Load(), err)
			}
		})
	}
}

func TestPlannerRuntimeStatusCopyOwnsCapabilities(t *testing.T) {
	providerStatus := validRuntimeStatus()
	snapshot := copyConnectorRuntimeStatus(providerStatus)
	providerStatus.Capabilities[0] = "provider-mutation"
	if snapshot.Capabilities[0] != CapabilityBackgroundPage {
		t.Fatal("provider mutation changed planner snapshot")
	}
	snapshot.Capabilities[1] = "consumer-mutation"
	if providerStatus.Capabilities[1] != CapabilityNoFocus {
		t.Fatal("planner mutation changed provider storage")
	}
	if snapshot.ConnectorID != providerStatus.ConnectorID || snapshot.ObservedAt != providerStatus.ObservedAt {
		t.Fatal("copy lost evidence envelope")
	}
}

func TestPlannerExplicitExternalPersistentDoesNotRequireAuthentication(t *testing.T) {
	root := t.TempDir()
	catalog, err := browserpolicy.NewCatalog(browserpolicy.CatalogDefinition{
		Profiles: []browserpolicy.ProfileDefinition{
			{ID: "user-chrome", Browser: browserpolicy.BrowserChrome, Class: browserpolicy.ProfileExternalPersistent},
		},
		Connectors: []browserpolicy.ConnectorDefinition{
			{ID: "chrome-ws", ProfileID: "user-chrome", Driver: browserpolicy.DriverChromeDevToolsMCPWS, Endpoint: "ws://127.0.0.1:9333/devtools/browser"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	status := validRuntimeStatus()
	status.ConnectorID, status.ProfileID, status.Endpoint = "chrome-ws", "user-chrome", "ws://127.0.0.1:9333/devtools/browser"
	status.Authenticated = false
	provider := testConnectorStatus(func(_ context.Context, id string) (ConnectorRuntimeStatus, error) {
		if id != "chrome-ws" {
			t.Fatalf("connector=%q", id)
		}
		return status, nil
	})
	planner, err := NewRoutePlanner([]browserpolicy.WorkspaceRootPolicy{{
		Root: root,
		Policy: browserpolicy.BrowserRoutePolicy{
			Class:                 browserpolicy.WorkspaceDefault,
			Route:                 browserpolicy.RouteManaged,
			AllowExplicitExternal: true,
		},
	}}, catalog, provider)
	if err != nil {
		t.Fatal(err)
	}
	scope := RequestScope{WorkspaceID: "personal", CanonicalWorkspaceRoot: root, OwnerTaskID: "task", Provenance: ScopeUpstream}
	override := &RouteOverride{Route: browserpolicy.RouteExternal, ConnectorID: "chrome-ws", Browser: BrowserChrome, ExplicitUserInstruction: true}
	decision, err := planner.Resolve(context.Background(), scope, override)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Route != browserpolicy.RouteExternal || decision.Start.Browser != BrowserChrome || decision.Start.ProfileClass != ProfileExternal || decision.Start.ProfileID != "user-chrome" || decision.Start.ConnectorID != "chrome-ws" {
		t.Fatalf("decision=%+v", decision)
	}
}
