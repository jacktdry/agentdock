//go:build !darwin

package process

import (
	"context"
	"testing"
)

func TestExitWitnessUnavailableOffDarwin(t *testing.T) {
	if _, err := WatchVerifiedExit(context.Background(), BirthIdentity{PID: 1, StartSec: 1, ProcessGroupID: 1}); err == nil {
		t.Fatal("unsupported platform acquired exit observation")
	}
}
