package ratelimit

import (
	"sync"
	"time"
)

// Limiter implements a token bucket rate limiter.
// A zero-value rate (<=0) means no limit is enforced.
type Limiter struct {
	mu       sync.Mutex
	rate     float64   // tokens per second
	burst    float64   // max tokens (== rate, i.e. 1 second worth)
	tokens   float64   // current tokens
	lastTime time.Time // last refill time
}

// NewLimiter creates a rate limiter that allows rate events per second.
// If rate <= 0, Allow always returns true (no limit).
func NewLimiter(rate int) *Limiter {
	r := float64(rate)
	return &Limiter{
		rate:     r,
		burst:    r,
		tokens:   r,
		lastTime: time.Now(),
	}
}

// Allow reports whether one event may happen now.
func (l *Limiter) Allow() bool {
	if l.rate <= 0 {
		return true
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(l.lastTime).Seconds()
	l.lastTime = now

	l.tokens += elapsed * l.rate
	if l.tokens > l.burst {
		l.tokens = l.burst
	}

	if l.tokens < 1 {
		return false
	}

	l.tokens--
	return true
}
