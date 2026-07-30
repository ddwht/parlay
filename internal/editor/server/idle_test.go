// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/idle-timeout-activity-tracking
// parlay-artifact: test

package server

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestIdleTrackerFiresAfterTimeout drives the tracker via a fast wake
// interval and asserts the fire channel receives the documented reason
// string within one full window after no requests.
func TestIdleTrackerFiresAfterTimeout(t *testing.T) {
	tracker := NewIdleTracker(50 * time.Millisecond)
	tracker.checkEvery = 10 * time.Millisecond // tests bypass the production 10s
	// Reset lastSeen to the past so the first tick observes elapsed > timeout.
	tracker.lastSeen = time.Now().Add(-1 * time.Second)

	fire := make(chan string, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go tracker.Run(ctx, fire)

	select {
	case reason := <-fire:
		if !strings.HasPrefix(reason, "idle: no requests for ") {
			t.Fatalf("reason=%q, want prefix %q", reason, "idle: no requests for ")
		}
		if !strings.Contains(reason, "50ms") {
			t.Fatalf("reason=%q, want timeout-string substring", reason)
		}
	case <-ctx.Done():
		t.Fatal("idle tracker did not fire within the test deadline")
	}
}

// TestIdleTrackerTouchDefersFiring asserts a Touch() call resets the
// elapsed-time counter so the tracker does not fire prematurely.
func TestIdleTrackerTouchDefersFiring(t *testing.T) {
	tracker := NewIdleTracker(100 * time.Millisecond)
	tracker.checkEvery = 10 * time.Millisecond

	fire := make(chan string, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	go tracker.Run(ctx, fire)

	// Touch every 20ms so elapsed never exceeds 100ms during the 60ms window.
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
loop:
	for {
		select {
		case <-tick.C:
			tracker.Touch()
		case <-ctx.Done():
			break loop
		case <-fire:
			t.Fatal("tracker fired despite repeated Touch() calls")
		}
	}
}

// TestIdleTrackerZeroTimeoutDoesNotFire asserts the tracker's Run loop
// returns immediately when timeout is zero — Boot is responsible for
// gating goroutine launch, but the safety net in Run keeps the invariant
// even if a caller forgets.
func TestIdleTrackerZeroTimeoutDoesNotFire(t *testing.T) {
	tracker := NewIdleTracker(0)
	fire := make(chan string, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() { tracker.Run(ctx, fire); close(done) }()

	select {
	case <-done:
		// expected: Run returned immediately
	case <-time.After(20 * time.Millisecond):
		t.Fatal("Run did not return for zero timeout")
	}
	select {
	case reason := <-fire:
		t.Fatalf("zero timeout fired reason=%q", reason)
	default:
	}
}

// TestIdleReasonFormat asserts the source file contains the literal
// format-string the buildfile specifies: "idle: no requests for %s".
func TestIdleReasonFormat(t *testing.T) {
	src := readIdleGoSource(t)
	const want = `"idle: no requests for %s"`
	if !strings.Contains(src, want) {
		t.Fatalf("idle.go: missing format string %s", want)
	}
}

// TestIdleSyncPrimitive asserts the tracker uses a synchronisation
// primitive so the goroutine cannot observe a torn read of lastSeen.
func TestIdleSyncPrimitive(t *testing.T) {
	src := readIdleGoSource(t)
	hits := []string{"sync.Mutex", "sync.RWMutex", "atomic.Value", "atomic.Pointer"}
	for _, want := range hits {
		if strings.Contains(src, want) {
			return
		}
	}
	t.Fatalf("idle.go: no synchronisation primitive found; want one of %v", hits)
}

// TestIdleHardCodedInterval asserts the production wake interval is
// 10 * time.Second and the override seam (var newTicker) exists.
func TestIdleHardCodedInterval(t *testing.T) {
	src := readIdleGoSource(t)
	for _, want := range []string{"10 * time.Second", "var newTicker"} {
		if !strings.Contains(src, want) {
			t.Fatalf("idle.go: missing %q", want)
		}
	}
	if strings.Contains(src, "config.IdleCheckInterval") {
		t.Fatal("idle.go: must not consult studio-config for the wake interval")
	}
}

func readIdleGoSource(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller: failed")
	}
	path := filepath.Join(filepath.Dir(file), "idle.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
