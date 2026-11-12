//go:build darwin

package desktopcontrol

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// CallVerifiedPID performs the same bounded local control request as Call, but
// additionally proves that the connected Unix socket peer is the expected Core
// PID and current user before trusting the response.
func CallVerifiedPID(ctx context.Context, runtimeRoot string, expectedPID int, method string, params, result any) error {
	if runtimeRoot == "" || expectedPID <= 0 {
		return errors.New("verified desktop control target is invalid")
	}
	if method == "" {
		return errors.New("desktop control method is required")
	}
	request := Request{ID: fmt.Sprintf("%d", time.Now().UnixNano()), Method: method}
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("encode desktop control params: %w", err)
		}
		request.Params = data
	}
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode desktop control request: %w", err)
	}
	responseData, err := callPlatformVerifiedPID(ctx, runtimeRoot, expectedPID, data)
	if err != nil {
		return err
	}
	var reply response
	if err := json.Unmarshal(responseData, &reply); err != nil {
		return fmt.Errorf("decode desktop control response: %w", err)
	}
	if reply.ID != request.ID {
		return errors.New("desktop control response id mismatch")
	}
	if reply.Error != nil {
		return errors.New(reply.Error.Message)
	}
	if result == nil || len(reply.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(reply.Result, result); err != nil {
		return fmt.Errorf("decode desktop control result: %w", err)
	}
	return nil
}

func callPlatformVerifiedPID(ctx context.Context, runtimeRoot string, expectedPID int, data []byte) ([]byte, error) {
	dialer := net.Dialer{Timeout: 2 * time.Second}
	connection, err := dialer.DialContext(ctx, "unix", endpointPath(runtimeRoot))
	if err != nil {
		return nil, fmt.Errorf("connect verified desktop control socket: %w", err)
	}
	defer connection.Close()
	unixConnection, ok := connection.(*net.UnixConn)
	if !ok {
		return nil, errors.New("verified desktop control transport is not Unix")
	}
	raw, err := unixConnection.SyscallConn()
	if err != nil {
		return nil, errors.New("inspect verified desktop control peer")
	}
	var peerPID int
	var peerUID uint32
	var peerErr error
	if err := raw.Control(func(fd uintptr) {
		peerPID, peerErr = unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
		if peerErr != nil {
			return
		}
		cred, credErr := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if credErr != nil {
			peerErr = credErr
			return
		}
		peerUID = cred.Uid
	}); err != nil {
		return nil, errors.New("inspect verified desktop control peer")
	}
	if peerErr != nil || peerPID != expectedPID || peerUID != uint32(os.Getuid()) {
		return nil, errors.New("verified desktop control peer mismatch")
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(30 * time.Second)
	}
	_ = connection.SetDeadline(deadline)
	if _, err := connection.Write(data); err != nil {
		return nil, fmt.Errorf("write verified desktop control request: %w", err)
	}
	_ = unixConnection.CloseWrite()
	responseData, err := io.ReadAll(io.LimitReader(bufio.NewReader(connection), maxMessageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read verified desktop control response: %w", err)
	}
	if len(responseData) > maxMessageBytes {
		return nil, errors.New("desktop control response is too large")
	}
	return responseData, nil
}
