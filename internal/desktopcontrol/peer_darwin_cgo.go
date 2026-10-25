//go:build darwin && cgo

package desktopcontrol

/*
#include <libproc.h>
#include <sys/proc_info.h>
static int sender_process(int pid, char *path, struct proc_bsdinfo *info) {
    if (proc_pidpath(pid, path, PROC_PIDPATHINFO_MAXSIZE) <= 0) return 0;
    return proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, info, sizeof(*info)) == sizeof(*info);
}
*/
import "C"

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Neither the key nor an injector is exported. JSON, environment variables and
// contexts created by callers cannot manufacture accepted transport evidence.
type acceptedPeerKey struct{}
type acceptedPeer struct {
	pid     int
	uid     uint32
	process peerProcess
}
type peerProcess struct {
	pid                 int
	uid, realUID        uint32
	path                string
	birthSec, birthUsec uint64
}

func acceptedPeerContext(ctx context.Context, connection net.Conn) context.Context {
	// Shadow even an inherited value: each connection gets fresh evidence.
	peer := acceptedPeer{}
	if conn, ok := connection.(*net.UnixConn); ok {
		if raw, err := conn.SyscallConn(); err == nil {
			var captureErr error
			err = raw.Control(func(fd uintptr) {
				peer.pid, captureErr = unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
				if captureErr != nil {
					return
				}
				cred, err := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
				if err != nil {
					captureErr = err
					return
				}
				peer.uid = cred.Uid
			})
			if err == nil && captureErr == nil && peer.pid > 0 && peer.uid == uint32(os.Getuid()) {
				peer.process, _ = observePeerProcess(peer.pid)
			} else {
				peer = acceptedPeer{}
			}
		}
	}
	return context.WithValue(ctx, acceptedPeerKey{}, peer)
}

// VerifiedNextSender is a fresh read-only OS peer + bundle provenance check.
// It is NOT human identity, M8 consent, or authority to start/mutate anything.
// Ad-hoc signing and same-UID replacement remain outside this proof boundary.
func VerifiedNextSender(ctx context.Context) bool {
	if ctx == nil || ctx.Err() != nil {
		return false
	}
	peer, ok := ctx.Value(acceptedPeerKey{}).(acceptedPeer)
	if !ok || peer.pid <= 0 || peer.uid != uint32(os.Getuid()) {
		return false
	}
	core, err := observePeerProcess(os.Getpid())
	if err != nil {
		return false
	}
	executable, err := os.Executable()
	if err != nil || core.path != executable || core.uid != peer.uid || core.realUID != peer.uid {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return verifyNextSender(ctx, peer, core.path, observePeerProcess, verifyNextSenderBundle)
}

// Private seams allow predicate tests without installing or starting a GUI.
func verifyNextSender(ctx context.Context, peer acceptedPeer, core string, observe func(int) (peerProcess, error), verifyBundle func(context.Context, string) bool) bool {
	if ctx.Err() != nil || !sameNextPeer(peer, peer.process, core) {
		return false
	}
	before, err := observe(peer.pid)
	if err != nil || before != peer.process || !sameNextPeer(peer, before, core) {
		return false
	}
	if !verifyBundle(ctx, core) {
		return false
	}
	after, err := observe(peer.pid)
	return err == nil && after == before && sameNextPeer(peer, after, core) && ctx.Err() == nil
}

func nextSenderBundle(core string) string {
	if !filepath.IsAbs(core) || filepath.Clean(core) != core {
		return ""
	}
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(core)))
	if filepath.Base(bundle) != "AgentDock Next.app" || core != filepath.Join(bundle, "Contents", "Helpers", "agentdock") {
		return ""
	}
	return bundle
}

func sameNextPeer(peer acceptedPeer, process peerProcess, core string) bool {
	bundle := nextSenderBundle(core)
	return bundle != "" && peer.pid > 0 && peer.pid != os.Getpid() && peer.uid == uint32(os.Getuid()) &&
		process.pid == peer.pid && process.uid == peer.uid && process.realUID == peer.uid && process.birthSec > 0 && process.birthUsec < 1000000 &&
		process.path == filepath.Join(bundle, "Contents", "MacOS", "AgentDock")
}

func verifyNextSenderBundle(ctx context.Context, core string) bool {
	bundle := nextSenderBundle(core)
	if bundle == "" || ctx.Err() != nil {
		return false
	}
	gui := filepath.Join(bundle, "Contents", "MacOS", "AgentDock")
	plist := filepath.Join(bundle, "Contents", "Info.plist")
	before := make(map[string]os.FileInfo)
	for _, target := range []string{core, gui, plist} {
		for path := target; ; path = filepath.Dir(path) {
			if _, seen := before[path]; !seen {
				info, err := os.Lstat(path)
				if err != nil || !nextSenderPrivateEntry(path, bundle, info) {
					return false
				}
				before[path] = info
			}
			if path == string(filepath.Separator) {
				break
			}
		}
	}
	for _, path := range []string{core, gui} {
		if !before[path].Mode().IsRegular() || before[path].Mode().Perm()&0111 == 0 {
			return false
		}
	}
	if !before[plist].Mode().IsRegular() {
		return false
	}
	for _, item := range []struct{ key, value string }{
		{"CFBundleIdentifier", "dev.dropabit.agentdock.next"},
		{"AgentDockVariant", "next"},
		{"CFBundleExecutable", "AgentDock"},
	} {
		output, err := nextSenderTool(ctx, "/usr/bin/plutil", "-extract", item.key, "raw", "-o", "-", plist)
		if err != nil || strings.TrimSpace(string(output)) != item.value {
			return false
		}
	}
	for _, item := range []struct{ path, identifier string }{
		{core, "dev.dropabit.agentdock.next.core"},
		{gui, "dev.dropabit.agentdock.next"},
	} {
		if _, err := nextSenderTool(ctx, "/usr/bin/codesign", "--verify", "--strict", "-R", `=identifier "`+item.identifier+`"`, item.path); err != nil {
			return false
		}
	}
	if _, err := nextSenderTool(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", "-R", `=identifier "dev.dropabit.agentdock.next"`, bundle); err != nil {
		return false
	}
	for path, prior := range before {
		current, err := os.Lstat(path)
		if err != nil || !nextSenderPrivateEntry(path, bundle, current) || !os.SameFile(prior, current) || prior.Mode() != current.Mode() ||
			(prior.Mode().IsRegular() && (prior.Size() != current.Size() || !prior.ModTime().Equal(current.ModTime()))) {
			return false
		}
	}
	return ctx.Err() == nil
}

// Match the existing Next signed-launcher ownership policy. No aliases or
// writable ancestors; root-owned sticky temp ancestors permit inert fixtures.
func nextSenderPrivateEntry(path, bundle string, info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || (!info.IsDir() && !info.Mode().IsRegular()) || info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		return false
	}
	inside := path == bundle || strings.HasPrefix(path, bundle+string(filepath.Separator))
	if stat.Uid != uint32(os.Getuid()) && (inside || stat.Uid != 0) {
		return false
	}
	if info.Mode().Perm()&0022 != 0 && (inside || !info.IsDir() || stat.Uid != 0 || info.Mode()&os.ModeSticky == 0) {
		return false
	}
	if info.Mode().IsRegular() && stat.Nlink != 1 {
		return false
	}
	return true
}

func nextSenderTool(ctx context.Context, tool string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	cmd.WaitDelay = 100 * time.Millisecond
	return cmd.Output()
}

func observePeerProcess(pid int) (peerProcess, error) {
	if pid <= 0 {
		return peerProcess{}, errors.New("process evidence unavailable")
	}
	var path [C.PROC_PIDPATHINFO_MAXSIZE]C.char
	var info C.struct_proc_bsdinfo
	if C.sender_process(C.int(pid), &path[0], &info) == 0 || info.pbi_pid != C.uint32_t(pid) || info.pbi_start_tvsec == 0 {
		return peerProcess{}, errors.New("process evidence unavailable")
	}
	return peerProcess{pid: pid, uid: uint32(info.pbi_uid), realUID: uint32(info.pbi_ruid), path: C.GoString(&path[0]), birthSec: uint64(info.pbi_start_tvsec), birthUsec: uint64(info.pbi_start_tvusec)}, nil
}
