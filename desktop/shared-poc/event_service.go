package main

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	defaultQueueCapacity = 256
	defaultMaxBatch      = 64
	defaultEventRateHz   = 500
	maxEventRateHz       = 2000
)

type streamSender interface {
	TrySend([]byte) error
}

type EventService struct {
	lifecycleMu   sync.Mutex
	mu            sync.Mutex
	cancel        context.CancelFunc
	done          chan struct{}
	queue         chan SyntheticEvent
	queueCapacity int
	maxBatch      int
	rateHz        int

	streamMu         sync.RWMutex
	stream           streamSender
	streamGeneration uint64

	produced         atomic.Int64
	delivered        atomic.Int64
	queueDropped     atomic.Int64
	transportDropped atomic.Int64
}

func NewEventService() *EventService {
	return newEventServiceWithLimits(defaultQueueCapacity, defaultMaxBatch)
}

func newEventServiceWithLimits(queueCapacity, maxBatch int) *EventService {
	return &EventService{queueCapacity: queueCapacity, maxBatch: maxBatch}
}

func (s *EventService) serveStream(conn *application.StreamConn) {
	generation := s.attachStream(conn)
	defer s.detachStream(generation)
	<-conn.Context().Done()
}

func (s *EventService) attachStream(sender streamSender) uint64 {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	s.streamGeneration++
	s.stream = sender
	return s.streamGeneration
}

func (s *EventService) detachStream(generation uint64) {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	if s.streamGeneration == generation {
		s.stream = nil
	}
}

func (s *EventService) currentStream() streamSender {
	s.streamMu.RLock()
	defer s.streamMu.RUnlock()
	return s.stream
}

func (s *EventService) Start(rateHz, batchIntervalMS int) EventControlResult {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	if rateHz == 0 {
		rateHz = defaultEventRateHz
	}
	if rateHz < 1 || rateHz > maxEventRateHz {
		return EventControlResult{Error: apiError("event_rate_invalid", errors.New("event rate must be between 1 and 2000 Hz"))}
	}
	if batchIntervalMS < minBatchIntervalMS || batchIntervalMS > maxBatchIntervalMS {
		return EventControlResult{Error: apiError("event_interval_invalid", errors.New("batch interval must be between 50 and 1000 milliseconds"))}
	}

	s.stop()

	s.mu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	s.queue = make(chan SyntheticEvent, s.queueCapacity)
	s.rateHz = rateHz
	s.produced.Store(0)
	s.delivered.Store(0)
	s.queueDropped.Store(0)
	s.transportDropped.Store(0)
	queue := s.queue
	done := s.done
	s.mu.Unlock()

	go s.run(ctx, queue, done, rateHz, batchIntervalMS)
	return EventControlResult{Status: s.Status()}
}

func (s *EventService) Stop() EventSourceStatus {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	return s.stop()
}

func (s *EventService) stop() EventSourceStatus {
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	s.cancel = nil
	s.done = nil
	s.queue = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
		if done != nil {
			<-done
		}
	}
	return s.Status()
}

func (s *EventService) ServiceShutdown() error {
	s.Stop()
	return nil
}

func (s *EventService) Status() EventSourceStatus {
	s.mu.Lock()
	running := s.cancel != nil
	queue := s.queue
	rateHz := s.rateHz
	s.mu.Unlock()

	depth := 0
	if queue != nil {
		depth = len(queue)
	}

	queueDropped := s.queueDropped.Load()
	transportDropped := s.transportDropped.Load()
	return EventSourceStatus{
		Running:          running,
		RateHz:           rateHz,
		Produced:         s.produced.Load(),
		Delivered:        s.delivered.Load(),
		Dropped:          queueDropped + transportDropped,
		QueueDropped:     queueDropped,
		TransportDropped: transportDropped,
		QueueDepth:       depth,
		QueueCapacity:    s.queueCapacity,
	}
}

func (s *EventService) run(ctx context.Context, queue chan SyntheticEvent, done chan struct{}, rateHz, batchIntervalMS int) {
	defer close(done)
	producerTicker := time.NewTicker(time.Second / time.Duration(rateHz))
	batchTicker := time.NewTicker(time.Duration(batchIntervalMS) * time.Millisecond)
	defer producerTicker.Stop()
	defer batchTicker.Stop()

	var sequence int64
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-producerTicker.C:
			sequence++
			s.produced.Add(1)
			event := SyntheticEvent{Sequence: sequence, Time: now.UTC().Format(time.RFC3339Nano)}
			select {
			case queue <- event:
			default:
				s.queueDropped.Add(1)
			}
		case <-batchTicker.C:
			batch := drainBatch(queue, s.maxBatch)
			if len(batch) == 0 {
				continue
			}
			s.tryDeliverBatch(batch, len(queue), batchIntervalMS)
		}
	}
}

func (s *EventService) tryDeliverBatch(events []SyntheticEvent, queueDepth, batchIntervalMS int) {
	sender := s.currentStream()
	if sender == nil {
		s.transportDropped.Add(int64(len(events)))
		return
	}

	deliveredAfter := s.delivered.Load() + int64(len(events))
	queueDropped := s.queueDropped.Load()
	transportDropped := s.transportDropped.Load()
	payload := EventBatch{
		Events:           events,
		Produced:         s.produced.Load(),
		Delivered:        deliveredAfter,
		Dropped:          queueDropped + transportDropped,
		QueueDropped:     queueDropped,
		TransportDropped: transportDropped,
		QueueDepth:       queueDepth,
		QueueCapacity:    s.queueCapacity,
		BatchIntervalMS:  batchIntervalMS,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		s.transportDropped.Add(int64(len(events)))
		return
	}
	if err := sender.TrySend(data); err != nil {
		s.transportDropped.Add(int64(len(events)))
		return
	}
	s.delivered.Add(int64(len(events)))
}

func drainBatch(queue chan SyntheticEvent, maxBatch int) []SyntheticEvent {
	batch := make([]SyntheticEvent, 0, maxBatch)
	for len(batch) < maxBatch {
		select {
		case event := <-queue:
			batch = append(batch, event)
		default:
			return batch
		}
	}
	return batch
}
