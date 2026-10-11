//go:build darwin

package launchregistry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/process"
)

// exec.Cmd pipes copy output asynchronously until Wait finishes. Every
// fixture diagnostic read must synchronize with those writer goroutines.
type fixtureSynchronizedOutput struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (o *fixtureSynchronizedOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buffer.Write(p)
}
func (o *fixtureSynchronizedOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buffer.String()
}

func fixturePrivateDirectory(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/private/tmp", "nxcore-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) }) // unique test-owned mkdtemp only
	return dir
}

func TestPrivateCoreListenerOwnsOnlyUniqueSocket(t *testing.T) {
	private := fixturePrivateDirectory(t)
	provider := func(context.Context) (TrustedScope, error) { return TrustedScope{}, ErrUnavailable }
	first, err := StartPrivateCoreListener(private, provider, "echo", testDigest)
	if err != nil {
		t.Fatal(err)
	}
	second, err := StartPrivateCoreListener(private, provider, "echo", testDigest)
	if err != nil {
		_ = first.Close()
		t.Fatal(err)
	}
	if first.SocketPath() == second.SocketPath() {
		t.Fatal("socket collision")
	}
	info, err := os.Stat(first.SocketPath())
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("socket mode = %#v %v", info, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal("double close: ", err)
	}
	if _, err := os.Stat(first.SocketPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("owned socket still exists")
	}
	if _, err := os.Stat(second.SocketPath()); err != nil {
		t.Fatal("other listener incorrectly removed")
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if len(first.Snapshot()) != 0 {
		t.Fatal("unexpected rows")
	}
}

func TestPrivateCoreListenerRejectsUnsafeRoot(t *testing.T) {
	root := fixturePrivateDirectory(t)
	bad := filepath.Join(root, "world")
	if err := os.Mkdir(bad, 0777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(bad, 0777); err != nil {
		t.Fatal(err)
	}
	provider := func(context.Context) (TrustedScope, error) { return TrustedScope{}, nil }
	if l, err := StartPrivateCoreListener(bad, provider, "echo", testDigest); err == nil {
		_ = l.Close()
		t.Fatal("world-writable root accepted")
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(bad, link); err != nil {
		t.Fatal(err)
	}
	if l, err := StartPrivateCoreListener(link, provider, "echo", testDigest); err == nil {
		_ = l.Close()
		t.Fatal("symlink accepted")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(bad); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateCoreListenerVerifiesRealAdapterAppServerWrapperMCPChain(t *testing.T) {
	root := fixturePrivateDirectory(t)
	appPIDFile := filepath.Join(root, "native-pid")
	goFile := filepath.Join(root, "go")
	doneFile := filepath.Join(root, "done")
	releaseFile := filepath.Join(root, "release")
	core, err := process.ObserveBirthIdentity(context.Background(), os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	coreJSON, err := json.Marshal(core)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.RWMutex
	var expected TrustedScope
	provider := func(ctx context.Context) (TrustedScope, error) {
		mu.RLock()
		defer mu.RUnlock()
		if expected.Adapter.PID == 0 {
			return TrustedScope{}, ErrUnavailable
		}
		return expected, nil
	}
	svc, err := StartPrivateCoreListener(root, provider, "echo", testDigest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := svc.Close(); err != nil {
			t.Errorf("owned listener cleanup: %v", err)
		}
	}()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLaunchRegistryHelperProcess$")
	cmd.Env = append(os.Environ(), "LAUNCH_REG_HELPER=adapter", "LAUNCH_TEST_APP_PID_FILE="+appPIDFile,
		"LAUNCH_TEST_GO_FILE="+goFile, "LAUNCH_TEST_DONE_FILE="+doneFile, "LAUNCH_TEST_RELEASE_FILE="+releaseFile,
		"LAUNCH_TEST_CORE_IDENTITY="+string(coreJSON), "LAUNCH_TEST_SERVER=echo", "LAUNCH_TEST_DIGEST="+testDigest,
		"AGENTDOCK_NEXT_MCP_LAUNCH_SOCKET="+svc.SocketPath())
	var output fixtureSynchronizedOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }() // strictly fixture-owned descendant
	if !waitFixtureFile(appPIDFile, 3*time.Second) {
		t.Fatalf("no appserver fixture PID: %s", output.String())
	}
	rawPID, err := os.ReadFile(appPIDFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(rawPID)))
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := process.ObserveBirthIdentity(context.Background(), cmd.Process.Pid)
	if err != nil {
		t.Fatal("fixture Adapter birth: ", err)
	}
	native, err := process.ObserveBirthIdentity(context.Background(), pid)
	if err != nil {
		t.Fatal("fixture native AppServer birth: ", err)
	}
	if adapter.ParentPID != os.Getpid() || native.ParentPID != adapter.PID || native.EffectiveUID != adapter.EffectiveUID {
		t.Fatalf("invalid test-owned lineage: core=%d adapter=%+v appserver=%+v", os.Getpid(), adapter, native)
	}
	mu.Lock()
	expected = TrustedScope{Adapter: adapter, AppServer: native}
	mu.Unlock()
	if err := os.WriteFile(goFile, []byte("1"), 0600); err != nil {
		t.Fatal(err)
	}
	if !waitFixtureFile(doneFile, 5*time.Second) {
		t.Fatalf("fixture MCP failed: %s", output.String())
	}
	var observed []Observation
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		observed = svc.Snapshot()
		if len(observed) != 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(observed) != 1 {
		t.Fatalf("verified Core registrar did not record chain: rows=%d output=%s", len(observed), output.String())
	}
	if observed[0].AppServer != native || observed[0].Wrapper.ParentPID != native.PID || observed[0].Child.ParentPID != observed[0].Wrapper.PID || !strings.Contains(output.String(), "mcp:hello\n") {
		t.Fatalf("wrong owned chain %+v output=%s", observed[0], output.String())
	}
	if err := os.WriteFile(releaseFile, []byte("1"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("test-only process chain error %v: %s", err, output.String())
	}
}

func TestPrivateCoreListenerRejectsUnverifiedOwnership(t *testing.T) {
	root := fixturePrivateDirectory(t)
	ctx := context.Background()
	core, _ := process.ObserveBirthIdentity(ctx, os.Getpid())
	// A fake local-process-only adapter/native pair never qualifies as Core's
	// actually running dedicated ACP Adapter→app-server lineage.
	wrong := TrustedScope{Adapter: core, AppServer: core}
	svc, err := StartPrivateCoreListener(root, func(context.Context) (TrustedScope, error) { return wrong, nil }, "echo", testDigest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	marker := filepath.Join(root, "forbidden")
	stdout, runErr := spawnWrapper(t, svc.SocketPath(), "echo", testDigest, "wrapper", "", marker)
	if runErr == nil || stdout != "" || len(svc.Snapshot()) != 0 {
		t.Fatalf("unexpected launched MCP: err=%v out=%q rows=%d", runErr, stdout, len(svc.Snapshot()))
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unauthorized wrapper spawned child: %v", err)
	}
}
