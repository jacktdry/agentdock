package browser

import "time"

const (
	browserLifecycleSweepInterval = 30 * time.Second
	cleanupRecoveryBaseDelay      = 30 * time.Second
	cleanupRecoveryMaxAttempts    = 3
)

func cleanupRecoveryDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return cleanupRecoveryBaseDelay
	}
	shift := attempt - 1
	if shift > 3 {
		shift = 3
	}
	return cleanupRecoveryBaseDelay * time.Duration(1<<shift)
}
