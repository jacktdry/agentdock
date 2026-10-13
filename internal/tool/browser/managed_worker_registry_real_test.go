//go:build darwin || linux

package browser

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Exercise the default Open path, including calls after its startup contexts
// have been canceled. Handshake readiness alone does not prove worker lifetime.
func TestManagedWorkerRegistryRealPostStart(t *testing.T) {
	if os.Getenv("AGENTDOCK_TEST_REAL_WORKER_POST_START") != "1" {
		t.Skip("opt-in only: AGENTDOCK_TEST_REAL_WORKER_POST_START=1")
	}
	if ManagedEnginePackageSpec != "chrome-devtools-mcp@1.7.0" {
		t.Fatal("this proof requires the pinned 1.7.0 contract")
	}
	cache := os.Getenv("NPM_CONFIG_CACHE")
	if cache == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		cache = filepath.Join(home, ".npm")
	}
	manifests, err := filepath.Glob(filepath.Join(cache, "_npx", "*", "node_modules", ManagedEnginePackage, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	cached := false
	for _, path := range manifests {
		data, err := os.ReadFile(path)
		var manifest struct{ Name, Version string }
		if err == nil && json.Unmarshal(data, &manifest) == nil && manifest.Name == ManagedEnginePackage && manifest.Version == ManagedEngineVersion {
			cached = true
			break
		}
	}
	if !cached {
		t.Skip("pinned package absent from npx cache; no network install permitted")
	}
	t.Setenv("NPM_CONFIG_OFFLINE", "true")
	t.Setenv("NPM_CONFIG_UPDATE_NOTIFIER", "false")
	cwd, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(cwd, "chrome-tmp")
	if err := os.Mkdir(tmp, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	pid := 0
	observed := make(map[int]string)
	r := NewWorkerRegistry(WorkerDependencies{StartedWitness: func(_ context.Context, ownedPID int) error {
		pid = ownedPID
		return nil
	}})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if _, err := realSharedProcesses(ctx, pid, tmp, observed); err != nil {
			t.Errorf("owned process observation: %v", err)
		}
		if err := r.Shutdown(ctx); err != nil {
			t.Errorf("owned registry shutdown: %s", realSharedDiagnosticText(err.Error()))
		}
		for {
			rows, err := realSharedProcesses(ctx, pid, tmp, observed)
			if err != nil {
				t.Errorf("cleanup observation: %v", err)
				return
			}
			if len(rows) == 0 {
				t.Log("owned MCP/Chrome processes gone")
				return
			}
			select {
			case <-ctx.Done():
				t.Errorf("owned cleanup pending: %d processes", len(rows))
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	})
	startCtx, cancelStart := context.WithCancel(context.Background())
	defer cancelStart()
	started := time.Now()
	w, err := r.StartManaged(startCtx, ManagedWorkerOptions{Cwd: cwd})
	if err != nil {
		t.Fatalf("StartManaged state=%s: %s", w.State, realSharedDiagnosticText(err.Error()))
	}
	if w.State != WorkerReady || pid <= 0 || w.Session.PID != pid {
		t.Fatal("ready worker did not match its owned start witness")
	}
	t.Log("default StartManaged ready")
	cancelStart()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	call := func(tool string, args map[string]any) map[string]any {
		t.Helper()
		result, err := r.Call(ctx, w.WorkerID, tool, args)
		if err == nil {
			err = leaseCallResultError(result)
		}
		if err != nil {
			t.Fatalf("post-start %s: %s", tool, realSharedDiagnosticText(err.Error()))
		}
		t.Logf("post-start %s succeeded", tool)
		return result
	}
	call("list_pages", map[string]any{})
	name := "agentdock-post-start"
	result := call("new_page", map[string]any{"url": "about:blank", "background": true, "isolatedContext": name})
	page, _, err := isolatedPageID(result, name)
	if err != nil {
		t.Fatal(realSharedDiagnosticText(err.Error()))
	}
	args := map[string]any{"pageId": page, "function": "() => location.href"}
	call("evaluate_script", args)
	// The default registry startup limit is 35s; verify it cannot expire the
	// already-ready transport, using only this worker's isolated blank page.
	if remaining := time.Until(started.Add(36 * time.Second)); remaining > 0 {
		select {
		case <-time.After(remaining):
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	call("list_pages", map[string]any{})
	call("evaluate_script", args)
	t.Log("worker persisted beyond startup deadline and caller cancellation")
	if _, err := realSharedProcesses(ctx, pid, tmp, observed); err != nil {
		t.Fatal(err)
	}
	if err := r.Stop(w.WorkerID); err != nil {
		t.Fatal(realSharedDiagnosticText(err.Error()))
	}
	info, err := r.Info(w.WorkerID)
	if err != nil || info.State != WorkerStopped {
		t.Fatal("explicit Stop did not reach stopped state")
	}
	t.Log("explicit Stop succeeded")
}
