package mcpowneraudit

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type nativeFixture func(context.Context, diagnosticRequest) (nativeEvidence, error)

func (f nativeFixture) nativeSnapshot(ctx context.Context, r diagnosticRequest) (nativeEvidence, error) {
	return f(ctx, r)
}

type pipeFixture func(context.Context, diagnosticRequest) (pipeEvidence, error)

func (f pipeFixture) pipeSnapshot(ctx context.Context, r diagnosticRequest, _ []transportChild) (pipeEvidence, error) {
	return f(ctx, r)
}

type correlationFixture struct {
	binding adapterBinding
	records []managedThread
	native  nativeEvidence
	pipes   pipeEvidence
	rows    []row
	now     time.Time
}

func newCorrelationFixture() correlationFixture {
	rows := []row{fixtureRow(10, 1, "adapter-secret"), fixtureRow(11, 10, "native-secret"), fixtureRow(12, 11, "arbitrary-CANARY-executable")}
	b := adapterBinding{incarnation: 7, adapter: rows[0].id, native: rows[1].id}
	stamp := evidenceStamp{binding: b, provenance: authenticatedAdapterDiagnostic, scope: correlationScope, complete: true}
	pipeStamp := stamp
	pipeStamp.provenance = kernelStdioPipeObservation
	return correlationFixture{b, []managedThread{{"local-CANARY", "thread-CANARY", 7}}, nativeEvidence{stamp, []string{"thread-CANARY"}, []transportChild{{"transport-CANARY", "thread-CANARY", rows[2].id, stdioPipes{21, 22}}}}, pipeEvidence{pipeStamp, []kernelTransport{{rows[2].id, stdioPipes{21, 22}}}}, rows, time.Unix(200, 0)}
}

func (f correlationFixture) run(ctx context.Context) diagnosticCorrelation {
	return correlate(ctx, f.binding, f.records,
		nativeFixture(func(_ context.Context, r diagnosticRequest) (nativeEvidence, error) {
			n := f.native
			n.stamp.challenge, n.stamp.observed = r.challenge, f.now
			return n, nil
		}), pipeFixture(func(_ context.Context, r diagnosticRequest) (pipeEvidence, error) {
			p := f.pipes
			p.stamp.challenge, p.stamp.observed = r.challenge, f.now
			return p, nil
		}), fixtureSource(f.rows), func() time.Time { return f.now })
}

func TestDiagnosticCorrelationRequiresNativeAndKernelEvidence(t *testing.T) {
	f := newCorrelationFixture()
	got := f.run(context.Background())
	if got.Owner != "owner_unverified" || got.Qualification != "verified_diagnostic_correlation" || got.ManagedMappings != 1 || !reflect.DeepEqual(got.CorrelatedPIDs, []int{12}) {
		t.Fatalf("unexpected diagnostic: %+v", got)
	}
	// Persisted mapping is knowledge, not proof that the native thread is loaded.
	f.native.loaded, f.native.children, f.pipes.children = nil, nil, nil
	got = f.run(context.Background())
	if got.Qualification != "not_qualified" || got.ManagedMappings != 1 || len(got.CorrelatedPIDs) != 0 {
		t.Fatal(got)
	}
	// Explicit loaded native threads outside managed records confer no correlation.
	f = newCorrelationFixture()
	f.records[0].thread = "unloaded"
	if got = f.run(context.Background()); got.Qualification != "not_qualified" {
		t.Fatal(got)
	}
}

func TestDiagnosticCorrelationRejectsIncompleteOrConflictingEvidence(t *testing.T) {
	cases := map[string]func(*correlationFixture){
		"missing_provenance":  func(f *correlationFixture) { f.native.stamp.provenance = 0 },
		"forged_provenance":   func(f *correlationFixture) { f.pipes.stamp.provenance = authenticatedAdapterDiagnostic },
		"missing_incarnation": func(f *correlationFixture) { f.binding.incarnation = 0 },
		"generation_mismatch": func(f *correlationFixture) { f.native.stamp.binding.incarnation++ },
		"pipe_generation":     func(f *correlationFixture) { f.pipes.stamp.binding.incarnation++ },
		"record_generation":   func(f *correlationFixture) { f.records[0].incarnation++ },
		"scope":               func(f *correlationFixture) { f.native.stamp.scope = "external" },
		"pipe_scope":          func(f *correlationFixture) { f.pipes.stamp.scope = "external" },
		"partial_loaded":      func(f *correlationFixture) { f.native.stamp.complete = false },
		"partial_pipe":        func(f *correlationFixture) { f.pipes.stamp.complete = false },
		"missing_loaded":      func(f *correlationFixture) { f.native.loaded = nil },
		"missing_pipes":       func(f *correlationFixture) { f.pipes.children = nil },
		"duplicate_local":     func(f *correlationFixture) { f.records = append(f.records, f.records[0]) },
		"conflicting_local": func(f *correlationFixture) {
			f.records = append(f.records, managedThread{f.records[0].local, "other", 7})
		},
		"duplicate_thread_mapping": func(f *correlationFixture) {
			f.records = append(f.records, managedThread{"other", f.records[0].thread, 7})
		},
		"duplicate_loaded": func(f *correlationFixture) { f.native.loaded = append(f.native.loaded, f.native.loaded[0]) },
		"duplicate_child": func(f *correlationFixture) {
			f.native.children = append(f.native.children, f.native.children[0])
			f.pipes.children = append(f.pipes.children, f.pipes.children[0])
		},
		"invalid_id":          func(f *correlationFixture) { f.records[0].thread = "" },
		"oversized_id":        func(f *correlationFixture) { f.records[0].local = strings.Repeat("x", 129) },
		"row_limit":           func(f *correlationFixture) { f.native.loaded = make([]string, maxCorrelationRows+1) },
		"record_limit":        func(f *correlationFixture) { f.records = make([]managedThread, maxCorrelationRows+1) },
		"native_uid_mismatch": func(f *correlationFixture) { f.binding.native.EffectiveUID++ },
		"child_uid_mismatch":  func(f *correlationFixture) { f.native.children[0].child.EffectiveUID++ },
		"native_birth":        func(f *correlationFixture) { f.rows[1].id.StartSec++ },
		"adapter_birth":       func(f *correlationFixture) { f.rows[0].id.StartUsec++ },
		"child_birth":         func(f *correlationFixture) { f.rows[2].id.StartSec++ },
		"reparent":            func(f *correlationFixture) { f.rows[2].id.ParentPID = 10 },
		"missing_child":       func(f *correlationFixture) { f.rows = f.rows[:2] },
		"uid_change":          func(f *correlationFixture) { f.rows[2].id.EffectiveUID++ },
		"pgid_change":         func(f *correlationFixture) { f.rows[2].id.ProcessGroupID++ },
		"forged_pipe_child":   func(f *correlationFixture) { f.pipes.children[0].child.StartSec++ },
		"pipe_mismatch":       func(f *correlationFixture) { f.pipes.children[0].pipes.stdin++ },
		"pipe_alias": func(f *correlationFixture) {
			f.native.children[0].pipes.stdout = 21
			f.pipes.children[0].pipes.stdout = 21
		},
		"missing_pipe_identity": func(f *correlationFixture) { f.native.children[0].pipes.stdin = 0; f.pipes.children[0].pipes.stdin = 0 },
		"partial_process_table": func(f *correlationFixture) { f.rows = append(f.rows, f.rows[2]) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newCorrelationFixture()
			mutate(&f)
			got := f.run(context.Background())
			if got.Qualification != "not_qualified" || got.Owner != "owner_unverified" || got.ManagedMappings != 0 || len(got.CorrelatedPIDs) != 0 {
				t.Fatalf("partial evidence escaped: %+v", got)
			}
		})
	}
	// A second row must not hide conflicts behind the first valid correlation.
	for _, conflict := range []string{"transport", "endpoint"} {
		t.Run(conflict, func(t *testing.T) {
			f := newCorrelationFixture()
			child := fixtureRow(13, 11, "node")
			f.rows = append(f.rows, child)
			c := transportChild{"second", "thread-CANARY", child.id, stdioPipes{23, 24}}
			if conflict == "transport" {
				c.transport = f.native.children[0].transport
			} else {
				c.pipes.stdin = 21
			}
			f.native.children = append(f.native.children, c)
			f.pipes.children = append(f.pipes.children, kernelTransport{c.child, c.pipes})
			if got := f.run(context.Background()); len(got.CorrelatedPIDs) != 0 || got.Qualification != "not_qualified" {
				t.Fatal(got)
			}
		})
	}
}

func TestDiagnosticFreshnessReplayAndUnavailableSources(t *testing.T) {
	for _, name := range []string{"stale", "future", "elapsed", "replay", "pipe_replay", "error", "cancelled", "missing_native", "missing_pipe", "kernel_error"} {
		t.Run(name, func(t *testing.T) {
			f := newCorrelationFixture()
			clock := f.now
			n := nativeFixture(func(_ context.Context, r diagnosticRequest) (nativeEvidence, error) {
				e := f.native
				e.stamp.challenge, e.stamp.observed = r.challenge, clock
				switch name {
				case "stale":
					e.stamp.observed = clock.Add(-time.Nanosecond)
				case "future":
					e.stamp.observed = clock.Add(time.Nanosecond)
				case "elapsed":
					clock = clock.Add(correlationWindow + time.Nanosecond)
				case "replay":
					e.stamp.challenge = [32]byte{}
				case "error":
					return e, errors.New("CANARY token URL")
				}
				return e, nil
			})
			p := pipeFixture(func(_ context.Context, r diagnosticRequest) (pipeEvidence, error) {
				e := f.pipes
				e.stamp.challenge, e.stamp.observed = r.challenge, clock
				if name == "pipe_replay" {
					e.stamp.challenge = [32]byte{}
				}
				return e, nil
			})
			var nativeSource nativeDiagnosticSource = n
			var pipeSource kernelPipeSource = p
			if name == "missing_native" {
				nativeSource = nil
			}
			if name == "missing_pipe" {
				pipeSource = nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if name == "cancelled" {
				cancel()
			}
			kernel := fixtureSource(f.rows)
			if name == "kernel_error" {
				kernel.table = func(context.Context) ([]row, error) { return nil, errors.New("CANARY") }
			}
			got := correlate(ctx, f.binding, f.records, nativeSource, pipeSource, kernel, func() time.Time { return clock })
			if got.Qualification != "not_qualified" || len(got.CorrelatedPIDs) != 0 {
				t.Fatal(got)
			}
		})
	}
	// Replaying an actual prior response fails despite unchanged generation/time.
	f := newCorrelationFixture()
	var saved nativeEvidence
	n := nativeFixture(func(_ context.Context, r diagnosticRequest) (nativeEvidence, error) {
		if saved.stamp.observed.IsZero() {
			saved = f.native
			saved.stamp.challenge, saved.stamp.observed = r.challenge, f.now
		}
		return saved, nil
	})
	p := pipeFixture(func(_ context.Context, r diagnosticRequest) (pipeEvidence, error) {
		e := f.pipes
		e.stamp.challenge, e.stamp.observed = r.challenge, f.now
		return e, nil
	})
	for i := 0; i < 2; i++ {
		got := correlate(context.Background(), f.binding, f.records, n, p, fixtureSource(f.rows), func() time.Time { return f.now })
		if (got.Qualification == "verified_diagnostic_correlation") != (i == 0) {
			t.Fatal(got)
		}
	}
}

func TestDiagnosticCorrelationConcurrentReadOnlyAndRedacted(t *testing.T) {
	f, before := newCorrelationFixture(), newCorrelationFixture()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := f.run(context.Background())
			if got.Qualification != "verified_diagnostic_correlation" {
				t.Error(got)
			}
			b, err := json.Marshal([]any{got, f.binding, f.records, f.native, f.pipes})
			if err != nil {
				t.Error(err)
			}
			for _, forbidden := range []string{"CANARY", "thread", "transport", "executable", "kill", "close", "managed_next"} {
				if strings.Contains(string(b), forbidden) {
					t.Errorf("leaked %s", forbidden)
				}
			}
			got.CorrelatedPIDs[0] = 999 // result cannot alias source inputs
		}()
	}
	wg.Wait()
	if !reflect.DeepEqual(f, before) {
		t.Fatal("read-only contract mutated inputs")
	}
}
