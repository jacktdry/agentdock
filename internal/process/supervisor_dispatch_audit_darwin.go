//go:build darwin

package process

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// AuditSupervisorDispatchJournal is a bounded, read-only structural inventory.
// Without the original signed frames it cannot authenticate HMACs or request
// digests. Counts are observational only, never restart or replay authority.
func AuditSupervisorDispatchJournal(stateDir string) (SupervisorDispatchJournalAudit, error) {
	var zero, audit SupervisorDispatchJournalAudit
	stateDir = filepath.Clean(stateDir)
	ledger, err := OpenSupervisorDispatchLedger(stateDir)
	if err != nil {
		return zero, ErrSupervisorDispatchLedgerUnavailable
	}
	defer ledger.Close()
	var dirStat unix.Stat_t
	if unix.Fstat(ledger.dir, &dirStat) != nil || !auditDirectoryPinned(ledger.dir, stateDir, dirStat) {
		return zero, ErrSupervisorDispatchLedgerUnavailable
	}
	entries, err := auditSupervisorEntries(ledger.dir)
	if err != nil {
		return zero, ErrSupervisorDispatchLedgerUnavailable
	}
	for name, before := range entries {
		receipt, err := auditSupervisorReceipt(ledger.dir, name, before)
		if err != nil {
			return zero, ErrSupervisorDispatchLedgerUnavailable
		}
		audit.Total++
		if receipt.State == "accepted" {
			audit.AcceptedOnly++
		} else {
			audit.Settled++
		}
		if receipt.Acknowledged {
			audit.Acknowledged++
		}
		if receipt.Outcome == "unknown" {
			audit.UnknownOutcome++
		}
	}
	// Re-inventory so changes to earlier receipts or directory membership
	// during the bounded read cannot produce partial/stale counts.
	after, err := auditSupervisorEntries(ledger.dir)
	if err != nil || len(after) != len(entries) || !auditDirectoryPinned(ledger.dir, stateDir, dirStat) {
		return zero, ErrSupervisorDispatchLedgerUnavailable
	}
	for name, before := range entries {
		current, exists := after[name]
		if !exists || !auditSameStat(before, current) {
			return zero, ErrSupervisorDispatchLedgerUnavailable
		}
	}
	return audit, nil
}

func auditSameStat(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode &&
		a.Uid == b.Uid && a.Gid == b.Gid && a.Nlink == b.Nlink &&
		a.Size == b.Size && a.Mtim == b.Mtim && a.Ctim == b.Ctim
}

func auditDirectoryPinned(fd int, path string, before unix.Stat_t) bool {
	var pinned, current unix.Stat_t
	return safeSupervisorDispatchDir(fd) && before.Mode&07777 == 0700 &&
		unix.Fstat(fd, &pinned) == nil && auditSameStat(before, pinned) &&
		unix.Fstatat(unix.AT_FDCWD, path, &current, unix.AT_SYMLINK_NOFOLLOW) == nil &&
		auditSameStat(pinned, current)
}

func auditSupervisorEntries(dir int) (map[string]unix.Stat_t, error) {
	fd, err := unix.Openat(dir, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrSupervisorDispatchLedgerUnavailable
	}
	file := os.NewFile(uintptr(fd), "private-journal")
	defer file.Close()
	entries, err := file.ReadDir(supervisorReceiptLimit + 1)
	if (err != nil && !errors.Is(err, io.EOF)) || len(entries) > supervisorReceiptLimit {
		return nil, ErrSupervisorDispatchLedgerUnavailable
	}
	stats := make(map[string]unix.Stat_t, len(entries))
	for _, entry := range entries {
		var st unix.Stat_t
		if !validDispatchName(entry.Name()) ||
			unix.Fstatat(dir, entry.Name(), &st, unix.AT_SYMLINK_NOFOLLOW) != nil || !auditSafeReceipt(st) {
			return nil, ErrSupervisorDispatchLedgerUnavailable
		}
		if _, duplicate := stats[entry.Name()]; duplicate {
			return nil, ErrSupervisorDispatchLedgerUnavailable
		}
		stats[entry.Name()] = st
	}
	return stats, nil
}

func auditSafeReceipt(st unix.Stat_t) bool {
	return st.Mode&unix.S_IFMT == unix.S_IFREG && st.Mode&07777 == 0600 &&
		st.Uid == uint32(os.Geteuid()) && st.Nlink == 1 && st.Size > 0 && st.Size <= supervisorReceiptMaxBytes
}

func auditSupervisorReceipt(dir int, name string, before unix.Stat_t) (SupervisorDispatchSnapshot, error) {
	var zero SupervisorDispatchSnapshot
	// NONBLOCK prevents a swapped-in FIFO from blocking before fstat rejects it.
	fd, err := unix.Openat(dir, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return zero, ErrSupervisorDispatchLedgerUnavailable
	}
	file := os.NewFile(uintptr(fd), "private-receipt")
	defer file.Close()
	pinned := func() bool {
		var st, byName unix.Stat_t
		return unix.Fstat(fd, &st) == nil && auditSafeReceipt(st) && auditSameStat(before, st) &&
			unix.Fstatat(dir, name, &byName, unix.AT_SYMLINK_NOFOLLOW) == nil && auditSameStat(st, byName)
	}
	if !pinned() {
		return zero, ErrSupervisorDispatchLedgerUnavailable
	}
	data := make([]byte, int(before.Size))
	if _, err := io.ReadFull(file, data); err != nil {
		return zero, ErrSupervisorDispatchLedgerUnavailable
	}
	var extra [1]byte
	if n, err := file.Read(extra[:]); n != 0 || err != io.EOF || !pinned() {
		return zero, ErrSupervisorDispatchLedgerUnavailable
	}
	return parseSupervisorAuditReceipt(name, data)
}

func parseSupervisorAuditReceipt(name string, data []byte) (SupervisorDispatchSnapshot, error) {
	var zero SupervisorDispatchSnapshot
	if len(data) == 0 || len(data) > supervisorReceiptMaxBytes || data[len(data)-1] != '\n' {
		return zero, ErrSupervisorDispatchLedgerUnavailable
	}
	lines := bytes.Split(data[:len(data)-1], []byte{'\n'})
	if len(lines) > 3 {
		return zero, ErrSupervisorDispatchLedgerUnavailable
	}
	var records [3]SupervisorDispatchSnapshot
	for i, line := range lines {
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&records[i]); err != nil {
			return zero, ErrSupervisorDispatchLedgerUnavailable
		}
		var extra any
		if dec.Decode(&extra) != io.EOF {
			return zero, ErrSupervisorDispatchLedgerUnavailable
		}
		// Durable receipts originate from json.Marshal(record) in writeSync.
		// Require that exact canonical shape to reject duplicate JSON keys,
		// alternate encodings, and ambiguous structural interpretations.
		canonical, err := json.Marshal(records[i])
		if err != nil || !bytes.Equal(canonical, line) {
			return zero, ErrSupervisorDispatchLedgerUnavailable
		}
	}
	first := records[0]
	expected, err := supervisorReceiptName(SupervisorToolCallFrame{Session: first.Session, Sequence: first.Sequence})
	if err != nil || expected != name || first.State != "accepted" ||
		!validLineageBirth(first.Core) || !validLineageBirth(first.Worker) ||
		!auditSHA256(first.RequestSHA256) || first.Outcome != "" || first.ReplySHA256 != "" || first.Acknowledged {
		return zero, ErrSupervisorDispatchLedgerUnavailable
	}
	if len(lines) >= 2 {
		second := records[1]
		if second.State != "settled" || (second.Outcome != "succeeded" && second.Outcome != "failed" && second.Outcome != "unknown") ||
			!auditSHA256(second.ReplySHA256) || second.Session != "" || second.Sequence != 0 ||
			second.Core != (BirthIdentity{}) || second.Worker != (BirthIdentity{}) || second.RequestSHA256 != "" || second.Acknowledged {
			return zero, ErrSupervisorDispatchLedgerUnavailable
		}
		first.State, first.Outcome, first.ReplySHA256 = second.State, second.Outcome, second.ReplySHA256
	}
	if len(lines) == 3 {
		ack := records[2]
		if ack.State != "acknowledged" || first.Outcome == "unknown" || ack.ReplySHA256 != first.ReplySHA256 ||
			ack.Session != "" || ack.Sequence != 0 || ack.Core != (BirthIdentity{}) || ack.Worker != (BirthIdentity{}) ||
			ack.RequestSHA256 != "" || ack.Outcome != "" || ack.Acknowledged {
			return zero, ErrSupervisorDispatchLedgerUnavailable
		}
		first.Acknowledged = true
	}
	return first, nil
}

func auditSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
