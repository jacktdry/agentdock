//go:build darwin

package desktopruntime

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func darwinPortSelection(selection NextPortRuntime) bool {
	// Bundled selection derives authority from its own signed helpers, without
	// consulting an external manifest that could redirect those helpers.
	executable, _ := os.Executable()
	if canonical, err := filepath.EvalSymlinks(executable); err == nil {
		executable = canonical
	}
	if darwinExecutableFromAppBundle(executable) {
		manifest, err := explicitNextDarwinRuntimeForExecutable(selection.Root, executable)
		return err == nil && portBinaryMatches(manifest.AgentDockBinary, selection.Binary)
	}
	// Read only the explicitly selected manifest; never use loadUnixRuntime's
	// caller-executable or stable defaults and never query launchd.
	data, err := os.ReadFile(filepath.Join(selection.Root, "desktop-runtime.json"))
	if err != nil {
		return false
	}
	var manifest unixRuntimeManifest
	if json.Unmarshal(data, &manifest) != nil {
		return false
	}
	return manifest.SchemaVersion == 1 && manifest.ServiceName == "dev.dropabit.agentdock.next.core" &&
		manifest.ServiceManager == "smappservice" && portBinaryMatches(manifest.AgentDockBinary, selection.Binary)
}

func darwinPortPIDs(output string) ([]int, error) {
	seen := map[int]bool{}
	var pids []int
	for _, field := range strings.Fields(output) {
		pid, err := strconv.Atoi(field)
		if err != nil || pid <= 0 {
			return nil, ErrPortPreflight
		}
		if !seen[pid] {
			pids = append(pids, pid)
			seen[pid] = true
		}
	}
	if len(pids) == 0 {
		return nil, ErrPortPreflight
	}
	return pids, nil
}

func platformPortOwner(ctx context.Context, request PortObservationRequest) (PortState, string) {
	if !darwinPortSelection(request.Runtime) {
		return PortUnknown, "next_identity_unavailable"
	}
	read := func() ([]int, error) {
		output, err := exec.CommandContext(ctx, "/usr/sbin/lsof", "-nP", "-iTCP:"+strconv.Itoa(request.CandidatePort), "-sTCP:LISTEN", "-t").Output()
		if err != nil {
			return nil, err
		}
		return darwinPortPIDs(string(output))
	}
	pids, err := read()
	if err != nil || len(pids) != 1 {
		return PortUnknown, "ownership_unavailable"
	}
	binary, instance, err := platformPortProcess(pids[0])
	state, reason := classifyPortOwner(request, pids, binary, instance, err)
	if state == PortOwnedByNext {
		again, readErr := read()
		afterBinary, after, processErr := platformPortProcess(pids[0])
		if readErr != nil || len(again) != 1 || again[0] != pids[0] || processErr != nil || after != instance || !portBinaryMatches(afterBinary, binary) || !darwinPortSelection(request.Runtime) {
			return PortUnknown, "listener_changed"
		}
	}
	return state, reason
}
