//go:build !darwin

package process

import (
	"context"
	"testing"
)

func TestGroupBirthUnavailableOffDarwin(t *testing.T) {
	if _, err := ObserveGroupBirths(context.Background(), 1); err == nil {
		t.Fatal("unsupported group enumeration succeeded")
	}
}
