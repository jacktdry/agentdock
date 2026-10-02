package desktopapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	ActivitySchemaVersion   = 1
	MaxActivityPayloadBytes = 256 << 10
)

type ActivityKind string

type ActivityEnvelope struct {
	SchemaVersion int             `json:"schemaVersion"`
	Epoch         string          `json:"epoch"`
	Sequence      string          `json:"sequence"`
	OccurredAt    time.Time       `json:"occurredAt"`
	Kind          ActivityKind    `json:"kind"`
	Source        string          `json:"source"`
	Data          json.RawMessage `json:"data,omitempty"`
}

type ActivityDrain struct {
	Epoch              string             `json:"epoch"`
	Events             []ActivityEnvelope `json:"events"`
	PublishedTotal     uint64             `json:"publishedTotal"`
	SourceDroppedTotal uint64             `json:"sourceDroppedTotal"`
	QueueDepth         int                `json:"queueDepth"`
	QueueCapacity      int                `json:"queueCapacity"`
}

type ActivityBufferStatus struct {
	Epoch              string `json:"epoch"`
	LatestSequence     string `json:"latestSequence"`
	PublishedTotal     uint64 `json:"publishedTotal"`
	SourceDroppedTotal uint64 `json:"sourceDroppedTotal"`
	QueueDepth         int    `json:"queueDepth"`
	QueueCapacity      int    `json:"queueCapacity"`
}

type ActivityBatch struct {
	SchemaVersion         int                `json:"schemaVersion"`
	Epoch                 string             `json:"epoch"`
	FirstSequence         string             `json:"firstSequence"`
	LastSequence          string             `json:"lastSequence"`
	Events                []ActivityEnvelope `json:"events"`
	PublishedTotal        string             `json:"publishedTotal"`
	DeliveredTotal        string             `json:"deliveredTotal"`
	SourceDroppedTotal    string             `json:"sourceDroppedTotal"`
	TransportDroppedTotal string             `json:"transportDroppedTotal"`
	QueueDepth            int                `json:"queueDepth"`
	QueueCapacity         int                `json:"queueCapacity"`
}

type ActivityBuffer struct {
	publishMu     sync.Mutex
	epoch         string
	queue         chan ActivityEnvelope
	nextSequence  atomic.Uint64
	published     atomic.Uint64
	sourceDropped atomic.Uint64
}

func NewActivityBuffer(epoch string, capacity int) *ActivityBuffer {
	if capacity < 1 {
		capacity = 1
	}
	epoch = strings.TrimSpace(epoch)
	if epoch == "" {
		epoch = newActivityEpoch()
	}
	return &ActivityBuffer{
		epoch: epoch,
		queue: make(chan ActivityEnvelope, capacity),
	}
}

func (b *ActivityBuffer) Epoch() string {
	if b == nil {
		return ""
	}
	return b.epoch
}

func (b *ActivityBuffer) Status() ActivityBufferStatus {
	if b == nil {
		return ActivityBufferStatus{}
	}
	return ActivityBufferStatus{
		Epoch:              b.epoch,
		LatestSequence:     strconv.FormatUint(b.nextSequence.Load(), 10),
		PublishedTotal:     b.published.Load(),
		SourceDroppedTotal: b.sourceDropped.Load(),
		QueueDepth:         len(b.queue),
		QueueCapacity:      cap(b.queue),
	}
}

func (b *ActivityBuffer) TryPublish(kind ActivityKind, source string, data json.RawMessage, occurredAt time.Time) (ActivityEnvelope, bool) {
	if b == nil {
		return ActivityEnvelope{}, false
	}
	b.publishMu.Lock()
	defer b.publishMu.Unlock()
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	} else {
		occurredAt = occurredAt.UTC()
	}
	sequence := b.nextSequence.Add(1)
	b.published.Add(1)
	event := ActivityEnvelope{
		SchemaVersion: ActivitySchemaVersion,
		Epoch:         b.epoch,
		Sequence:      strconv.FormatUint(sequence, 10),
		OccurredAt:    occurredAt,
		Kind:          ActivityKind(strings.TrimSpace(string(kind))),
		Source:        strings.TrimSpace(source),
		Data:          append(json.RawMessage(nil), data...),
	}
	if event.Kind == "" ||
		event.Source == "" ||
		len(event.Data) > MaxActivityPayloadBytes ||
		(len(event.Data) > 0 && !json.Valid(event.Data)) {
		b.sourceDropped.Add(1)
		return event, false
	}
	select {
	case b.queue <- event:
		return event, true
	default:
		b.sourceDropped.Add(1)
		return event, false
	}
}

func (b *ActivityBuffer) Drain(maxEvents int) ActivityDrain {
	if b == nil {
		return ActivityDrain{Events: []ActivityEnvelope{}}
	}
	if maxEvents < 1 {
		maxEvents = 1
	}
	events := make([]ActivityEnvelope, 0, maxEvents)
	for len(events) < maxEvents {
		select {
		case event := <-b.queue:
			events = append(events, event)
		default:
			return ActivityDrain{
				Epoch:              b.epoch,
				Events:             events,
				PublishedTotal:     b.published.Load(),
				SourceDroppedTotal: b.sourceDropped.Load(),
				QueueDepth:         len(b.queue),
				QueueCapacity:      cap(b.queue),
			}
		}
	}
	return ActivityDrain{
		Epoch:              b.epoch,
		Events:             events,
		PublishedTotal:     b.published.Load(),
		SourceDroppedTotal: b.sourceDropped.Load(),
		QueueDepth:         len(b.queue),
		QueueCapacity:      cap(b.queue),
	}
}

func NewActivityBatch(drain ActivityDrain, deliveredTotal, transportDroppedTotal uint64) ActivityBatch {
	batch := ActivityBatch{
		SchemaVersion:         ActivitySchemaVersion,
		Epoch:                 drain.Epoch,
		Events:                append([]ActivityEnvelope(nil), drain.Events...),
		PublishedTotal:        strconv.FormatUint(drain.PublishedTotal, 10),
		DeliveredTotal:        strconv.FormatUint(deliveredTotal, 10),
		SourceDroppedTotal:    strconv.FormatUint(drain.SourceDroppedTotal, 10),
		TransportDroppedTotal: strconv.FormatUint(transportDroppedTotal, 10),
		QueueDepth:            drain.QueueDepth,
		QueueCapacity:         drain.QueueCapacity,
	}
	if len(batch.Events) > 0 {
		batch.FirstSequence = batch.Events[0].Sequence
		batch.LastSequence = batch.Events[len(batch.Events)-1].Sequence
	}
	return batch
}

func newActivityEpoch() string {
	var bytes [12]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return hex.EncodeToString(bytes[:])
	}
	return time.Now().UTC().Format("20060102T150405.000000000")
}
