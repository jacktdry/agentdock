package computer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeProvider struct {
	mu          sync.Mutex
	observeCall int
	actCall     int
	active      int
	peak        int
	before      *AppIdentity
	after       *AppIdentity
	block       chan struct{}
}

func (p *fakeProvider) ID() ProviderID { return ProviderOrca }
func (p *fakeProvider) FrontmostApp(context.Context) (*AppIdentity, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.actCall > 0 || p.observeCall > 0 {
		return p.after, nil
	}
	return p.before, nil
}
func (p *fakeProvider) Observe(ctx context.Context, req ObservationRequest) (map[string]any, error) {
	p.mu.Lock()
	p.observeCall++
	p.active++
	if p.active > p.peak {
		p.peak = p.active
	}
	p.mu.Unlock()
	if p.block != nil {
		select {
		case <-p.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	p.mu.Lock()
	p.active--
	p.mu.Unlock()
	return map[string]any{"action": req.Action}, nil
}
func (p *fakeProvider) Act(ctx context.Context, req ActionRequest) (map[string]any, error) {
	p.mu.Lock()
	p.actCall++
	p.active++
	if p.active > p.peak {
		p.peak = p.active
	}
	p.mu.Unlock()
	if p.block != nil {
		select {
		case <-p.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	p.mu.Lock()
	p.active--
	p.mu.Unlock()
	return map[string]any{"verification": map[string]any{"status": "verified"}}, nil
}

func TestForegroundForbiddenNeverInvokesProviderAction(t *testing.T) {
	provider := &fakeProvider{}
	broker, _ := NewBroker(provider)
	meta, err := broker.Acquire(AcquireRequest{Owner: OwnerScope{Kind: OwnerDirect}, Capability: CapabilityAct, ForegroundPolicy: ForegroundForbidden})
	if err != nil {
		t.Fatal(err)
	}
	_, err = broker.ActDirect(context.Background(), meta.SessionID, ActionRequest{Action: "click", App: "Test", ElementIndex: intPtr(1)})
	var computerErr *Error
	if !errors.As(err, &computerErr) || computerErr.Code != ErrForegroundRequired {
		t.Fatalf("err=%v", err)
	}
	provider.mu.Lock()
	calls := provider.actCall
	provider.mu.Unlock()
	if calls != 0 {
		t.Fatalf("provider action calls=%d", calls)
	}
}

func TestRestoreWindowRequiresForegroundBeforeProviderCall(t *testing.T) {
	provider := &fakeProvider{}
	broker, _ := NewBroker(provider)
	meta, _ := broker.Acquire(AcquireRequest{Owner: OwnerScope{Kind: OwnerDirect}, Capability: CapabilityObserve, ForegroundPolicy: ForegroundForbidden})
	_, err := broker.ObserveDirect(context.Background(), meta.SessionID, ObservationRequest{Action: "get_app_state", App: "Test", RestoreWindow: true})
	var computerErr *Error
	if !errors.As(err, &computerErr) || computerErr.Code != ErrForegroundRequired {
		t.Fatalf("err=%v", err)
	}
	provider.mu.Lock()
	calls := provider.observeCall
	provider.mu.Unlock()
	if calls != 0 {
		t.Fatalf("provider observe calls=%d", calls)
	}
}

func TestACPComputerSessionOwnerIsolationAndIdempotentRelease(t *testing.T) {
	provider := &fakeProvider{}
	broker, _ := NewBroker(provider)
	ownerA := OwnerScope{Kind: OwnerACP, OwnerACPSessionID: "acps-a", OwnerProfileID: "codex"}
	ownerB := OwnerScope{Kind: OwnerACP, OwnerACPSessionID: "acps-b", OwnerProfileID: "codex"}
	meta, err := broker.Acquire(AcquireRequest{Owner: ownerA, Capability: CapabilityObserve, ForegroundPolicy: ForegroundForbidden})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := broker.ObserveOwned(context.Background(), meta.SessionID, ownerB, ObservationRequest{Action: "capabilities"}); err == nil {
		t.Fatal("cross-owner observe succeeded")
	} else {
		var computerErr *Error
		if !errors.As(err, &computerErr) || computerErr.Code != ErrOwnerMismatch {
			t.Fatalf("err=%v", err)
		}
	}
	first, err := broker.ReleaseOwned(meta.SessionID, ownerA)
	if err != nil {
		t.Fatal(err)
	}
	second, err := broker.ReleaseOwned(meta.SessionID, ownerA)
	if err != nil {
		t.Fatal(err)
	}
	if first.CleanupState != "complete" || second.CleanupState != "complete" {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
}

func TestObserveCapabilityCannotAct(t *testing.T) {
	provider := &fakeProvider{}
	broker, _ := NewBroker(provider)
	meta, _ := broker.Acquire(AcquireRequest{Owner: OwnerScope{Kind: OwnerDirect}, Capability: CapabilityObserve, ForegroundPolicy: ForegroundAllowed})
	_, err := broker.ActDirect(context.Background(), meta.SessionID, ActionRequest{Action: "click", App: "Test", ElementIndex: intPtr(1)})
	var computerErr *Error
	if !errors.As(err, &computerErr) || computerErr.Code != ErrCapabilityDenied {
		t.Fatalf("err=%v", err)
	}
}

func TestProviderOperationsSerializeGlobally(t *testing.T) {
	provider := &fakeProvider{block: make(chan struct{})}
	broker, _ := NewBroker(provider)
	a, _ := broker.Acquire(AcquireRequest{Owner: OwnerScope{Kind: OwnerDirect}, Capability: CapabilityObserve})
	b, _ := broker.Acquire(AcquireRequest{Owner: OwnerScope{Kind: OwnerDirect}, Capability: CapabilityObserve})
	done := make(chan error, 2)
	go func() {
		_, err := broker.ObserveDirect(context.Background(), a.SessionID, ObservationRequest{Action: "capabilities", Timeout: time.Second})
		done <- err
	}()
	go func() {
		_, err := broker.ObserveDirect(context.Background(), b.SessionID, ObservationRequest{Action: "capabilities", Timeout: time.Second})
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	provider.mu.Lock()
	peakBefore := provider.peak
	provider.mu.Unlock()
	close(provider.block)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	peak := provider.peak
	provider.mu.Unlock()
	if peakBefore > 1 || peak != 1 {
		t.Fatalf("provider peak concurrency=%d before=%d", peak, peakBefore)
	}
}

func intPtr(v int) *int { return &v }

func TestBackgroundObservationReportsFocusViolation(t *testing.T) {
	provider := &fakeProvider{
		before: &AppIdentity{Name: "ChatGPT", BundleID: "com.openai.codex", PID: 1},
		after:  &AppIdentity{Name: "Target", BundleID: "example.target", PID: 2},
	}
	broker, _ := NewBroker(provider)
	meta, _ := broker.Acquire(AcquireRequest{Owner: OwnerScope{Kind: OwnerDirect}, Capability: CapabilityObserve, ForegroundPolicy: ForegroundForbidden})
	_, err := broker.ObserveDirect(context.Background(), meta.SessionID, ObservationRequest{Action: "get_app_state", App: "Target"})
	var computerErr *Error
	if !errors.As(err, &computerErr) || computerErr.Code != ErrFocusViolation {
		t.Fatalf("err=%v", err)
	}
}

func TestPermissionsRequireForegroundBeforeProviderCall(t *testing.T) {
	provider := &fakeProvider{}
	broker, _ := NewBroker(provider)
	meta, _ := broker.Acquire(AcquireRequest{Owner: OwnerScope{Kind: OwnerDirect}, Capability: CapabilityObserve, ForegroundPolicy: ForegroundForbidden})
	_, err := broker.ObserveDirect(context.Background(), meta.SessionID, ObservationRequest{Action: "permissions"})
	var computerErr *Error
	if !errors.As(err, &computerErr) || computerErr.Code != ErrForegroundRequired {
		t.Fatalf("err=%v", err)
	}
	provider.mu.Lock()
	calls := provider.observeCall
	provider.mu.Unlock()
	if calls != 0 {
		t.Fatalf("permissions provider calls=%d", calls)
	}
}
