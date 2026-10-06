package desktopruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ACPAdapterProbe struct {
	Availability     string
	Command          string
	Args             []string
	InstalledVersion string
	VersionState     string
	BlockedReason    string
}

func ProbeACPProfile(ctx context.Context, runtimeRoot, preset string, profile ACPProfileSettings) (ACPAdapterProbe, error) {
	if ctx == nil {
		return ACPAdapterProbe{}, errors.New("context is required")
	}
	command, args, ok := platformResolveACPProbe(runtimeRoot, preset, profile)
	if !ok {
		return ACPAdapterProbe{
			Availability:  "unavailable",
			VersionState:  "unavailable",
			BlockedReason: "adapter_not_found",
		}, nil
	}

	probe := ACPAdapterProbe{
		Availability:  "available",
		Command:       command,
		Args:          append([]string(nil), args...),
		VersionState:  "unsupported",
		BlockedReason: "update_not_available",
	}

	switch preset {
	case "codex":
		if version, ok := codexACPInstalledVersion(command, args); ok {
			probe.InstalledVersion = version
			probe.VersionState = "not_checked"
		} else {
			probe.VersionState = "unavailable"
			probe.BlockedReason = "version_unavailable"
		}
		return probe, nil
	case "antigravity":
		// Antigravity is the only adapter whose version probe executes a binary.
		// It must be the exact Next-owned target, and configured runtime arguments
		// are never forwarded to the read-only --version probe.
		if !trustedAntigravityTarget(runtimeRoot, command) {
			return probe, nil
		}
	default:
		return probe, nil
	}

	versionCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(versionCtx, command, "--version")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		if versionCtx.Err() != nil {
			return ACPAdapterProbe{}, versionCtx.Err()
		}
		probe.VersionState = "unavailable"
		probe.BlockedReason = "version_unavailable"
		return probe, nil
	}
	probe.InstalledVersion = normalizeACPVersion(output.String())
	if probe.InstalledVersion == "" {
		probe.VersionState = "unavailable"
		probe.BlockedReason = "version_unavailable"
		return probe, nil
	}
	probe.VersionState = "not_checked"
	return probe, nil
}

func codexACPInstalledVersion(command string, args []string) (string, bool) {
	entry := strings.TrimSpace(command)
	base := strings.ToLower(filepath.Base(entry))
	if base == "node" || base == "node.exe" || base == "node.com" {
		if len(args) == 0 {
			return "", false
		}
		entry = strings.TrimSpace(args[0])
	}
	if entry == "" || !filepath.IsAbs(entry) {
		return "", false
	}
	if canonical, err := filepath.EvalSymlinks(entry); err == nil {
		entry = canonical
	}
	entry = filepath.Clean(entry)
	info, err := os.Stat(entry)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}

	dir := filepath.Dir(entry)
	for depth := 0; depth < 8; depth++ {
		packagePath := filepath.Join(dir, "package.json")
		if data, err := os.ReadFile(packagePath); err == nil && len(data) <= 1<<20 {
			var pkg struct {
				Name    string          `json:"name"`
				Version string          `json:"version"`
				Bin     json.RawMessage `json:"bin"`
			}
			if json.Unmarshal(data, &pkg) == nil && pkg.Name == "@agentclientprotocol/codex-acp" && strings.TrimSpace(pkg.Version) != "" {
				if codexPackageBinMatches(dir, pkg.Bin, entry) {
					return strings.TrimSpace(pkg.Version), true
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

func codexPackageBinMatches(packageRoot string, raw json.RawMessage, entry string) bool {
	var binPath string
	if len(raw) == 0 {
		return false
	}
	if raw[0] == '"' {
		if json.Unmarshal(raw, &binPath) != nil {
			return false
		}
	} else {
		var bins map[string]string
		if json.Unmarshal(raw, &bins) != nil {
			return false
		}
		binPath = bins["codex-acp"]
	}
	if strings.TrimSpace(binPath) == "" {
		return false
	}
	candidate := filepath.Clean(filepath.Join(packageRoot, filepath.FromSlash(binPath)))
	if canonical, err := filepath.EvalSymlinks(candidate); err == nil {
		candidate = canonical
	}
	return filepath.Clean(candidate) == filepath.Clean(entry)
}

func normalizeACPVersion(raw string) string {
	line := strings.TrimSpace(raw)
	if index := strings.IndexByte(line, '\n'); index >= 0 {
		line = strings.TrimSpace(line[:index])
	}
	if line == "" {
		return ""
	}
	fields := strings.Fields(line)
	for i := len(fields) - 1; i >= 0; i-- {
		value := strings.TrimSpace(strings.TrimPrefix(fields[i], "v"))
		if value != "" && strings.ContainsAny(value, "0123456789") {
			return value
		}
	}
	return ""
}
