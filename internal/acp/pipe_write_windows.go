package acp

import (
	"context"
	"io"
	"runtime"
	"time"

	"golang.org/x/sys/windows"
)

var cancelACPWriteIO = windows.NewLazySystemDLL("kernel32.dll").NewProc("CancelSynchronousIo")

// Windows anonymous pipes use synchronous I/O. Cancel the exact writing thread,
// then join it before returning so no write survives to corrupt the next frame.
func writeACPStream(ctx context.Context, writer io.WriteCloser, data []byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	type writeResult struct {
		started bool
		err     error
	}
	ready := make(chan windows.Handle, 1)
	done := make(chan writeResult, 1)
	released := make(chan struct{})
	defer close(released)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		// Do not let Go reuse this thread until cancellation has stopped.
		defer func() { <-released }()
		thread, err := windows.OpenThread(windows.THREAD_TERMINATE, false, windows.GetCurrentThreadId())
		ready <- thread
		if err != nil {
			done <- writeResult{err: err}
			return
		}
		if err := ctx.Err(); err != nil {
			done <- writeResult{err: err}
			return
		}
		written, err := writer.Write(data)
		if err == nil && written != len(data) {
			err = io.ErrShortWrite
		}
		done <- writeResult{started: true, err: err}
	}()
	thread := <-ready
	if thread != 0 {
		defer windows.CloseHandle(thread)
	}
	select {
	case result := <-done:
		return result.started, result.err
	case <-ctx.Done():
		// Cancellation may precede the syscall or fall between internal writes.
		// Retry until Write exits, while the thread still belongs to this call.
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			_, _, _ = cancelACPWriteIO.Call(uintptr(thread))
			select {
			case result := <-done:
				return result.started, ctx.Err()
			case <-ticker.C:
			}
		}
	}
}
