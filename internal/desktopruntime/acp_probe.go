package desktopruntime

import (
	"bytes"
	"context"
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
	if !trustedACPVersionProbe(runtimeRoot, preset, command, args) {
		return probe, nil
	}

	versionCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	versionArgs := append(append([]string(nil), args...), "--version")
	cmd := exec.CommandContext(versionCtx, command, versionArgs...)
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

func trustedACPVersionProbe(runtimeRoot, preset, command string, args []string) bool {
	base := strings.ToLower(filepath.Base(command))
	switch preset {
	case "codex":
		if base == "codex-acp" || base == "codex-acp.exe" || base == "codex-acp.com" {
			return true
		}
		if (base == "node" || base == "node.exe" || base == "node.com") && len(args) > 0 {
			entry := strings.ToLower(filepath.ToSlash(args[0]))
			return strings.Contains(entry, "/@agentclientprotocol/codex-acp/") || strings.Contains(entry, "/codex-acp/")
		}
		return false
	case "legacy":
		return strings.Contains(base, "claude-agent-acp") || base == "grok" || base == "grok.exe" || base == "grok.com"
	case "antigravity":
		if !NextManagedRoot(runtimeRoot) || (base != "antigravity-acp" && base != "antigravity-acp.exe" && base != "antigravity-acp.com") {
			return false
		}
		resolved := command
		if canonical, err := filepath.EvalSymlinks(command); err == nil {
			resolved = canonical
		}
		resolved = filepath.Clean(resolved)
		home, _ := os.UserHomeDir()
		for _, root := range []string{
			filepath.Join(home, ".agentdock-next", "bin"),
			filepath.Join(runtimeRoot, "bin"),
		} {
			root = filepath.Clean(root)
			relative, err := filepath.Rel(root, resolved)
			if err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
				return true
			}
			if err == nil && relative == "." {
				return true
			}
		}
		return false
	default:
		return false
	}
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
