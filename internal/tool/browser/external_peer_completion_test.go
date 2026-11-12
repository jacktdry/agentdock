package browser

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TEST ONLY: run a hook after a fake page operation, before returning its result.
type releasePhaseBackend struct {
	*externalFakeBackend
	after func(string)
}

func (b *releasePhaseBackend) CallExternalFenced(ctx context.Context, worker WorkerInfo, peer externalPeerIdentity, tool string, args map[string]any) (map[string]any, error) {
	result, err := b.externalFakeBackend.CallExternalFenced(ctx, worker, peer, tool, args)
	b.after(tool)
	return result, err
}

func TestExternalReleaseQuarantinesPeerAtCloseBoundaries(t *testing.T) {
	for _, phase := range []string{"list_pages", "close_page"} {
		for _, mode := range []string{"source", "incarnation", "revoked", "unavailable", "canceled", "close error"} {
			for _, recovery := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/recovery=%t", phase, mode, recovery), func(t *testing.T) {
					m, b, v, meta, d := acquiredControlledPeer(t)
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					if recovery {
						b.stopErr = errors.New("owned connector stop failed")
					}
					if mode == "close error" {
						b.closeErr = errors.New("close-secret ws://private PID=456")
					}
					m.backend = &releasePhaseBackend{b, func(tool string) {
						if tool != phase {
							return
						}
						wantChecks := 4 // acquire's three checks, then release's initial check
						if phase == "close_page" {
							wantChecks = 5
						}
						if v.count() != wantChecks {
							t.Errorf("peer check not at expected phase: %d", v.count())
						}
						if mode == "canceled" {
							cancel()
						} else {
							rejectMode := mode
							if mode == "close error" {
								rejectMode = "unavailable"
							}
							rejectControlledPeer(m, v, rejectMode)
						}
					}}
					var cleaned LeaseMetadata
					var err error
					if mode == "source" && !recovery {
						err = m.SweepExpired(meta.ExpiresAt.Add(time.Second))
						lease, _ := m.lease(meta.BrowserLeaseID)
						cleaned = lease.metadata
					} else {
						cleaned, err = m.Release(ctx, d.Scope, meta.BrowserLeaseID)
					}
					l, _ := m.lease(meta.BrowserLeaseID)
					if !l.peerLost {
						t.Fatal("peer loss was not latched")
					}
					wantChecks := 5
					if phase == "close_page" {
						wantChecks++
					}
					if mode == "canceled" {
						wantChecks-- // canceled contexts reject before invoking verifier
					}
					if v.count() != wantChecks {
						t.Fatalf("peer checks=%d want=%d", v.count(), wantChecks)
					}
					for _, text := range []string{cleaned.CleanupError, fmt.Sprint(err)} {
						if strings.Contains(text, "secret") || strings.Contains(text, "ws://") || strings.Contains(text, "PID=") {
							t.Fatal("verifier/close details escaped quarantine")
						}
					}
					if recovery {
						assertBrowserCode(t, err, ErrActionFailed)
						if cleaned.CleanupState != CleanupFailed {
							t.Fatal("Stop failure lost recovery state")
						}
						b.stopErr = nil
						v.mu.Lock()
						v.observed, v.err = d.grant.peer, nil
						v.mu.Unlock()
						if err := m.RecoverFailed(time.Now().UTC().Add(time.Hour)); err != nil {
							t.Fatal(err)
						}
					}
					if !recovery && err != nil {
						t.Fatal(err)
					}
					// Repeated Release/Sweep never regain page authority, even if peer recovers.
					cleaned, err = m.Release(context.Background(), d.Scope, meta.BrowserLeaseID)
					if err != nil || cleaned.CleanupState != CleanupComplete || !strings.Contains(cleaned.CleanupReason, "owned page close unconfirmed") {
						t.Fatalf("false close confirmation: %+v %v", cleaned, err)
					}
					if err := m.SweepExpired(meta.ExpiresAt.Add(time.Hour)); err != nil {
						t.Fatal(err)
					}
					wantCloses, wantStops := 0, 1
					if phase == "close_page" {
						wantCloses = 1
					}
					if recovery {
						wantStops++
					}
					if externalCallCount(b, "list_pages") != 2 || externalCallCount(b, "close_page") != wantCloses || externalCallCount(b, "stop") != wantStops || v.count() != wantChecks {
						t.Fatal("cleanup/recovery repeated page operation or verification")
					}
					for _, call := range b.calls {
						if call.worker != meta.WorkerID || (call.tool == "close_page" && call.args["pageId"] != l.pageID) {
							t.Fatal("cleanup touched resources outside owned connector/page")
						}
					}
				})
			}
		}
	}
}

// Synthetic TEST-ONLY peer changes are injected inside fake backend operations.
// None of these tests connect to CDP, Edge, or a real browser process.
func TestExternalAdmissionQuarantinesPeerChangeAfterNewPage(t *testing.T) {
	for _, name := range []string{"source rollover", "incarnation rollover", "revoked", "verification unavailable", "stop recovery"} {
		t.Run(name, func(t *testing.T) {
			d := externalRoute(externalStart())
			b := &externalFakeBackend{}
			v := &controlledExternalPeer{observed: d.grant.peer}
			m := NewExternalLeaseManager(b)
			m.verifier = v
			b.afterCreate = func() {
				v.mu.Lock()
				defer v.mu.Unlock()
				switch name {
				case "source rollover":
					v.observed.source = &connectorEvidenceIdentity{}
				case "incarnation rollover":
					v.observed.incarnation = &connectorEvidenceIdentity{}
				case "revoked", "stop recovery":
					v.observed = externalPeerIdentity{}
				case "verification unavailable":
					v.err = errors.New("sensitive-provider-secret ws://private")
				}
			}
			if name == "stop recovery" {
				b.stopErr = errors.New("owned connector stop failed")
			}
			meta, binding, err := m.Acquire(context.Background(), d, "about:blank")
			assertBrowserCode(t, err, ErrRequiredRouteUnavailable)
			if meta != (LeaseMetadata{}) || binding != (EngineBinding{}) {
				t.Fatal("unqualified lease published")
			}
			if strings.Contains(err.Error(), "provider-secret") || strings.Contains(err.Error(), "ws://") {
				t.Fatal("provider error escaped to caller")
			}
			if v.count() != 3 || b.starts != 1 ||
				externalCallCount(b, "list_pages") != 1 ||
				externalCallCount(b, "new_page") != 1 ||
				externalCallCount(b, "close_page") != 0 ||
				externalCallCount(b, "stop") != 1 || len(m.leases) != 0 {
				t.Fatalf("ambiguous post-create cleanup: checks=%d calls=%+v", v.count(), b.calls)
			}
			if name == "stop recovery" {
				if len(m.orphans) != 1 {
					t.Fatal("connector recovery not recorded")
				}
				b.mu.Lock()
				b.stopErr = nil
				b.mu.Unlock()
				if err := m.RecoverFailed(time.Now().UTC().Add(time.Hour)); err != nil || len(m.orphans) != 0 {
					t.Fatalf("connector recovery: %v", err)
				}
				if externalCallCount(b, "close_page") != 0 || externalCallCount(b, "stop") != 2 {
					t.Fatal("connector recovery touched an unqualified page")
				}
			}
		})
	}
}

func TestExternalCallQuarantinesResultAfterPeerChangesDuringAction(t *testing.T) {
	for _, name := range []string{"source rollover", "incarnation rollover", "revoked", "provider error", "cancel", "tool error and rollover"} {
		t.Run(name, func(t *testing.T) {
			m, b, v, meta, d := acquiredControlledPeer(t)
			entered := make(chan struct{}, 1)
			gate := make(chan struct{})
			b.mu.Lock()
			b.entered, b.gate = entered, gate
			b.actionResult = map[string]any{
				"structuredContent": map[string]any{"snapshot": "private-browser-result"},
				"content":           []any{map[string]any{"type": "text", "text": "secret-tab-data"}},
			}
			if name == "tool error and rollover" {
				b.actionErr = errors.New("sensitive-tool-error")
			}
			b.mu.Unlock()

			l, leaseErr := m.lease(meta.BrowserLeaseID)
			if leaseErr != nil {
				t.Fatal(leaseErr)
			}
			l.mu.Lock()
			beforeActive, beforeExpiry := l.metadata.LastActiveAt, l.metadata.ExpiresAt
			l.mu.Unlock()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			type reply struct {
				result map[string]any
				err    error
			}
			finished := make(chan reply, 1)
			go func() {
				res, err := m.Call(ctx, d.Scope, meta.BrowserLeaseID, "take_snapshot", nil)
				finished <- reply{res, err}
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				close(gate)
				t.Fatal("fake action did not enter")
			}

			v.mu.Lock()
			switch name {
			case "source rollover", "tool error and rollover":
				v.observed.source = &connectorEvidenceIdentity{}
			case "incarnation rollover":
				v.observed.incarnation = &connectorEvidenceIdentity{}
			case "revoked":
				v.observed = externalPeerIdentity{}
			case "provider error":
				v.err = errors.New("sensitive-verifier-error")
			case "cancel":
				cancel()
			}
			v.mu.Unlock()
			close(gate)
			var got reply
			select {
			case got = <-finished:
			case <-time.After(5 * time.Second):
				t.Fatal("call did not return after identity loss")
			}
			if got.result != nil {
				t.Fatalf("stale browser result escaped: %+v", got.result)
			}
			assertBrowserCode(t, got.err, ErrRequiredRouteUnavailable)
			if strings.Contains(got.err.Error(), "sensitive") || strings.Contains(got.err.Error(), "secret") {
				t.Fatal("provider/tool error leaked")
			}
			l.mu.Lock()
			active, expiry, lost := l.metadata.LastActiveAt, l.metadata.ExpiresAt, l.peerLost
			l.mu.Unlock()
			if !lost || !active.Equal(beforeActive) || !expiry.Equal(beforeExpiry) {
				t.Fatal("stale action refreshed lease or failed to latch peer loss")
			}
			v.mu.Lock()
			v.observed, v.err = d.grant.peer, nil
			v.mu.Unlock()
			if result, err := m.Call(context.Background(), d.Scope, meta.BrowserLeaseID, "click", nil); result != nil {
				t.Fatal("old lease returned result after peer recovered", err)
			} else {
				assertBrowserCode(t, err, ErrLeaseTargetMismatch)
			}
			cleaned, err := m.Release(context.Background(), d.Scope, meta.BrowserLeaseID)
			if err != nil || cleaned.CleanupState != CleanupComplete || !strings.Contains(cleaned.CleanupReason, "page close unconfirmed") {
				t.Fatalf("cleanup after changed peer: %+v %v", cleaned, err)
			}
			if externalCallCount(b, "close_page") != 0 || externalCallCount(b, "stop") != 1 ||
				externalCallCount(b, "take_snapshot") != 1 || externalCallCount(b, "click") != 0 {
				t.Fatalf("changed peer touched another page: %+v", b.calls)
			}
		})
	}
}
