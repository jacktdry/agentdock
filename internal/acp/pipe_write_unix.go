//go:build !windows

package acp

import (
	"context"
	"io"
)

// The bool reports whether Write was attempted: a cancelled waiter must not
// invalidate a healthy stream. Once attempted, errors require connection close.
func writeACPStream(ctx context.Context, writer io.WriteCloser, data []byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	interrupted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = writer.Close()
		close(interrupted)
	})
	written, err := writer.Write(data)
	if !stop() {
		<-interrupted
	}
	if ctx.Err() != nil {
		return true, ctx.Err()
	}
	if err == nil && written != len(data) {
		err = io.ErrShortWrite
	}
	return true, err
}
