//go:build darwin || linux

package desktopapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/app"
	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/desktopcontrol"
	"github.com/uvwt/agentdock/internal/permission"
	"github.com/uvwt/agentdock/internal/runtimeapi"
)

func TestPermissionServiceNativeBootstrapAndHTTPControl(t *testing.T) {
	tempRoot, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(tempRoot, "ad-m8-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	rt, err := app.NewRuntime(config.Config{AgentDockHome: filepath.Join(root, "home"), AgentDockDefaultDir: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	credential := rt.DesktopPermissionControlCredential()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- desktopcontrol.Serve(ctx, root, func(_ context.Context, request desktopcontrol.Request) (any, error) {
			if request.Method != "permission.bootstrap" {
				t.Errorf("unexpected native method %s", request.Method)
			}
			return rt.DesktopPermissionBootstrap(request.Params)
		})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(2 * time.Second):
			t.Error("native test IPC did not stop")
		}
	})
	for deadline := time.Now().Add(time.Second); ; {
		if token, err := bootstrapPermissionCredential(ctx, root); err == nil && token == credential {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("native bootstrap did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var result map[string]any
		var callErr error
		if r.Method == http.MethodPost {
			if r.Header.Get("Authorization") != "Bearer "+credential {
				t.Error("mutation used ordinary credential")
			}
			body := make([]byte, 0)
			var input runtimeapi.PermissionControlRequest
			if json.NewDecoder(r.Body).Decode(&input) != nil {
				t.Error("invalid mutation body")
			}
			body, _ = json.Marshal(input)
			result, callErr = runtimeapi.DispatchDesktopPermissionControl(r.Context(), rt, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), runtimeapi.Request{Method: r.Method, Path: r.URL.Path, Body: body})
		} else {
			if r.Header.Get("Authorization") != "Bearer ordinary-secret" {
				t.Error("read used desktop credential")
			}
			result, callErr = runtimeapi.Dispatch(r.Context(), rt, runtimeapi.Request{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query()})
		}
		if callErr != nil {
			te := callErr.(*app.ToolError)
			w.WriteHeader(409)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": te.Code, "error": "remote-secret-must-not-leak"})
			return
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()
	address, _ := url.Parse(server.URL)
	envPath := filepath.Join(root, "agentdock.env")
	if err := os.WriteFile(envPath, []byte("AGENTDOCK_HOST=127.0.0.1\nAGENTDOCK_PORT="+address.Port()+"\nAGENTDOCK_AUTH_TOKEN=ordinary-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.Marshal(map[string]any{"schema_version": 1, "agentdock_binary": filepath.Join(root, "core"), "environment_file": envPath})
	if err := os.WriteFile(filepath.Join(root, "desktop-runtime.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewPermissionService(root)
	status := service.Status(ctx)
	if status.Error != nil || status.Policy == nil {
		t.Fatalf("status: %#v", status)
	}
	next := *status.Policy
	next.GlobalMode = permission.Rules
	input := permission.ControlMutationRequest{Kind: permission.ControlMutationUpdatePolicy, PolicyRevision: next.Revision, Policy: &next}
	confirmation := service.BeginConfirmation(ctx, input)
	if confirmation.Error != nil || confirmation.Confirmation == nil {
		t.Fatalf("begin: %#v", confirmation)
	}
	id := confirmation.Confirmation.ID
	updated := service.UpdatePolicy(ctx, id, next.Revision, next)
	if updated.Error != nil || updated.Policy == nil || updated.Policy.GlobalMode != permission.Rules {
		t.Fatalf("update: %#v", updated)
	}
	replay := service.UpdatePolicy(ctx, id, next.Revision, next)
	if replay.Error == nil || replay.Error.Code != "CONFIRMATION_NOT_FOUND" || replay.Error.Category != ErrorCategoryConflict {
		t.Fatalf("replay: %#v", replay)
	}
	for _, result := range []PermissionResult{status, confirmation, updated, replay, service.History(ctx, "pending", 20)} {
		data, _ := json.Marshal(result)
		for _, secret := range []string{credential, "ordinary-secret", "remote-secret-must-not-leak"} {
			if strings.Contains(string(data), secret) {
				t.Fatal("frontend result leaked credential or raw remote error")
			}
		}
	}
	service.bootstrap = func(context.Context, string) (string, error) { return "", nil }
	failed := service.BeginConfirmation(ctx, input)
	if failed.Error == nil || failed.Error.Code != "DESKTOP_CONTROL_UNAUTHORIZED" {
		t.Fatal("bootstrap failure fell back to normal token")
	}
}

func TestPermissionContractOperationsAndAuthorityMetadata(t *testing.T) {
	result := NewContractService().Negotiate(NegotiationRequest{ClientVersion: ProtocolVersion, Domains: []Domain{DomainPermission}})
	if !result.Accepted || len(result.Capabilities) != 1 {
		t.Fatal("permission contract unavailable")
	}
	capability := result.Capabilities[0]
	if capability.Version != 1 || capability.Availability != AvailabilityAvailable || len(capability.Operations) != 8 {
		t.Fatal("permission contract incomplete")
	}
	for _, op := range capability.Operations {
		read := op.Name == "status" || op.Name == "history" || op.Name == "approval"
		if read && (op.Access != AccessRead || op.RequiresConfirmation) {
			t.Fatal("read operation requires mutation authority")
		}
		if !read && op.Access != AccessPrivileged {
			t.Fatal("permission mutation omitted native authority")
		}
		if !read && op.Name != "beginConfirmation" && !op.RequiresConfirmation {
			t.Fatal("mutation omitted challenge metadata")
		}
	}
}
