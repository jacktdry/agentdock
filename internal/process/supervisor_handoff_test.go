package process

import (
	"bytes"
	"errors"
	"sync"
	"testing"
)

func testHandoffIdentities() (BirthIdentity, BirthIdentity, BirthIdentity) {
	core := BirthIdentity{PID: 200, ParentPID: 1, StartSec: 1000, StartUsec: 1, EffectiveUID: 501, ProcessGroupID: 200}
	supervisor := BirthIdentity{PID: 100, ParentPID: 1, StartSec: 900, StartUsec: 1, EffectiveUID: 501, ProcessGroupID: 100}
	worker := BirthIdentity{PID: 300, ParentPID: 200, StartSec: 1001, StartUsec: 1, EffectiveUID: 501, ProcessGroupID: 300}
	return core, supervisor, worker
}

func TestReadOnlySupervisorHandoffReplayAndTerminalPhases(t *testing.T) {
	core, supervisor, worker := testHandoffIdentities()
	secret := bytes.Repeat([]byte{0x7b}, 32)
	h, err := NewReadOnlySupervisorHandoff(core, supervisor, worker, secret)
	if err != nil {
		t.Fatal(err)
	}
	c, err := h.Challenge()
	if err != nil {
		t.Fatal(err)
	}
	frame, err := HandoffAttachMAC(secret, c, core, supervisor, worker)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.attachVerifiedPeer(core, frame); err != nil {
		t.Fatal(err)
	}
	if h.Phase() != SupervisorCoreAttached {
		t.Fatal("core attach not recorded")
	}
	if err := h.attachVerifiedPeer(core, frame); !errors.Is(err, ErrSupervisorHandoffUnavailable) {
		t.Fatal("replay accepted")
	}
	if err := h.recordCoreExit(core); err != nil {
		t.Fatal(err)
	}
	if h.Phase() != SupervisorCoreExited {
		t.Fatal("core crash not recorded")
	}
	if _, err := h.Challenge(); err == nil {
		t.Fatal("new challenge after crash")
	}
	if err := h.attachVerifiedPeer(core, frame); err == nil {
		t.Fatal("crashed generation reattached")
	}
	if err := h.recordWorkerExit(worker); err != nil {
		t.Fatal(err)
	}
	if h.Phase() != SupervisorWorkerExited {
		t.Fatal("wrapper NOTE_EXIT missing")
	}
	if err := h.recordWorkerExit(worker); err == nil {
		t.Fatal("duplicate event accepted")
	}
	if err := h.recordCoreExit(core); err == nil {
		t.Fatal("Core exit after worker exit accepted")
	}
	h.MarkUncertain()
	if h.Phase() != SupervisorUncertain {
		t.Fatal("uncertain did not block")
	}
	if err := h.recordCoreExit(core); err == nil {
		t.Fatal("uncertain escaped through event")
	}
}

func TestReadOnlySupervisorHandoffAuthenticationRejections(t *testing.T) {
	core, supervisor, worker := testHandoffIdentities()
	secret := bytes.Repeat([]byte{0x47}, 32)
	h, err := NewReadOnlySupervisorHandoff(core, supervisor, worker, secret)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := h.Challenge()
	if err != nil {
		t.Fatal(err)
	}
	valid, err := HandoffAttachMAC(secret, challenge, core, supervisor, worker)
	if err != nil {
		t.Fatal(err)
	}
	bad := valid
	bad.MAC = bytesToFakeMAC()
	badSeq := valid
	badSeq.Sequence = 2
	badNonce := valid
	badNonce.Nonce = "deadbeef"
	badVersion := valid
	badVersion.Version = 2
	badOperation := valid
	badOperation.Operation = "kill-worker"
	changedWorker := worker
	changedWorker.StartUsec++
	changed, err := HandoffAttachMAC(secret, challenge, core, supervisor, changedWorker)
	if err != nil {
		t.Fatal(err)
	}
	wrongSecret := bytes.Repeat([]byte{0x48}, 32)
	forged, err := HandoffAttachMAC(wrongSecret, challenge, core, supervisor, worker)
	if err != nil {
		t.Fatal(err)
	}
	wrongPeer := core
	wrongPeer.StartUsec++
	for _, tc := range []struct {
		name  string
		peer  BirthIdentity
		frame SupervisorHandoffAttach
	}{
		{"mac", core, bad}, {"sequence", core, badSeq}, {"nonce", core, badNonce},
		{"version", core, badVersion}, {"operation", core, badOperation},
		{"worker-birth", core, changed}, {"secret", core, forged}, {"os-peer-birth", wrongPeer, valid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := h.attachVerifiedPeer(tc.peer, tc.frame); !errors.Is(err, ErrSupervisorHandoffUnavailable) {
				t.Fatalf("accepted %s: %v", tc.name, err)
			}
			if h.Phase() != SupervisorAwaitingCore {
				t.Fatal("invalid attempt mutated state")
			}
		})
	}
	if err := h.attachVerifiedPeer(core, valid); err != nil {
		t.Fatal("valid Core rejected after forged attempts:", err)
	}
}

func bytesToFakeMAC() string { return "aa" + string(bytes.Repeat([]byte{'0'}, 62)) }

func TestReadOnlySupervisorHandoffConcurrentlyOneAttach(t *testing.T) {
	core, supervisor, worker := testHandoffIdentities()
	secret := bytes.Repeat([]byte{1}, 32)
	h, _ := NewReadOnlySupervisorHandoff(core, supervisor, worker, secret)
	challenge, _ := h.Challenge()
	frame, _ := HandoffAttachMAC(secret, challenge, core, supervisor, worker)
	var wg sync.WaitGroup
	accepted := make(chan bool, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); accepted <- h.attachVerifiedPeer(core, frame) == nil }()
	}
	wg.Wait()
	close(accepted)
	count := 0
	for ok := range accepted {
		if ok {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("non-idempotent replay under concurrency: %d attachments admitted", count)
	}
}

func TestReadOnlySupervisorHandoffInvalidBirthAndEntropy(t *testing.T) {
	core, supervisor, worker := testHandoffIdentities()
	key := make([]byte, 32)
	if _, err := NewReadOnlySupervisorHandoff(core, supervisor, worker, key[:31]); err == nil {
		t.Fatal("short key admitted")
	}
	bad := worker
	bad.ParentPID = 999
	if _, err := NewReadOnlySupervisorHandoff(core, supervisor, bad, key); err == nil {
		t.Fatal("wrong parent admitted")
	}
	bad = worker
	bad.EffectiveUID++
	if _, err := NewReadOnlySupervisorHandoff(core, supervisor, bad, key); err == nil {
		t.Fatal("wrong UID admitted")
	}
	bad = worker
	bad.StartSec = 1
	if _, err := NewReadOnlySupervisorHandoff(core, supervisor, bad, key); err == nil {
		t.Fatal("pre-Core birth admitted")
	}
	first, _ := NewReadOnlySupervisorHandoff(core, supervisor, worker, key)
	second, _ := NewReadOnlySupervisorHandoff(core, supervisor, worker, key)
	c1, _ := first.Challenge()
	c2, _ := second.Challenge()
	if c1.Session == c2.Session || c1.Nonce == c2.Nonce {
		t.Fatal("reused entropy for separate Core sessions")
	}
}

func TestSupervisorOwnedReadOnlyHandoffParentMode(t *testing.T) {
	core, supervisor, worker := testHandoffIdentities()
	worker.ParentPID = supervisor.PID
	key := bytes.Repeat([]byte{0x39}, 32)
	if _, err := NewReadOnlySupervisorHandoff(core, supervisor, worker, key); err == nil {
		t.Fatal("Core-owned contract admitted Supervisor-owned worker")
	}
	h, err := NewSupervisorOwnedReadOnlyHandoff(core, supervisor, worker, key)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := h.Challenge()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := HandoffAttachMAC(key, challenge, core, supervisor, worker); err == nil {
		t.Fatal("Legacy Core-owned MAC admitted different worker parent")
	}
	frame, err := HandoffAttachMACForWorkerParent(key, challenge, core, supervisor, worker, SupervisorWorkerParentSupervisor)
	if err != nil {
		t.Fatal(err)
	}
	wrong := frame
	wrong.MAC = bytesToFakeMAC()
	if err := h.attachVerifiedPeer(core, wrong); err == nil {
		t.Fatal("bad MAC admitted")
	}
	if err := h.attachVerifiedPeer(core, frame); err != nil {
		t.Fatal(err)
	}
	if err := h.attachVerifiedPeer(core, frame); err == nil {
		t.Fatal("replayed Supervisor-owned MAC accepted")
	}
	changed := worker
	changed.StartUsec++
	if _, err := NewSupervisorOwnedReadOnlyHandoff(core, supervisor, changed, key); err != nil {
		t.Fatalf("separate birth with same parent invalid: %v", err)
	}
	invalid := worker
	invalid.ParentPID = 123
	if _, err := NewSupervisorOwnedReadOnlyHandoff(core, supervisor, invalid, key); err == nil {
		t.Fatal("unowned worker admitted")
	}
	if _, err := HandoffAttachMACForWorkerParent(key, challenge, core, supervisor, worker, "unknown"); err == nil {
		t.Fatal("unknown ownership mode admitted")
	}
}
