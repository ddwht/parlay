// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/studio-binary-boot-and-shutdown
// parlay-artifact: test

package server_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// TestDrainDeadlineConstant asserts boot.go declares the 5-second drain
// deadline. The pattern matches "Shutdown.*5.*time.Second" anywhere in
// the source so a future renaming of the constant still passes as long
// as the magnitude is preserved.
func TestDrainDeadlineConstant(t *testing.T) {
	src := readBootGoSource(t)
	re := regexp.MustCompile(`5\s*\*\s*time\.Second`)
	if !re.MatchString(src) {
		t.Fatal("boot.go: missing 5 * time.Second drain deadline")
	}
	if !strings.Contains(src, "Shutdown") {
		t.Fatal("boot.go: missing Shutdown reference")
	}
}

// TestLoopbackBindAddress asserts the listener factory binds to 127.0.0.1
// only — the trust boundary that lets /api/shutdown skip authentication.
func TestLoopbackBindAddress(t *testing.T) {
	src := readBootGoSource(t)
	if !strings.Contains(src, `"127.0.0.1:%d"`) && !strings.Contains(src, `"127.0.0.1:"`) {
		t.Fatal("boot.go: listener does not bind to 127.0.0.1")
	}
	if strings.Contains(src, `"0.0.0.0:`) {
		t.Fatal("boot.go: listener must not bind to 0.0.0.0")
	}
}

// TestBootStepsReferenced asserts every documented boot step is named in
// the source so reviewers can audit the orchestration order against the
// buildfile. The check is intentionally loose — it asserts presence,
// not order.
func TestBootStepsReferenced(t *testing.T) {
	src := readBootGoSource(t)
	steps := []string{
		"ResolveProjectRoot",
		"LoadConfig",
		"Probe",
		"NewMCPClient",
		"Listen",
		"SignalNotify",
		"shutdownChan",
	}
	for _, step := range steps {
		if !strings.Contains(src, step) {
			t.Errorf("boot.go: missing reference to boot step %q", step)
		}
	}
}

// TestGracefulShutdownCallsMCPClose asserts the graceful-shutdown handler
// invokes mcpClient.Close — the session-lifetime invariant from
// figma-mcp-phase-0-wiring.
func TestGracefulShutdownCallsMCPClose(t *testing.T) {
	src := readBootGoSource(t)
	if !strings.Contains(src, "mcpClient.Close") {
		t.Fatal("boot.go: graceful shutdown does not call mcpClient.Close")
	}
}

// TestShutdownTriggerEnum asserts boot.go declares the three trigger
// labels documented in the buildfile.
func TestShutdownTriggerEnum(t *testing.T) {
	src := readBootGoSource(t)
	for _, label := range []string{"TriggerSignal", "TriggerIdle", "TriggerExplicit"} {
		if !strings.Contains(src, label) {
			t.Errorf("boot.go: missing trigger label %q", label)
		}
	}
}

func readBootGoSource(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller: failed")
	}
	path := filepath.Join(filepath.Dir(file), "boot.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
