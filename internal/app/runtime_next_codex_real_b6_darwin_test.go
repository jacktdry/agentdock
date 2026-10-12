//go:build darwin && next_b6_real

package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	acpruntime "github.com/uvwt/agentdock/internal/acp"
	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/httpx/requestmeta"
	"github.com/uvwt/agentdock/internal/permission"
	toolacp "github.com/uvwt/agentdock/internal/tool/acp"
)

// Explicit release-gate smoke. This test NEVER copies auth material, calls a
// remote model, edits global ~/.codex, or uses a user-owned MCP. The launched
// real native Codex app-server and all children belong to this test-only Core.
// Invocation: go test -tags next_b6_real -run TestB6RealNextCodex -timeout=150s ./internal/app
func TestB6RealNextCodexAdapterNativeWrapperMCP(t *testing.T) {
	launcher := os.Getenv("NEXT_B6_OWNED_LAUNCHER")
	native := os.Getenv("NEXT_B6_NATIVE_CODEX_BINARY")
	adapter := os.Getenv("NEXT_B6_CODEX_ACP_ENTRY")
	if launcher == "" || native == "" || adapter == "" {
		t.Skip("explicit real source-only B6 binaries not supplied")
	}
	for _, path := range []string{launcher, native, adapter} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	root := os.Getenv("NEXT_B6_AUTH_FIXTURE_ROOT")
	if root == "" {
		created, err := os.MkdirTemp("/private/tmp", "next-b6-real-")
		if err != nil {
			t.Fatal(err)
		}
		root = created
		t.Cleanup(func() { _ = os.RemoveAll(root) })
	} else {
		canonical, err := filepath.EvalSymlinks(root)
		info, statErr := os.Lstat(root)
		if err != nil || statErr != nil || canonical != root || !filepath.IsAbs(root) || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm() != 0700 || !b6FileOwned(info) {
			t.Fatal("dedicated auth fixture root must be real absolute owner-private directory")
		}
		// The user explicitly provisioned and signed into this separate
		// fixture; NEVER delete its authentication material in test cleanup.
	}
	t.Setenv("HOME", root)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	t.Setenv(nextCodexMCPRegistrationEnv, "source-only")
	state := filepath.Join(root, ".agentdock-next")
	if err := ensureB6PrivateDir(state); err != nil {
		t.Fatal(err)
	}
	isolated := filepath.Join(state, "codex-mcp-isolated")
	if err := ensureB6PrivateDir(isolated); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(root, "work")
	if err := ensureB6PrivateDir(work); err != nil {
		t.Fatal(err)
	}
	backendFile, err := os.CreateTemp(root, "owned-b6-mcp-*.py")
	if err != nil {
		t.Fatal(err)
	}
	backend := backendFile.Name()
	_ = backendFile.Close()
	t.Cleanup(func() { _ = os.Remove(backend) })
	src := `import json,sys
for line in sys.stdin:
 try: m=json.loads(line)
 except: continue
 if 'id' not in m: continue
 meth=m.get('method')
 if meth=='initialize': response={'protocolVersion':'2024-11-05','capabilities':{'tools':{}},'serverInfo':{'name':'next-owned-b6','version':'1'}}
 elif meth=='tools/list': response={'tools':[{'name':'ping','description':'Test only','inputSchema':{'type':'object','properties':{}}}]}
 elif meth=='tools/call': response={'content':[{'type':'text','text':'PONG'}]}
 else: response={}
 print(json.dumps({'jsonrpc':'2.0','id':m['id'],'result':response}),flush=True)
`
	if err := os.WriteFile(backend, []byte(src), 0600); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	secret := hex.EncodeToString(key)
	envToCopy := map[string]string{
		"HOME": "NEXT_B6_TEMP_HOME", "CODEX_HOME": "NEXT_B6_TEMP_CODEX_HOME", "CODEX_PATH": "NEXT_B6_TEMP_NATIVE", "AGENTDOCK_NEXT_MCP_OWNER_DIAGNOSTIC_KEY": "NEXT_B6_TEMP_OWNER_KEY", "AGENTDOCK_NEXT_CODEX_MCP_ISOLATION": "NEXT_B6_TEMP_ISOLATION", "PATH": "NEXT_B6_TEMP_PATH",
	}
	t.Setenv("NEXT_B6_TEMP_HOME", root)
	t.Setenv("NEXT_B6_TEMP_CODEX_HOME", isolated)
	t.Setenv("NEXT_B6_TEMP_NATIVE", native)
	t.Setenv("NEXT_B6_TEMP_OWNER_KEY", secret)
	t.Setenv("NEXT_B6_TEMP_ISOLATION", "source-only")
	t.Setenv("NEXT_B6_TEMP_PATH", os.Getenv("PATH"))
	profile := config.ACPProfile{ID: "codex", Kind: "codex", Enabled: true, Command: "/opt/homebrew/bin/node", Args: []string{"--import", filepath.Join(filepath.Dir(adapter), "..", "node_modules", "tsx", "dist", "loader.mjs"), adapter}, EnvFromEnv: envToCopy}
	cfg := config.Config{AgentDockHome: state, AgentDockDefaultDir: work, ACPEnabled: true, ACPProfiles: []config.ACPProfile{profile}, ACPDefaultProfile: "codex"}
	rt, err := NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rt.Close(); err != nil {
			t.Errorf("owned Core Close: %v", err)
		}
	}()
	cur := rt.permissions.Policy()
	next := cur
	next.GlobalMode = permission.Full
	if _, err := rt.permissions.ReplacePolicy(cur.Revision, next); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(requestmeta.WithAuthPrincipal(context.Background(), requestmeta.AuthPrincipal{Kind: "test", ID: "owned-b6", Authenticated: true, Stable: true}), 105*time.Second)
	defer cancel()
	c := nextCodexMCPConfig{ServerID: "owned_b6", WrapperExecutable: launcher, BackendExecutable: "/usr/bin/python3", BackendArgs: []string{"-u", backend}}
	c.ConfigDigest = nextCodexMCPConfigDigest(c.ServerID, c.WrapperExecutable, c.BackendExecutable, c.BackendArgs)
	socket, err := rt.prepareNextCodexMCPRegistry(ctx, profile, secret, c)
	if err != nil {
		t.Fatalf("B6 native Adapter and HMAC birth bootstrap: %v", err)
	}
	if socket == "" {
		t.Fatal("Core socket not opened")
	}
	if err := rt.attachNextCodexInstrumentedMCP(ctx, c); err != nil {
		t.Fatalf("B6 Core M8 stdio wrapper install: %v", err)
	}
	result, err := rt.acp.Session(ctx, toolacp.SessionRequest{Action: "new", ProfileID: "codex", CWD: work})
	if err != nil {
		entries, _ := os.ReadDir(isolated)
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			info, _ := os.Lstat(filepath.Join(isolated, entry.Name()))
			if info != nil {
				names = append(names, fmt.Sprintf("%s:%04o", entry.Name(), info.Mode().Perm()))
			}
		}
		t.Logf("native-owned isolated home entries (names only): %v", names)
		t.Fatalf("B6 real native Codex thread start: %v", err)
	}
	session, ok := result["session"].(acpruntime.SessionRecord)
	if !ok || session.ID == "" {
		t.Fatalf("B6 session result not a live local session: %T", result["session"])
	}
	defer func() {
		if _, err := rt.acp.Session(context.Background(), toolacp.SessionRequest{Action: "close", ProfileID: "codex", SessionID: session.ID}); err != nil {
			t.Errorf("owned native Session Close: %v", err)
		}
	}()
	deadline := time.Now().Add(9 * time.Second)
	for time.Now().Before(deadline) {
		if len(rt.NextCodexMCPRegistrationSnapshot()) == 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	observed := rt.NextCodexMCPRegistrationSnapshot()
	if len(observed) != 1 {
		t.Fatalf("B6 real connected MCP had no kernel verified wrapper chain: %d", len(observed))
	}
	t.Logf("B6 native end-to-end kernel provenance: app-server pid=%d wrapper pid=%d child pid=%d", observed[0].AppServer.PID, observed[0].Wrapper.PID, observed[0].Child.PID)
}

func ensureB6PrivateDir(path string) error {
	err := os.Mkdir(path, 0700)
	if err != nil && !os.IsExist(err) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 || !b6FileOwned(info) {
		return fmt.Errorf("B6 owner-private directory invalid: %s", path)
	}
	return nil
}

func b6FileOwned(info os.FileInfo) bool {
	if info == nil {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Uid == uint32(os.Geteuid())
}
