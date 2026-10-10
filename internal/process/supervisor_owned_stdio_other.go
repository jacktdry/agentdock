//go:build !darwin

package process

import (
	"context"
	"io"
	"os/exec"
)

type SupervisorOwnedStdio struct{}

func StartSupervisorOwnedStdio(context.Context, *exec.Cmd) (*SupervisorOwnedStdio, error) {
	return nil, ErrSupervisorHandoffUnavailable
}
func (*SupervisorOwnedStdio) Birth() BirthIdentity       { return BirthIdentity{} }
func (*SupervisorOwnedStdio) Stdin() io.WriteCloser      { return nil }
func (*SupervisorOwnedStdio) Stdout() io.ReadCloser      { return nil }
func (*SupervisorOwnedStdio) Wait(context.Context) error { return ErrSupervisorHandoffUnavailable }
