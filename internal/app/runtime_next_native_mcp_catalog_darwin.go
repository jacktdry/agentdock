//go:build darwin

package app

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Core-local, read-only selection. The ONLY caller input is a registered ID;
// paths, argv and digest come from our own sealed NEXT Shared bundle. This is
// not launch authority: a future operator/M8 start owner must revalidate it.
// Deliberately not wired to B6 prepare/attach or any production start route.
func resolveNextNativeMCPServer(id string) (nextCodexMCPConfig, error) {
	executable, err := os.Executable()
	if err != nil {
		return nextCodexMCPConfig{}, errNextNativeMCPUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return resolveNextNativeMCPServerForExecutable(ctx, executable, id)
}

// The executable seam is for signed temporary bundle tests only.
func resolveNextNativeMCPServerForExecutable(ctx context.Context, executable, id string) (nextCodexMCPConfig, error) {
	fail := func() (nextCodexMCPConfig, error) { return nextCodexMCPConfig{}, errNextNativeMCPUnavailable }
	if ctx == nil || ctx.Err() != nil || !nextNativeMCPServerID.MatchString(id) || !filepath.IsAbs(executable) || filepath.Clean(executable) != executable {
		return fail()
	}
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(executable)))
	if filepath.Base(bundle) != "AgentDock Next.app" || executable != filepath.Join(bundle, "Contents", "Helpers", "agentdock") {
		return fail()
	}
	before := make(map[string]os.FileInfo)
	track := func(target string) bool {
		for path := target; ; path = filepath.Dir(path) {
			if _, seen := before[path]; !seen {
				info, err := os.Lstat(path)
				if err != nil || !nextCodexLauncherPrivateEntry(path, bundle, info) {
					return false
				}
				before[path] = info
			}
			if path == string(filepath.Separator) {
				return true
			}
		}
	}
	read := func(path string) ([]byte, error) {
		if !track(path) || !before[path].Mode().IsRegular() || before[path].Size() > nextNativeMCPCatalogLimit {
			return nil, errNextNativeMCPUnavailable
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, errNextNativeMCPUnavailable
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !os.SameFile(before[path], info) {
			return nil, errNextNativeMCPUnavailable
		}
		return io.ReadAll(io.LimitReader(file, nextNativeMCPCatalogLimit+1))
	}
	marker, err := read(filepath.Join(bundle, "Contents", "Resources", "desktop-product.json"))
	if err != nil {
		return fail()
	}
	value, err := nextNativeMCPStrictJSON(marker)
	product, ok := value.(map[string]any)
	if err != nil || !ok || len(product) != 4 || product["schema_version"] != json.Number("1") || product["product_name"] != "AgentDock Next" || product["desktop_variant"] != "next" || product["ui"] != "shared-wails" {
		return fail()
	}
	raw, err := read(filepath.Join(bundle, "Contents", "Resources", "native-mcp-servers.json"))
	if err != nil {
		return fail()
	}
	servers, err := parseNextNativeMCPCatalog(raw)
	if err != nil {
		return fail()
	}
	var selected *nextNativeMCPServer
	for i := range servers {
		if servers[i].id == id {
			selected = &servers[i]
		}
	}
	if selected == nil {
		return fail()
	}
	backend := filepath.Join(bundle, "Contents", filepath.FromSlash(selected.executable))
	if !track(backend) || !before[backend].Mode().IsRegular() || before[backend].Mode().Perm()&0111 == 0 || !nextCodexLauncherARM64(backend) {
		return fail()
	}
	identifier := "dev.dropabit.agentdock.next.mcp-server." + id
	if _, err := nextCodexLauncherTool(ctx, "/usr/bin/codesign", "--verify", "--strict", "-R", `=identifier "`+identifier+`"`, backend); err != nil {
		return fail()
	}
	// Verify our exact Core/launcher identities and recursive bundle seal AFTER
	// reading config/backend. No caller-supplied paths or verifier overrides.
	launcher, err := selectNextCodexSignedLauncherForExecutable(ctx, executable)
	if err != nil {
		return fail()
	}
	for path, prior := range before {
		current, err := os.Lstat(path)
		if err != nil || !nextCodexLauncherPrivateEntry(path, bundle, current) || !os.SameFile(prior, current) || prior.Mode() != current.Mode() || (prior.Mode().IsRegular() && (prior.Size() != current.Size() || !prior.ModTime().Equal(current.ModTime()))) {
			return fail()
		}
	}
	if ctx.Err() != nil {
		return fail()
	}
	c := nextCodexMCPConfig{ServerID: id, WrapperExecutable: launcher.path, BackendExecutable: backend, BackendArgs: append([]string{}, selected.args...)}
	c.ConfigDigest = nextCodexMCPConfigDigest(c.ServerID, c.WrapperExecutable, c.BackendExecutable, c.BackendArgs)
	return c, nil
}
