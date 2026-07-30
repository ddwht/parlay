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
	"sort"
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

// The subprocess end-to-end suite that used to live here is gone with the
// merge. It exercised invokeStudioSubprocess against a real parlay-studio on
// PATH — exit-code propagation, the launch-failure line, the synthesized 127
// for a start failure. None of those exist now: the editor is a function call,
// so it either returns an error or it does not, and there is no second binary
// to locate, version-check, or fail to start.
//
// What replaced it is a direct call to OpenDomainEditor. The remaining suite
// below still checks the surface contract that survives.

func TestParlayStudioSubcommandContract(t *testing.T) {
	// Derived from the map, not restated. The list used to be hard-coded with
	// all three names, which let it diverge from what Core actually dispatches
	// — and it did: two of the three were dropped because no such surface was
	// ever built, and a hand-kept list would still be asserting them.
	//
	// A note on this test's history, because it is the useful part. It is
	// deliberately fail-hard red rather than skipped (see below), and it was
	// red on main for the right reason: two subcommands did not exist. It went
	// green when parlay-studio learned to answer `--help` before dispatching —
	// which satisfied this test's literal assertion while the subcommands were
	// still missing. Asserting `--help` succeeds is not the same as asserting
	// the subcommand exists, and the gap between those two is where the broken
	// hand-off lived.
	var contractSubcommands []string
	for _, sub := range trioToStudioSubcommand {
		contractSubcommands = append(contractSubcommands, sub)
	}
	sort.Strings(contractSubcommands)
	if len(contractSubcommands) == 0 {
		t.Fatal("no trio subcommands to check — trioToStudioSubcommand is empty")
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
