package oauthclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"testing"
)

func (s *store) saveGrant(key string, grant Grant) error {
	epoch, err := s.reserve(key)
	if err != nil {
		return err
	}
	return s.saveGrantChecked(key, epoch, grant)
}

func TestLegacyGrantMigrationAndTombstone(t *testing.T) {
	s, err := newStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path, _ := s.grantPath("demo")
	legacy := []byte(`{"schema_version":1,"endpoint":"https://example.invalid/mcp","access_token":"canary-token"}`)
	if err := os.WriteFile(path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	if grant, err := s.loadGrant("demo"); err != nil || grant.AccessToken != "canary-token" {
		t.Fatal("legacy grant unreadable")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, legacy) {
		t.Fatal("passive load mutated legacy grant")
	}
	grant, epoch, err := s.loadGrantWithEpoch("demo")
	if err != nil {
		t.Fatal(err)
	}
	rawEpoch, err := base64.RawURLEncoding.DecodeString(epoch)
	if err != nil || len(rawEpoch) != 32 {
		t.Fatal("epoch is not random 256-bit identity")
	}
	if grant.AccessToken != "canary-token" {
		t.Fatal("migration lost grant")
	}
	if err := s.removeGrant("demo"); err != nil {
		t.Fatal(err)
	}
	tombstone, err := s.readEnvelope("demo")
	if err != nil || tombstone.SchemaVersion != 2 || tombstone.Epoch == epoch || tombstone.Grant != nil {
		t.Fatal("clear did not advance tombstone")
	}
	if _, err := s.loadGrant("demo"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("tombstone exposed a grant")
	}
	var required *AuthRequiredError
	if err := s.saveGrantChecked("demo", epoch, grant); !errors.As(err, &required) {
		t.Fatal("stale writer replaced tombstone")
	}
	if err := s.clearChecked("demo", &epoch); !errors.As(err, &required) {
		t.Fatal("stale failure cleared newer authorization")
	}
	if err := s.removeGrant("demo"); err != nil {
		t.Fatal(err)
	}
	next, err := s.readEnvelope("demo")
	if err != nil || next.Epoch == tombstone.Epoch {
		t.Fatal("clear of absent grant did not advance epoch")
	}
}

func TestIndependentManagerInvalidatesReservedBegin(t *testing.T) {
	fake := newFakeOAuthService(t)
	home := t.TempDir()
	m := prepareOAuthManager(t, fake, home)
	other, err := New(home)
	if err != nil {
		t.Fatal(err)
	}
	reservation, err := m.ReserveAuthorization("cloudflare", "cloudflare", fake.endpoint(), CallbackLocal)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Clear("cloudflare"); err != nil {
		t.Fatal(err)
	}
	result, done, err := m.BeginReserved(context.Background(), reservation)
	requireCancelled(t, err)
	if result.AuthorizationURL != "" || done != nil {
		t.Fatal("invalidated reservation created flow")
	}
	requireNoGrant(t, m)
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.beginning) != 0 {
		t.Fatal("cancelled continuation retained reservation")
	}
}
