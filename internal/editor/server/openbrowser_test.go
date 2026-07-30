// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/studio-binary-boot-and-shutdown
// parlay-artifact: test
package server

import (
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// TestApplyBootDefaultsInstallsARealBrowserLauncher is the regression that was
// missed for the entire life of the feature.
//
// BootDeps.OpenBrowser was declared, called from Boot, and defaulted to
// `func(string) error { return nil }`. Nothing in production ever assigned it
// — the only assignments in the repo were in this package's own tests — so
// open_browser resolved to true, boot took the branch, and called a function
// that did nothing. --no-browser and its absence were behaviourally
// identical, and the whole documented "run with --no-browser for scripted
// checks" convention was vacuous because there was no other mode.
//
// Asserting "not nil" would have passed against the no-op, so this asserts
// the default is the real launcher.
func TestApplyBootDefaultsInstallsARealBrowserLauncher(t *testing.T) {
	deps := applyBootDefaults(BootDeps{})

	if deps.OpenBrowser == nil {
		t.Fatal("OpenBrowser default is nil")
	}
	got := runtime.FuncForPC(reflect.ValueOf(deps.OpenBrowser).Pointer()).Name()
	if !strings.HasSuffix(got, ".openBrowser") {
		t.Errorf("OpenBrowser default = %s, want the real launcher (server.openBrowser). "+
			"A no-op default is what made --no-browser indistinguishable from its absence.", got)
	}
}

// The mapping is split out from openBrowser so it can be asserted without
// executing anything.
func TestBrowserCommandCoversThisPlatform(t *testing.T) {
	name, args := browserCommand("http://127.0.0.1:1234/domain-model")
	if name == "" {
		t.Fatalf("no browser command for GOOS=%s", runtime.GOOS)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "http://127.0.0.1:1234/domain-model") {
		t.Errorf("args %v do not carry the url", args)
	}

	switch runtime.GOOS {
	case "darwin":
		if name != "open" {
			t.Errorf("darwin launcher = %q, want open", name)
		}
	case "windows":
		if name != "rundll32" {
			t.Errorf("windows launcher = %q, want rundll32", name)
		}
	default:
		if name != "xdg-open" {
			t.Errorf("%s launcher = %q, want xdg-open", runtime.GOOS, name)
		}
	}
}
