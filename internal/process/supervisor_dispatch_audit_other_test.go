//go:build !darwin

package process

import "testing"

func TestSupervisorDispatchAuditUnsupported(t *testing.T) {
	got, err := AuditSupervisorDispatchJournal("/unused")
	if got != (SupervisorDispatchJournalAudit{}) || err != ErrSupervisorDispatchLedgerUnavailable {
		t.Fatal(got, err)
	}
}
