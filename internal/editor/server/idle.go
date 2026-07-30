// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/idle-timeout-activity-tracking

package server

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// idleCheckInterval is the production wake-tick for the idle goroutine.
// The interval is hard-coded — studio-config does NOT expose it. Tests
// swap the ticker constructor (var newTicker, below) when they need a
// faster rhythm.
const idleCheckInterval = 10 * time.Second

// newTicker is a test-only override seam. Production callers leave it at
// the time.NewTicker default; tests replace it with a controllable fake.
var newTicker = time.NewTicker

// IdleTracker observes /api/* requests (excluding /api/health) and fires
// a shutdown trigger when the elapsed time since the last observed request
// exceeds the configured timeout.
//
// Touch and Run both acquire the tracker mutex so a torn read of lastSeen
// is impossible.
type IdleTracker struct {
	mu        sync.Mutex
	lastSeen  time.Time
	timeout   time.Duration
	checkEvery time.Duration
}

// NewIdleTracker constructs a tracker with the configured timeout. The
// lastSeen field is initialised to time.Now() so the first idle-check
// fires at least one full timeout window after construction — a
// freshly-started harness with zero requests cannot trigger an early
// shutdown.
func NewIdleTracker(timeout time.Duration) *IdleTracker {
	return &IdleTracker{
		lastSeen:   time.Now(),
		timeout:    timeout,
		checkEvery: idleCheckInterval,
	}
}

// Touch advances the lastSeen timestamp. Called by the IdleTimeoutReset
// middleware on every qualifying /api/* request.
func (t *IdleTracker) Touch() {
	t.mu.Lock()
	t.lastSeen = time.Now()
	t.mu.Unlock()
}

// Run is the idle-check goroutine entry point. The goroutine wakes every
// checkEvery interval, computes (now - lastSeen), and sends the formatted
// reason string onto fire when the elapsed time exceeds the configured
// timeout. The goroutine exits on context cancellation or after firing.
//
// Reason string format (exact):
//
//	"idle: no requests for %s"
//
// where %s is the timeout formatted via time.Duration.String() (e.g.
// "30m0s", "100ms"). The format is documented in the buildfile and
// asserted by the idle-timeout suite.
func (t *IdleTracker) Run(ctx context.Context, fire chan<- string) {
	if t.timeout <= 0 {
		return
	}
	ticker := newTicker(t.checkEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.mu.Lock()
			elapsed := time.Since(t.lastSeen)
			timeout := t.timeout
			t.mu.Unlock()
			if elapsed >= timeout {
				reason := fmt.Sprintf("idle: no requests for %s", timeout.String())
				select {
				case fire <- reason:
				default:
				}
				return
			}
		}
	}
}
