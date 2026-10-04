package browser

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

func TestExternalWorkerConfig(t *testing.T) {
	options := ExternalWorkerOptions{Cwd: "/workspace", Route: externalRoute(externalStart())}
	cfg := externalWorkerConfig(options)
	want := []string{"--yes", "chrome-devtools-mcp@1.7.0", "--wsEndpoint=" + options.Route.Start.Endpoint, "--experimentalPageIdRouting", "--experimentalStructuredContent", "--no-usage-statistics", "--no-performance-crux"}
	if cfg.Command != "npx" || cfg.Transport != mcpclient.TransportStdio || cfg.Cwd != options.Cwd || cfg.TimeoutMS != 60000 || cfg.Name != ManagedEngineServerName || !reflect.DeepEqual(cfg.Args, want) {
		t.Fatalf("unsafe external config: %+v", cfg)
	}
	s := fakeManagedSession(t, "external")
	probed, opened := false, false
	r := NewWorkerRegistry(WorkerDependencies{Probe: func(_ context.Context, actual mcpclient.ServerConfig) (string, error) {
		probed = true
		if !reflect.DeepEqual(actual, cfg) {
			t.Fatal("wrong probe config")
		}
		return PreferredEngineVersion, nil
	}, Open: func(_ context.Context, actual mcpclient.ServerConfig) (WorkerSession, error) {
		opened = true
		if !reflect.DeepEqual(actual, cfg) {
			t.Fatal("wrong launch config")
		}
		return s, nil
	}})
	w, err := r.StartExternal(context.Background(), options)
	if err != nil || !probed || !opened || !w.Compatibility.PageIDRouting {
		t.Fatalf("%+v %v", w, err)
	}
	if err := r.Stop(w.WorkerID); err != nil {
		t.Fatal(err)
	}
	if s.closes != 1 {
		t.Fatal("connector not closed exactly once")
	}
}

func TestExternalWorkerUsesManagedValidation(t *testing.T) {
	for _, name := range []string{"probe", "name", "version", "schema", "partial open"} {
		t.Run(name, func(t *testing.T) {
			s := fakeManagedSession(t, "external")
			code := ErrEngineVersionIncompatible
			switch name {
			case "name":
				s.info.ServerName = "other"
			case "version":
				s.info.ServerVersion = "1.8.0"
			case "schema":
				delete(s.tools, "close_page")
				code = ErrEngineSchemaIncompatible
			case "partial open":
				code = ErrEngineUnavailable
			}
			r := NewWorkerRegistry(WorkerDependencies{Probe: func(context.Context, mcpclient.ServerConfig) (string, error) {
				if name == "probe" {
					return "1.8.0", nil
				}
				return PreferredEngineVersion, nil
			}, Open: func(context.Context, mcpclient.ServerConfig) (WorkerSession, error) {
				if name == "partial open" {
					return s, errors.New("open failed")
				}
				return s, nil
			}})
			w, err := r.StartExternal(context.Background(), ExternalWorkerOptions{Route: externalRoute(externalStart())})
			assertBrowserCode(t, err, code)
			if w.State != WorkerFailed || name != "probe" && !s.closed || name == "probe" && s.closed {
				t.Fatal("invalid worker published or cleanup wrong")
			}
			if err := r.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type externalIdentitySession struct {
	*fakeWorkerSession
	identityMu       sync.Mutex
	observed         mcpclient.SessionInfo
	changeDuringCall bool
	reconnect        bool
	entered, gate    chan struct{}
}

func (s *externalIdentitySession) Info() mcpclient.SessionInfo {
	s.identityMu.Lock()
	defer s.identityMu.Unlock()
	return s.observed
}
func (s *externalIdentitySession) Call(ctx context.Context, tool string, args map[string]any) (map[string]any, error) {
	if s.entered != nil {
		close(s.entered)
		<-s.gate
	}
	s.identityMu.Lock()
	defer s.identityMu.Unlock()
	if s.changeDuringCall {
		s.observed.PID++
	}
	if s.reconnect {
		return map[string]any{"structuredContent": map[string]any{"reconnected": true}}, nil
	}
	return s.fakeWorkerSession.Call(ctx, tool, args)
}

func TestExternalWorkerGeneration(t *testing.T) {
	for _, name := range []string{"stopped", "creation generation", "session generation", "cross worker", "live identity", "during call", "reconnected"} {
		t.Run(name, func(t *testing.T) {
			fake := fakeManagedSession(t, "external")
			s := &externalIdentitySession{fakeWorkerSession: fake, observed: fake.info}
			r := NewWorkerRegistry(WorkerDependencies{Probe: func(context.Context, mcpclient.ServerConfig) (string, error) { return PreferredEngineVersion, nil }, Open: func(context.Context, mcpclient.ServerConfig) (WorkerSession, error) { return s, nil }})
			w, err := r.StartExternal(context.Background(), ExternalWorkerOptions{Route: externalRoute(externalStart())})
			if err != nil {
				t.Fatal(err)
			}
			expected := w
			switch name {
			case "stopped":
				if err := r.Stop(w.WorkerID); err != nil {
					t.Fatal(err)
				}
			case "creation generation":
				expected.CreatedAt = expected.CreatedAt.Add(time.Second)
			case "session generation":
				expected.Session.PID++
			case "cross worker":
				other := fakeManagedSession(t, "other")
				r.deps.Open = func(context.Context, mcpclient.ServerConfig) (WorkerSession, error) { return other, nil }
				second, err := r.StartExternal(context.Background(), ExternalWorkerOptions{Route: externalRoute(externalStart())})
				if err != nil {
					t.Fatal(err)
				}
				expected.WorkerID = second.WorkerID
			case "live identity":
				s.observed.PID++
			case "during call":
				s.changeDuringCall = true
			case "reconnected":
				s.reconnect = true
			}
			result, err := r.CallExternal(context.Background(), expected, "click", map[string]any{"pageId": float64(7), "uid": "u"})
			assertBrowserCode(t, err, ErrLeaseTargetMismatch)
			if result != nil {
				t.Fatal("stale result exposed")
			}
			if name == "during call" || name == "reconnected" {
				s.changeDuringCall = false
				s.reconnect = false
				_, err = r.CallExternal(context.Background(), w, "close_page", map[string]any{"pageId": float64(7)})
				assertBrowserCode(t, err, ErrLeaseTargetMismatch)
			}
			if err := r.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestExternalWorkerStopWaitsForOperation(t *testing.T) {
	fake := fakeManagedSession(t, "external")
	s := &externalIdentitySession{fakeWorkerSession: fake, observed: fake.info, entered: make(chan struct{}), gate: make(chan struct{})}
	r := NewWorkerRegistry(WorkerDependencies{Probe: func(context.Context, mcpclient.ServerConfig) (string, error) { return PreferredEngineVersion, nil }, Open: func(context.Context, mcpclient.ServerConfig) (WorkerSession, error) { return s, nil }})
	w, err := r.StartExternal(context.Background(), ExternalWorkerOptions{Route: externalRoute(externalStart())})
	if err != nil {
		t.Fatal(err)
	}
	called := make(chan error, 1)
	stopped := make(chan error, 1)
	go func() {
		_, err := r.CallExternal(context.Background(), w, "click", map[string]any{"pageId": float64(7), "uid": "u"})
		called <- err
	}()
	<-s.entered
	go func() { stopped <- r.Stop(w.WorkerID) }()
	close(s.gate)
	if err := <-called; err != nil {
		t.Fatal(err)
	}
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if fake.closes != 1 {
		t.Fatal("connector stop count")
	}
	_, err = r.CallExternal(context.Background(), w, "click", nil)
	assertBrowserCode(t, err, ErrLeaseTargetMismatch)
}

func TestWorkerRegistryTypedNilSessionFailureDoesNotPanic(t *testing.T) {
	var typedNil *mcpclient.StdioSession
	r := NewWorkerRegistry(WorkerDependencies{
		Probe: func(context.Context, mcpclient.ServerConfig) (string, error) {
			return PreferredEngineVersion, nil
		},
		Open: func(context.Context, mcpclient.ServerConfig) (WorkerSession, error) {
			return typedNil, errors.New("open failed")
		},
	})
	_, err := r.StartExternal(context.Background(), ExternalWorkerOptions{Route: externalRoute(externalStart())})
	assertBrowserCode(t, err, ErrEngineUnavailable)
	if shutdownErr := r.Shutdown(context.Background()); shutdownErr != nil {
		t.Fatal(shutdownErr)
	}
}
