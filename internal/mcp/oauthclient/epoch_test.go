package oauthclient

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"
)

func TestIndependentManagerInvalidatesExchange(t *testing.T) {
	for _, remove := range []bool{false, true} {
		t.Run(map[bool]string{false: "clear", true: "remove"}[remove], func(t *testing.T) {
			fake := newFakeOAuthService(t)
			home := t.TempDir()
			m := prepareOAuthManager(t, fake, home)
			other, err := New(home)
			if err != nil {
				t.Fatal(err)
			}
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
				err = other.RemoveGrant("cloudflare")
			} else {
				err = other.Clear("cloudflare")
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

func TestIndependentManagerInvalidatesRefresh(t *testing.T) {
	for _, removalFails := range []bool{false} {
		t.Run(map[bool]string{false: "removed", true: "removal_failure"}[removalFails], func(t *testing.T) {
			fake := newFakeOAuthService(t)
			home := t.TempDir()
			m := prepareOAuthManager(t, fake, home)
			other, err := New(home)
			if err != nil {
				t.Fatal(err)
			}
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

			err = other.Clear("cloudflare")
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
