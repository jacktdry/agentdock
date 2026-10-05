//go:build windows

package desktopruntime

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/uvwt/agentdock/internal/updateengine"
	"golang.org/x/sys/windows"
)

var portTCPTable = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")

func windowsPortSelection(selection NextPortRuntime) bool {
	manifest, err := Load(filepath.Join(selection.Root, "runtime.json"))
	if err != nil || !samePath(manifest.InstallRoot, selection.Root) {
		return false
	}
	// Do not use ActiveCoreBinary: its legacy fallback must never prove Next
	// ownership when an active generation is missing or unreadable.
	store, err := updateengine.NewStore(selection.Root)
	if err != nil {
		return false
	}
	active, err := store.ReadActive()
	if err != nil {
		return false
	}
	layout, err := updateengine.NewWindowsLayout(selection.Root)
	if err != nil {
		return false
	}
	selected := layout.GenerationCore(active.ActiveVersion)
	info, err := os.Stat(selected)
	return err == nil && info.Mode().IsRegular() && samePath(selected, selection.Binary)
}

func platformPortProcess(pid int) (string, string, error) {
	if pid <= 0 {
		return "", "", ErrPortPreflight
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", "", err
	}
	defer windows.CloseHandle(process)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil {
		return "", "", err
	}
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(process, &created, &exited, &kernel, &user); err != nil {
		return "", "", err
	}
	return windows.UTF16ToString(buffer[:size]), fmt.Sprintf("%d", uint64(created.HighDateTime)<<32|uint64(created.LowDateTime)), nil
}

func windowsPortPIDs(port int) ([]int, error) {
	seen := map[int]bool{}
	var pids []int
	for _, family := range []uint32{windows.AF_INET, windows.AF_INET6} {
		// TCP_TABLE_OWNER_PID_LISTENER = 3. Query both families and conservatively
		// include every address on this port, without reading command lines.
		var size uint32
		code, _, _ := portTCPTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, uintptr(family), 3, 0)
		if code != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) || size < 4 || size > 16<<20 {
			return nil, ErrPortPreflight
		}
		buffer := make([]byte, size)
		code, _, _ = portTCPTable.Call(uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)), 0, uintptr(family), 3, 0)
		if code != 0 || int(size) > len(buffer) || size < 4 {
			return nil, ErrPortPreflight
		}
		stride, portOffset, pidOffset := 24, 8, 20
		if family == windows.AF_INET6 {
			stride, portOffset, pidOffset = 56, 20, 52
		}
		count := int(binary.LittleEndian.Uint32(buffer[:4]))
		if count > (int(size)-4)/stride {
			return nil, ErrPortPreflight
		}
		for i := 0; i < count; i++ {
			row := buffer[4+i*stride : 4+(i+1)*stride]
			if int(binary.BigEndian.Uint16(row[portOffset:portOffset+2])) != port {
				continue
			}
			pid := int(binary.LittleEndian.Uint32(row[pidOffset : pidOffset+4]))
			if pid <= 0 {
				return nil, ErrPortPreflight
			}
			if !seen[pid] {
				pids = append(pids, pid)
				seen[pid] = true
			}
		}
	}
	return pids, nil
}

func platformPortOwner(ctx context.Context, request PortObservationRequest) (PortState, string) {
	if ctx.Err() != nil {
		return PortUnknown, "observation_cancelled"
	}
	if !windowsPortSelection(request.Runtime) {
		return PortUnknown, "next_identity_unavailable"
	}
	pids, err := windowsPortPIDs(request.CandidatePort)
	if err != nil || len(pids) != 1 {
		return PortUnknown, "ownership_unavailable"
	}
	path, instance, err := platformPortProcess(pids[0])
	state, reason := classifyPortOwner(request, pids, path, instance, err)
	if state == PortOwnedByNext {
		again, readErr := windowsPortPIDs(request.CandidatePort)
		afterBinary, after, processErr := platformPortProcess(pids[0])
		if ctx.Err() != nil || readErr != nil || len(again) != 1 || again[0] != pids[0] || processErr != nil || after != instance || !portBinaryMatches(afterBinary, path) || !windowsPortSelection(request.Runtime) {
			return PortUnknown, "listener_changed"
		}
	}
	return state, reason
}
