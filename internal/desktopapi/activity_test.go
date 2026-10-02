package desktopapi

import (
	"bytes"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"
)

func TestActivityBufferKeepsEpochAndMonotonicSequenceAcrossDrain(t *testing.T) {
	buffer := NewActivityBuffer("epoch-a", 4)
	first, ok := buffer.TryPublish("call.started", "test", json.RawMessage(`{"id":"1"}`), time.Unix(1, 0))
	if !ok {
		t.Fatal("first publish unexpectedly dropped")
	}
	second, ok := buffer.TryPublish("call.completed", "test", nil, time.Unix(2, 0))
	if !ok {
		t.Fatal("second publish unexpectedly dropped")
	}

	drain := buffer.Drain(1)
	if drain.Epoch != "epoch-a" || len(drain.Events) != 1 {
		t.Fatalf("first drain = %#v", drain)
	}
	if drain.Events[0].Sequence != first.Sequence || first.Sequence != "1" {
		t.Fatalf("first sequence = %#v", first)
	}

	third, ok := buffer.TryPublish("call.started", "test", nil, time.Unix(3, 0))
	if !ok {
		t.Fatal("third publish unexpectedly dropped")
	}
	if second.Sequence != "2" || third.Sequence != "3" {
		t.Fatalf("sequences = %s %s %s", first.Sequence, second.Sequence, third.Sequence)
	}

	drain = buffer.Drain(10)
	if len(drain.Events) != 2 ||
		drain.Events[0].Sequence != "2" ||
		drain.Events[1].Sequence != "3" {
		t.Fatalf("second drain = %#v", drain)
	}
}

func TestActivityBufferReportsSourceDropsAndSequenceGap(t *testing.T) {
	buffer := NewActivityBuffer("epoch-b", 2)
	for index := 0; index < 4; index++ {
		buffer.TryPublish("probe", "test", nil, time.Unix(int64(index+1), 0))
	}

	first := buffer.Drain(10)
	if first.PublishedTotal != 4 || first.SourceDroppedTotal != 2 {
		t.Fatalf("first drain metrics = %#v", first)
	}
	if len(first.Events) != 2 || first.Events[0].Sequence != "1" || first.Events[1].Sequence != "2" {
		t.Fatalf("first drain events = %#v", first.Events)
	}

	event, ok := buffer.TryPublish("probe", "test", nil, time.Unix(5, 0))
	if !ok || event.Sequence != "5" {
		t.Fatalf("post-drop event = %#v ok=%v", event, ok)
	}
	second := buffer.Drain(10)
	if len(second.Events) != 1 || second.Events[0].Sequence != "5" {
		t.Fatalf("sequence gap not visible after drop: %#v", second.Events)
	}
}

func TestActivityBufferRejectsInvalidOrOversizedPayloadWithVisibleGap(t *testing.T) {
	buffer := NewActivityBuffer("epoch-invalid", 4)

	invalid, ok := buffer.TryPublish("", "test", nil, time.Unix(1, 0))
	if ok || invalid.Sequence != "1" {
		t.Fatalf("invalid kind = %#v ok=%v", invalid, ok)
	}

	oversizedPayload := make(json.RawMessage, MaxActivityPayloadBytes+1)
	oversizedPayload[0] = '{'
	oversized, ok := buffer.TryPublish("probe", "test", oversizedPayload, time.Unix(2, 0))
	if ok || oversized.Sequence != "2" {
		t.Fatalf("oversized payload = %#v ok=%v", oversized, ok)
	}

	accepted, ok := buffer.TryPublish("probe", "test", json.RawMessage(`{"ok":true}`), time.Unix(3, 0))
	if !ok || accepted.Sequence != "3" {
		t.Fatalf("accepted event = %#v ok=%v", accepted, ok)
	}

	drain := buffer.Drain(10)
	if drain.SourceDroppedTotal != 2 || len(drain.Events) != 1 || drain.Events[0].Sequence != "3" {
		t.Fatalf("invalid payload accounting = %#v", drain)
	}
}

func TestActivityBatchCarriesCumulativeTransportMetrics(t *testing.T) {
	buffer := NewActivityBuffer("epoch-c", 4)
	buffer.TryPublish("probe", "test", nil, time.Unix(1, 0))
	buffer.TryPublish("probe", "test", nil, time.Unix(2, 0))

	batch := NewActivityBatch(buffer.Drain(10), 2, 7)
	if batch.SchemaVersion != ActivitySchemaVersion ||
		batch.Epoch != "epoch-c" ||
		batch.FirstSequence != "1" ||
		batch.LastSequence != "2" ||
		batch.DeliveredTotal != "2" ||
		batch.TransportDroppedTotal != "7" {
		t.Fatalf("batch = %#v", batch)
	}
}

func TestActivityContractFixturePreservesDecimalStringCursor(t *testing.T) {
	data, err := os.ReadFile("testdata/activity_batch_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var batch ActivityBatch
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&batch); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if batch.SchemaVersion != ActivitySchemaVersion ||
		batch.FirstSequence != "9007199254740993" ||
		batch.LastSequence != "9007199254740994" ||
		batch.PublishedTotal != "9007199254740994" ||
		batch.DeliveredTotal != "9007199254740994" ||
		len(batch.Events) != 2 ||
		batch.Events[0].Sequence != "9007199254740993" {
		t.Fatalf("fixture contract = %#v", batch)
	}
}

func TestActivityBufferConcurrentPublishStaysBounded(t *testing.T) {
	buffer := NewActivityBuffer("epoch-d", 32)
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := 0; index < 100; index++ {
				buffer.TryPublish("probe", "test", nil, time.Now())
			}
		}()
	}
	wg.Wait()

	drain := buffer.Drain(1000)
	if len(drain.Events) > 32 || drain.QueueCapacity != 32 || drain.QueueDepth != 0 {
		t.Fatalf("bounded drain = %#v", drain)
	}
	if drain.PublishedTotal != 800 {
		t.Fatalf("published = %d, want 800", drain.PublishedTotal)
	}
	if drain.SourceDroppedTotal+uint64(len(drain.Events)) != drain.PublishedTotal {
		t.Fatalf("drop accounting mismatch: %#v", drain)
	}
}
