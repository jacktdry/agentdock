//go:build darwin

package app

import (
	"context"
	"debug/macho"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestNextNativeMCPServerRejectsSourceRunner(t *testing.T) {
	c, err := resolveNextNativeMCPServer("fixture")
	if err != errNextNativeMCPUnavailable || !reflect.DeepEqual(c, nextCodexMCPConfig{}) {
		t.Fatalf("source runner selected a backend: %v", err)
	}
}

func TestNextNativeMCPServerSignedBundle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Reuse real launcher bytes as inert native fixture files. None of these
	// executables run; this is verifier evidence, not a demo MCP backend/UAT.
	binary := filepath.Join(root, "launcher")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "../../cmd/agentdock-next-mcp-launch")
	build.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v: %s", err, output)
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{
		"valid", "unknown-id", "path-as-id", "empty", "missing-catalog", "unknown-field", "arbitrary-path",
		"catalog-symlink", "catalog-hardlink", "catalog-writable", "backend-symlink", "backend-hardlink",
		"backend-writable", "backend-nonexecutable", "backend-script", "backend-library", "backend-x86",
		"backend-ancestor-symlink", "backend-wrong-identifier", "backend-other-server-identifier",
		"catalog-tampered", "backend-tampered", "marker-missing", "marker-nonshared", "canceled",
	} {
		t.Run(scenario, func(t *testing.T) {
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			dir, err := filepath.EvalSymlinks(t.TempDir())
			must(err)
			bundle := filepath.Join(dir, "AgentDock Next.app")
			contents := filepath.Join(bundle, "Contents")
			helpers := filepath.Join(contents, "Helpers")
			resources := filepath.Join(contents, "Resources")
			for _, path := range []string{filepath.Join(helpers, "native-mcp"), resources, filepath.Join(contents, "MacOS")} {
				must(os.MkdirAll(path, 0700))
			}
			write := func(path string, data []byte, mode os.FileMode) { t.Helper(); must(os.WriteFile(path, data, mode)) }
			sign := func(path, identifier string) {
				t.Helper()
				if output, err := exec.CommandContext(ctx, "/usr/bin/codesign", "--force", "--sign", "-", "--identifier", identifier, path).CombinedOutput(); err != nil {
					t.Fatalf("sign fixture: %v: %s", err, output)
				}
			}
			core := filepath.Join(helpers, "agentdock")
			launcher := filepath.Join(helpers, "agentdock-next-mcp-launch")
			backend := filepath.Join(helpers, "native-mcp", "fixture")
			for _, path := range []string{core, launcher, backend, filepath.Join(contents, "MacOS", "AgentDock")} {
				write(path, data, 0700)
			}
			write(filepath.Join(contents, "Info.plist"), []byte(`<?xml version="1.0"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>dev.dropabit.agentdock.next</string><key>AgentDockVariant</key><string>next</string><key>CFBundleExecutable</key><string>AgentDock</string><key>CFBundlePackageType</key><string>APPL</string></dict></plist>`), 0600)
			marker := filepath.Join(resources, "desktop-product.json")
			write(marker, []byte(`{"schema_version":1,"product_name":"AgentDock Next","desktop_variant":"next","ui":"shared-wails"}`), 0600)
			catalog := filepath.Join(resources, "native-mcp-servers.json")
			manifest := `{"schema_version":1,"servers":[{"id":"fixture","executable":"Helpers/native-mcp/fixture","args":["--stdio","literal $HOME; echo x"]}]}`
			switch scenario {
			case "empty":
				manifest = `{"schema_version":1,"servers":[]}`
			case "unknown-field":
				manifest = `{"schema_version":1,"servers":[],"env":{"PATH":"/tmp"}}`
			case "arbitrary-path":
				manifest = `{"schema_version":1,"servers":[{"id":"fixture","executable":"/bin/echo"}]}`
			}
			write(catalog, []byte(manifest), 0600)
			sign(core, "dev.dropabit.agentdock.next.core")
			sign(launcher, "dev.dropabit.agentdock.next.mcp-launch")
			identifier := "dev.dropabit.agentdock.next.mcp-server.fixture"
			if scenario == "backend-wrong-identifier" {
				identifier = "com.uvwt.agentdock.mcp-server.fixture"
			}
			if scenario == "backend-other-server-identifier" {
				identifier = "dev.dropabit.agentdock.next.mcp-server.other"
			}
			if scenario == "backend-library" || scenario == "backend-x86" {
				changed := append([]byte{}, data...)
				if scenario == "backend-library" {
					changed[12] = byte(macho.TypeDylib)
				} else {
					changed[4] = 7
				}
				write(backend, changed, 0700)
			}
			sign(backend, identifier)
			sign(bundle, "dev.dropabit.agentdock.next")
			id := "fixture"
			checkCtx := ctx
			switch scenario {
			case "unknown-id":
				id = "unregistered"
			case "path-as-id":
				id = "/bin/echo"
			case "missing-catalog":
				must(os.Remove(catalog))
			case "catalog-symlink":
				must(os.Rename(catalog, filepath.Join(dir, "catalog")))
				must(os.Symlink(filepath.Join(dir, "catalog"), catalog))
			case "catalog-hardlink":
				must(os.Link(catalog, filepath.Join(dir, "alias")))
			case "catalog-writable":
				must(os.Chmod(catalog, 0660))
			case "backend-symlink":
				must(os.Remove(backend))
				must(os.Symlink(launcher, backend))
			case "backend-hardlink":
				must(os.Link(backend, filepath.Join(dir, "alias")))
			case "backend-writable":
				must(os.Chmod(backend, 0770))
			case "backend-nonexecutable":
				must(os.Chmod(backend, 0600))
			case "backend-script":
				write(backend, []byte("#!/bin/sh\nexit 0\n"), 0700)
			case "backend-ancestor-symlink":
				native := filepath.Dir(backend)
				must(os.Rename(native, filepath.Join(dir, "native")))
				must(os.Symlink(filepath.Join(dir, "native"), native))
			case "catalog-tampered":
				write(catalog, []byte(`{"schema_version":1,"servers":[{"id":"fixture","executable":"Helpers/native-mcp/fixture","args":["changed"]}]}`), 0600)
			case "backend-tampered":
				changed, err := os.ReadFile(backend)
				must(err)
				file, err := macho.Open(backend)
				must(err)
				section := file.Section("__text")
				if section == nil {
					t.Fatal("fixture code section missing")
				}
				changed[section.Offset] ^= 1
				must(file.Close())
				write(backend, changed, 0700)
			case "marker-missing":
				must(os.Remove(marker))
			case "marker-nonshared":
				write(marker, []byte(`{"schema_version":1,"product_name":"AgentDock Next","desktop_variant":"next","ui":"native"}`), 0600)
			case "canceled":
				var stop context.CancelFunc
				checkCtx, stop = context.WithCancel(ctx)
				stop()
			}
			config, err := resolveNextNativeMCPServerForExecutable(checkCtx, core, id)
			if scenario != "valid" {
				if err != errNextNativeMCPUnavailable || !reflect.DeepEqual(config, nextCodexMCPConfig{}) {
					t.Fatalf("unsafe backend selected or raw error leaked: %v", err)
				}
				return
			}
			if err != nil || config.ServerID != id || config.BackendExecutable != backend || config.WrapperExecutable != launcher || !reflect.DeepEqual(config.BackendArgs, []string{"--stdio", "literal $HOME; echo x"}) || !validNextCodexMCPConfig(config) {
				t.Fatalf("signed selection rejected or immutable config incorrect: %v", err)
			}
			originalDigest := config.ConfigDigest
			config.BackendArgs[0] = "caller-mutated"
			again, err := resolveNextNativeMCPServerForExecutable(ctx, core, id)
			if err != nil || again.BackendArgs[0] != "--stdio" || again.ConfigDigest != originalDigest || validNextCodexMCPConfig(config) {
				t.Fatal("caller mutation escaped immutable selection/digest")
			}
		})
	}
}
