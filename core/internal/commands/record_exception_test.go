package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// recordExceptionFixture builds a feature with one capability criterion and a
// testcases file whose single case observes that criterion weakly.
func recordExceptionFixture(t *testing.T, cases string) string {
	t.Helper()
	dir := setupTestDir(t)
	feat := filepath.Join(dir, "spec", "intents", "demo")
	build := filepath.Join(dir, ".parlay", "build", "demo")
	for _, d := range []string{feat, build} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	caps := "schema_version: 2\nfeature: demo\noperations:\n" +
		"  - id: store.put\n    kind: command\n    verify:\n      - \"the write is durable\"\n"
	if err := os.WriteFile(filepath.Join(feat, "capabilities.yaml"), []byte(caps), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(build, "testcases.yaml"), []byte(cases), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func recordExceptionRun(t *testing.T, args ...string) error {
	t.Helper()
	cmd := testCommandWithContext(t, testContext(t))
	cmd.Args = cobra.ExactArgs(1)
	return runRecordException(cmd, args)
}

const weakCase = `schema_version: 3
suites:
  - name: Store honesty
    cases:
      - name: writes land
        coverage: state-only
        criterion: {ref: "@demo/operation:store.put", text: "the write is durable"}
        steps: ["put a key", "read the store"]
`

// The three ways a state-only decision can be about nothing. Each was
// previously accepted and written to the ledger, where it read as a reviewed
// downgrade.
func TestRecordException_RefusesADecisionAboutNothing(t *testing.T) {
	for _, tc := range []struct{ what, suite, caseName, wantErr string }{
		{"case does not exist", "Store honesty", "no such case", "declares no case"},
		{"suite does not exist", "No such suite", "writes land", "declares no case"},
	} {
		t.Run(tc.what, func(t *testing.T) {
			recordExceptionFixture(t, weakCase)
			resetFlagsAfterTest(t, recordExceptionCmd.Flags())
			recordExceptionKind = "state-only"
			recordExceptionRef = "@demo/operation:store.put"
			recordExceptionText = "the write is durable"
			recordExceptionReason = "state is the only honest observation"
			recordExceptionBy = "interactive decision"
			recordExceptionSuite, recordExceptionCase = tc.suite, tc.caseName

			err := recordExceptionRun(t, "demo")
			if err == nil {
				t.Fatalf("%s: must be refused", tc.what)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("%s: want %q, got: %v", tc.what, tc.wantErr, err)
			}
		})
	}
}

// A case that is not state-only has no weakened observation to accept.
func TestRecordException_RefusesACaseThatIsNotWeak(t *testing.T) {
	recordExceptionFixture(t, strings.Replace(weakCase, "coverage: state-only", "coverage: full", 1))
	resetFlagsAfterTest(t, recordExceptionCmd.Flags())
	recordExceptionKind = "state-only"
	recordExceptionRef = "@demo/operation:store.put"
	recordExceptionText = "the write is durable"
	recordExceptionReason = "r"
	recordExceptionBy = "interactive decision"
	recordExceptionSuite, recordExceptionCase = "Store honesty", "writes land"

	err := recordExceptionRun(t, "demo")
	if err == nil || !strings.Contains(err.Error(), "not state-only") {
		t.Fatalf("approving a full-coverage case must be refused; got: %v", err)
	}
}

// A decision must record the case's content, not just its name.
func TestRecordException_WritesTheCaseFingerprint(t *testing.T) {
	dir := recordExceptionFixture(t, weakCase)
	resetFlagsAfterTest(t, recordExceptionCmd.Flags())
	recordExceptionKind = "state-only"
	recordExceptionRef = "@demo/operation:store.put"
	recordExceptionText = "the write is durable"
	recordExceptionReason = "state is the only honest observation for a write"
	recordExceptionBy = "interactive decision"
	recordExceptionSuite, recordExceptionCase = "Store honesty", "writes land"

	if err := recordExceptionRun(t, "demo"); err != nil {
		t.Fatalf("a decision about a real weak case must be accepted: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".parlay", "build", "demo", "coverage-exceptions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := fingerprintOf(t, weakCase, 0)
	if !strings.Contains(string(data), want) {
		t.Fatalf("the written decision must carry the case fingerprint %s; got:\n%s", want, data)
	}
}
