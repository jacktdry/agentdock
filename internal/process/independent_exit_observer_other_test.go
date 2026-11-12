//go:build !darwin

package process

import (
	"context"
	"errors"
	"testing"
)

func TestIndependentObserverAlwaysRefusedOutsideDarwin(t *testing.T) {
	if err := WatchIndependentWorkerExit(context.Background(), IndependentExitRequest{}, func() error {
		t.Fatal("unsupported observer armed")
		return nil
	}); !errors.Is(err, ErrIndependentExitObserverUnavailable) {
		t.Fatalf("non-Darwin observer was not denied: %v", err)
	}
}
