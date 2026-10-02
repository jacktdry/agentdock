package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type fakeStreamSender struct {
	err   error
	sends atomic.Int64
}

func (f *fakeStreamSender) TrySend(_ []byte) error {
	f.sends.Add(1)
	return f.err
}

func TestEventServiceBoundsQueueAndReportsDrops(t *testing.T) {
	service := newEventServiceWithLimits(8, 2)
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
	if status.QueueDropped == 0 {
		t.Fatalf("expected overload to drop queued events, got %#v", status)
	}
	if status.Delivered == 0 || sender.sends.Load() == 0 {
		t.Fatalf("expected bounded batches to reach stream sender, got %#v sends=%d", status, sender.sends.Load())
	}
}

func TestEventServiceDropsWhenStreamBackpressures(t *testing.T) {
	service := newEventServiceWithLimits(32, 8)
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
	if status.TransportDropped == 0 {
		t.Fatalf("expected transport backpressure drops, got %#v", status)
	}
	if status.Delivered != 0 {
		t.Fatalf("expected zero delivered events while stream is full, got %#v", status)
	}
}

func TestEventServiceDropsWithoutConnectedStream(t *testing.T) {
	service := newEventServiceWithLimits(32, 8)
	t.Cleanup(func() { service.Stop() })

	result := service.Start(500, 50)
	if result.Error != nil {
		t.Fatalf("Start() error = %#v", result.Error)
	}
	time.Sleep(120 * time.Millisecond)

	status := service.Status()
	if status.TransportDropped == 0 {
		t.Fatalf("expected disconnected transport drops, got %#v", status)
	}
}

func TestStreamGenerationDoesNotDetachNewerConnection(t *testing.T) {
	service := NewEventService()
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

func TestEventServiceConcurrentStartStop(t *testing.T) {
	service := newEventServiceWithLimits(32, 8)
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
	if status.Running {
		t.Fatalf("expected stopped service after concurrent lifecycle calls, got %#v", status)
	}
	if status.QueueDepth != 0 {
		t.Fatalf("expected cleared queue after stop, got %#v", status)
	}
}

func TestResolveRuntimeRootPrefersExplicitValue(t *testing.T) {
	t.Setenv(runtimeRootEnv, "/from-env")
	got, err := resolveRuntimeRoot("/explicit")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/explicit" {
		t.Fatalf("resolveRuntimeRoot() = %q", got)
	}
}
