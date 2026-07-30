// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/blocking-domain-edit-invocation
// parlay-artifact: test

package server

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ddwht/parlay/studio/internal/config"
)

// bootHarness captures the observable outcomes the blocking-domain-edit
// contract cares about: whether the browser was opened and the exact URL it
// landed on.
type bootHarness struct {
	opened    bool
	openedURL string
}

// fakeBootDeps builds a BootDeps wired entirely to fakes so Boot runs without
// touching the real filesystem, config, or a browser. browserPath threads the
// landing-path suffix; idle drives the walk-away guard; signalFire (when set)
// collapses the shutdown channel to unblock Boot on the graceful path.
func fakeBootDeps(t *testing.T, h *bootHarness, browserPath string, idle time.Duration, signalFire bool) BootDeps {
	t.Helper()
	root := t.TempDir()
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open devnull: %v", err)
	}
	t.Cleanup(func() { _ = devnull.Close() })

	return BootDeps{
		Args:   []string{},
		Env:    map[string]string{},
		Stderr: devnull,
		ResolveProjectRoot: func([]string, map[string]string, string, string) (string, config.Source, error) {
			return root, config.SourceDefault, nil
		},
		LoadConfig: func(context.Context, []string, string, map[string]string, config.LoadOptions) (*config.Config, []config.Trace, error) {
			return &config.Config{ServerPort: 0, IdleTimeout: idle, OpenBrowser: true}, nil, nil
		},
		OpenBrowser: func(url string) error {
			h.opened = true
			h.openedURL = url
			return nil
		},
		SignalNotify: func(ch chan<- string) {
			if signalFire {
				go func() { ch <- "signal: TEST" }()
			}
		},
		BrowserPath: browserPath,
	}
}

// runBoot runs Boot in a goroutine and returns its error, failing the test if
// it does not return within a generous budget.
func runBoot(t *testing.T, deps BootDeps) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- Boot(context.Background(), deps) }()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Boot did not return within 5s")
		return nil
	}
}

// TestDomainEditLandsBrowserOnEditorRoute asserts domain-edit's browser-open
// hook receives the bound URL suffixed with /domain-model, while the bare
// invocation receives the root URL.
func TestDomainEditLandsBrowserOnEditorRoute(t *testing.T) {
	// domain-edit: BrowserPath "/domain-model".
	var h bootHarness
	if err := runBoot(t, fakeBootDeps(t, &h, "/domain-model", 0, true)); err != nil {
		t.Fatalf("Boot (domain-edit) returned error: %v", err)
	}
	if !h.opened {
		t.Fatal("domain-edit did not open the browser")
	}
	if !strings.HasSuffix(h.openedURL, "/domain-model") {
		t.Fatalf("domain-edit browser URL = %q, want a /domain-model suffix", h.openedURL)
	}

	// Bare invocation: default BrowserPath, lands on the root.
	var bare bootHarness
	if err := runBoot(t, fakeBootDeps(t, &bare, "", 0, true)); err != nil {
		t.Fatalf("Boot (bare) returned error: %v", err)
	}
	if strings.HasSuffix(bare.openedURL, "/domain-model") {
		t.Fatalf("bare invocation must not land on /domain-model, got %q", bare.openedURL)
	}
	if !strings.HasSuffix(bare.openedURL, "/") {
		t.Fatalf("bare invocation should land on the root path, got %q", bare.openedURL)
	}
}

// TestDomainEditIdleTimeoutExitsZero asserts a domain-edit session with a short
// idle timeout and no requests exits zero (graceful path) — the walk-away guard
// unblocks a waiting caller.
func TestDomainEditIdleTimeoutExitsZero(t *testing.T) {
	// Speed the idle-check tick way up for the duration of this test.
	orig := newTicker
	newTicker = func(time.Duration) *time.Ticker { return time.NewTicker(time.Millisecond) }
	t.Cleanup(func() { newTicker = orig })

	var h bootHarness
	// No signal fire — the only path out is the idle timeout.
	err := runBoot(t, fakeBootDeps(t, &h, "/domain-model", time.Nanosecond, false))
	if err != nil {
		t.Fatalf("idle-driven graceful shutdown should exit zero, got %v", err)
	}
}

// TestDomainEditBootFailureExitsNonZeroBeforeBrowser asserts a boot-step
// failure exits non-zero before any browser opens.
func TestDomainEditBootFailureExitsNonZeroBeforeBrowser(t *testing.T) {
	var h bootHarness
	deps := fakeBootDeps(t, &h, "/domain-model", 0, false)
	// Fail at the listener bind (step 5) — before the browser-open step (7).
	deps.Listen = func(int) (net.Listener, error) {
		return nil, errors.New("studio-boot-listener-failed: bind refused")
	}

	err := runBoot(t, deps)
	if err == nil {
		t.Fatal("expected a non-nil boot error on a listener failure")
	}
	if h.opened {
		t.Fatal("the browser must not open when a boot step fails")
	}
}
