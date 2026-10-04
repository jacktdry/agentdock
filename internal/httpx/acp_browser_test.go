package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestACPBrowserLoopbackWrapperRejectsRemoteAndProxiedRequests(t *testing.T) {
	for _, tc := range []struct {
		remote, host, proxy string
		allowed             bool
	}{
		{"127.0.0.1:1234", "127.0.0.1:27123", "", true}, {"[::1]:1234", "[::1]:27123", "", true},
		{"198.51.100.5:1234", "127.0.0.1:27123", "", false}, {"127.0.0.1:1234", "public.example", "", false}, {"127.0.0.1:1234", "localhost", "198.51.100.1", false},
	} {
		called := false
		handler := loopbackOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(http.StatusNoContent) }))
		req := httptest.NewRequest("POST", "http://localhost/internal/acp-browser/mcp", nil)
		req.RemoteAddr = tc.remote
		req.Host = tc.host
		req.Header.Set("Authorization", "Bearer valid-capability")
		req.Header.Set("X-Forwarded-For", tc.proxy)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		want := http.StatusForbidden
		if tc.allowed {
			want = http.StatusNoContent
		}
		if called != tc.allowed || rec.Code != want {
			t.Fatalf("case=%+v called=%v status=%d", tc, called, rec.Code)
		}
	}
}
