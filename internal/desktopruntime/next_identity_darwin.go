//go:build darwin

package desktopruntime

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Explicit Connection authority is stricter than generic bundled startup.
func explicitNextDarwinRuntime(root string) (unixRuntimeManifest, error) {
	executable, _ := os.Executable()
	if canonical, err := filepath.EvalSymlinks(executable); err == nil {
		executable = canonical
	}
	return explicitNextDarwinRuntimeForExecutable(root, executable)
}

func explicitNextDarwinRuntimeForExecutable(root, executable string) (unixRuntimeManifest, error) {
	fail := func() (unixRuntimeManifest, error) { return unixRuntimeManifest{}, ErrNextIdentityUnavailable }
	if os.Getenv("AGENTDOCK_DESKTOP_VARIANT") != "next" {
		return fail()
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil || canonical != filepath.Clean(root) {
		return fail()
	}
	if darwinExecutableFromAppBundle(executable) {
		manifest, _, err := loadUnixRuntimeForExecutable(root, executable)
		if err != nil || validateNextDarwinHelpers(manifest) != nil {
			return fail()
		}
		return manifest, nil
	}
	path := filepath.Join(root, "desktop-runtime.json")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return fail()
	}
	data, err := os.ReadFile(path)
	var manifest unixRuntimeManifest
	if err != nil || json.Unmarshal(data, &manifest) != nil ||
		manifest.SchemaVersion != 1 || manifest.ServiceManager != "smappservice" ||
		manifest.ServiceName != "dev.dropabit.agentdock.next.core" || manifest.TunnelServiceName != "dev.dropabit.agentdock.next.tunnel" ||
		manifest.EnvironmentFile != filepath.Join(root, "agentdock.env") || manifest.TunnelEnvironment != filepath.Join(root, "cloudflared.env") {
		return fail()
	}
	if validateNextDarwinHelpers(manifest) != nil {
		return fail()
	}
	authority, _, err := loadUnixRuntime(root)
	if err != nil || authority.AgentDockBinary != manifest.AgentDockBinary || authority.CloudflaredBinary != manifest.CloudflaredBinary {
		return fail()
	}
	return manifest, nil
}

func validateNextDarwinHelpers(manifest unixRuntimeManifest) error {
	core, tunnel := manifest.AgentDockBinary, manifest.CloudflaredBinary
	if !filepath.IsAbs(core) || filepath.Base(core) != "agentdock" || filepath.Base(tunnel) != "cloudflared" || filepath.Dir(core) != filepath.Dir(tunnel) ||
		filepath.Base(filepath.Dir(core)) != "Helpers" || filepath.Base(filepath.Dir(filepath.Dir(core))) != "Contents" {
		return ErrNextIdentityUnavailable
	}
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(core)))
	if filepath.Base(bundle) != "AgentDock Next.app" {
		return ErrNextIdentityUnavailable
	}
	for _, path := range []string{core, tunnel} {
		canonical, err := filepath.EvalSymlinks(path)
		info, statErr := os.Lstat(path)
		if err != nil || canonical != path || statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			return ErrNextIdentityUnavailable
		}
	}
	// Explicit dev manifests may select only helpers carrying Next identifiers.
	// Inspect signatures without executing a helper (especially with a Named token).
	for path, identifier := range map[string]string{core: "dev.dropabit.agentdock.next.core", tunnel: "dev.dropabit.agentdock.next.cloudflared"} {
		if err := exec.Command("/usr/bin/codesign", "--verify", "--strict", path).Run(); err != nil {
			return ErrNextIdentityUnavailable
		}
		output, err := exec.Command("/usr/bin/codesign", "-d", "--verbose=4", path).CombinedOutput()
		if err != nil || !strings.Contains("\n"+string(output), "\nIdentifier="+identifier+"\n") {
			return ErrNextIdentityUnavailable
		}
	}
	// plutil parses both binary and XML property lists; no helper is executed.
	output, err := exec.Command("/usr/bin/plutil", "-extract", "CFBundleIdentifier", "raw", "-o", "-", filepath.Join(bundle, "Contents", "Info.plist")).Output()
	if err != nil || strings.TrimSpace(string(output)) != "dev.dropabit.agentdock.next" {
		return ErrNextIdentityUnavailable
	}
	output, err = exec.Command("/usr/bin/plutil", "-extract", "AgentDockVariant", "raw", "-o", "-", filepath.Join(bundle, "Contents", "Info.plist")).Output()
	if err != nil || strings.TrimSpace(string(output)) != "next" {
		return ErrNextIdentityUnavailable
	}
	return nil
}
