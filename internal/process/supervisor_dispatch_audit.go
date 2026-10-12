package process

// SupervisorDispatchJournalAudit contains structural counts only. Settled
// includes Acknowledged; UnknownOutcome is a subset of Settled. AcceptedOnly
// and UnknownOutcome have uncertain outcomes. Even zero counts or an ACK
// NEVER authorize restart, recovery, replay, permission, or cleanup.
type SupervisorDispatchJournalAudit struct {
	Total          int
	AcceptedOnly   int
	Settled        int
	Acknowledged   int
	UnknownOutcome int
}
