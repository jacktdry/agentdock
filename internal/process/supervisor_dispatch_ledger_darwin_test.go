//go:build darwin

package process

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func dispatchTestFrame(t *testing.T) (SupervisorToolCallFrame, BirthIdentity, BirthIdentity) {
	t.Helper()
	core, supervisor, worker := testHandoffIdentities()
	worker.ParentPID = supervisor.PID
	key := bytes.Repeat([]byte{0x44}, 32)
	handoff, err := NewSupervisorOwnedReadOnlyHandoff(core, supervisor, worker, key)
	if err != nil {
		t.Fatal(err)
	}
	challenge, _ := handoff.Challenge()
	frame, err := SignSupervisorOwnedToolCallPrototype(key, challenge, core, supervisor, worker, 1, "echo", map[string]any{"text": "durable-private"})
	if err != nil {
		t.Fatal(err)
	}
	return frame, core, worker
}
func privateDispatchDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}
func TestSupervisorDispatchReceiptAcceptedSettledReadOnly(t *testing.T) {
	dir := privateDispatchDir(t)
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
	accepted, err := InspectSupervisorDispatchReceipt(dir, frame)
	if err != nil || accepted.State != "accepted" || accepted.Core != core || accepted.Worker != worker ||
		accepted.Sequence != 1 || accepted.RequestSHA256 == "" {
		t.Fatalf("accepted-only record did not survive inspection: %+v %v", accepted, err)
	}
	if _, err := ledger.BeginVerified(frame, core, worker); !errors.Is(err, ErrSupervisorDispatchLedgerUnavailable) {
		t.Fatal("duplicate accepted frame opened another receipt")
	}
	// Outcome is a hash of the reply, not a record containing secrets or an
	// assertion that Core actually received the response.
	reply, _ := json.Marshal(map[string]any{"result": "echo"})
	if err := ticket.Finish("succeeded", reply); err != nil {
		t.Fatal(err)
	}
	if err := ticket.Finish("succeeded", reply); err == nil {
		t.Fatal("duplicate outcome written")
	}
	if err := ticket.Close(); err != nil {
		t.Fatal(err)
	}
	observed, err := InspectSupervisorDispatchReceipt(dir, frame)
	if err != nil || observed.State != "settled" || observed.Outcome != "succeeded" ||
		len(observed.ReplySHA256) != 64 || observed.ReplySHA256 == string(reply) {
		t.Fatalf("completed receipt not durable: %+v %v", observed, err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.BeginVerified(frame, core, worker); err == nil {
		t.Fatal("closed ledger allowed append")
	}
}
func TestSupervisorDispatchAcceptedOnlyStillAmbiguous(t *testing.T) {
	dir := privateDispatchDir(t)
	ledger, err := OpenSupervisorDispatchLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	frame, core, worker := dispatchTestFrame(t)
	ticket, err := ledger.BeginVerified(frame, core, worker)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate OS crash between fsynced ACCEPTED and response/outcome append.
	if err := ticket.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	observed, err := InspectSupervisorDispatchReceipt(dir, frame)
	if err != nil || observed.State != "accepted" || observed.Outcome != "" || observed.ReplySHA256 != "" {
		t.Fatalf("ambiguous accepted-only result was presumed committed: %+v %v", observed, err)
	}
	// Restarting the ledger does not grant replay of a receipt filename.
	reopened, err := OpenSupervisorDispatchLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := reopened.BeginVerified(frame, core, worker); err == nil {
		t.Fatal("replay after ledger restart accepted")
	}
}
func TestSupervisorDispatchDirectoryAndSymlinkFailClosed(t *testing.T) {
	dir := t.TempDir()
	frame, core, worker := dispatchTestFrame(t)
	if _, err := OpenSupervisorDispatchLedger(dir); err == nil {
		t.Fatal("non-0700 dir admitted")
	}
	_ = os.Chmod(dir, 0700)
	sym := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(dir, sym); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSupervisorDispatchLedger(sym); err == nil {
		t.Fatal("symlink state dir admitted")
	}
	ledger, err := OpenSupervisorDispatchLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	ticket, err := ledger.BeginVerified(frame, core, worker)
	if err != nil {
		t.Fatal(err)
	}
	name, err := supervisorReceiptName(frame)
	if err != nil {
		t.Fatal(err)
	}
	// Parent directory remains pinned; changing receipt path inode must
	// prevent any future settled record from being credited to the old file.
	path := filepath.Join(dir, name)
	backup := filepath.Join(t.TempDir(), "gone")
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(backup, path); err != nil {
		t.Fatal(err)
	}
	if err := ticket.Finish("succeeded", nil); !errors.Is(err, ErrSupervisorDispatchLedgerUnavailable) {
		t.Fatal("replaced receipt inode got a signed completion")
	}
	_ = ticket.Close()
	if _, err := InspectSupervisorDispatchReceipt(dir, frame); err == nil {
		t.Fatal("symlink receipt read as valid")
	}
	_ = os.Remove(path)
	// Both a partial write and extra unknown records are rejected, never
	// turned into a safe outcome.
	raw, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, []byte("{unclosed")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectSupervisorDispatchReceipt(dir, frame); err == nil {
		t.Fatal("torn receipt accepted")
	}
}
func TestSupervisorDispatchConcurrentDuplicateCreate(t *testing.T) {
	dir := privateDispatchDir(t)
	ledger, err := OpenSupervisorDispatchLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	frame, core, worker := dispatchTestFrame(t)
	var wg sync.WaitGroup
	admitted := make(chan *SupervisorDispatchTicket, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticket, err := ledger.BeginVerified(frame, core, worker)
			if err == nil {
				admitted <- ticket
			}
		}()
	}
	wg.Wait()
	close(admitted)
	count := 0
	for ticket := range admitted {
		count++
		_ = ticket.Close()
	}
	if count != 1 {
		t.Fatalf("durable sequence opened multiple receipts: %d", count)
	}
}
func TestSupervisorDispatchCorruptStateAndBadNames(t *testing.T) {
	dir := privateDispatchDir(t)
	ledger, err := OpenSupervisorDispatchLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	frame, core, worker := dispatchTestFrame(t)
	frame.Session = "../" + frame.Session
	if _, err := ledger.BeginVerified(frame, core, worker); err == nil {
		t.Fatal("unsafe file name admitted")
	}
	frame.Session = frame.Session[3:]
	frame.Sequence = 0
	if _, err := ledger.BeginVerified(frame, core, worker); err == nil {
		t.Fatal("zero sequence admitted")
	}
	frame.Sequence = 1
	if err := os.WriteFile(filepath.Join(dir, "unknown-file"), []byte("junk"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.BeginVerified(frame, core, worker); err == nil {
		t.Fatal("unknown entry in private state dir ignored")
	}
}
func TestSupervisorDispatchRandomSessionUnrelatedToUser(t *testing.T) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	frame, _, _ := dispatchTestFrame(t)
	if _, err := supervisorReceiptName(frame); err != nil {
		t.Fatal(err)
	}
}

func TestSupervisorDispatchFailedPrepareDoesNotConsumeSequence(t *testing.T) {
	core, supervisor, worker := testHandoffIdentities()
	worker.ParentPID = supervisor.PID
	key := bytes.Repeat([]byte{17}, 32)
	handoff, err := NewSupervisorOwnedReadOnlyHandoff(core, supervisor, worker, key)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := handoff.Challenge()
	if err != nil {
		t.Fatal(err)
	}
	attach, err := HandoffAttachMACForWorkerParent(key, challenge, core, supervisor, worker, SupervisorWorkerParentSupervisor)
	if err != nil {
		t.Fatal(err)
	}
	if err := handoff.attachVerifiedPeer(core, attach); err != nil {
		t.Fatal(err)
	}
	frame, err := SignSupervisorOwnedToolCallPrototype(key, challenge, core, supervisor, worker, 1, "echo", map[string]any{"text": "never-run"})
	if err != nil {
		t.Fatal(err)
	}
	dir := privateDispatchDir(t)
	ledger, err := OpenSupervisorDispatchLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	ticket, err := handoff.acceptVerifiedToolCallPrepared(core, frame, map[string]struct{}{"echo": {}}, ledger)
	if ticket != nil || !errors.Is(err, ErrSupervisorHandoffUnavailable) {
		t.Fatalf("unsafe receipt write allowed execution: %+v %v", ticket, err)
	}
	if handoff.Phase() != SupervisorUncertain || handoff.lastToolSequence != 0 {
		t.Fatal("fsync/unsafe-dir failure consumed sequence or left generation executable")
	}
	// Do not treat repair of the directory as permission to resume a
	// potentially partially persisted/ambiguous request.
	_ = os.Chmod(dir, 0700)
	if ticket, err := handoff.acceptVerifiedToolCallPrepared(core, frame, map[string]struct{}{"echo": {}}, ledger); ticket != nil || err == nil {
		t.Fatal("repair unexpectedly unquarantined request generation")
	}
}
func TestSupervisorDispatchSymlinkReceiptPoisonsNewSequence(t *testing.T) {
	dir := privateDispatchDir(t)
	frame, core, worker := dispatchTestFrame(t)
	name, err := supervisorReceiptName(frame)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "target"), filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}
	ledger, err := OpenSupervisorDispatchLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	frame.Sequence = 2
	if _, err := ledger.BeginVerified(frame, core, worker); err == nil {
		t.Fatal("symlink in private journal went unnoticed")
	}
}
