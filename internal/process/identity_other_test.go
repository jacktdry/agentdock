//go:build !darwin

package process

import (
	"context"
	"testing"
)

func TestBirthIdentityUnavailableOffDarwin(t *testing.T) {
	if _, err := ObserveBirthIdentity(context.Background(), 1); err == nil {
		t.Fatal("unsupported platform granted identity authority")
	}
}
