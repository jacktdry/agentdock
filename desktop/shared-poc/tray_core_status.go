package main

import (
	"context"
	"sync"
	"time"
)

// coreTrayReading is deliberately narrower than the Runtime API result: it
// contains no runtime paths, credentials, or speculative connector health.
type coreTrayReading struct {
	running bool
	healthy bool
	err     bool
}

// coreTrayProbe performs no polling. Only an explicit menu click starts a
// bounded, Next-owned Core observation; the one-shot timer only marks it stale.
type coreTrayProbe struct {
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	closed   bool
	checking bool
	sequence uint64
	expiry   *time.Timer

	labels     nativeMenuLabels
	read       func(context.Context) coreTrayReading
	dispatch   func(func())
	publish    func(string)
	now        func() time.Time
	staleAfter time.Duration
}

func newCoreTrayProbe(labels nativeMenuLabels, read func(context.Context) coreTrayReading, dispatch func(func()), publish func(string)) *coreTrayProbe {
	ctx, cancel := context.WithCancel(context.Background())
	return &coreTrayProbe{
		ctx: ctx, cancel: cancel,
		labels: labels, read: read, dispatch: dispatch, publish: publish,
		now: time.Now, staleAfter: 90 * time.Second,
	}
}

// enqueue is synchronized with Close to ensure no UI callback is submitted
// after the owner has started quitting. Executed callbacks also check closed.
func (p *coreTrayProbe) enqueue(fn func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		p.dispatch(fn)
	}
}

func (p *coreTrayProbe) Refresh() {
	p.mu.Lock()
	if p.closed || p.checking {
		p.mu.Unlock()
		return
	}
	p.checking = true
	p.sequence++
	seq := p.sequence
	if p.expiry != nil {
		p.expiry.Stop()
		p.expiry = nil
	}
	p.mu.Unlock()

	p.enqueue(func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if !p.closed && seq == p.sequence {
			p.publish(p.labels.trayChecking)
		}
	})

	go func() {
		ctx, cancel := context.WithTimeout(p.ctx, 6*time.Second)
		defer cancel()
		value := p.read(ctx)
		if ctx.Err() != nil {
			value.err = true
		}
		observed := p.now()
		p.enqueue(func() {
			p.mu.Lock()
			defer p.mu.Unlock()
			if p.closed || seq != p.sequence {
				return
			}
			p.checking = false
			p.publish(p.labels.trayCoreLabel(value, observed))
			p.expiry = time.AfterFunc(p.staleAfter, func() {
				p.enqueue(func() {
					p.mu.Lock()
					defer p.mu.Unlock()
					if !p.closed && seq == p.sequence && !p.checking {
						p.publish(p.labels.trayExpired)
					}
				})
			})
		})
	}()
}

func (p *coreTrayProbe) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	p.cancel()
	if p.expiry != nil {
		p.expiry.Stop()
		p.expiry = nil
	}
}
