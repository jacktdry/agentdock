package desktopruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testACPUpdateSource(t *testing.T, handler http.Handler) (*httptest.Server, ACPUpdateSource) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return server, ACPUpdateSource{
		ReleaseAPI:        server.URL + "/latest",
		AllowedAssetHosts: []string{parsed.Hostname()},
	}
}

func TestCheckACPProfileUpdateFailClosesWhenTrustedReleaseMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	target := filepath.Join(home, ".agentdock-next", "bin", "antigravity-acp")

	server, source := testACPUpdateSource(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	got, err := CheckACPProfileUpdate(context.Background(), server.Client(), source, t.TempDir(), "antigravity",
		ACPProfileSettings{ID: "antigravity", Kind: "custom", Command: target},
		ACPAdapterProbe{Availability: "available", InstalledVersion: "1.2.0-agentdock.6"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Supported || got.Available || got.VersionState != "unavailable" || got.BlockedReason != "trusted_release_unavailable" {
		t.Fatalf("update = %#v", got)
	}
}

func TestCheckACPProfileUpdateReturnsNewerVerifiedForkRelease(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	target := filepath.Join(home, ".agentdock-next", "bin", "antigravity-acp")
	assetName, ok := antigravityAssetName(runtime.GOOS, runtime.GOARCH)
	if !ok {
		t.Skip("unsupported test platform")
	}
	asset := []byte("fixture")
	sum := sha256.Sum256(asset)
	digest := "sha256:" + hex.EncodeToString(sum[:])

	var server *httptest.Server
	server, source := testACPUpdateSource(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/latest" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(acpRelease{
			TagName: "v1.2.0-agentdock.7",
			Assets:  []acpReleaseAsset{{Name: assetName, URL: server.URL + "/asset", Digest: digest, Size: int64(len(asset))}},
		})
	}))
	defer server.Close()

	got, err := CheckACPProfileUpdate(context.Background(), server.Client(), source, t.TempDir(), "antigravity",
		ACPProfileSettings{ID: "antigravity", Kind: "custom", Command: target},
		ACPAdapterProbe{Availability: "available", InstalledVersion: "1.2.0-agentdock.6"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Supported || !got.Available || got.LatestVersion != "1.2.0-agentdock.7" || got.VersionState != "update_available" || got.Digest != digest {
		t.Fatalf("update = %#v", got)
	}
}

func TestCheckACPProfileUpdateRejectsSharedOrUnmanagedTargets(t *testing.T) {
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	probe := ACPAdapterProbe{Availability: "available", InstalledVersion: "1.2.0-agentdock.6"}

	codex, err := CheckACPProfileUpdate(context.Background(), nil, DefaultACPUpdateSource(), t.TempDir(), "codex",
		ACPProfileSettings{ID: "codex", Kind: "codex", Command: "/opt/homebrew/bin/codex-acp"}, probe)
	if err != nil {
		t.Fatal(err)
	}
	if codex.BlockedReason != "shared_install_not_managed" || codex.Supported {
		t.Fatalf("codex update = %#v", codex)
	}

	external, err := CheckACPProfileUpdate(context.Background(), nil, DefaultACPUpdateSource(), t.TempDir(), "antigravity",
		ACPProfileSettings{ID: "antigravity", Kind: "custom", Command: filepath.Join(t.TempDir(), "antigravity-acp")}, probe)
	if err != nil {
		t.Fatal(err)
	}
	if external.BlockedReason != "update_target_not_next_owned" || external.Supported {
		t.Fatalf("external update = %#v", external)
	}
}

func TestAgentDockAdapterVersionOrdering(t *testing.T) {
	left, ok := parseAgentDockAdapterVersion("v1.2.0-agentdock.7")
	if !ok {
		t.Fatal("left version did not parse")
	}
	right, ok := parseAgentDockAdapterVersion("1.2.0-agentdock.6")
	if !ok {
		t.Fatal("right version did not parse")
	}
	if compareAgentDockAdapterVersions(left, right) <= 0 {
		t.Fatalf("compare = %d", compareAgentDockAdapterVersions(left, right))
	}
	if _, ok := parseAgentDockAdapterVersion("1.2.0"); ok {
		t.Fatal("upstream version unexpectedly accepted as hardened fork version")
	}
}

func TestTrustedAntigravityTargetRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics require platform privileges on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	root := filepath.Join(home, ".agentdock-next", "bin")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	actual := filepath.Join(t.TempDir(), "antigravity-acp")
	if err := os.WriteFile(actual, []byte("x"), 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "antigravity-acp")
	if err := os.Symlink(actual, link); err != nil {
		t.Fatal(err)
	}
	if trustedAntigravityTarget(t.TempDir(), link) {
		t.Fatal("symlink target unexpectedly trusted")
	}
}
