//go:build darwin

package app

import (
	"context"
	"debug/buildinfo"
	"debug/macho"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var errNextCodexLauncherUnavailable = errors.New("Next signed launcher unavailable")

// A private value snapshot, not launch authority. Never serialize this path or
// accept a replacement from a client. A future start owner must revalidate and
// bind the backend/argv allowlist and independently authenticated M8 operator.
type nextCodexSignedLauncher struct{ path string }

func selectNextCodexSignedLauncher() (nextCodexSignedLauncher, error) {
	executable, err := os.Executable()
	if err != nil {
		return nextCodexSignedLauncher{}, errNextCodexLauncherUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return selectNextCodexSignedLauncherForExecutable(ctx, executable)
}

// The only production entry above supplies os.Executable(), never a base path,
// environment override, caller digest, or desktop request parameter.
func selectNextCodexSignedLauncherForExecutable(ctx context.Context, executable string) (nextCodexSignedLauncher, error) {
	fail := func() (nextCodexSignedLauncher, error) {
		return nextCodexSignedLauncher{}, errNextCodexLauncherUnavailable
	}
	if ctx == nil || ctx.Err() != nil || !filepath.IsAbs(executable) || filepath.Clean(executable) != executable {
		return fail()
	}
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(executable)))
	if filepath.Base(bundle) != "AgentDock Next.app" || executable != filepath.Join(bundle, "Contents", "Helpers", "agentdock") {
		return fail()
	}
	launcher := filepath.Join(bundle, "Contents", "Helpers", "agentdock-next-mcp-launch")
	plist := filepath.Join(bundle, "Contents", "Info.plist")
	// Refuse aliases in every ancestor, including outside the bundle. Bundle
	// entries must be owned by this operator, with no group/world write or
	// special bits; ancestors may also be root-owned. Root-owned sticky temp
	// directories allow isolated signed fixtures without trusting other writers.
	before := make(map[string]os.FileInfo)
	for _, target := range []string{executable, launcher, plist} {
		for path := target; ; path = filepath.Dir(path) {
			if _, seen := before[path]; !seen {
				info, err := os.Lstat(path)
				if err != nil || !nextCodexLauncherPrivateEntry(path, bundle, info) {
					return fail()
				}
				before[path] = info
			}
			if path == string(filepath.Separator) {
				break
			}
		}
	}
	for _, path := range []string{executable, launcher} {
		if !before[path].Mode().IsRegular() || before[path].Mode().Perm()&0111 == 0 || !nextCodexLauncherARM64(path) {
			return fail()
		}
	}
	if !before[plist].Mode().IsRegular() {
		return fail()
	}
	metadata, err := buildinfo.ReadFile(launcher)
	if err != nil || metadata.Path != "github.com/uvwt/agentdock/cmd/agentdock-next-mcp-launch" {
		return fail()
	}
	settings := make(map[string]string)
	for _, setting := range metadata.Settings {
		settings[setting.Key] = setting.Value
	}
	if settings["GOOS"] != "darwin" || settings["GOARCH"] != "arm64" {
		return fail()
	}
	for _, item := range []struct{ key, value string }{
		{"CFBundleIdentifier", "dev.dropabit.agentdock.next"},
		{"AgentDockVariant", "next"},
	} {
		output, err := nextCodexLauncherTool(ctx, "/usr/bin/plutil", "-extract", item.key, "raw", "-o", "-", plist)
		if err != nil || strings.TrimSpace(string(output)) != item.value {
			return fail()
		}
	}
	for _, item := range []struct{ path, identifier string }{
		{executable, "dev.dropabit.agentdock.next.core"},
		{launcher, "dev.dropabit.agentdock.next.mcp-launch"},
	} {
		if _, err := nextCodexLauncherTool(ctx, "/usr/bin/codesign", "--verify", "--strict", "-R", `=identifier "`+item.identifier+`"`, item.path); err != nil {
			return fail()
		}
	}
	if _, err := nextCodexLauncherTool(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", "-R", `=identifier "dev.dropabit.agentdock.next"`, bundle); err != nil {
		return fail()
	}
	// Reject observed replacement during verification. This read-only snapshot
	// cannot prevent subsequent same-user writes and must never authorize exec.
	for path, prior := range before {
		current, err := os.Lstat(path)
		if err != nil || !nextCodexLauncherPrivateEntry(path, bundle, current) || !os.SameFile(prior, current) || prior.Mode() != current.Mode() {
			return fail()
		}
		if prior.Mode().IsRegular() && (prior.Size() != current.Size() || !prior.ModTime().Equal(current.ModTime())) {
			return fail()
		}
	}
	if ctx.Err() != nil {
		return fail()
	}
	return nextCodexSignedLauncher{path: launcher}, nil
}

func nextCodexLauncherPrivateEntry(path, bundle string, info os.FileInfo) bool {
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
	return !info.Mode().IsRegular() || stat.Nlink == 1
}

func nextCodexLauncherARM64(path string) bool {
	file, err := macho.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	return file.Cpu == macho.CpuArm64 && file.Type == macho.TypeExec
}

func nextCodexLauncherTool(ctx context.Context, tool string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	cmd.WaitDelay = 100 * time.Millisecond
	output, err := cmd.Output()
	if err != nil {
		return nil, errNextCodexLauncherUnavailable
	}
	return output, nil
}
