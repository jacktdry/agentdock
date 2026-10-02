package main

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uvwt/agentdock/internal/desktopapi"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	defaultActivityQueueCapacity = 256
	defaultActivityMaxBatch      = 64
	defaultActivityRateHz        = 500
	maxActivityRateHz            = 2000
)

type streamSender interface {
	TrySend([]byte) error
}

type ActivityProbeService struct {
	lifecycleMu sync.Mutex
	mu          sync.Mutex
	cancel      context.CancelFunc
	done        chan struct{}
	rateHz      int
	buffer      *desktopapi.ActivityBuffer
	maxBatch    int

	streamMu         sync.RWMutex
	stream           streamSender
	streamGeneration uint64

	delivered        atomic.Uint64
	transportDropped atomic.Uint64
}

func NewActivityProbeService() *ActivityProbeService {
	return newActivityProbeServiceWithLimits(defaultActivityQueueCapacity, defaultActivityMaxBatch)
}

func newActivityProbeServiceWithLimits(queueCapacity, maxBatch int) *ActivityProbeService {
	if maxBatch < 1 {
		maxBatch = 1
	}
	return &ActivityProbeService{
		buffer:   desktopapi.NewActivityBuffer("", queueCapacity),
		maxBatch: maxBatch,
	}
}

func (s *ActivityProbeService) serveStream(conn *application.StreamConn) {
	generation := s.attachStream(conn)
	defer s.detachStream(generation)
	<-conn.Context().Done()
}

func (s *ActivityProbeService) attachStream(sender streamSender) uint64 {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	s.streamGeneration++
	s.stream = sender
	return s.streamGeneration
}

func (s *ActivityProbeService) detachStream(generation uint64) {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	if s.streamGeneration == generation {
		s.stream = nil
	}
}

func (s *ActivityProbeService) currentStream() streamSender {
	s.streamMu.RLock()
	defer s.streamMu.RUnlock()
	return s.stream
}

func (s *ActivityProbeService) Start(rateHz, batchIntervalMS int) ActivityProbeControlResult {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	if rateHz == 0 {
		rateHz = defaultActivityRateHz
	}
	if rateHz < 1 || rateHz > maxActivityRateHz {
		return ActivityProbeControlResult{
			Status: s.Status(),
			Error: desktopapi.NewError(
				"activity_probe_rate_invalid",
				"activity probe rate must be between 1 and 2000 Hz",
				desktopapi.ErrorCategoryValidation,
				false,
				nil,
			),
		}
	}
	if batchIntervalMS < minBatchIntervalMS || batchIntervalMS > maxBatchIntervalMS {
		return ActivityProbeControlResult{
			Status: s.Status(),
			Error: desktopapi.NewError(
				"activity_probe_interval_invalid",
				"activity probe batch interval must be between 50 and 1000 milliseconds",
				desktopapi.ErrorCategoryValidation,
				false,
				nil,
			),
		}
	}

	s.stop()

	s.mu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	s.rateHz = rateHz
	done := s.done
	s.mu.Unlock()

	go s.run(ctx, done, rateHz, batchIntervalMS)
	return ActivityProbeControlResult{Status: s.Status()}
}

func (s *ActivityProbeService) Stop() ActivityProbeStatus {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	return s.stop()
}

func (s *ActivityProbeService) stop() ActivityProbeStatus {
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	s.cancel = nil
	s.done = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
		if done != nil {
			<-done
		}
	}
	metrics := s.buffer.Status()
	if metrics.QueueDepth > 0 {
		drain := s.buffer.Drain(metrics.QueueDepth)
		s.transportDropped.Add(uint64(len(drain.Events)))
	}
	return s.Status()
}

func (s *ActivityProbeService) ServiceShutdown() error {
	s.Stop()
	return nil
}

func (s *ActivityProbeService) Status() ActivityProbeStatus {
	s.mu.Lock()
	running := s.cancel != nil
	rateHz := s.rateHz
	s.mu.Unlock()

	metrics := s.buffer.Status()
	delivered := s.delivered.Load()
	transportDropped := s.transportDropped.Load()
	return ActivityProbeStatus{
		Running:               running,
		RateHz:                rateHz,
		Epoch:                 metrics.Epoch,
		LatestSequence:        metrics.LatestSequence,
		PublishedTotal:        strconv.FormatUint(metrics.PublishedTotal, 10),
		DeliveredTotal:        strconv.FormatUint(delivered, 10),
		DroppedTotal:          strconv.FormatUint(metrics.SourceDroppedTotal+transportDropped, 10),
		SourceDroppedTotal:    strconv.FormatUint(metrics.SourceDroppedTotal, 10),
		TransportDroppedTotal: strconv.FormatUint(transportDropped, 10),
		QueueDepth:            metrics.QueueDepth,
		QueueCapacity:         metrics.QueueCapacity,
	}
}

func (s *ActivityProbeService) run(ctx context.Context, done chan struct{}, rateHz, batchIntervalMS int) {
	defer close(done)
	producerTicker := time.NewTicker(time.Second / time.Duration(rateHz))
	batchTicker := time.NewTicker(time.Duration(batchIntervalMS) * time.Millisecond)
	defer producerTicker.Stop()
	defer batchTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-producerTicker.C:
			s.buffer.TryPublish(desktopapi.ActivityKind("probe.tick"), "m2.synthetic", nil, now)
		case <-batchTicker.C:
			drain := s.buffer.Drain(s.maxBatch)
			if len(drain.Events) == 0 {
				continue
			}
			s.tryDeliverBatch(drain)
		}
	}
}

func (s *ActivityProbeService) tryDeliverBatch(drain desktopapi.ActivityDrain) {
	sender := s.currentStream()
	if sender == nil {
		s.transportDropped.Add(uint64(len(drain.Events)))
		return
	}

	projectedDelivered := s.delivered.Load() + uint64(len(drain.Events))
	batch := desktopapi.NewActivityBatch(drain, projectedDelivered, s.transportDropped.Load())
	data, err := json.Marshal(batch)
	if err != nil {
		s.transportDropped.Add(uint64(len(drain.Events)))
		return
	}
	if err := sender.TrySend(data); err != nil {
		s.transportDropped.Add(uint64(len(drain.Events)))
		return
	}
	s.delivered.Add(uint64(len(drain.Events)))
}
