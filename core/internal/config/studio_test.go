// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: runtime-studio-detection
// parlay-artifact: test

package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeLookPath returns a stub LookPath function that resolves a
// single name to the provided path. Empty path produces ErrNotFound.
func fakeLookPath(name, resolved string) func(string) (string, error) {
	return func(n string) (string, error) {
		if n != name || resolved == "" {
			return "", errors.New("not found")
		}
		return resolved, nil
	}
}

// makeExecutable writes a stub file at path, optionally with the
// executable bit set.
func makeExecutable(t *testing.T, path string, executable bool) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho parlay-studio 1.4.0\n"), 0644); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	if executable {
		if err := os.Chmod(path, 0755); err != nil {
			t.Fatalf("chmod: %v", err)
		}
	}
}

func TestDetectStudio_PathLookupExecutable(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "parlay-studio")
	makeExecutable(t, stub, true)

	d := detectStudio(map[string]string{}, fakeLookPath("parlay-studio", stub))
	if !d.Detected {
		t.Fatalf("expected Detected=true, got %+v", d)
	}
	if d.BinaryPath != stub {
		t.Fatalf("expected BinaryPath=%s, got %q", stub, d.BinaryPath)
	}
	if d.Reason != StudioReasonDetected {
		t.Fatalf("expected Reason=detected, got %q", d.Reason)
	}
}

func TestDetectStudio_AbsentFromPath(t *testing.T) {
	d := detectStudio(map[string]string{}, fakeLookPath("parlay-studio", ""))
	if d.Detected {
		t.Fatalf("expected Detected=false, got %+v", d)
	}
	if d.Reason != StudioReasonAbsentFromPath {
		t.Fatalf("expected Reason=absent-from-path, got %q", d.Reason)
	}
}

func TestDetectStudio_EnvOverrideTakesPrecedenceOverPath(t *testing.T) {
	dir := t.TempDir()
	override := filepath.Join(dir, "parlay-studio-dev")
	makeExecutable(t, override, true)

	pathStub := filepath.Join(dir, "parlay-studio")
	makeExecutable(t, pathStub, true)

	env := map[string]string{"PARLAY_STUDIO": override}
	d := detectStudio(env, fakeLookPath("parlay-studio", pathStub))
	if !d.Detected {
		t.Fatalf("expected Detected=true, got %+v", d)
	}
	if d.BinaryPath != override {
		t.Fatalf("expected env-override path %s, got %q", override, d.BinaryPath)
	}
}

func TestDetectStudio_EnvEmptyStringSuppresses(t *testing.T) {
	dir := t.TempDir()
	pathStub := filepath.Join(dir, "parlay-studio")
	makeExecutable(t, pathStub, true)

	env := map[string]string{"PARLAY_STUDIO": ""}
	d := detectStudio(env, fakeLookPath("parlay-studio", pathStub))
	if d.Detected {
		t.Fatalf("expected Detected=false on empty env, got %+v", d)
	}
	if d.Reason != StudioReasonSuppressedByEnv {
		t.Fatalf("expected Reason=suppressed-by-env, got %q", d.Reason)
	}
}

func TestDetectStudio_NotExecutableIsNotDetected(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "parlay-studio")
	makeExecutable(t, stub, false)

	d := detectStudio(map[string]string{}, fakeLookPath("parlay-studio", stub))
	if d.Detected {
		t.Fatalf("expected Detected=false for non-executable file, got %+v", d)
	}
	if d.Reason != StudioReasonNotExecutable {
		t.Fatalf("expected Reason=not-executable, got %q", d.Reason)
	}
	if d.BinaryPath != stub {
		t.Fatalf("expected BinaryPath=%s for diagnostics, got %q", stub, d.BinaryPath)
	}
}

// makeBlockingExecutable writes an executable stub that never exits on
// its own (it sleeps far longer than any test timeout) — used to
// simulate a hung or misbehaving parlay-studio binary.
//
// The script uses `exec sleep` rather than a plain `sleep` followed by
// another statement. A plain `sleep 300` as a non-final statement forks
// sleep as a child of the shell interpreter; killing the shell (the
// process exec.CommandContext's timeout actually signals) then leaves
// the orphaned `sleep` grandchild running, still holding the stdout
// pipe open — so cmd.Output() would block forever waiting for EOF even
// though the "hung binary" was correctly killed. `exec` replaces the
// shell process with sleep in place (same PID), so there is exactly one
// process to kill and no orphan can survive it.
func makeBlockingExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexec sleep 300\n"), 0755); err != nil {
		t.Fatalf("write blocking stub: %v", err)
	}
}

// TestProbeStudioVersion_TimesOutOnHungBinary confirms probeStudioVersion
// cannot hang forever against a Studio binary that never exits. Before
// this test existed, probeStudioVersion had no timeout at all: a hung
// `parlay-studio --version` blocked every single parlay command that
// resolves a root (PersistentPreRunE calls detectStudioFromOS on every
// invocation once a Studio binary is detected on PATH).
func TestProbeStudioVersion_TimesOutOnHungBinary(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "parlay-studio")
	makeBlockingExecutable(t, stub)

	orig := studioVersionProbeTimeout
	studioVersionProbeTimeout = 100 * time.Millisecond
	defer func() { studioVersionProbeTimeout = orig }()

	start := time.Now()
	version, ok := probeStudioVersion(stub)
	elapsed := time.Since(start)

	if ok {
		t.Errorf("expected probe to fail against a hung binary, got version=%q", version)
	}
	if elapsed > 2*time.Second {
		t.Errorf("probe took %v, want it bounded near the 100ms timeout — it did not time out", elapsed)
	}
}

func TestVersionMismatch(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{"1.4.0", false},
		{"1.0.0", false},
		{"2.5.0", false},
		{"0.9.0", true},
		{"0.1.2", true},
		{"", false},     // empty = unknown, no warning
		{"abc", true},   // unparseable, treat as mismatch
		{"1-rc.1", false},
	}
	for _, tc := range cases {
		got := versionMismatch(tc.version)
		if got != tc.want {
			t.Errorf("versionMismatch(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}

func TestStudioDetectionAccessor(t *testing.T) {
	c := &Context{}
	if got := c.StudioDetection(); got.Detected {
		t.Fatalf("zero-Context StudioDetection should be empty, got %+v", got)
	}
	want := StudioDetection{
		Detected:   true,
		BinaryPath: "/opt/parlay-studio",
		Version:    "1.4.0",
		Reason:     StudioReasonDetected,
	}
	c.SetStudioDetection(want)
	if got := c.StudioDetection(); got != want {
		t.Fatalf("StudioDetection() = %+v, want %+v", got, want)
	}
}

func TestStudioDetectionAccessor_NilContext(t *testing.T) {
	var c *Context
	if got := c.StudioDetection(); got.Detected {
		t.Fatalf("nil-Context StudioDetection should be empty, got %+v", got)
	}
	// SetStudioDetection on nil receiver should not panic.
	c.SetStudioDetection(StudioDetection{Detected: true})
}
