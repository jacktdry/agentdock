//go:build darwin

package process

import (
	"context"
	"os"
	"syscall"
	"testing"
)

// Observe our own existing process group without creating or signalling any
// subprocess. An absent group result is a test failure, not proof of no Chrome.
func TestGroupBirthKernelReadOnlySelf(t *testing.T) {
	pgid, err := syscall.Getpgid(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	group, err := ObserveGroupBirths(context.Background(), pgid)
	if err != nil {
		t.Fatal(err)
	}
	self, err := ObserveBirthIdentity(context.Background(), os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range group.Members {
		if m.PID == self.PID {
			found = true
			if m != self {
				t.Fatalf("same self PID had different birth identity")
			}
		}
	}
	if !found {
		t.Fatal("kernel group enumeration did not include current test process")
	}
}
