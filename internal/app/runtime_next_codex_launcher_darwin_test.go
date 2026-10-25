//go:build darwin

package app

import (
	"context"
	"debug/macho"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNextCodexSignedLauncherRejectsSourceRunner(t *testing.T) {
	selection, err := selectNextCodexSignedLauncher()
	if err != errNextCodexLauncherUnavailable || selection.path != "" {
		t.Fatalf("source runner must not select a launcher: %v", err)
	}
}

func TestNextCodexSignedLauncherBundle(t *testing.T) {
	// Compile the actual launcher, but NEVER execute it (or the simulated
	// app/Core). Like the packaging fixture, sign only temporary bundle copies.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "launcher")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "../../cmd/agentdock-next-mcp-launch")
	build.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fixture launcher: %v: %s", err, output)
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	sign := func(path, identifier string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "/usr/bin/codesign", "--force", "--sign", "-", "--identifier", identifier, path)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("sign fixture: %v: %s", err, output)
		}
	}
	for _, scenario := range []string{
		"valid", "missing", "renamed", "symlink", "hardlink", "ancestor-symlink",
		"writable", "writable-bundle", "nonexecutable", "wrong-identifier",
		"wrong-core-identifier", "wrong-bundle-id", "wrong-variant", "tampered-launcher", "broken-outer-seal", "canceled",
	} {
		t.Run(scenario, func(t *testing.T) {
			dir, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			bundle := filepath.Join(dir, "AgentDock Next.app")
			contents := filepath.Join(bundle, "Contents")
			helpers := filepath.Join(contents, "Helpers")
			for _, path := range []string{helpers, filepath.Join(contents, "MacOS"), filepath.Join(contents, "Resources")} {
				if err := os.MkdirAll(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			write := func(path string, bytes []byte, mode os.FileMode) {
				t.Helper()
				if err := os.WriteFile(path, bytes, mode); err != nil {
					t.Fatal(err)
				}
			}
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			core := filepath.Join(helpers, "agentdock")
			launcher := filepath.Join(helpers, "agentdock-next-mcp-launch")
			write(core, data, 0700)
			write(launcher, data, 0700)
			write(filepath.Join(contents, "MacOS", "AgentDock"), data, 0700)
			id, variant := "dev.dropabit.agentdock.next", "next"
			if scenario == "wrong-bundle-id" {
				id = "com.uvwt.agentdock"
			}
			if scenario == "wrong-variant" {
				variant = "stable"
			}
			write(filepath.Join(contents, "Info.plist"), []byte(`<?xml version="1.0"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>`+id+`</string><key>AgentDockVariant</key><string>`+variant+`</string><key>CFBundleExecutable</key><string>AgentDock</string><key>CFBundlePackageType</key><string>APPL</string></dict></plist>`), 0600)
			resource := filepath.Join(contents, "Resources", "marker")
			write(resource, []byte("sealed"), 0600)
			sign(core, "dev.dropabit.agentdock.next.core")
			sign(launcher, "dev.dropabit.agentdock.next.mcp-launch")
			if scenario == "wrong-identifier" {
				sign(launcher, "dev.dropabit.agentdock.next.mcp-launch.extra")
			}
			if scenario == "wrong-core-identifier" {
				sign(core, "com.uvwt.agentdock.core")
			}
			sign(bundle, "dev.dropabit.agentdock.next")
			checkCtx := ctx
			switch scenario {
			case "missing":
				must(os.Remove(launcher))
			case "renamed":
				must(os.Rename(launcher, launcher+"-other"))
			case "symlink":
				must(os.Remove(launcher))
				must(os.Symlink(core, launcher))
			case "hardlink":
				must(os.Link(launcher, filepath.Join(dir, "alias")))
			case "ancestor-symlink":
				alias := filepath.Join(dir, "alias")
				must(os.Symlink(dir, alias))
				core = filepath.Join(alias, "AgentDock Next.app", "Contents", "Helpers", "agentdock")
			case "writable":
				must(os.Chmod(launcher, 0770))
			case "writable-bundle":
				must(os.Chmod(bundle, 0770))
			case "nonexecutable":
				must(os.Chmod(launcher, 0600))
			case "tampered-launcher":
				changed, err := os.ReadFile(launcher)
				must(err)
				file, err := macho.Open(launcher)
				must(err)
				section := file.Section("__text")
				if section == nil || section.Seg != "__TEXT" || section.Size == 0 || uint64(section.Offset)+section.Size > uint64(len(changed)) {
					file.Close()
					t.Fatal("fixture has no mapped code section")
				}
				changed[section.Offset] ^= 1
				must(file.Close())
				write(launcher, changed, 0700)
				// Prove this changes signed code, rather than unsigned padding in
				// the signature container. The native verifier must reject it too.
				output, err := exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--strict", launcher).CombinedOutput()
				if err == nil || ctx.Err() != nil || !strings.Contains(string(output), "invalid signature") {
					t.Fatalf("native codesign did not prove signature rejection: %v: %s", err, output)
				}
				t.Logf("native tamper rejection: %s", strings.ReplaceAll(strings.TrimSpace(string(output)), launcher, "<fixture-launcher>"))
			case "broken-outer-seal":
				write(resource, []byte("changed"), 0600)
			case "canceled":
				var stop context.CancelFunc
				checkCtx, stop = context.WithCancel(ctx)
				stop()
			}
			selection, err := selectNextCodexSignedLauncherForExecutable(checkCtx, core)
			if scenario == "valid" {
				if err != nil || selection.path != launcher {
					t.Fatalf("signed simulated bundle rejected: %v", err)
				}
				output, err := exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", "--verbose=2", bundle).CombinedOutput()
				must(err)
				t.Logf("native positive verification: %s", strings.ReplaceAll(strings.TrimSpace(string(output)), bundle, "<fixture-bundle>"))
			} else if err != errNextCodexLauncherUnavailable || selection.path != "" {
				t.Fatalf("unsafe selection accepted or raw error leaked: %v", err)
			}
		})
	}
}

func TestNextCodexLauncherToolCancellationIsRedacted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if output, err := nextCodexLauncherTool(ctx, "/usr/bin/codesign", "--verify", "/private/canary"); err != errNextCodexLauncherUnavailable || output != nil {
		t.Fatalf("canceled verifier leaked output/error: %v", err)
	}
}
