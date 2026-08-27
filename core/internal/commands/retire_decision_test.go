package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func retireRun(t *testing.T, args ...string) error {
	t.Helper()
	cmd := testCommandWithContext(t, testContext(t))
	cmd.Args = cobra.ExactArgs(1)
	return runRetireDecision(cmd, args)
}

func recordAWeakDecision(t *testing.T, suite, caseName string) {
	t.Helper()
	recordExceptionKind = "state-only"
	recordExceptionRef = "@demo/operation:store.put"
	recordExceptionText = "the write is durable"
	recordExceptionReason = "state is the only honest observation for a write"
	recordExceptionBy = "interactive decision"
	recordExceptionSuite, recordExceptionCase = suite, caseName
	if err := recordExceptionRun(t, "demo"); err != nil {
		t.Fatalf("recording the decision: %v", err)
	}
}

// The whole point of retiring: a decision reported stale or orphaned must be
// repairable through the CLI. Without this, the only route is hand-editing the
// ledger — the unreviewable change the ledger exists to prevent.
func TestRetireDecision_RepairsAStaleDecision(t *testing.T) {
	dir := recordExceptionFixture(t, weakCase)
	resetFlagsAfterTest(t, recordExceptionCmd.Flags())
	resetFlagsAfterTest(t, retireDecisionCmd.Flags())
	recordAWeakDecision(t, "Store honesty", "writes land")

	// The case is rewritten, so the approval goes stale.
	drifted := strings.Replace(weakCase, `steps: ["put a key", "read the store"]`, `steps: ["check a counter"]`, 1)
	if drifted == weakCase {
		t.Fatal("fixture mutation did not apply")
	}
	tcPath := filepath.Join(dir, ".parlay", "build", "demo", "testcases.yaml")
	if err := os.WriteFile(tcPath, []byte(drifted), 0o644); err != nil {
		t.Fatal(err)
	}

	retireDecisionRef = "@demo/operation:store.put"
	retireDecisionText = "the write is durable"
	retireDecisionSuite, retireDecisionCase = "Store honesty", "writes land"
	retireDecisionReason = "the case now observes a counter, which is a different judgment"
	retireDecisionBy = "interactive decision"
	if err := retireRun(t, "demo"); err != nil {
		t.Fatalf("retiring the stale decision: %v", err)
	}

	ledger, err := os.ReadFile(filepath.Join(dir, ".parlay", "build", "demo", "coverage-exceptions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(ledger)
	if !strings.Contains(got, "retired_decisions") {
		t.Fatalf("the retirement must be recorded, not the entry deleted:\n%s", got)
	}
	// Both halves of the story must survive: what was originally decided, and
	// the decision to withdraw it.
	for _, want := range []string{
		"state is the only honest observation for a write",
		"the case now observes a counter",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("the ledger must keep %q:\n%s", want, got)
		}
	}

	// And a fresh decision against the case as it now stands must be accepted.
	recordAWeakDecision(t, "Store honesty", "writes land")
	if out := unapprovedDowngrades(tcPath, []byte(drifted), decisionsFrom(t, dir)); len(out) != 0 {
		t.Fatalf("after retiring and re-recording, the case must be clean; got:\n%s", strings.Join(out, "\n"))
	}
}

func decisionsFrom(t *testing.T, dir string) []DowngradeDecision {
	t.Helper()
	rec, err := loadCoverageExceptions(testContext(t), "demo")
	if err != nil {
		t.Fatal(err)
	}
	var out []DowngradeDecision
	for _, ex := range rec.Exceptions {
		if ex.Kind == ExceptionStateOnly {
			out = append(out, DowngradeDecision{
				Ref: ex.Ref, Text: ex.Text, Suite: ex.Suite,
				Case: ex.Case, CaseHash: ex.CaseHash,
			})
		}
	}
	return out
}

// Retiring one downgrade must not take a sibling that shares the criterion.
func TestRetireDecision_DoesNotTakeASibling(t *testing.T) {
	two := weakCase + `      - name: deletes land
        coverage: state-only
        criterion: {ref: "@demo/operation:store.put", text: "the write is durable"}
        steps: ["delete a key", "read the store"]
`
	recordExceptionFixture(t, two)
	resetFlagsAfterTest(t, recordExceptionCmd.Flags())
	resetFlagsAfterTest(t, retireDecisionCmd.Flags())
	recordAWeakDecision(t, "Store honesty", "writes land")
	recordAWeakDecision(t, "Store honesty", "deletes land")

	retireDecisionRef = "@demo/operation:store.put"
	retireDecisionText = "the write is durable"
	// Retire the SECOND decision deliberately. Retiring the first would pass
	// even if the case selector were ignored entirely, since the code takes
	// the first match — the assertion would witness nothing.
	retireDecisionSuite, retireDecisionCase = "Store honesty", "deletes land"
	retireDecisionReason = "no longer the honest answer here"
	retireDecisionBy = "interactive decision"
	if err := retireRun(t, "demo"); err != nil {
		t.Fatal(err)
	}

	left := decisionsFrom(t, t.TempDir())
	if len(left) != 1 {
		t.Fatalf("exactly the named decision must be retired; %d remain", len(left))
	}
	if left[0].Case != "writes land" {
		t.Fatalf("the sibling must survive; got %q", left[0].Case)
	}
}

// Retiring is itself a decision, and one with no reason or attribution cannot
// be reviewed later.
func TestRetireDecision_RequiresAReasonAndAttribution(t *testing.T) {
	recordExceptionFixture(t, weakCase)
	resetFlagsAfterTest(t, retireDecisionCmd.Flags())
	retireDecisionRef = "@demo/operation:store.put"
	retireDecisionReason, retireDecisionBy = "", ""
	if err := retireRun(t, "demo"); err == nil || !strings.Contains(err.Error(), "--reason and --by are required") {
		t.Fatalf("want a refusal naming the missing fields; got: %v", err)
	}
}
