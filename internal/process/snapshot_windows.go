//go:build windows

package process

import (
	"context"
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Toolhelp enumerates processes without opening process or Job Object handles.
// RSS is explicitly unavailable. Presence is observation-time liveness only.
func observeProcesses(ctx context.Context) ([]observedProcess, error) {
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(h)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	err = windows.Process32First(h, &entry)
	var rows []observedProcess
	for err == nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rows = append(rows, observedProcess{pid: int(entry.ProcessID), parent: int(entry.ParentProcessID)})
		entry.Size = uint32(unsafe.Sizeof(entry))
		err = windows.Process32Next(h, &entry)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, err
	}
	return rows, nil
}
