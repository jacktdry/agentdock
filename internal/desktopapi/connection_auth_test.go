package desktopapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/desktopruntime"
)

func fixtureConnectionConfig(origin string) desktopruntime.ConnectionConfig {
	return desktopruntime.ConnectionConfig{CoreEndpoint: "http://127.0.0.1:8767", Port: 8767,
		PublicOrigin: origin, Mode: "named", OAuthEnabled: true, TunnelGeneration: "fixture-generation"}
}

func TestConnectionSnapshotAndRevealIsolation(t *testing.T) {
	config := fixtureConnectionConfig("https://example.test")
	reads := 0
	service := NewConnectionServiceWithDependencies(t.TempDir(), ConnectionDependencies{
		ReadConfig: func(context.Context, string) (desktopruntime.ConnectionConfig, error) { return config, nil },
		ReadPasswordState: func(context.Context, string) desktopruntime.OAuthPasswordState {
			return desktopruntime.OAuthPasswordStored
		},
		ReadPassword: func(context.Context, string) (string, desktopruntime.OAuthPasswordState) {
			reads++
			return "PASSWORD_SENTINEL", desktopruntime.OAuthPasswordStored
		},
	})
	snapshot := service.Snapshot(context.Background())
	data, _ := json.Marshal(snapshot)
	if snapshot.Error != nil || strings.Contains(string(data), "PASSWORD_SENTINEL") ||
		snapshot.Snapshot.LocalMCPURL != "http://127.0.0.1:8767/mcp" || snapshot.Snapshot.PublicMCPURL != "https://example.test/mcp" {
		t.Fatalf("invalid safe snapshot: %s", data)
	}
	if reads != 0 {
		t.Fatal("snapshot invoked the password reveal reader")
	}
	reveal := service.RevealOAuthPassword(context.Background())
	if reveal.Password != "PASSWORD_SENTINEL" || reveal.ConfigRevision != snapshot.Snapshot.ConfigRevision || reads != 1 {
		t.Fatal("dedicated reveal did not return the current stored password")
	}
	for _, state := range []desktopruntime.OAuthPasswordState{
		desktopruntime.OAuthPasswordMissing, desktopruntime.OAuthPasswordUnreadable, desktopruntime.OAuthPasswordUnavailable, "invalid",
	} {
		service.foundation.ReadPassword = func(context.Context, string) (string, desktopruntime.OAuthPasswordState) {
			return "PASSWORD_SENTINEL", state
		}
		if got := service.RevealOAuthPassword(context.Background()); got.Password != "" {
			t.Fatal("non-stored password escaped")
		}
	}
	service.foundation.ReadPassword = func(context.Context, string) (string, desktopruntime.OAuthPasswordState) {
		config.TunnelGeneration = "new-generation"
		return "PASSWORD_SENTINEL", desktopruntime.OAuthPasswordStored
	}
	if result := service.RevealOAuthPassword(context.Background()); result.Password != "" || result.Error == nil || result.Error.Code != "connection_config_stale" {
		t.Fatal("stale reveal escaped")
	}
	service.foundation.ReadConfig = func(context.Context, string) (desktopruntime.ConnectionConfig, error) {
		return config, errors.New("ENV_CONTENT_SENTINEL")
	}
	data, _ = json.Marshal(service.Snapshot(context.Background()))
	if strings.Contains(string(data), "ENV_CONTENT_SENTINEL") {
		t.Fatal("raw error escaped")
	}
}

func TestSnapshotNeverInvokesRevealReader(t *testing.T) {
	config := fixtureConnectionConfig("https://example.test")
	service := NewConnectionServiceWithDependencies(t.TempDir(), ConnectionDependencies{
		ReadConfig: func(context.Context, string) (desktopruntime.ConnectionConfig, error) { return config, nil },
		ReadPasswordState: func(context.Context, string) desktopruntime.OAuthPasswordState {
			return desktopruntime.OAuthPasswordStored
		},
		ReadPassword: func(context.Context, string) (string, desktopruntime.OAuthPasswordState) {
			t.Fatal("ordinary snapshot requested password bytes")
			return "", desktopruntime.OAuthPasswordUnavailable
		},
	})
	if got := service.Snapshot(context.Background()); got.Error != nil || got.Snapshot.OAuthPasswordState != desktopruntime.OAuthPasswordStored {
		t.Fatal(got)
	}
}

func TestPublicEndpointRejectsUnsafeDNSBeforeDial(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "::1", "10.0.0.1", "172.16.0.1", "192.168.1.1",
		"169.254.1.1", "fe80::1", "fc00::1", "0.0.0.0", "::", "224.0.0.1", "ff02::1", "::ffff:127.0.0.1", "100.64.0.1"} {
		t.Run(address, func(t *testing.T) {
			dialer := publicEndpointDialer{
				lookup: func(context.Context, string) ([]net.IPAddr, error) {
					// Reject mixed public/private answers too.
					return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP(address)}}, nil
				},
				dial: func(context.Context, string, string) (net.Conn, error) {
					t.Fatal("unsafe DNS address was dialed")
					return nil, nil
				},
			}
			config := fixtureConnectionConfig("https://public.example.test")
			service := NewConnectionServiceWithDependencies(t.TempDir(), ConnectionDependencies{
				ReadConfig: func(context.Context, string) (desktopruntime.ConnectionConfig, error) { return config, nil },
				Transport:  &http.Transport{DialContext: dialer.DialContext},
			})
			got := service.TestPublicEndpoint(context.Background())
			if got.State != "unreachable" || got.ReasonCode != "request_failed" {
				t.Fatal(got)
			}
		})
	}
}

func TestPublicEndpointPinsValidatedDNSAddress(t *testing.T) {
	lookups := 0
	client, peer := net.Pipe()
	defer peer.Close()
	dialer := publicEndpointDialer{
		lookup: func(context.Context, string) ([]net.IPAddr, error) {
			lookups++
			if lookups > 1 {
				return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
			}
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		},
		dial: func(_ context.Context, _, address string) (net.Conn, error) {
			if address != "8.8.8.8:443" {
				t.Fatalf("dial did not pin numeric IP: %s", address)
			}
			return client, nil
		},
	}
	conn, err := dialer.DialContext(context.Background(), "tcp", "public.example.test:443")
	if err != nil || lookups != 1 {
		t.Fatal("DNS was resolved again after validation")
	}
	_ = conn.Close()
}

type endpointRoundTripFunc func(*http.Request) (*http.Response, error)

func (f endpointRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPublicEndpointCancellationIsBoundedAndSafe(t *testing.T) {
	config := fixtureConnectionConfig("https://public.example.test")
	service := NewConnectionServiceWithDependencies(t.TempDir(), ConnectionDependencies{
		ReadConfig: func(context.Context, string) (desktopruntime.ConnectionConfig, error) { return config, nil },
		Transport: endpointRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if _, ok := r.Context().Deadline(); !ok {
				t.Fatal("request has no deadline")
			}
			<-r.Context().Done()
			return nil, errors.New("NETWORK_SECRET")
		}),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	result := service.TestPublicEndpoint(ctx)
	data, _ := json.Marshal(result)
	if time.Since(start) > time.Second || strings.Contains(string(data), "NETWORK_SECRET") || result.State == "reachable" {
		t.Fatal("canceled request was not bounded and safe")
	}
}

func TestConnectionRevision(t *testing.T) {
	config := fixtureConnectionConfig("https://example.test")
	before := ConnectionConfigRevision(config)
	if before == "" || before != ConnectionConfigRevision(config) {
		t.Fatal("revision not deterministic")
	}
	for _, mutate := range []func(*desktopruntime.ConnectionConfig){
		func(c *desktopruntime.ConnectionConfig) { c.Port = 9000; c.CoreEndpoint = "http://127.0.0.1:9000" },
		func(c *desktopruntime.ConnectionConfig) { c.PublicOrigin = "https://other.test" },
		func(c *desktopruntime.ConnectionConfig) { c.Mode = "quick" },
		func(c *desktopruntime.ConnectionConfig) { c.OAuthEnabled = false },
		func(c *desktopruntime.ConnectionConfig) { c.TunnelGeneration = "new-generation" },
	} {
		changed := config
		mutate(&changed)
		if ConnectionConfigRevision(changed) == before {
			t.Fatal("safe configuration change did not invalidate revision")
		}
	}
	config.PublicOrigin = "https://example.test/?secret=FIRST"
	one := ConnectionConfigRevision(config)
	config.PublicOrigin = "https://example.test/?secret=SECOND"
	if one != ConnectionConfigRevision(config) {
		t.Fatal("revision included rejected secret-bearing origin")
	}
	for _, raw := range []string{"https://example.test?", "https://user:PASS@example.test", "http://example.test", "https://example.test/path", "https://example.test/%2f"} {
		if publicOrigin(raw) != "" {
			t.Fatal("unsafe origin accepted")
		}
	}
}

func writeDiscovery(w http.ResponseWriter, path, origin string) {
	w.Header().Set("Content-Type", "application/json")
	var value any
	if path == "/.well-known/oauth-protected-resource/mcp" {
		value = endpointMetadata{Resource: origin + "/mcp", AuthorizationServers: []string{origin}, BearerMethods: []string{"header"}}
	} else {
		value = endpointMetadata{Issuer: origin, AuthorizationEndpoint: origin + "/oauth/authorize",
			TokenEndpoint: origin + "/oauth/token", RegistrationEndpoint: origin + "/register",
			ResponseTypes: []string{"code"}, GrantTypes: []string{"authorization_code"},
			CodeChallengeMethods: []string{"S256"}, TokenAuthMethods: []string{"none"}, ResourceIndicators: true}
	}
	_ = json.NewEncoder(w).Encode(value)
}

func TestPublicEndpointTLSDiscovery(t *testing.T) {
	for _, scenario := range []string{"valid", "wrong_origin", "plain_200", "redirect", "oversize", "bad_json", "status", "stale", "oauth_mismatch"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.URL.RawQuery != "" ||
					r.Header.Get("Proxy-Authorization") != "" {
					t.Error("credentials were sent")
				}
				switch scenario {
				case "redirect":
					w.Header().Set("Location", "http://127.0.0.1:1/private")
					w.WriteHeader(http.StatusFound)
					return
				case "plain_200":
					_, _ = w.Write([]byte("OK BODY_SECRET"))
					return
				case "oversize":
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(strings.Repeat("X", 65537)))
					return
				case "bad_json":
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte("{BODY_SECRET"))
					return
				case "status":
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				origin := server.URL
				if scenario == "wrong_origin" || (scenario == "oauth_mismatch" && calls == 2) {
					origin = "https://foreign.test"
				}
				writeDiscovery(w, r.URL.Path, origin)
			}))
			defer server.Close()
			config := fixtureConnectionConfig(server.URL)
			configReads := 0
			service := NewConnectionServiceWithDependencies(t.TempDir(), ConnectionDependencies{
				ReadConfig: func(context.Context, string) (desktopruntime.ConnectionConfig, error) {
					configReads++
					if scenario == "stale" && configReads > 1 {
						config.TunnelGeneration = "changed"
					}
					return config, nil
				},
				ReadPassword: func(context.Context, string) (string, desktopruntime.OAuthPasswordState) {
					t.Fatal("public test read password")
					return "", ""
				},
				Transport: server.Client().Transport,
			})
			result := service.TestPublicEndpoint(context.Background())
			want := "unexpected_endpoint"
			switch scenario {
			case "valid":
				want = "reachable"
			case "stale":
				want = "stale"
			case "status":
				want = "unreachable"
			}
			if result.State != want || result.ConfigRevision == "" || result.CheckedAt.IsZero() {
				t.Fatalf("result = %+v; want %s", result, want)
			}
			if scenario == "redirect" && calls != 1 {
				t.Fatal("followed redirect")
			}
			data, _ := json.Marshal(result)
			if strings.Contains(string(data), "BODY_SECRET") {
				t.Fatal("response body escaped")
			}
		})
	}
}

func TestPublicEndpointNoConfiguredHTTPS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("HTTP target was contacted") }))
	defer server.Close()
	config := fixtureConnectionConfig(server.URL)
	service := NewConnectionServiceWithDependencies(t.TempDir(), ConnectionDependencies{
		ReadConfig: func(context.Context, string) (desktopruntime.ConnectionConfig, error) { return config, nil },
	})
	if got := service.TestPublicEndpoint(context.Background()); got.State != "not_configured" {
		t.Fatal(got)
	}
	config.PublicOrigin = "https://example.test"
	config.Mode = "none"
	if got := service.TestPublicEndpoint(context.Background()); got.State != "not_configured" {
		t.Fatal(got)
	}
}
