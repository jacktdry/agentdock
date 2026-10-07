package httpx

import (
	"github.com/uvwt/agentdock/internal/app"
	"github.com/uvwt/agentdock/internal/auth"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestRuntimePluginDesktopCandidateRouteIsLoopbackOnlyAndSafe(t *testing.T) {
	cfg := testConfig(t)
	runtime, err := app.NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	handler := runtimeAPIHandler(runtime, cfg, auth.NewOAuthStore())

	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "plugin.json"), []byte(`{"name":"http-candidate","version":"1.0.0","description":"HTTP candidate"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	body := `{"action":"prepare","source":"` + source + `","kind":"install"}`

	remote := httptest.NewRequest(http.MethodPost, "/internal/runtime/plugin/desktop/candidate", strings.NewReader(body))
	remote.RemoteAddr = "203.0.113.9:4321"
	remote.Host = "127.0.0.1"
	remoteResponse := httptest.NewRecorder()
	handler.ServeHTTP(remoteResponse, remote)
	if remoteResponse.Code != http.StatusForbidden {
		t.Fatalf("remote candidate status=%d body=%s", remoteResponse.Code, remoteResponse.Body.String())
	}
	if strings.Contains(remoteResponse.Body.String(), source) {
		t.Fatal("remote candidate path leaked")
	}

	local := httptest.NewRequest(http.MethodPost, "/internal/runtime/plugin/desktop/candidate", strings.NewReader(body))
	local.RemoteAddr = "127.0.0.1:4321"
	local.Host = "127.0.0.1"
	localResponse := httptest.NewRecorder()
	handler.ServeHTTP(localResponse, local)
	if localResponse.Code != http.StatusOK {
		t.Fatalf("local candidate status=%d body=%s", localResponse.Code, localResponse.Body.String())
	}
	for _, forbidden := range []string{source, "review_token", "storage_key", "runtime_name"} {
		if strings.Contains(localResponse.Body.String(), forbidden) {
			t.Fatalf("candidate response leaked %q: %s", forbidden, localResponse.Body.String())
		}
	}

	get := httptest.NewRequest(http.MethodGet, "/internal/runtime/plugin/desktop/candidate", nil)
	get.RemoteAddr = "127.0.0.1:4321"
	get.Host = "127.0.0.1"
	getResponse := httptest.NewRecorder()
	handler.ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusMethodNotAllowed {
		t.Fatalf("candidate GET status=%d", getResponse.Code)
	}
}
