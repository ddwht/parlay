// parlay-section: cross-cutting
// parlay-extends: studio-support/studio-cli-hooks/runtime-studio-detection
//
// Studio detection lives in package config because the resolution
// Context owns the per-process record of whether parlay-studio is on
// PATH and whether its version is compatible. Detection is read-only:
// we stat a candidate binary to confirm the executable bit, then read
// its `parlay-studio --version` output once. We never invoke Studio
// solely to confirm it is there, and a failed version probe never
// changes Detected — Studio remains "detected" for hook purposes
// regardless of version.

package config

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// StudioReason names every classified outcome of detectStudio. The
// string form is what diagnostic surfaces (parlay status, --verbose
// diagnostics) print; downstream code switches on the constant rather
// than the string.
type StudioReason string

const (
	// StudioReasonDetected — the binary was found, the executable bit
	// is set, and (when probed) the version was readable.
	StudioReasonDetected StudioReason = "detected"

	// StudioReasonAbsentFromPath — PATH lookup found nothing AND
	// PARLAY_STUDIO is not set.
	StudioReasonAbsentFromPath StudioReason = "absent-from-path"

	// StudioReasonSuppressedByEnv — PARLAY_STUDIO was set to the empty
	// string, an explicit "do not detect" signal that wins over PATH.
	StudioReasonSuppressedByEnv StudioReason = "suppressed-by-env"

	// StudioReasonNotExecutable — a file was found but its executable
	// bit is unset. Reported as not-detected so callers do not try to
	// invoke an unrunnable binary; --verbose surfaces a one-line
	// diagnostic naming the path.
	StudioReasonNotExecutable StudioReason = "not-executable"

	// StudioReasonVersionUnknown — the binary was located and runnable
	// but the version probe failed (no output, parse error, exit
	// non-zero). Detected stays true; the version field is empty.
	StudioReasonVersionUnknown StudioReason = "version-unknown"
)

// StudioDetection is the per-process record produced once during root
// resolution and consulted by every hook surface afterward. It is the
// single source of truth for "is parlay-studio available?" — no other
// code path re-checks PATH or env.
type StudioDetection struct {
	// Detected is true exactly when the binary is on PATH (or pointed
	// at by PARLAY_STUDIO), is executable, and is intended to run.
	// Version compatibility never changes this flag.
	Detected bool

	// BinaryPath is the resolved absolute path to parlay-studio. Empty
	// when Detected is false unless Reason == not-executable, in which
	// case it points at the file whose executable bit is missing.
	BinaryPath string

	// Version is the version string reported by `parlay-studio --version`.
	// Empty when detection failed or when the version probe could not
	// produce a parseable line.
	Version string

	// Reason classifies the outcome — see the StudioReason constants.
	// Always populated.
	Reason StudioReason
}

// expectedStudioVersionRange is the range Core expects parlay-studio
// to satisfy. Compile-time-encoded; bumped together with the public
// agent surface contract whenever a breaking change ships.
const expectedStudioVersionRange = ">=1.0.0"

// detectStudio is the pure detection routine, parameterized over the
// environment lookup and the PATH probe so unit tests can drive it
// without touching the live filesystem. The order of the gates matters:
//
//  1. PARLAY_STUDIO env var, when set, wins over PATH.
//     - Empty string ("") is an explicit suppression.
//     - Non-empty is the candidate path (still subject to executable bit).
//  2. PATH lookup via lookPath when PARLAY_STUDIO is unset.
//
// detectStudio never invokes Studio. The version probe is layered on
// top by the caller (resolver.go) which uses probeStudioVersion below.
func detectStudio(env map[string]string, lookPath func(string) (string, error)) StudioDetection {
	// Gate 1: PARLAY_STUDIO env var.
	if raw, ok := env["PARLAY_STUDIO"]; ok {
		if raw == "" {
			return StudioDetection{Detected: false, Reason: StudioReasonSuppressedByEnv}
		}
		return classifyStudioCandidate(raw)
	}

	// Gate 2: PATH lookup.
	resolved, err := lookPath("parlay-studio")
	if err != nil || resolved == "" {
		return StudioDetection{Detected: false, Reason: StudioReasonAbsentFromPath}
	}
	return classifyStudioCandidate(resolved)
}

// classifyStudioCandidate stats path and decides whether the file is
// runnable. It reports BinaryPath in both the detected and
// not-executable cases so diagnostics can name the offending file.
func classifyStudioCandidate(path string) StudioDetection {
	abs := path
	if !filepath.IsAbs(path) {
		if a, err := filepath.Abs(path); err == nil {
			abs = a
		}
	}
	info, err := os.Stat(abs)
	if err != nil {
		return StudioDetection{Detected: false, Reason: StudioReasonAbsentFromPath}
	}
	if info.IsDir() {
		return StudioDetection{Detected: false, Reason: StudioReasonAbsentFromPath}
	}
	// Executable bit check — any of user/group/world-x is enough; the
	// kernel will refuse if none are set when we (or our subprocess)
	// later try to exec it.
	if info.Mode().Perm()&0o111 == 0 {
		return StudioDetection{
			Detected:   false,
			BinaryPath: abs,
			Reason:     StudioReasonNotExecutable,
		}
	}
	return StudioDetection{
		Detected:   true,
		BinaryPath: abs,
		Reason:     StudioReasonDetected,
	}
}

// studioVersionProbeTimeout bounds how long probeStudioVersion waits for
// `<binary> --version` to respond. classifyStudioCandidate never invokes
// the binary, but the version probe does — and this runs on every
// command's PersistentPreRunE whenever a Studio binary is detected on
// PATH. A hung or misbehaving Studio binary must never be able to hang
// every parlay invocation. Two seconds is generous for a normal
// --version print and short enough that a timeout is a barely-noticeable
// pause, not a lockup. Var (not const) so tests can shrink it.
var studioVersionProbeTimeout = 2 * time.Second

// probeStudioVersion runs `<binary> --version` under a bounded timeout
// and returns the first line of stdout, trimmed. On any failure (exit
// non-zero, empty output, or the timeout firing and killing the
// process) it returns ("", false) and the caller should treat the
// version as unknown without flipping Detected.
func probeStudioVersion(binaryPath string) (string, bool) {
	if binaryPath == "" {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), studioVersionProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryPath, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if line == "" {
		return "", false
	}
	// Common shapes: "parlay-studio 1.4.0", "parlay-studio v1.4.0",
	// "1.4.0", and — the shape parlay-studio actually emits —
	// "parlay-studio 0.1.2 (commit abc1234)".
	//
	// Take the first field that looks like a version, not the last. Taking
	// the last field worked only for banners with no trailing metadata:
	// against the real banner it selected "abc1234)" as the version, which
	// then failed the range comparison and printed
	//   warning: parlay-studio version abc1234) is older than expected …
	// to stderr on every single parlay invocation — an unparseable token
	// with an unbalanced paren, for the life of the process.
	for _, f := range strings.Fields(line) {
		candidate := strings.TrimPrefix(f, "v")
		if looksLikeVersion(candidate) {
			return candidate, true
		}
	}
	return "", false
}

// looksLikeVersion reports whether s starts with a digit and contains only
// characters legal in a semver-ish token. Deliberately permissive about
// pre-release suffixes (1.2.0-rc.1) and strict about the first character,
// which is what separates "0.1.2" from "parlay-studio" and from "(commit".
func looksLikeVersion(s string) bool {
	if s == "" || s[0] < '0' || s[0] > '9' {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9',
			r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r == '.', r == '-', r == '+':
		default:
			return false
		}
	}
	return true
}

// detectStudioFromOS is the live-filesystem variant used by
// ResolveActiveRoot. It snapshots the relevant env keys and delegates
// to detectStudio, then layers the version probe on top of a
// successful detection.
func detectStudioFromOS() StudioDetection {
	env := map[string]string{}
	if v, ok := os.LookupEnv("PARLAY_STUDIO"); ok {
		env["PARLAY_STUDIO"] = v
	}
	d := detectStudio(env, exec.LookPath)
	if d.Detected {
		if v, ok := probeStudioVersion(d.BinaryPath); ok {
			d.Version = v
		} else {
			// Version probe failed but the binary is runnable — keep
			// Detected true and record the reason for diagnostic
			// surfaces.
			d.Reason = StudioReasonVersionUnknown
		}
	}
	return d
}

// versionMismatch reports whether v sits outside expectedStudioVersionRange.
// Today the range is the single rule ">=1.0.0", so any version that
// parses to a major < 1 is a mismatch. An unparseable version is
// reported as a mismatch — diagnostics should surface a warning rather
// than silently treating an unknown version as compatible.
func versionMismatch(v string) bool {
	if v == "" {
		// Empty version is "version-unknown"; we do not warn for
		// version-unknown — the not-executable / no-output case is its
		// own diagnostic and a mismatch warning would be misleading.
		return false
	}
	parts := strings.SplitN(v, ".", 2)
	if len(parts) == 0 {
		return true
	}
	major := parts[0]
	// Strip any pre-release suffix (e.g. "1-rc.1").
	if i := strings.IndexAny(major, "-+"); i >= 0 {
		major = major[:i]
	}
	if major == "" {
		return true
	}
	if major[0] < '0' || major[0] > '9' {
		return true
	}
	// Compare lexicographically against "1" — works for single-digit
	// majors which is all the range cares about today.
	return major < "1"
}

// EmitStudioVersionWarningOnce prints the version-mismatch warning to
// stderr at most once per process, regardless of how many call sites
// consult StudioDetection. The guard is sync.Once so concurrent
// invocations from cobra subcommands collapse to a single line.
//
// Exported so the cobra entry layer (package commands) can invoke it
// from PersistentPreRunE without re-implementing the once-guard.
var versionWarningOnce sync.Once

func EmitStudioVersionWarningOnce(d StudioDetection) {
	if !d.Detected || d.Version == "" {
		return
	}
	if !versionMismatch(d.Version) {
		return
	}
	versionWarningOnce.Do(func() {
		fmt.Fprintf(os.Stderr,
			"warning: parlay-studio version %s is older than expected (need %s); some hooks may not work.\n",
			d.Version, expectedStudioVersionRange)
	})
}

// ResetStudioVersionWarningForTest is a test-only escape hatch that
// rearms the once-guard so two test cases in the same process can each
// observe the warning. It deliberately lives next to the production
// helper so the once-guard's existence is documented at the same
// reading site.
func ResetStudioVersionWarningForTest() {
	versionWarningOnce = sync.Once{}
}

// StudioDetection accessor on Context — the single read path that
// trio-command handlers and `parlay status` consult. Returns the
// zero-valued StudioDetection (Detected: false, Reason: "") when no
// detection has been recorded yet, which matches the no-Studio case
// from a downstream perspective (reason will be empty rather than
// absent-from-path, but Detected: false short-circuits both gates).
func (c *Context) StudioDetection() StudioDetection {
	if c == nil {
		return StudioDetection{}
	}
	return c.studioDetection
}

// SetStudioDetection installs a detection record on the Context. Used
// by the resolver during Context construction and by tests that drive
// hook surfaces directly without going through ResolveActiveRoot.
func (c *Context) SetStudioDetection(d StudioDetection) {
	if c == nil {
		return
	}
	c.studioDetection = d
}
