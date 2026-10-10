//go:build darwin

package process

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func signedReceiptACK(t *testing.T, request SupervisorToolCallFrame, core, worker BirthIdentity, reply any) SupervisorDispatchAckFrame {
	t.Helper()
	supervisor := core
	supervisor.PID = worker.ParentPID
	supervisor.StartSec = worker.StartSec - 1
	supervisor.StartUsec = 0
	supervisor.EffectiveUID = worker.EffectiveUID
	supervisor.ProcessGroupID = worker.ParentPID
	key := bytes.Repeat([]byte{0xa7}, 32)
	ack, err := SignSupervisorDispatchAckPrototype(key, core, supervisor, worker, request, reply)
	if err != nil {
		t.Fatal(err)
	}
	return ack
}

func TestSupervisorDispatchACKOnlyAfterSettledAndNoReplay(t *testing.T) {
	dir := privateDispatchDir(t)
	ledger, err := OpenSupervisorDispatchLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	request, core, worker := dispatchTestFrame(t)
	ticket, err := ledger.BeginVerified(request, core, worker)
	if err != nil {
		t.Fatal(err)
	}
	defer ticket.Close()
	reply := map[string]any{"version": 1, "sequence": 1, "ok": true}
	ack := signedReceiptACK(t, request, core, worker, reply)
	if err := ledger.ackVerified(request, ack, core, worker); !errors.Is(err, ErrSupervisorDispatchLedgerUnavailable) {
		t.Fatal("Core ACK admitted before Supervisor tool outcome", err)
	}
	snapshot, err := InspectSupervisorDispatchReceipt(dir, request)
	if err != nil || snapshot.State != "accepted" || snapshot.Acknowledged {
		t.Fatalf("premature ACK altered accepted-only receipt: %+v %v", snapshot, err)
	}
	encoded, _ := json.Marshal(reply)
	if err := ticket.Finish("succeeded", encoded); err != nil {
		t.Fatal(err)
	}
	snapshot, err = InspectSupervisorDispatchReceipt(dir, request)
	if err != nil || snapshot.State != "settled" || snapshot.Acknowledged {
		t.Fatalf("settled without Core ACK was assumed acknowledged: %+v %v", snapshot, err)
	}
	if err := ledger.ackVerified(request, ack, core, worker); err != nil {
		t.Fatal(err)
	}
	snapshot, err = InspectSupervisorDispatchReceipt(dir, request)
	if err != nil || !snapshot.Acknowledged || snapshot.State != "settled" ||
		snapshot.Outcome != "succeeded" || snapshot.ReplySHA256 != ack.ReplySHA256 {
		t.Fatalf("valid Core ACK not durably recorded: %+v %v", snapshot, err)
	}
	if err := ledger.ackVerified(request, ack, core, worker); err == nil {
		t.Fatal("ACK replay appended fourth entry")
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	recovered, err := InspectSupervisorDispatchReceipt(dir, request)
	if err != nil || !recovered.Acknowledged {
		t.Fatal("reopen lost persisted ACK", err)
	}
}

func TestSupervisorDispatchACKWrongReplyAndIdentity(t *testing.T) {
	dir := privateDispatchDir(t)
	ledger, err := OpenSupervisorDispatchLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	request, core, worker := dispatchTestFrame(t)
	ticket, err := ledger.BeginVerified(request, core, worker)
	if err != nil {
		t.Fatal(err)
	}
	defer ticket.Close()
	reply := map[string]any{"ok": true, "result": "was-performed"}
	encoded, _ := json.Marshal(reply)
	if err := ticket.Finish("failed", encoded); err != nil {
		t.Fatal(err)
	}
	ack := signedReceiptACK(t, request, core, worker, reply)
	invalid := []struct {
		name   string
		mutate func(*SupervisorDispatchAckFrame)
	}{
		{"different-reply", func(a *SupervisorDispatchAckFrame) { a.ReplySHA256 = bytesToHex(sha256.Sum256([]byte("other"))) }},
		{"different-request", func(a *SupervisorDispatchAckFrame) { a.RequestSHA256 = bytesToHex(sha256.Sum256([]byte("different"))) }},
		{"different-seq", func(a *SupervisorDispatchAckFrame) { a.Sequence++ }},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			modified := ack
			tc.mutate(&modified)
			if err := ledger.ackVerified(request, modified, core, worker); err == nil {
				t.Fatal("altered receipt ACK credited")
			}
		})
	}
	wrongCore := core
	wrongCore.StartUsec++
	if err := ledger.ackVerified(request, ack, wrongCore, worker); err == nil {
		t.Fatal("ACK attributed to wrong Core birth")
	}
	wrongWorker := worker
	wrongWorker.StartSec++
	if err := ledger.ackVerified(request, ack, core, wrongWorker); err == nil {
		t.Fatal("ACK attributed to other Worker birth")
	}
	snapshot, err := InspectSupervisorDispatchReceipt(dir, request)
	if err != nil || snapshot.Acknowledged {
		t.Fatal("tampered ACK changed receipt", snapshot, err)
	}
	// "failed" only describes observed SDK error, never that no external
	// side effect happened. Core may still acknowledge that exact reply.
	if err := ledger.ackVerified(request, ack, core, worker); err != nil {
		t.Fatal(err)
	}
}

func bytesToHex(digest [32]byte) string {
	const symbols = "0123456789abcdef"
	buf := make([]byte, 64)
	for i, b := range digest {
		buf[i*2] = symbols[b>>4]
		buf[i*2+1] = symbols[b&0x0f]
	}
	return string(buf)
}
func TestSupervisorDispatchACKUnknownAndDamagedReceipt(t *testing.T) {
	dir := privateDispatchDir(t)
	ledger, err := OpenSupervisorDispatchLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	request, core, worker := dispatchTestFrame(t)
	ticket, err := ledger.BeginVerified(request, core, worker)
	if err != nil {
		t.Fatal(err)
	}
	defer ticket.Close()
	reply := map[string]any{"ok": false}
	encoded, _ := json.Marshal(reply)
	if err := ticket.Finish("unknown", encoded); err != nil {
		t.Fatal(err)
	}
	ack := signedReceiptACK(t, request, core, worker, reply)
	if err := ledger.ackVerified(request, ack, core, worker); err == nil {
		t.Fatal("unknown SDK outcome falsely acknowledged as completed")
	}
	name, err := supervisorReceiptName(request)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(payload, []byte("{incomplete")...), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ledger.ackVerified(request, ack, core, worker); err == nil {
		t.Fatal("torn receipt accepted ACK")
	}
	if _, err := InspectSupervisorDispatchReceipt(dir, request); err == nil {
		t.Fatal("torn ACK recovery accepted")
	}
}
func TestSupervisorDispatchACKConcurrentExactlyOne(t *testing.T) {
	dir := privateDispatchDir(t)
	ledger, err := OpenSupervisorDispatchLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	request, core, worker := dispatchTestFrame(t)
	ticket, err := ledger.BeginVerified(request, core, worker)
	if err != nil {
		t.Fatal(err)
	}
	defer ticket.Close()
	reply := map[string]any{"ok": true}
	encoded, _ := json.Marshal(reply)
	if err := ticket.Finish("succeeded", encoded); err != nil {
		t.Fatal(err)
	}
	ack := signedReceiptACK(t, request, core, worker, reply)
	var wg sync.WaitGroup
	successes := make(chan bool, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); successes <- ledger.ackVerified(request, ack, core, worker) == nil }()
	}
	wg.Wait()
	close(successes)
	count := 0
	for ok := range successes {
		if ok {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("ACK replay/concurrency produced %d durable ACKs", count)
	}
	snapshot, err := InspectSupervisorDispatchReceipt(dir, request)
	if err != nil || !snapshot.Acknowledged {
		t.Fatal("concurrent ACK receipt corrupted", err)
	}
}
