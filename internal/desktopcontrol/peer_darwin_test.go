//go:build darwin && cgo

package desktopcontrol

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestNextSenderPredicateRejectsUnboundOrChangedProcess(t *testing.T) {
	core := "/Applications/AgentDock Next.app/Contents/Helpers/agentdock"
	process := peerProcess{pid: os.Getpid() + 1, uid: uint32(os.Getuid()), realUID: uint32(os.Getuid()), path: "/Applications/AgentDock Next.app/Contents/MacOS/AgentDock", birthSec: 123, birthUsec: 456}
	for _, scenario := range []string{"valid", "wrong-uid", "wrong-pid", "wrong-process-uid", "wrong-real-uid", "foreign-path", "missing-birth", "birth-microseconds", "stale-at-accept", "reused-pid", "exited", "untrusted-bundle", "canceled", "stable-core"} {
		t.Run(scenario, func(t *testing.T) {
			peer := acceptedPeer{pid: process.pid, uid: process.uid, process: process}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ownCore := core
			switch scenario {
			case "wrong-uid":
				peer.uid++
			case "wrong-pid":
				peer.pid++
			case "wrong-process-uid":
				peer.process.uid++
			case "wrong-real-uid":
				peer.process.realUID++
			case "foreign-path":
				peer.process.path = "/tmp/AgentDock"
			case "missing-birth":
				peer.process.birthSec = 0
			case "birth-microseconds":
				peer.process.birthUsec = 1000000
			case "stale-at-accept":
				peer.process.birthUsec++
			case "canceled":
				cancel()
			case "stable-core":
				ownCore = "/Applications/AgentDock.app/Contents/Helpers/agentdock"
			}
			observations, verifications := 0, 0
			got := verifyNextSender(ctx, peer, ownCore, func(pid int) (peerProcess, error) {
				observations++
				if pid != process.pid {
					t.Fatal("observer given caller PID")
				}
				if scenario == "exited" {
					return peerProcess{}, errors.New("gone")
				}
				p := process
				if scenario == "reused-pid" && observations == 2 {
					p.birthUsec++
				}
				return p, nil
			}, func(context.Context, string) bool { verifications++; return scenario != "untrusted-bundle" })
			if got != (scenario == "valid") {
				t.Fatalf("verified=%v", got)
			}
			if scenario == "valid" && (observations != 2 || verifications != 1) {
				t.Fatal("missing double process observation")
			}
		})
	}
}

func TestNextSenderCannotBeSuppliedByCaller(t *testing.T) {
	ctx := context.WithValue(context.Background(), "acceptedPeerKey", map[string]any{"pid": os.Getpid(), "uid": os.Getuid(), "path": "/Applications/AgentDock Next.app/Contents/MacOS/AgentDock"})
	if VerifiedNextSender(ctx) || VerifiedNextSender(context.Background()) || VerifiedNextSender(nil) {
		t.Fatal("caller manufactured transport evidence")
	}
	reply := handleMessage(ctx, []byte(`{"id":"1","method":"next_codex.operator_identity","params":{"pid":1,"uid":0,"path":"/Applications/AgentDock Next.app/Contents/MacOS/AgentDock","digest":"trusted"}}`), func(ctx context.Context, _ Request) (any, error) {
		return VerifiedNextSender(ctx), nil
	})
	var result response
	if err := jsonUnmarshal(reply, &result); err != nil || string(result.Result) != "false" {
		t.Fatalf("request spoof accepted: %s", reply)
	}
}

func TestNextSenderEntryRejectsUnsafeOwnershipAndModes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Mode and ownership rejection matches the existing signed-launcher policy.
	if err := os.Chmod(path, 0666); err != nil {
		t.Fatal(err)
	}
	unsafeInfo, _ := os.Lstat(path)
	if nextSenderPrivateEntry(path, dir, unsafeInfo) {
		t.Fatal("writable entry accepted")
	}
	stat := *info.Sys().(*syscall.Stat_t)
	stat.Uid = uint32(os.Getuid()) + 1
	if nextSenderPrivateEntry(path, dir, senderFileInfo{FileInfo: info, stat: &stat}) {
		t.Fatal("foreign owner accepted")
	}
}

type senderFileInfo struct {
	os.FileInfo
	stat *syscall.Stat_t
}

func (i senderFileInfo) Sys() any { return i.stat }

func TestAcceptedSocketBindsKernelPeerBeforeHandler(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "ad-peer-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	listener, err := net.Listen("unix", filepath.Join(root, "peer.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		peerCtx := acceptedPeerContext(ctx, conn)
		serveConnection(peerCtx, conn, func(ctx context.Context, _ []byte) []byte {
			peer, ok := ctx.Value(acceptedPeerKey{}).(acceptedPeer)
			if !ok || peer.pid != os.Getpid() || peer.uid != uint32(os.Getuid()) || peer.process.pid != os.Getpid() || peer.process.birthSec == 0 {
				done <- errors.New("kernel peer or complete process birth missing")
			} else if VerifiedNextSender(ctx) {
				done <- errors.New("test executable became a Next GUI")
			} else {
				done <- nil
			}
			return []byte("ok")
		})
	}()
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("caller PID/path must not matter")); err != nil {
		t.Fatal(err)
	}
	if err := conn.(*net.UnixConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestNextSenderTemporarySignedBundle(t *testing.T) {
	// Small inert console executable, never an installed app or GUI. This tests
	// provenance, including ad-hoc signatures, not a trusted human/operator UAT.
	data, err := os.ReadFile("/bin/sleep")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"valid", "unsigned", "wrong-identifier", "outer-seal-tampered", "gui-symlink", "parent-writable"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			bundle := filepath.Join(root, "AgentDock Next.app")
			core := filepath.Join(bundle, "Contents", "Helpers", "agentdock")
			gui := filepath.Join(bundle, "Contents", "MacOS", "AgentDock")
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, path := range []string{core, gui} {
				must(os.MkdirAll(filepath.Dir(path), 0700))
				must(os.WriteFile(path, data, 0700))
			}
			plist := filepath.Join(bundle, "Contents", "Info.plist")
			must(os.WriteFile(plist, []byte(`<?xml version="1.0"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>dev.dropabit.agentdock.next</string><key>AgentDockVariant</key><string>next</string><key>CFBundleExecutable</key><string>AgentDock</string><key>CFBundlePackageType</key><string>APPL</string></dict></plist>`), 0600))
			tool := func(args ...string) {
				t.Helper()
				if output, err := exec.CommandContext(ctx, "/usr/bin/codesign", args...).CombinedOutput(); err != nil {
					t.Fatalf("codesign fixture: %v: %s", err, output)
				}
			}
			tool("--force", "--sign", "-", "--identifier", "dev.dropabit.agentdock.next.core", core)
			identifier := "dev.dropabit.agentdock.next"
			if scenario == "wrong-identifier" {
				identifier = "foreign.gui"
			}
			tool("--force", "--sign", "-", "--identifier", identifier, gui)
			tool("--force", "--sign", "-", "--identifier", identifier, bundle)
			switch scenario {
			case "unsigned":
				tool("--remove-signature", gui)
			case "outer-seal-tampered":
				must(os.WriteFile(plist, append(mustReadSenderFile(t, plist), []byte("\n<!-- tampered -->")...), 0600))
			case "gui-symlink":
				must(os.Rename(gui, filepath.Join(root, "foreign")))
				must(os.Symlink(filepath.Join(root, "foreign"), gui))
			case "parent-writable":
				must(os.Chmod(root, 0777))
			}
			if got := verifyNextSenderBundle(ctx, core); got != (scenario == "valid") {
				t.Fatalf("bundle verified=%v", got)
			}
			if scenario != "valid" {
				return
			}
			child := exec.CommandContext(ctx, gui, "30")
			must(child.Start())
			defer func() { cancel(); _ = child.Wait() }()
			process, err := observePeerProcess(child.Process.Pid)
			must(err)
			peer := acceptedPeer{pid: child.Process.Pid, uid: uint32(os.Getuid()), process: process}
			if !verifyNextSender(ctx, peer, core, observePeerProcess, verifyNextSenderBundle) {
				t.Fatal("running signed fixture provenance rejected")
			}
		})
	}
}

func mustReadSenderFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
