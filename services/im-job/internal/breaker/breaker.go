package breaker

import (
	"sync"
	"time"
)

// Simple circuit breaker per key (e.g. cometAddr).
// - When failures reach Threshold within Window, breaker opens for OpenFor duration.
// - On success, failure counter resets.
type Breaker struct {
	mu        sync.Mutex
	threshold int
	window    time.Duration
	openFor   time.Duration

	state map[string]*st
}

type st struct {
	failCount int
	firstFail time.Time
	openUntil time.Time
}

type Options struct {
	Threshold int
	Window    time.Duration
	OpenFor   time.Duration
}

func New(opt Options) *Breaker {
	if opt.Threshold <= 0 {
		opt.Threshold = 5
	}
	if opt.Window <= 0 {
		opt.Window = 10 * time.Second
	}
	if opt.OpenFor <= 0 {
		opt.OpenFor = 5 * time.Second
	}
	return &Breaker{
		threshold: opt.Threshold,
		window:    opt.Window,
		openFor:   opt.OpenFor,
		state:     make(map[string]*st),
	}
}

func (b *Breaker) Allow(key string) bool {
	now := time.Now()
	b.mu.Lock()
	defer b.mu.Unlock()

	s, ok := b.state[key]
	if !ok {
		return true
	}
	if !s.openUntil.IsZero() && now.Before(s.openUntil) {
		return false
	}
	return true
}

func (b *Breaker) Success(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.state, key)
}

func (b *Breaker) Failure(key string) (opened bool) {
	now := time.Now()
	b.mu.Lock()
	defer b.mu.Unlock()

	s, ok := b.state[key]
	if !ok {
		s = &st{failCount: 1, firstFail: now}
		b.state[key] = s
		return false
	}

	// If window expired, reset counter
	if !s.firstFail.IsZero() && now.Sub(s.firstFail) > b.window {
		s.failCount = 1
		s.firstFail = now
		s.openUntil = time.Time{}
		return false
	}

	s.failCount++
	if s.failCount >= b.threshold {
		s.openUntil = now.Add(b.openFor)
		return true
	}
	return false
}
