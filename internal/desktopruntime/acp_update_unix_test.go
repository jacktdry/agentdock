//go:build darwin || linux

package desktopruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyACPProfileUpdateVerifiesStagingAndAtomicallyPromotes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing-noexec-style-temp"))
	target := filepath.Join(home, ".agentdock-next", "bin", "antigravity-acp")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("#!/bin/sh\necho 1.2.0-agentdock.6\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	candidate := []byte("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 1.2.0-agentdock.7; exit 0; fi\nexit 0\n")
	sum := sha256.Sum256(candidate)
	digest := "sha256:" + hex.EncodeToString(sum[:])

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/asset" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(candidate)
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	source := ACPUpdateSource{AllowedAssetHosts: []string{parsed.Hostname()}}

	update := ACPAdapterUpdate{
		Supported: true, Available: true, LatestVersion: "1.2.0-agentdock.7",
		AssetURL: server.URL + "/asset", Digest: digest, TargetPath: target,
	}
	if err := ApplyACPProfileUpdate(context.Background(), server.Client(), source, t.TempDir(), update); err != nil {
		t.Fatal(err)
	}
	output, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != string(candidate) {
		t.Fatal("target bytes did not match verified candidate")
	}
}

func TestApplyACPProfileUpdateKeepsCurrentBinaryOnDigestFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	target := filepath.Join(home, ".agentdock-next", "bin", "antigravity-acp")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	current := []byte("#!/bin/sh\necho 1.2.0-agentdock.6\n")
	if err := os.WriteFile(target, current, 0o700); err != nil {
		t.Fatal(err)
	}
	candidate := []byte("#!/bin/sh\necho 1.2.0-agentdock.7\n")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(candidate) }))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	source := ACPUpdateSource{AllowedAssetHosts: []string{parsed.Hostname()}}
	update := ACPAdapterUpdate{
		Supported: true, Available: true, LatestVersion: "1.2.0-agentdock.7",
		AssetURL: server.URL, Digest: "sha256:" + string(make([]byte, 64)), TargetPath: target,
	}
	if err := ApplyACPProfileUpdate(context.Background(), server.Client(), source, t.TempDir(), update); err == nil {
		t.Fatal("expected digest failure")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(current) {
		t.Fatal("current binary changed after failed update")
	}
}
