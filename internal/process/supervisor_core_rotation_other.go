//go:build !darwin

package process

import "context"

// Cross-platform API is fail-closed until native peer and process ownership
// witnesses exist for that OS.
type SupervisorCoreRotation struct{}

func NewSupervisorCoreRotation(context.Context, BirthIdentity, BirthIdentity, BirthIdentity, []byte) (*SupervisorCoreRotation, error) {
	return nil, ErrSupervisorHandoffUnavailable
}
func (*SupervisorCoreRotation) CurrentHandoff() (*ReadOnlySupervisorHandoff, error) {
	return nil, ErrSupervisorHandoffUnavailable
}
func (*SupervisorCoreRotation) Generation() uint64 { return 0 }
func (*SupervisorCoreRotation) WitnessCoreExit(context.Context) error {
	return ErrSupervisorHandoffUnavailable
}
func (*SupervisorCoreRotation) AdmitSuccessor(context.Context, BirthIdentity, []byte) (*ReadOnlySupervisorHandoff, error) {
	return nil, ErrSupervisorHandoffUnavailable
}
func (*SupervisorCoreRotation) MarkUncertain() {}
