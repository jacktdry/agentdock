package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/app"
	"github.com/uvwt/agentdock/internal/auth"
	"github.com/uvwt/agentdock/internal/runtimeapi"
)

func TestRuntimeBrowserDesktopLocalAuthenticatedGETOnly(t *testing.T) {
	cfg := testConfig(t)
	cfg.AuthToken = "fixture-token"
	var runtime *app.Runtime // Empty Core fixture, no processes or browser.
	mux := http.NewServeMux()
	registerRuntimeAPI(mux, runtime, cfg, auth.NewOAuthStore())
	for _, test := range []struct {
		method, remote, token, forwarded string
		status                           int
	}{
		{"GET", "127.0.0.1:4321", "fixture-token", "", 200},
		{"GET", "127.0.0.1:4321", "", "", 401},
		{"GET", "127.0.0.1:4321", "wrong", "", 401},
		{"GET", "203.0.113.9:4321", "fixture-token", "", 403},
		{"GET", "127.0.0.1:4321", "fixture-token", "203.0.113.9", 403},
		{"POST", "127.0.0.1:4321", "fixture-token", "", 405},
		{"DELETE", "127.0.0.1:4321", "fixture-token", "", 405},
		{"PUT", "127.0.0.1:4321", "fixture-token", "", 405},
	} {
		r := httptest.NewRequest(test.method, "/internal/runtime/browser/desktop", nil)
		r.RemoteAddr, r.Host = test.remote, "127.0.0.1"
		if test.token != "" {
			r.Header.Set("Authorization", "Bearer "+test.token)
		}
		if test.forwarded != "" {
			r.Header.Set("X-Forwarded-For", test.forwarded)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("%s %s got %d %s", test.method, test.remote, w.Code, w.Body.String())
		}
		if w.Code == 405 && w.Header().Get("Allow") != "GET" {
			t.Fatal("mutation advertised")
		}
		if w.Code == 200 && !strings.Contains(w.Body.String(), `"availability":"core_unavailable"`) {
			t.Fatal(w.Body.String())
		}
	}
	// Even when general Core authentication is disabled, Browser requires it.
	cfg.AuthToken, cfg.OAuthEnabled = "", false
	r := httptest.NewRequest("GET", "/internal/runtime/browser/desktop", nil)
	r.RemoteAddr, r.Host = "127.0.0.1:4321", "127.0.0.1"
	w := httptest.NewRecorder()
	runtimeAPIHandler(runtime, cfg, auth.NewOAuthStore()).ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("unauthenticated snapshot accepted")
	}
}

// Embedding only the required interface must remain source compatible. It does
// not acquire the optional Browser capability or fabricate an empty snapshot.
type browserUnsupportedRuntime struct{ runtimeapi.Runtime }

func TestRuntimeBrowserDesktopOptionalCapability(t *testing.T) {
	_, err := runtimeapi.Dispatch(context.Background(), browserUnsupportedRuntime{}, runtimeapi.Request{Method: "GET", Path: "/internal/runtime/browser/desktop"})
	if e, ok := err.(*app.ToolError); !ok || e.Code != "BROWSER_DESKTOP_UNSUPPORTED" {
		t.Fatal(err)
	}
}
