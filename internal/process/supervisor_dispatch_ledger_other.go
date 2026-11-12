//go:build !darwin

package process

import "errors"

var ErrSupervisorDispatchLedgerUnavailable = errors.New("supervisor dispatch ledger unavailable")

type SupervisorDispatchLedger struct{}
type SupervisorDispatchTicket struct{}
type SupervisorDispatchSnapshot struct {
	State        string
	Session      string
	Sequence     uint64
	Acknowledged bool
}

func OpenSupervisorDispatchLedger(string) (*SupervisorDispatchLedger, error) {
	return nil, ErrSupervisorDispatchLedgerUnavailable
}
func (*SupervisorDispatchLedger) Close() error { return ErrSupervisorDispatchLedgerUnavailable }
func (*SupervisorDispatchLedger) BeginVerified(SupervisorToolCallFrame, BirthIdentity, BirthIdentity) (*SupervisorDispatchTicket, error) {
	return nil, ErrSupervisorDispatchLedgerUnavailable
}
func (*SupervisorDispatchTicket) Finish(string, []byte) error {
	return ErrSupervisorDispatchLedgerUnavailable
}
func (*SupervisorDispatchTicket) Close() error { return ErrSupervisorDispatchLedgerUnavailable }
func InspectSupervisorDispatchReceipt(string, SupervisorToolCallFrame) (SupervisorDispatchSnapshot, error) {
	return SupervisorDispatchSnapshot{}, ErrSupervisorDispatchLedgerUnavailable
}
