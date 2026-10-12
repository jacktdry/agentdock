//go:build darwin

package process

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func auditTestReceipt(t *testing.T, dir, outcome string, ack bool) (string, []byte) {
	t.Helper()
	ledger, err := OpenSupervisorDispatchLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	frame, core, worker := dispatchTestFrame(t)
	ticket, err := ledger.BeginVerified(frame, core, worker)
	if err != nil {
		t.Fatal(err)
	}
	defer ticket.Close()
	if outcome != "" {
		if err := ticket.Finish(outcome, []byte(`{"ok":true}`)); err != nil {
			t.Fatal(err)
		}
	}
	if ack {
		reply := map[string]any{"ok": true}
		if err := ledger.ackVerified(frame, signedReceiptACK(t, frame, core, worker, reply), core, worker); err != nil {
			t.Fatal(err)
		}
	}
	name, err := supervisorReceiptName(frame)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return name, raw
}

func requireAuditClosed(t *testing.T, dir string) {
	t.Helper()
	got, err := AuditSupervisorDispatchJournal(dir)
	if got != (SupervisorDispatchJournalAudit{}) || err != ErrSupervisorDispatchLedgerUnavailable {
		t.Fatalf("failure must return zero counts and only generic error: %+v %v", got, err)
	}
}

func TestSupervisorDispatchAuditCountsReadOnly(t *testing.T) {
	dir := privateDispatchDir(t)
	got, err := AuditSupervisorDispatchJournal(dir)
	if err != nil || got != (SupervisorDispatchJournalAudit{}) {
		t.Fatal(got, err)
	}
	before := make(map[string][]byte)
	for _, tc := range []struct {
		outcome string
		ack     bool
	}{
		{"", false}, {"succeeded", false}, {"failed", false}, {"unknown", false}, {"succeeded", true}, {"failed", true},
	} {
		name, raw := auditTestReceipt(t, dir, tc.outcome, tc.ack)
		before[name] = raw
	}
	want := SupervisorDispatchJournalAudit{Total: 6, AcceptedOnly: 1, Settled: 5, Acknowledged: 2, UnknownOutcome: 1}
	got, err = AuditSupervisorDispatchJournal(dir)
	if err != nil || got != want {
		t.Fatal(got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != len(before) {
		t.Fatal("inventory changed", err)
	}
	for name, raw := range before {
		after, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !bytes.Equal(after, raw) {
			t.Fatal("audit mutated receipt", err)
		}
	}
	// Public summary has no paths, identities, digests, authority flags or secrets.
	typ := reflect.TypeOf(got)
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).Type.Kind() != reflect.Int {
			t.Fatal("non-numeric summary field")
		}
	}
}

func auditTestRecords(t *testing.T, raw []byte) []SupervisorDispatchSnapshot {
	t.Helper()
	var records []SupervisorDispatchSnapshot
	for _, line := range bytes.Split(bytes.TrimSuffix(raw, []byte{'\n'}), []byte{'\n'}) {
		var record SupervisorDispatchSnapshot
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	return records
}

func auditTestEncode(t *testing.T, records []SupervisorDispatchSnapshot) []byte {
	t.Helper()
	var raw []byte
	for _, record := range records {
		line, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		raw = append(raw, append(line, '\n')...)
	}
	return raw
}

func TestSupervisorDispatchAuditMalformedZeroSummary(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"malformed-json", func([]byte) []byte { return []byte("{\n") }},
		{"unknown-field", func(raw []byte) []byte {
			return bytes.Replace(raw, []byte(`"State":`), []byte(`"secret":"private","State":`), 1)
		}},
		{"duplicate-field", func(raw []byte) []byte {
			return bytes.Replace(raw, []byte(`"State":`), []byte(`"State":"accepted","State":`), 1)
		}},
		{"noncanonical-whitespace", func(raw []byte) []byte {
			return bytes.Replace(raw, []byte(`"State":`), []byte(` "State":`), 1)
		}},
		{"extra-json", func(raw []byte) []byte { return bytes.Replace(raw, []byte("\n"), []byte(" {}\n"), 1) }},
		{"truncated", func(raw []byte) []byte { return raw[:len(raw)-3] }},
		{"missing-newline", func(raw []byte) []byte { return raw[:len(raw)-1] }},
		{"extra-line", func(raw []byte) []byte { return append(raw, []byte("{}\n")...) }},
		{"blank-line", func(raw []byte) []byte { return append(raw, '\n') }},
		{"oversized", func([]byte) []byte { return bytes.Repeat([]byte{'x'}, 4097) }},
		{"empty", func([]byte) []byte { return nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := privateDispatchDir(t)
			auditTestReceipt(t, dir, "", false) // No partial count may escape.
			name, raw := auditTestReceipt(t, dir, "succeeded", true)
			if err := os.WriteFile(filepath.Join(dir, name), tc.mutate(raw), 0600); err != nil {
				t.Fatal(err)
			}
			requireAuditClosed(t, dir)
		})
	}
}

func TestSupervisorDispatchAuditInvalidRecords(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func([]SupervisorDispatchSnapshot)
	}{
		{"session", func(r []SupervisorDispatchSnapshot) { r[0].Session = "00000000000000000000000000000000" }},
		{"sequence", func(r []SupervisorDispatchSnapshot) { r[0].Sequence++ }},
		{"zero-sequence", func(r []SupervisorDispatchSnapshot) { r[0].Sequence = 0 }},
		{"birth", func(r []SupervisorDispatchSnapshot) { r[0].Worker.StartUsec = 1000000 }},
		{"request-digest", func(r []SupervisorDispatchSnapshot) { r[0].RequestSHA256 = string(bytes.Repeat([]byte{'z'}, 64)) }},
		{"accepted-outcome", func(r []SupervisorDispatchSnapshot) { r[0].Outcome = "failed" }},
		{"accepted-flag", func(r []SupervisorDispatchSnapshot) { r[0].Acknowledged = true }},
		{"order", func(r []SupervisorDispatchSnapshot) { r[0], r[1] = r[1], r[0] }},
		{"outcome", func(r []SupervisorDispatchSnapshot) { r[1].Outcome = "safe" }},
		{"reply-digest", func(r []SupervisorDispatchSnapshot) { r[1].ReplySHA256 = "bad" }},
		{"settled-hidden-birth", func(r []SupervisorDispatchSnapshot) { r[1].Core.StartSec = 1 }},
		{"settled-hidden-worker", func(r []SupervisorDispatchSnapshot) { r[1].Worker.EffectiveUID = 1 }},
		{"settled-session", func(r []SupervisorDispatchSnapshot) { r[1].Session = r[0].Session }},
		{"settled-request", func(r []SupervisorDispatchSnapshot) { r[1].RequestSHA256 = r[0].RequestSHA256 }},
		{"settled-flag", func(r []SupervisorDispatchSnapshot) { r[1].Acknowledged = true }},
		{"ack-hidden-birth", func(r []SupervisorDispatchSnapshot) { r[2].Worker.StartUsec = 1 }},
		{"ack-outcome", func(r []SupervisorDispatchSnapshot) { r[2].Outcome = "succeeded" }},
		{"ack-mismatch", func(r []SupervisorDispatchSnapshot) { r[2].ReplySHA256 = r[0].RequestSHA256 }},
		{"ack-flag", func(r []SupervisorDispatchSnapshot) { r[2].Acknowledged = true }},
		{"unknown-ack", func(r []SupervisorDispatchSnapshot) { r[1].Outcome = "unknown" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := privateDispatchDir(t)
			name, raw := auditTestReceipt(t, dir, "succeeded", true)
			r := auditTestRecords(t, raw)
			tc.mutate(r)
			if err := os.WriteFile(filepath.Join(dir, name), auditTestEncode(t, r), 0600); err != nil {
				t.Fatal(err)
			}
			requireAuditClosed(t, dir)
		})
	}
}

func TestSupervisorDispatchAuditUnsafeEntries(t *testing.T) {
	for _, kind := range []string{"arbitrary", "symlink", "hardlink", "directory", "fifo", "mode", "special-mode", "owner"} {
		t.Run(kind, func(t *testing.T) {
			dir := privateDispatchDir(t)
			name, raw := auditTestReceipt(t, dir, "", false)
			path := filepath.Join(dir, name)
			var err error
			switch kind {
			case "arbitrary":
				err = os.WriteFile(filepath.Join(dir, "secret-name"), raw, 0600)
			case "symlink", "directory", "fifo":
				if err = os.Remove(path); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "symlink":
					err = os.Symlink("missing", path)
				case "directory":
					err = os.Mkdir(path, 0600)
				case "fifo":
					err = unix.Mkfifo(path, 0600)
				}
			case "hardlink":
				err = os.Link(path, filepath.Join(t.TempDir(), "linked"))
			case "mode":
				err = os.Chmod(path, 0644)
			case "owner", "special-mode":
				// Exercise stat rejection without root/chown or relying on
				// the filesystem preserving set-ID bits across reads.
				var st unix.Stat_t
				if err = unix.Lstat(path, &st); err == nil {
					if kind == "owner" {
						st.Uid++
					} else {
						st.Mode |= 04000
					}
					if auditSafeReceipt(st) {
						t.Fatal("unsafe receipt stat admitted")
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			requireAuditClosed(t, dir)
		})
	}
	for _, mode := range []os.FileMode{0000, 0755} {
		dir := privateDispatchDir(t)
		if err := os.Chmod(dir, mode); err != nil {
			t.Fatal(err)
		}
		requireAuditClosed(t, dir)
		if err := os.Chmod(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	dir := privateDispatchDir(t)
	link := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	requireAuditClosed(t, link)
	requireAuditClosed(t, link+"/")
	requireAuditClosed(t, "relative")
}

func TestSupervisorDispatchAuditLimit(t *testing.T) {
	dir := privateDispatchDir(t)
	_, raw := auditTestReceipt(t, dir, "", false)
	record := auditTestRecords(t, raw)
	for i := 2; i <= 65; i++ {
		record[0].Sequence = uint64(i)
		path := filepath.Join(dir, fmt.Sprintf("dispatch-%s-%016x.v1", record[0].Session, i))
		if err := os.WriteFile(path, auditTestEncode(t, record), 0600); err != nil {
			t.Fatal(err)
		}
		if i == 64 {
			got, err := AuditSupervisorDispatchJournal(dir)
			if err != nil || got.Total != 64 || got.AcceptedOnly != 64 {
				t.Fatal(got, err)
			}
		}
	}
	requireAuditClosed(t, dir)
}

func TestSupervisorDispatchAuditPinnedReplacement(t *testing.T) {
	dir := privateDispatchDir(t)
	name, raw := auditTestReceipt(t, dir, "", false)
	ledger, err := OpenSupervisorDispatchLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	entries, err := auditSupervisorEntries(ledger.dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	// Same inode, same size, changed content/mtime must also fail closed.
	changed := append([]byte(nil), raw...)
	changed[0] = '['
	if err := os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, time.Unix(1, 0), time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	got, err := auditSupervisorReceipt(ledger.dir, name, entries[name])
	if err != ErrSupervisorDispatchLedgerUnavailable || got != (SupervisorDispatchSnapshot{}) {
		t.Fatal("content change credited", got, err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	entries, err = auditSupervisorEntries(ledger.dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(t.TempDir(), "old")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	got, err = auditSupervisorReceipt(ledger.dir, name, entries[name])
	if err != ErrSupervisorDispatchLedgerUnavailable || got != (SupervisorDispatchSnapshot{}) {
		t.Fatal("replacement credited", got, err)
	}
	var before unix.Stat_t
	if err := unix.Fstat(ledger.dir, &before); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dir, filepath.Join(t.TempDir(), "old-dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if auditDirectoryPinned(ledger.dir, dir, before) {
		t.Fatal("replaced directory credited")
	}
}

func TestSupervisorDispatchAuditStructuralOnly(t *testing.T) {
	dir := privateDispatchDir(t)
	name, raw := auditTestReceipt(t, dir, "", false)
	records := auditTestRecords(t, raw)
	// No original frame is available: a well-shaped digest is not authenticated.
	records[0].RequestSHA256 = string(bytes.Repeat([]byte{'0'}, 64))
	if err := os.WriteFile(filepath.Join(dir, name), auditTestEncode(t, records), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := AuditSupervisorDispatchJournal(dir)
	if err != nil || got != (SupervisorDispatchJournalAudit{Total: 1, AcceptedOnly: 1}) {
		t.Fatal(got, err)
	}
	// The existing signed-frame inspection must still enforce the exact digest.
	frame := SupervisorToolCallFrame{Session: records[0].Session, Sequence: records[0].Sequence}
	if _, err := InspectSupervisorDispatchReceipt(dir, frame); err != ErrSupervisorDispatchLedgerUnavailable {
		t.Fatal("structural audit weakened signed-frame inspection", err)
	}
}
