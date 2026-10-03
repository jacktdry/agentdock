package desktopapi

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRuntimeMutationLockSerializesDomains(t *testing.T) {
	root := t.TempDir()
	release, err := acquireRuntimeMutation(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	second, err := acquireRuntimeMutation(ctx, root)
	if second != nil {
		second()
	}
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second lock error = %v, want deadline exceeded", err)
	}

	release()
	release = func() {}
	next, err := acquireRuntimeMutation(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	next()
}
