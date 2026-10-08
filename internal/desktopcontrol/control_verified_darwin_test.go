//go:build darwin

package desktopcontrol

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestCallVerifiedPIDAcceptsExpectedPeerAndRejectsMismatch(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "adcv-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, root, func(_ context.Context, request Request) (any, error) {
			return map[string]string{"method": request.Method}, nil
		})
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var result map[string]string
		err := CallVerifiedPID(context.Background(), root, os.Getpid(), "ping", nil, &result)
		if err == nil {
			if result["method"] != "ping" {
				t.Fatalf("result=%v", result)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("verified peer never became ready: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := CallVerifiedPID(context.Background(), root, os.Getpid()+1, "ping", nil, nil); err == nil {
		t.Fatal("mismatched peer PID was accepted")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not stop")
	}
}
