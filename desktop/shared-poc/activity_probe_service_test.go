package main

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/desktopapi"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type fakeStreamSender struct {
	err      error
	sends    atomic.Int64
	lastData atomic.Value
}

func (f *fakeStreamSender) TrySend(data []byte) error {
	f.sends.Add(1)
	f.lastData.Store(append([]byte(nil), data...))
	return f.err
}

func TestActivityProbeBoundsSourceQueueAndDeliversContractBatches(t *testing.T) {
	service := newActivityProbeServiceWithLimits(8, 2)
	sender := &fakeStreamSender{}
	generation := service.attachStream(sender)
	t.Cleanup(func() {
		service.Stop()
		service.detachStream(generation)
	})

	result := service.Start(2000, 200)
	if result.Error != nil {
		t.Fatalf("Start() error = %#v", result.Error)
	}
	time.Sleep(350 * time.Millisecond)

	status := service.Status()
	if status.QueueDepth > status.QueueCapacity {
		t.Fatalf("queue depth %d exceeded capacity %d", status.QueueDepth, status.QueueCapacity)
	}
	if status.SourceDroppedTotal == "0" {
		t.Fatalf("expected overload to drop source events, got %#v", status)
	}
	if status.DeliveredTotal == "0" || sender.sends.Load() == 0 {
		t.Fatalf("expected bounded batches to reach stream sender, got %#v sends=%d", status, sender.sends.Load())
	}

	raw, _ := sender.lastData.Load().([]byte)
	var batch desktopapi.ActivityBatch
	if err := json.Unmarshal(raw, &batch); err != nil {
		t.Fatalf("decode ActivityBatch: %v; payload=%q", err, raw)
	}
	if batch.SchemaVersion != desktopapi.ActivitySchemaVersion || batch.Epoch != status.Epoch {
		t.Fatalf("unexpected batch contract = %#v, status=%#v", batch, status)
	}
	if batch.FirstSequence == "" || batch.LastSequence == "" {
		t.Fatalf("activity cursor missing from batch: %#v", batch)
	}
	for _, event := range batch.Events {
		if event.Kind != "probe.tick" || event.Source != "m2.synthetic" {
			t.Fatalf("unexpected activity event = %#v", event)
		}
	}
}

func TestActivityProbeReportsTransportBackpressure(t *testing.T) {
	service := newActivityProbeServiceWithLimits(32, 8)
	sender := &fakeStreamSender{err: application.ErrStreamFull}
	generation := service.attachStream(sender)
	t.Cleanup(func() {
		service.Stop()
		service.detachStream(generation)
	})

	result := service.Start(500, 50)
	if result.Error != nil {
		t.Fatalf("Start() error = %#v", result.Error)
	}
	time.Sleep(180 * time.Millisecond)

	status := service.Status()
	if status.TransportDroppedTotal == "0" {
		t.Fatalf("expected transport backpressure drops, got %#v", status)
	}
	if status.DeliveredTotal != "0" {
		t.Fatalf("expected zero delivered events while stream is full, got %#v", status)
	}
}

func TestActivityProbeReportsDisconnectedTransportDrops(t *testing.T) {
	service := newActivityProbeServiceWithLimits(32, 8)
	t.Cleanup(func() { service.Stop() })

	result := service.Start(500, 50)
	if result.Error != nil {
		t.Fatalf("Start() error = %#v", result.Error)
	}
	time.Sleep(120 * time.Millisecond)

	status := service.Status()
	if status.TransportDroppedTotal == "0" {
		t.Fatalf("expected disconnected transport drops, got %#v", status)
	}
}

func TestActivityProbeOldStreamCleanupDoesNotDetachNewerConnection(t *testing.T) {
	service := NewActivityProbeService()
	first := &fakeStreamSender{}
	second := &fakeStreamSender{}

	firstGeneration := service.attachStream(first)
	secondGeneration := service.attachStream(second)
	service.detachStream(firstGeneration)

	if got := service.currentStream(); got != second {
		t.Fatalf("old stream cleanup detached newer stream: got %#v", got)
	}
	service.detachStream(secondGeneration)
	if got := service.currentStream(); got != nil {
		t.Fatalf("expected stream to be detached, got %#v", got)
	}
}

func TestActivityProbeStopClearsPendingQueueAndPreservesEpochCursor(t *testing.T) {
	service := newActivityProbeServiceWithLimits(64, 8)
	initial := service.Status()
	if initial.Epoch == "" || initial.LatestSequence != "0" {
		t.Fatalf("initial contract = %#v", initial)
	}

	first := service.Start(500, 1000)
	if first.Error != nil {
		t.Fatalf("first Start() error = %#v", first.Error)
	}
	time.Sleep(80 * time.Millisecond)
	stopped := service.Stop()

	if stopped.Running || stopped.QueueDepth != 0 {
		t.Fatalf("stopped status = %#v", stopped)
	}
	if stopped.Epoch != initial.Epoch || stopped.LatestSequence == "0" {
		t.Fatalf("epoch/cursor did not survive source stop: initial=%#v stopped=%#v", initial, stopped)
	}
	if stopped.TransportDroppedTotal == "0" {
		t.Fatalf("pending source queue was not counted as dropped: %#v", stopped)
	}

	second := service.Start(100, 50)
	if second.Error != nil {
		t.Fatalf("second Start() error = %#v", second.Error)
	}
	if second.Status.Epoch != initial.Epoch {
		t.Fatalf("epoch changed across probe restart: %#v", second.Status)
	}
	service.Stop()
}

func TestActivityProbeConcurrentStartStopIsSerialized(t *testing.T) {
	service := newActivityProbeServiceWithLimits(32, 8)
	sender := &fakeStreamSender{}
	generation := service.attachStream(sender)
	t.Cleanup(func() {
		service.Stop()
		service.detachStream(generation)
	})

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 3 {
				result := service.Start(100, 50)
				if result.Error != nil {
					t.Errorf("Start() error = %#v", result.Error)
					return
				}
				time.Sleep(5 * time.Millisecond)
				service.Stop()
			}
		}()
	}
	wg.Wait()

	status := service.Status()
	if status.Running || status.QueueDepth != 0 {
		t.Fatalf("expected clean stopped service after concurrent lifecycle calls, got %#v", status)
	}
}
