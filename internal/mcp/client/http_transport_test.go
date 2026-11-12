package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"
)

type transportRoundTripFunc func(*http.Request) (*http.Response, error)

func (f transportRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type transportOAuth struct{}

func (transportOAuth) TokenSource(context.Context) (oauth2.TokenSource, error) {
	return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "oauth-canary"}), nil
}
func (transportOAuth) Authorize(context.Context, *http.Request, *http.Response) error {
	return errors.New("unexpected authorization flow")
}

func TestHTTPTransportRedirectPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, target string
		allowed      bool
	}{
		{"direct", "", true},
		{"same-origin", "https://mcp.example/target", true},
		{"foreign-host", "https://other.example/target?token=static-canary", false},
		{"subdomain", "https://child.mcp.example/target", false},
		{"foreign-port", "https://mcp.example:8443/target", false},
		{"downgrade", "http://mcp.example/target", false},
	} {
		for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, status), func(t *testing.T) {
				endpoint := "https://mcp.example/start"
				cfg := ServerConfig{Name: "redirect", Transport: TransportStreamableHTTP, URL: endpoint,
					ProtocolVersion: "2025-11-25", StaticHeaders: map[string]string{"X-API-Key": "static-canary"},
					HeaderEnv: map[string]string{"X-Env-Key": "MCP_REDIRECT_TOKEN"}, RuntimeEnv: map[string]string{"MCP_REDIRECT_TOKEN": "env-canary"}}
				client := newStreamableHTTPClient(cfg, transportOAuth{})
				initial, redirected := 0, 0
				client.httpClient = &http.Client{Transport: transportRoundTripFunc(func(r *http.Request) (*http.Response, error) {
					if r.URL.String() == endpoint {
						initial++
					} else {
						redirected++
					}
					if !tc.allowed && r.URL.String() != endpoint {
						t.Error("forbidden destination received a request")
					}
					for key, want := range map[string]string{"X-API-Key": "static-canary", "X-Env-Key": "env-canary", "Authorization": "Bearer oauth-canary"} {
						if got := r.Header.Values(key); len(got) != 1 || got[0] != want {
							t.Errorf("missing or duplicated %s", key)
						}
					}
					response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: r}
					body := ""
					if tc.target != "" && r.URL.String() == endpoint {
						response.StatusCode = status
						response.Header.Set("Location", tc.target)
					} else {
						var msg struct {
							ID     any    `json:"id"`
							Method string `json:"method"`
						}
						if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
							t.Error(err)
						}
						if msg.Method == "notifications/initialized" {
							response.StatusCode = http.StatusAccepted
						} else {
							result := map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "serverInfo": map[string]any{"name": "test", "version": "1"}}
							data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
							body = string(data)
							response.Header.Set("Content-Type", "application/json")
						}
					}
					response.Body = io.NopCloser(strings.NewReader(body))
					return response, nil
				})}
				err := client.initialize(context.Background())
				defer client.close()
				if tc.allowed && err != nil {
					t.Fatal(err)
				}
				if !tc.allowed {
					if !errors.Is(err, errMCPOriginRejected) {
						t.Fatalf("expected origin rejection, got %v", err)
					}
					for e := err; e != nil; e = errors.Unwrap(e) {
						for _, secret := range []string{"static-canary", "env-canary", "oauth-canary"} {
							if strings.Contains(fmt.Sprintf("%v %+v %#v", e, e, e), secret) {
								t.Fatal("error leaked credential")
							}
						}
					}
					if redirected != 0 {
						t.Fatal("redirect reached forbidden destination")
					}
				}
				if initial == 0 || (tc.allowed && tc.target != "" && redirected == 0) {
					t.Fatal("expected requests missing")
				}
			})
		}
	}
}

func TestHTTPTransportOriginGuardAndAuthorizationPrecedence(t *testing.T) {
	cfg := ServerConfig{Transport: TransportStreamableHTTP, URL: "https://mcp.example/start", StaticHeaders: map[string]string{"Authorization": "Bearer configured-canary"}}
	client := newStreamableHTTPClient(cfg, transportOAuth{})
	calls := 0
	client.httpClient = &http.Client{Transport: transportRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if got := r.Header.Values("Authorization"); len(got) != 1 || got[0] != "Bearer configured-canary" {
			t.Fatal("configured Authorization precedence lost")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	transport, err := client.transport()
	if err != nil {
		t.Fatal(err)
	}
	sdk := transport.(*mcpsdk.StreamableClientTransport)
	if sdk.OAuthHandler != nil {
		t.Fatal("explicit Authorization must disable automatic OAuth")
	}
	req, _ := http.NewRequest(http.MethodGet, cfg.URL, nil)
	req.Header.Set("Authorization", "Bearer oauth-canary")
	resp, err := sdk.HTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if req.Header.Get("Authorization") != "Bearer oauth-canary" {
		t.Fatal("wrapper mutated caller headers")
	}
	req.URL, _ = url.Parse("https://foreign.example/")
	if _, err := sdk.HTTPClient.Transport.RoundTrip(req); !errors.Is(err, errMCPOriginRejected) {
		t.Fatal("direct foreign dispatch was not blocked")
	}
	if calls != 1 {
		t.Fatal("foreign dispatch reached base transport")
	}
}

func TestHTTPTransportPreservesNoRedirectAndLimit(t *testing.T) {
	for _, noRedirect := range []bool{false, true} {
		client := newStreamableHTTPClient(ServerConfig{Transport: TransportStreamableHTTP, URL: "https://mcp.example/"}, nil)
		client.httpClient = &http.Client{}
		if noRedirect {
			client.httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		}
		transport, err := client.transport()
		if err != nil {
			t.Fatal(err)
		}
		req, _ := http.NewRequest(http.MethodGet, client.cfg.URL, nil)
		check := transport.(*mcpsdk.StreamableClientTransport).HTTPClient.CheckRedirect
		if noRedirect {
			if !errors.Is(check(req, nil), http.ErrUseLastResponse) {
				t.Fatal("no-redirect policy lost")
			}
		} else if check(req, make([]*http.Request, 10)) == nil {
			t.Fatal("redirect limit lost")
		}
	}
}
