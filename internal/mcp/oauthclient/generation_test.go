package oauthclient

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type gatedTransport struct {
	base    http.RoundTripper
	path    string
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (g *gatedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Path == g.path {
		g.once.Do(func() { close(g.started); <-g.release })
	}
	return g.base.RoundTrip(r)
}

func gateOAuth(t *testing.T, m *Manager, path string) *gatedTransport {
	t.Helper()
	g := &gatedTransport{base: http.DefaultTransport, path: path, started: make(chan struct{}), release: make(chan struct{})}
	m.httpClient.Transport = g
	t.Cleanup(func() {
		select {
		case <-g.release:
		default:
			close(g.release)
		}
	})
	return g
}

func awaitOAuth[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("OAuth operation did not reach checkpoint")
	}
	var zero T
	return zero
}

func requireCancelled(t *testing.T, err error) {
	t.Helper()
	var flowErr *FlowError
	if !errors.As(err, &flowErr) || flowErr.Code != "MCP_AUTH_CANCELLED" {
		t.Fatalf("expected cancelled flow, got %v", err)
	}
}

func requireNoGrant(t *testing.T, m *Manager) {
	t.Helper()
	if _, err := m.store.loadGrant("cloudflare"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("grant survived invalidation: %v", err)
	}
}

func TestClearDuringBeginFencesReservation(t *testing.T) {
	for _, path := range []string{"/resource-metadata", "/register"} {
		t.Run(path, func(t *testing.T) {
			fake := newFakeOAuthService(t)
			m := prepareOAuthManager(t, fake, t.TempDir())
			g := gateOAuth(t, m, path)
			outcome := make(chan error, 1)
			go func() {
				result, done, err := m.Begin(context.Background(), "cloudflare", "cloudflare", fake.endpoint(), CallbackLocal)
				if err == nil || done != nil || result.AuthorizationURL != "" {
					outcome <- errors.New("invalidated Begin created a flow")
					return
				}
				outcome <- err
			}()
			awaitOAuth(t, g.started)
			if err := m.Clear("cloudflare"); err != nil {
				t.Fatal(err)
			}
			m.mu.Lock()
			if _, ok := m.beginning["cloudflare"]; ok {
				t.Error("Clear retained beginning reservation")
			}
			m.mu.Unlock()
			close(g.release)
			requireCancelled(t, awaitOAuth(t, outcome))
			m.mu.Lock()
			if len(m.flows) != 0 || len(m.beginning) != 0 || len(m.activeByStorage) != 0 {
				t.Error("late Begin restored flow state")
			}
			m.mu.Unlock()
			requireNoGrant(t, m)
		})
	}
}

func TestInvalidationDuringExchangeAndCallbackReplay(t *testing.T) {
	for _, remove := range []bool{false, true} {
		t.Run(map[bool]string{false: "clear", true: "remove"}[remove], func(t *testing.T) {
			fake := newFakeOAuthService(t)
			m := prepareOAuthManager(t, fake, t.TempDir())
			g := gateOAuth(t, m, "/token")
			result, done, err := m.Begin(context.Background(), "cloudflare", "cloudflare", fake.endpoint(), CallbackLocal)
			if err != nil {
				t.Fatal(err)
			}
			u, _ := url.Parse(result.AuthorizationURL)
			callback := CallbackResult{State: u.Query().Get("state"), Code: "code-1", Issuer: fake.server.URL}
			if err := m.DeliverCallback(callback); err != nil {
				t.Fatal(err)
			}
			awaitOAuth(t, g.started)
			if err := m.DeliverCallback(callback); err == nil {
				t.Fatal("repeated callback accepted")
			}
			if remove {
				err = m.RemoveGrant("cloudflare")
			} else {
				err = m.Clear("cloudflare")
			}
			if err != nil {
				t.Fatal(err)
			}
			close(g.release)
			requireCancelled(t, awaitOAuth(t, done))
			requireNoGrant(t, m)
			if err := m.DeliverCallback(callback); err == nil {
				t.Fatal("callback replay accepted after invalidation")
			}
		})
	}
}

func TestClearDuringRefreshFencesPersistence(t *testing.T) {
	for _, removalFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "removed", true: "removal_failure"}[removalFails], func(t *testing.T) {
			fake := newFakeOAuthService(t)
			m := prepareOAuthManager(t, fake, t.TempDir())
			authorizeFakeService(t, m, fake)
			grant, err := m.store.loadGrant("cloudflare")
			if err != nil {
				t.Fatal(err)
			}
			grant.Expiry = time.Now().Add(-time.Minute)
			if err := m.store.saveGrant("cloudflare", grant); err != nil {
				t.Fatal(err)
			}
			g := gateOAuth(t, m, "/token")
			source, err := m.tokenSource("cloudflare", fake.endpoint())
			if err != nil || source == nil {
				t.Fatalf("source: %v", err)
			}
			outcome := make(chan error, 1)
			go func() {
				token, err := source.Token()
				if token != nil {
					outcome <- errors.New("invalidated refresh returned token")
					return
				}
				outcome <- err
			}()
			awaitOAuth(t, g.started)
			if removalFails {
				path, _ := m.store.grantPath("cloudflare")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "block"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			err = m.Clear("cloudflare")
			if (err != nil) != removalFails {
				t.Fatalf("unexpected removal outcome: %v", err)
			}
			close(g.release)
			var required *AuthRequiredError
			if err := awaitOAuth(t, outcome); !errors.As(err, &required) {
				t.Fatalf("expected safe auth required, got %v", err)
			}
			if !removalFails {
				requireNoGrant(t, m)
			}
			if _, err := source.Token(); !errors.As(err, &required) {
				t.Fatalf("stale source reused: %v", err)
			}
		})
	}
}

func TestEndpointMismatchDoesNotUseGrant(t *testing.T) {
	fake := newFakeOAuthService(t)
	m := prepareOAuthManager(t, fake, t.TempDir())
	authorizeFakeService(t, m, fake)
	source, err := m.tokenSource("cloudflare", fake.endpoint()+"/other")
	if err != nil || source != nil {
		t.Fatalf("endpoint mismatch source: %v, %v", source, err)
	}
}
