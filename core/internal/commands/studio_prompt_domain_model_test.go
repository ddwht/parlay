// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: studio-prompt-domain-model
// parlay-artifact: test
//
// External, end-to-end tests that exercise invokeStudioSubprocess
// against a real parlay-studio binary on PATH. The two suites in
// this file split on what they assert about the binary's existence:
//
//   - TestInvokeStudioSubprocess_EndToEnd is a skip-until-binary
//     integration test. It cannot meaningfully run without the
//     binary, so absence is correctly signaled via t.Skip — the
//     skipped count surfaces the still-pending Studio dependency.
//
//   - TestParlayStudioSubcommandContract is a fail-hard red contract
//     test. Absence of parlay-studio from PATH is itself a contract
//     violation while Studio is still pending; the case fails red
//     (not skip) so the dependency surfaces as a louder signal in
//     CI output. The test transitions from red (binary absent) to
//     green (binary present and all three subcommands honor --help)
//     automatically once Studio ships — no test code change needed.

package commands

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// studioProbeTimeout bounds how long a test will wait on a
// parlay-studio subprocess before treating it as a hang. A real
// quick-exit subcommand (e.g. one that honors --help) returns in well
// under a second; anything that blocks past this deadline — notably a
// subcommand that boots the long-running server harness instead of
// exiting — is killed so the test fails/propagates rather than hanging
// the suite indefinitely.
const studioProbeTimeout = 15 * time.Second

// resolveParlayStudioBinary returns the absolute path to a
// parlay-studio binary on PATH, or "" when none is found. Both
// suites in this file gate on the empty-string return — but they
// react differently: the integration test skips, the contract test
// fails red.
func resolveParlayStudioBinary(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("parlay-studio")
	if err != nil {
		return ""
	}
	return path
}

// TestInvokeStudioSubprocess_EndToEnd is the skip-until-binary
// integration test. When parlay-studio is on PATH, it drives the
// real invokeStudioSubprocess against the real binary and checks
// the wait-and-resume contract, exit-code propagation, and the
// failure-line wording on non-zero exit. When the binary is absent
// (the world today, until Studio ships), it skips with a clear
// message — the skipped count is the signal the dependency is still
// pending. Skip is correct here because the body of the test
// genuinely cannot run without the binary; there is no observable
// behavior to assert about its absence beyond what the contract
// test below already covers.
func TestInvokeStudioSubprocess_EndToEnd(t *testing.T) {
	binary := resolveParlayStudioBinary(t)
	if binary == "" {
		t.Skip("parlay-studio not on PATH")
	}

	// Studio is on PATH. Drive the real subprocess via
	// invokeStudioSubprocess for the domain-edit subcommand. The
	// helper passes --root and any feature context; we exercise the
	// full launch-and-wait cycle.
	t.Run("domain-edit propagates exit code and waits", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		// Bound the subprocess: today's parlay-studio boots a blocking
		// server harness for domain-edit and never exits on its own, so
		// an unbounded invocation would hang this suite forever. With a
		// deadline the subprocess is killed and surfaces through the
		// non-zero-exit failure-line contract asserted below.
		ctx, cancel := context.WithTimeout(context.Background(), studioProbeTimeout)
		defer cancel()
		err := invokeStudioSubprocess(invokeStudioOptions{
			BinaryPath: binary,
			Subcommand: "domain-edit",
			ActiveRoot: t.TempDir(),
			FeatureCtx: "",
			Stdin:      strings.NewReader(""),
			Stdout:     stdout,
			Stderr:     stderr,
			Ctx:        ctx,
		})
		// The contract: invokeStudioSubprocess returns nil on a
		// zero exit, returns an error AND prints the failure line
		// on a non-zero exit. Either outcome is acceptable here
		// (the real binary's behavior is its own concern); the
		// test asserts the dispatch surface stays consistent.
		if err != nil {
			// Non-zero exit path: the failure-line wording must
			// appear on stderr verbatim and the error must name
			// the subcommand.
			gotErr := err.Error()
			if !strings.Contains(gotErr, "parlay-studio domain-edit exited with code") {
				t.Errorf("error wording = %q; want a 'parlay-studio domain-edit exited with code N' substring", gotErr)
			}
			gotStderr := stderr.String()
			wantPrefix := "[ERR] parlay-studio domain-edit exited with code"
			wantSuffix := "see Studio's output above. Trio-command artifact completed before Studio launched and is on disk."
			if !strings.Contains(gotStderr, wantPrefix) || !strings.Contains(gotStderr, wantSuffix) {
				t.Errorf("stderr failure line = %q; want prefix %q and suffix %q", gotStderr, wantPrefix, wantSuffix)
			}
		}
	})
}

// TestParlayStudioSubcommandContract is the fail-hard red contract
// test for the three subcommand names hard-coded in
// trioToStudioSubcommand. Each case invokes
// `parlay-studio <subcommand> --help` and asserts a non-error
// exit. When the binary is absent from PATH, every case fails red
// (not skip) — the absence of parlay-studio from PATH is itself a
// contract violation while Studio is still pending, and a yellow
// skipped result would let the contract drift unobserved. The
// test auto-greens the moment Studio ships and is reachable on
// PATH AND honors all three subcommand names; no test code change
// is required when that happens.
//
// Distinguished from TestInvokeStudioSubprocess_EndToEnd above:
// that integration test cannot meaningfully run without the binary,
// so skip is correct there; this contract test asserts a fact about
// the binary's existence, so absence is a fail.
//
// Sits alongside the self-referential
// TestTrioToStudioSubcommandTable in studio_hook_test.go: the
// table test is the static check on the in-process map; this is
// the external-binary contract check. Both are needed.
func TestParlayStudioSubcommandContract(t *testing.T) {
	contractSubcommands := []string{
		"domain-edit",
		"artifacts-review",
		"reconcile",
	}

	binary := resolveParlayStudioBinary(t)

	for _, subcommand := range contractSubcommands {
		subcommand := subcommand
		t.Run(subcommand, func(t *testing.T) {
			if binary == "" {
				// Fail-hard red: the absence of parlay-studio from
				// PATH is itself a contract violation while Studio
				// is still pending. We do NOT t.Skip here — the
				// louder red signal is intentional, because the
				// subcommand-name table inside Core is the single
				// source of truth today and a yellow/skipped test
				// would let the contract drift unobserved.
				t.Errorf("parlay-studio not on PATH — contract for subcommand %q cannot be verified; this is a fail-hard signal that Studio is still pending", subcommand)
				return
			}
			// Bound the probe: the contract is that `--help` exits
			// non-error and quickly. A subcommand that instead blocks
			// (today's parlay-studio boots the server harness rather
			// than honoring --help) must register as an unhonored
			// contract — a red failure — not hang the suite. The
			// deadline turns that block into a clean red.
			ctx, cancel := context.WithTimeout(context.Background(), studioProbeTimeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, subcommand, "--help")
			cmd.Stdin = strings.NewReader("")
			cmd.Stdout = &bytes.Buffer{}
			cmd.Stderr = &bytes.Buffer{}
			err := cmd.Run()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				t.Errorf("parlay-studio %s --help did not exit within %s; the binary is on PATH but blocks (boots the server harness) instead of honoring the %q subcommand (red until Studio ships and recognizes it)", subcommand, studioProbeTimeout, subcommand)
			} else if err != nil {
				t.Errorf("parlay-studio %s --help returned error %v; the binary is on PATH but does not honor the %q subcommand (red until Studio ships and recognizes it)", subcommand, err, subcommand)
			}
		})
	}

	// Documents the dual-mode semantics at the suite level: the
	// test is RED when parlay-studio is absent from PATH and GREEN
	// only when the binary is present and all three subcommands
	// above passed. Maps to the testcases.yaml case
	// "contract test fails red when parlay-studio is absent from
	// PATH" — the asserted state is StudioInvocation.exit-code == 1
	// for the red-when-absent contract.
	t.Run("documents dual-mode semantics", func(t *testing.T) {
		if binary == "" {
			// Red: parlay-studio absent. Equivalent to
			// StudioInvocation.exit-code == 1 in the testcases
			// state model — the contract is unverifiable, which
			// is itself the violation.
			t.Errorf("parlay-studio not on PATH — contract cannot be verified; this is a fail-hard signal that Studio is still pending")
			return
		}
		// Binary present: the per-subcommand cases above ran for
		// real. This case is a no-op marker confirming dual-mode
		// transition green.
	})
}
