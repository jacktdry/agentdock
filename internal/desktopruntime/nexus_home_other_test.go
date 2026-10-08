//go:build !darwin

package desktopruntime

import (
	"context"
	"errors"
	"testing"
)

func TestNexusHomeUnavailableOnUnverifiedPlatforms(t *testing.T) {
	if home, err := ResolveNextAgentDockHome(context.Background(), "fixture"); home != "" || !errors.Is(err, ErrNextIdentityUnavailable) {
		t.Fatal(home, err)
	}
	if data, err := ReadNextNexusIdentity(context.Background(), "fixture"); data != nil || !errors.Is(err, ErrNextIdentityUnavailable) {
		t.Fatal(data, err)
	}
}
