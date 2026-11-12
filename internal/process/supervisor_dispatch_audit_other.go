//go:build !darwin

package process

func AuditSupervisorDispatchJournal(string) (SupervisorDispatchJournalAudit, error) {
	return SupervisorDispatchJournalAudit{}, ErrSupervisorDispatchLedgerUnavailable
}
