package process

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestSupervisorToolCallScopedHMACAndReplay(t *testing.T) {
	core, supervisor, worker := testHandoffIdentities()
	worker.ParentPID = supervisor.PID
	secret := bytes.Repeat([]byte{0x5a}, 32)
	session, err := NewSupervisorOwnedReadOnlyHandoff(core, supervisor, worker, secret)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := session.Challenge()
	if err != nil {
		t.Fatal(err)
	}
	attach, err := HandoffAttachMACForWorkerParent(secret, challenge, core, supervisor, worker, SupervisorWorkerParentSupervisor)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.attachVerifiedPeer(core, attach); err != nil {
		t.Fatal(err)
	}
	scope := map[string]struct{}{"echo": {}}
	signed, err := SignSupervisorOwnedToolCallPrototype(secret, challenge, core, supervisor, worker, 1, "echo", map[string]any{"text": "ok"})
	if err != nil {
		t.Fatal(err)
	}
	invalid := []struct {
		name  string
		frame SupervisorToolCallFrame
		scope map[string]struct{}
	}{}
	mutate := func(name string, apply func(*SupervisorToolCallFrame)) {
		f := signed
		apply(&f)
		invalid = append(invalid, struct {
			name  string
			frame SupervisorToolCallFrame
			scope map[string]struct{}
		}{name, f, scope})
	}
	mutate("tampered-arguments", func(f *SupervisorToolCallFrame) { f.Arguments = json.RawMessage("{\"text\":\"hacked\"}") })
	mutate("wrong-worker", func(f *SupervisorToolCallFrame) { f.WorkerStartUsec++ })
	mutate("wrong-sequence", func(f *SupervisorToolCallFrame) { f.Sequence = 2 })
	mutate("bad-mac", func(f *SupervisorToolCallFrame) { f.MAC = strings.Repeat("0", 64) })
	mutate("wrong-session", func(f *SupervisorToolCallFrame) { f.Session = strings.Repeat("1", 32) })
	mutate("wrong-operation", func(f *SupervisorToolCallFrame) { f.Operation = "kill" })
	mutate("unknown-tool", func(f *SupervisorToolCallFrame) { f.Tool = "destroy" })
	mutate("non-object-args", func(f *SupervisorToolCallFrame) { f.Arguments = json.RawMessage("[1,2]") })
	invalid = append(invalid, struct {
		name  string
		frame SupervisorToolCallFrame
		scope map[string]struct{}
	}{"no-local-grant", signed, map[string]struct{}{}})
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if err := session.acceptVerifiedToolCall(core, tc.frame, tc.scope); !errors.Is(err, ErrSupervisorHandoffUnavailable) {
				t.Fatalf("unsafe frame admitted: %v", err)
			}
		})
	}
	wrongPeer := core
	wrongPeer.StartUsec++
	if err := session.acceptVerifiedToolCall(wrongPeer, signed, scope); err == nil {
		t.Fatal("wrong OS peer birth accepted")
	}
	if err := session.acceptVerifiedToolCall(core, signed, scope); err != nil {
		t.Fatal(err)
	}
	if err := session.acceptVerifiedToolCall(core, signed, scope); err == nil {
		t.Fatal("replay accepted")
	}
	signed2, err := SignSupervisorOwnedToolCallPrototype(secret, challenge, core, supervisor, worker, 2, "echo", map[string]any{"text": "next"})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.acceptVerifiedToolCall(core, signed2, scope); err != nil {
		t.Fatal(err)
	}
	if err := session.recordCoreExit(core); err != nil {
		t.Fatal(err)
	}
	signed3, _ := SignSupervisorOwnedToolCallPrototype(secret, challenge, core, supervisor, worker, 3, "echo", nil)
	if err := session.acceptVerifiedToolCall(core, signed3, scope); err == nil {
		t.Fatal("Core-exited generation used tools")
	}
}

func TestSupervisorToolCallConcurrentSingleAdmission(t *testing.T) {
	core, supervisor, worker := testHandoffIdentities()
	worker.ParentPID = supervisor.PID
	key := bytes.Repeat([]byte{0x33}, 32)
	h, _ := NewSupervisorOwnedReadOnlyHandoff(core, supervisor, worker, key)
	challenge, _ := h.Challenge()
	attach, _ := HandoffAttachMACForWorkerParent(key, challenge, core, supervisor, worker, SupervisorWorkerParentSupervisor)
	if err := h.attachVerifiedPeer(core, attach); err != nil {
		t.Fatal(err)
	}
	frame, _ := SignSupervisorOwnedToolCallPrototype(key, challenge, core, supervisor, worker, 1, "echo", map[string]any{"text": "single"})
	var wg sync.WaitGroup
	success := make(chan bool, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			success <- h.acceptVerifiedToolCall(core, frame, map[string]struct{}{"echo": {}}) == nil
		}()
	}
	wg.Wait()
	close(success)
	n := 0
	for value := range success {
		if value {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("duplicate per-call execution authorization: %d", n)
	}
}

func TestSupervisorToolCallInvalidInput(t *testing.T) {
	core, supervisor, worker := testHandoffIdentities()
	worker.ParentPID = supervisor.PID
	key := bytes.Repeat([]byte{5}, 32)
	h, _ := NewSupervisorOwnedReadOnlyHandoff(core, supervisor, worker, key)
	c, _ := h.Challenge()
	if _, err := SignSupervisorOwnedToolCallPrototype(key[:31], c, core, supervisor, worker, 1, "echo", nil); err == nil {
		t.Fatal("short key admitted")
	}
	if _, err := SignSupervisorOwnedToolCallPrototype(key, c, core, supervisor, worker, 0, "echo", nil); err == nil {
		t.Fatal("seq zero admitted")
	}
	if _, err := SignSupervisorOwnedToolCallPrototype(key, c, core, supervisor, worker, 1, "../bad", nil); err == nil {
		t.Fatal("invalid name admitted")
	}
	if _, err := SignSupervisorOwnedToolCallPrototype(key, c, core, supervisor, worker, 1, "echo", map[string]any{"huge": strings.Repeat("z", SupervisorToolCallMaxArguments)}); err == nil {
		t.Fatal("oversized arguments admitted")
	}
	if _, err := SignSupervisorOwnedToolCallPrototype(key, c, core, supervisor, worker, 1, "echo", map[string]any{"bad": make(chan int)}); err == nil {
		t.Fatal("non-JSON admitted")
	}
}
