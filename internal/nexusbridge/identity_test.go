package nexusbridge

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPairPersistsOnlyNexusDeviceIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes/pair" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var request map[string]string
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["code"] != "pair_test" || request["device_id"] == "" {
			t.Fatalf("request = %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"node":{"id":"node_test"},"device_token":"device-secret"}`))
	}))
	defer server.Close()

	home := t.TempDir()
	identity, err := Pair(t.Context(), home, PairOptions{Endpoint: server.URL, Code: "pair_test", Name: "DockMini"})
	if err != nil {
		t.Fatal(err)
	}
	if identity.NodeID != "node_test" || identity.DeviceToken != "device-secret" {
		t.Fatalf("identity = %#v", identity)
	}
	loaded, err := Load(home)
	if err != nil || loaded != identity {
		t.Fatalf("loaded = %#v err=%v", loaded, err)
	}
	info, err := os.Stat(filepath.Join(home, "nexus", "device.json"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("identity permissions = %o", info.Mode().Perm())
	}
	status, err := ReadStatus(home)
	if err != nil || !status.Paired || !status.DeviceTokenStored || status.Endpoint != server.URL || status.NodeID != "node_test" {
		t.Fatalf("status = %#v err=%v", status, err)
	}
}

func TestPairRejectsAllRedirectsWithoutForwarding(t *testing.T) {
	var reached atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer target.Close()
	for _, status := range []int{301, 302, 303, 307, 308} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Location", target.URL+"/stolen?token=secret")
				w.WriteHeader(status)
			}))
			defer server.Close()
			home := t.TempDir()
			_, err := Pair(t.Context(), home, PairOptions{Endpoint: server.URL, Code: "secret"})
			if err == nil || strings.Contains(err.Error(), "secret") || requests.Load() != 1 || reached.Load() != 0 {
				t.Fatalf("err=%v requests=%d reached=%d", err, requests.Load(), reached.Load())
			}
			if _, err := Load(home); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed pairing persisted identity: %v", err)
			}
		})
	}
}

func TestPairErrorsNeverExposeRemoteContentsAndPreserveIdentity(t *testing.T) {
	for _, body := range []string{"pair-code token-secret provider-body", `{"node":pair-code}`, strings.Repeat("x", (64<<10)+1)} {
		for _, status := range []int{http.StatusBadRequest, http.StatusCreated} {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(body))
			}))
			home := t.TempDir()
			previous := Identity{Version: 1, Endpoint: "https://example.com", NodeID: "old", DeviceID: "old", DeviceToken: "old-secret"}
			if err := Save(home, previous); err != nil {
				t.Fatal(err)
			}
			_, err := Pair(t.Context(), home, PairOptions{Endpoint: server.URL, Code: "pair-code"})
			server.Close()
			if err == nil || strings.Contains(err.Error(), "pair-code") || strings.Contains(err.Error(), "token-secret") || strings.Contains(err.Error(), "provider-body") || len(err.Error()) > 200 {
				t.Fatalf("unsafe error: %v", err)
			}
			loaded, err := Load(home)
			if err != nil || loaded != previous {
				t.Fatalf("previous identity lost: %v", err)
			}
		}
	}
}

func TestPairDestinationPolicy(t *testing.T) {
	blocked := []string{"0.0.0.0", "10.0.0.1", "100.100.100.200", "127.0.0.1", "169.254.169.254", "172.16.1.1", "192.168.1.1", "192.0.0.8", "192.0.2.1", "192.88.99.1", "198.18.0.1", "198.51.100.1", "203.0.113.1", "224.0.0.1", "240.0.0.1", "255.255.255.255", "::", "::1", "fc00::1", "fe80::1", "ff02::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1", "64:ff9b::a00:1", "2001::1", "2001:db8::1", "2002:7f00:1::", "3fff::1"}
	for _, address := range blocked {
		t.Run(address, func(t *testing.T) {
			ip := netip.MustParseAddr(address)
			if publicPairIP(ip) {
				t.Fatal("nonpublic address accepted")
			}
			lookup := func(context.Context, string, string) ([]netip.Addr, error) {
				// Even a mixed public/private DNS answer must fail before dialing.
				return []netip.Addr{netip.MustParseAddr("8.8.8.8"), ip}, nil
			}
			dial := func(context.Context, string, string) (net.Conn, error) {
				t.Error("blocked DNS result reached dial")
				return nil, errors.New("unexpected")
			}
			_, err := pairWithNetwork(t.Context(), t.TempDir(), PairOptions{Endpoint: "https://nexus.example", Code: "secret"}, lookup, dial)
			if err == nil {
				t.Fatal("blocked DNS answer accepted")
			}
		})
	}
	for _, address := range []string{"8.8.8.8", "2606:4700:4700::1111", "::ffff:8.8.8.8"} {
		if !publicPairIP(netip.MustParseAddr(address)) {
			t.Fatalf("public address rejected: %s", address)
		}
	}
}

func TestPairDialPinsValidatedIPAndDisablesProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	u, _ := url.Parse("https://nexus.example:8443")
	calls := 0
	transport := pairTransport(u, func(context.Context, string, string) ([]netip.Addr, error) {
		calls++
		if calls > 1 {
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}, func(_ context.Context, _ string, address string) (net.Conn, error) {
		if address != "8.8.8.8:8443" {
			t.Fatalf("dial did not pin validated IP: %s", address)
		}
		return nil, errors.New("dial-secret")
	})
	if transport.Proxy != nil || transport.TLSClientConfig != nil {
		t.Fatal("unsafe proxy/TLS override")
	}
	_, _ = transport.DialContext(t.Context(), "tcp", "nexus.example:8443")
	if calls != 1 {
		t.Fatalf("DNS resolved %d times", calls)
	}
	_, err := transport.DialContext(t.Context(), "tcp", "nexus.example:8443")
	if err == nil {
		t.Fatal("rebound address accepted")
	}
}

func TestPairLocalhostDNSMustRemainLoopback(t *testing.T) {
	u, _ := url.Parse("http://localhost:8080")
	for _, address := range []string{"127.0.0.1", "::1", "::ffff:127.0.0.1", "10.0.0.1", "8.8.8.8"} {
		called := false
		transport := pairTransport(u, func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr(address)}, nil
		}, func(context.Context, string, string) (net.Conn, error) {
			called = true
			return nil, errors.New("offline")
		})
		_, _ = transport.DialContext(t.Context(), "tcp", "localhost:8080")
		if called != netip.MustParseAddr(address).Unmap().IsLoopback() {
			t.Fatalf("loopback exception allowed wrong peer: %s dial=%v", address, called)
		}
	}
	if publicPairIP(netip.MustParseAddr("168.63.129.16")) {
		t.Fatal("Azure infrastructure address accepted")
	}
}

func TestPairTLSVerifiesOriginalHostname(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(201) }))
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	for _, host := range []string{"example.com", "wrong.example"} {
		u, _ := url.Parse("https://" + host)
		transport := pairTransport(u, func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		}, func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != "8.8.8.8:443" {
				t.Fatalf("unexpected dial %s", address)
			}
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		})
		// Only the offline test trusts the fixture certificate.
		transport.TLSClientConfig = &tls.Config{RootCAs: roots}
		client := &http.Client{Transport: transport, Timeout: time.Second}
		response, err := client.Get(u.String())
		if response != nil {
			response.Body.Close()
		}
		transport.CloseIdleConnections()
		if (host == "example.com") != (err == nil) {
			t.Fatalf("host=%s err=%v", host, err)
		}
	}
}

func TestPairRejectsAmbiguousEndpointsAndOversizedInputOffline(t *testing.T) {
	for _, endpoint := range []string{"file:///tmp/nexus", "http://public.example", "https://user:secret@example.com", "https://example.com?", "https://example.com#", "https://example.com/a/../b", "https://example.com/%2fprivate", "https://example.com//base", "https://example.com:0", "https://example.com:65536", "https://[fe80::1%25en0]", "https://127.0.0.1", strings.Repeat("x", 4097)} {
		if _, err := normalizeEndpoint(endpoint); err == nil {
			t.Fatalf("accepted %q", endpoint)
		}
	}
	for _, endpoint := range []string{"http://localhost:8080", "http://[::1]:8080", "http://[::ffff:127.0.0.1]:8080", "https://example.com/base/path"} {
		if _, err := normalizeEndpoint(endpoint); err != nil {
			t.Fatalf("endpoint=%s err=%v", endpoint, err)
		}
	}
	for _, options := range []PairOptions{{Endpoint: "https://example.com", Code: strings.Repeat("s", 257)}, {Endpoint: "https://example.com", Code: "s", Name: strings.Repeat("n", 257)}} {
		_, err := Pair(t.Context(), t.TempDir(), options)
		if err == nil {
			t.Fatal("oversized input accepted")
		}
	}
}

func TestPairNetworkFailureAndDeadlineAreSanitized(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		lookup := func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		}
		dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
			if deadline {
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return nil, errors.New("code-secret provider-body")
		}
		_, err := pairWithNetwork(ctx, t.TempDir(), PairOptions{Endpoint: "https://example.com", Code: "code-secret"}, lookup, dial)
		cancel()
		if err == nil || strings.Contains(err.Error(), "code-secret") || strings.Contains(err.Error(), "provider-body") {
			t.Fatalf("unsafe error=%v", err)
		}
	}
}

func TestReadStatusDoesNotExposeDeviceToken(t *testing.T) {
	status, err := ReadStatus(t.TempDir())
	if err != nil || status.Paired || status.DeviceTokenStored {
		t.Fatalf("status = %#v err=%v", status, err)
	}
	data, err := json.Marshal(Status{Paired: true, DeviceTokenStored: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "device_token\"") {
		t.Fatalf("status JSON exposed Device Token field: %s", data)
	}
}

func TestPublicEndpointRequiresHTTPS(t *testing.T) {
	if _, err := normalizeEndpoint("http://nexus.example.com"); err == nil {
		t.Fatal("public HTTP endpoint was accepted")
	}
	if endpoint, err := normalizeEndpoint("http://127.0.0.1:8080/"); err != nil || endpoint != "http://127.0.0.1:8080" {
		t.Fatalf("loopback endpoint=%q err=%v", endpoint, err)
	}
}

func TestPairCheckedGenerationAndReplaceConfirmation(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"node":{"id":"node_checked"},"device_token":"token_checked"}`))
	}))
	defer server.Close()

	home := t.TempDir()
	absent := AbsentGeneration()
	identity, err := PairChecked(t.Context(), home, PairOptions{Endpoint: server.URL, Code: "first-code", Name: "Next"}, absent, false)
	if err != nil || identity.NodeID != "node_checked" || requests.Load() != 1 {
		t.Fatalf("first pair identity=%+v err=%v requests=%d", identity, err, requests.Load())
	}
	if _, err := PairChecked(t.Context(), home, PairOptions{Endpoint: server.URL, Code: "stale-code"}, absent, true); !errors.Is(err, ErrGenerationConflict) || requests.Load() != 1 {
		t.Fatalf("stale generation err=%v requests=%d", err, requests.Load())
	}
	current, paired, err := CurrentGeneration(home)
	if err != nil || !paired || current != Generation(identity) {
		t.Fatalf("current=%q paired=%v err=%v", current, paired, err)
	}
	if _, err := PairChecked(t.Context(), home, PairOptions{Endpoint: server.URL, Code: "replace-code"}, current, false); !errors.Is(err, ErrReplaceConfirmationNeeded) || requests.Load() != 1 {
		t.Fatalf("missing replace confirmation err=%v requests=%d", err, requests.Load())
	}
	if _, err := PairChecked(t.Context(), home, PairOptions{Endpoint: server.URL, Code: "replace-code"}, current, true); err != nil || requests.Load() != 2 {
		t.Fatalf("confirmed replace err=%v requests=%d", err, requests.Load())
	}
}

func TestPairCheckedRevalidatesBeforeCommit(t *testing.T) {
	home := t.TempDir()
	previous := Identity{Version: 1, Endpoint: "https://example.com", NodeID: "old", DeviceID: "old-device", DeviceToken: "old-secret"}
	if err := Save(home, previous); err != nil {
		t.Fatal(err)
	}
	expected := Generation(previous)
	external := Identity{Version: 1, Endpoint: "https://example.net", NodeID: "external", DeviceID: "external-device", DeviceToken: "external-secret"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := Save(home, external); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"node":{"id":"new"},"device_token":"new-secret"}`))
	}))
	defer server.Close()
	_, err := PairChecked(t.Context(), home, PairOptions{Endpoint: server.URL, Code: "one-time"}, expected, true)
	if !errors.Is(err, ErrGenerationConflict) {
		t.Fatalf("err=%v", err)
	}
	loaded, err := Load(home)
	if err != nil || loaded != external {
		t.Fatalf("external identity overwritten: %+v err=%v", loaded, err)
	}
}

func TestPairCheckedRejectsNodeIDDesktopCannotRead(t *testing.T) {
	for _, invalid := range []string{"node/path", "node\nvalue", strings.Repeat("n", 129)} {
		t.Run(fmt.Sprintf("len-%d", len(invalid)), func(t *testing.T) {
			home := t.TempDir()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
				_, _ = fmt.Fprintf(w, `{"node":{"id":%q},"device_token":"private-secret"}`, invalid)
			}))
			defer server.Close()
			if _, err := PairChecked(t.Context(), home, PairOptions{Endpoint: server.URL, Code: "one-time"}, AbsentGeneration(), false); err == nil {
				t.Fatal("stored a server node ID that Desktop would reject")
			}
			if _, paired, err := CurrentGeneration(home); err != nil || paired {
				t.Fatalf("unsafe node identity persisted: paired=%v err=%v", paired, err)
			}
		})
	}
}

func TestPairSerializesAcrossCallers(t *testing.T) {
	home := t.TempDir()
	entered := make(chan int, 2)
	releaseFirst := make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(requests.Add(1))
		entered <- n
		if n == 1 {
			<-releaseFirst
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"node":{"id":"node_%d"},"device_token":"token_%d"}`, n, n)))
	}))
	defer server.Close()

	results := make(chan error, 2)
	go func() {
		_, err := Pair(context.Background(), home, PairOptions{Endpoint: server.URL, Code: "one"})
		results <- err
	}()
	select {
	case n := <-entered:
		if n != 1 {
			t.Fatalf("first request=%d", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first pairing never reached server")
	}
	go func() {
		_, err := Pair(context.Background(), home, PairOptions{Endpoint: server.URL, Code: "two"})
		results <- err
	}()
	select {
	case n := <-entered:
		t.Fatalf("second pairing bypassed lock, request=%d", n)
	case <-time.After(150 * time.Millisecond):
	}
	close(releaseFirst)
	select {
	case n := <-entered:
		if n != 2 {
			t.Fatalf("second request=%d", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second pairing did not resume")
	}
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

func TestPairRejectsSymlinkedStateBeforeNetworkOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix no-follow storage test")
	}
	parent := t.TempDir()
	stable := filepath.Join(parent, "stable")
	if err := os.Mkdir(stable, 0o700); err != nil {
		t.Fatal(err)
	}
	original := Identity{Version: 1, Endpoint: "https://stable.example", NodeID: "stable", DeviceID: "stable-device", DeviceToken: "stable-secret"}
	if err := Save(stable, original); err != nil {
		t.Fatal(err)
	}

	requests := atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"node":{"id":"attacker"},"device_token":"attacker-secret"}`))
	}))
	defer server.Close()

	t.Run("home symlink", func(t *testing.T) {
		link := filepath.Join(parent, "next-home-link")
		if err := os.Symlink(stable, link); err != nil {
			t.Fatal(err)
		}
		if _, err := Pair(t.Context(), link, PairOptions{Endpoint: server.URL, Code: "one-time"}); err == nil {
			t.Fatal("Pair accepted symlinked AgentDockHome")
		}
	})

	t.Run("nexus directory symlink", func(t *testing.T) {
		home := filepath.Join(parent, "next-home")
		if err := os.Mkdir(home, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(stable, "nexus"), filepath.Join(home, "nexus")); err != nil {
			t.Fatal(err)
		}
		if _, err := PairChecked(t.Context(), home, PairOptions{Endpoint: server.URL, Code: "one-time"}, AbsentGeneration(), false); err == nil {
			t.Fatal("PairChecked accepted symlinked nexus directory")
		}
	})
	if requests.Load() != 0 {
		t.Fatalf("one-time code reached network despite unsafe state: requests=%d", requests.Load())
	}
	loaded, err := Load(stable)
	if err != nil || loaded != original {
		t.Fatalf("stable identity changed: %+v err=%v", loaded, err)
	}
}

func TestPairCheckedInitializesMissingHomeBeforeConsumingCode(t *testing.T) {
	parent := t.TempDir()
	home := filepath.Join(parent, "fresh-next-state")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"node":{"id":"fresh-node"},"device_token":"fresh-token"}`))
	}))
	defer server.Close()
	identity, err := PairChecked(t.Context(), home, PairOptions{Endpoint: server.URL, Code: "fresh-code"}, AbsentGeneration(), false)
	if err != nil || identity.NodeID != "fresh-node" || requests.Load() != 1 {
		t.Fatalf("fresh pairing failed before persisting: identity=%+v err=%v requests=%d", identity, err, requests.Load())
	}
	if info, err := os.Lstat(home); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("fresh home missing or unsafe: %v %v", info, err)
	}
	loaded, err := Load(home)
	if err != nil || loaded != identity {
		t.Fatalf("new identity not persisted: loaded=%+v err=%v", loaded, err)
	}
}

func TestPairCheckedMissingHomeRejectsSymlinkParentBeforeNetworkOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix no-follow parent check")
	}
	parent := t.TempDir()
	stable := filepath.Join(parent, "stable")
	if err := os.Mkdir(stable, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "swapped-parent")
	if err := os.Symlink(stable, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"node":{"id":"unexpected"},"device_token":"unexpected"}`))
	}))
	defer server.Close()
	_, err := PairChecked(t.Context(), filepath.Join(link, "fresh-next"), PairOptions{Endpoint: server.URL, Code: "one-time-code"}, AbsentGeneration(), false)
	if err == nil || requests.Load() != 0 {
		t.Fatalf("unsafe parent consumed code: err=%v requests=%d", err, requests.Load())
	}
	if _, err := os.Stat(filepath.Join(stable, "fresh-next")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stable directory unexpectedly created: %v", err)
	}
	if entries, err := os.ReadDir(stable); err != nil || len(entries) != 0 {
		t.Fatalf("unsafe pairing created lock or state inside stable: entries=%v err=%v", entries, err)
	}
}
