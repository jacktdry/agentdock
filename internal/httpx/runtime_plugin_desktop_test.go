package httpx

import (
	"github.com/uvwt/agentdock/internal/app"
	"github.com/uvwt/agentdock/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRuntimePluginDesktopLocalBoundary(t *testing.T) {
	cfg := testConfig(t)
	runtime, err := app.NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	handler := runtimeAPIHandler(runtime, cfg, auth.NewOAuthStore())
	for _, test := range []struct {
		method, remote, body string
		status               int
	}{
		{"GET", "203.0.113.9:4321", "", http.StatusForbidden},
		{"POST", "203.0.113.9:4321", `{"action":"desktop_snapshot"}`, http.StatusForbidden},
		{"GET", "127.0.0.1:4321", "", http.StatusOK},
		{"POST", "127.0.0.1:4321", `{"action":"desktop_snapshot"}`, http.StatusOK},
		{"POST", "127.0.0.1:4321", `{"action":"install","source":"/private/RAW_CANARY"}`, http.StatusBadRequest},
		{"DELETE", "127.0.0.1:4321", "", http.StatusMethodNotAllowed},
	} {
		request := httptest.NewRequest(test.method, "/internal/runtime/plugin/desktop", strings.NewReader(test.body))
		request.RemoteAddr = test.remote
		request.Host = "127.0.0.1"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("%s %s status=%d body=%s", test.method, test.remote, response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "RAW_CANARY") {
			t.Fatal("raw error leaked")
		}
	}
}
