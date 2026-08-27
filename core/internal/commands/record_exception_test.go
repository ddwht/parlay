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
	// These are package-level singletons shared with every other test in this
	// package. A prior test leaving --from-legacy set sends this one looking
	// for a retired review that is not there, and the failure names a missing
	// file rather than the leak that caused it.
	recordExceptionFromLegacy = false
	recordExceptionLegacyFP, recordExceptionLegacyDup = "", 0
	t.Cleanup(func() {
		recordExceptionFromLegacy = false
		recordExceptionLegacyFP, recordExceptionLegacyDup = "", 0
	})
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
			recordExceptionFromLegacy = false

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
	recordExceptionFromLegacy = false

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
	data, err := os.ReadFile(filepath.Join(dir, ".parlay", "build", "demo", "coverage-decisions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := fingerprintOf(t, weakCase, 0)
	if !strings.Contains(string(data), want) {
		t.Fatalf("the written decision must carry the case fingerprint %s; got:\n%s", want, data)
	}
}

// A project written before the rename must not have to rename a file by hand,
// and must never end up with two ledgers where nothing says which is live.
func TestCoverageDecisions_PreRenameFileIsMigratedOnWrite(t *testing.T) {
	dir := recordExceptionFixture(t, weakCase)
	build := filepath.Join(dir, ".parlay", "build", "demo")
	oldPath := filepath.Join(build, "coverage-exceptions.yaml")
	newPath := filepath.Join(build, "coverage-decisions.yaml")

	// A ledger under the pre-rename name, holding a decision somebody made.
	old := "schema_version: 1\nfeature: demo\ngranted_at: \"2026-08-27T00:00:00Z\"\n" +
		"exceptions:\n  - ref: \"@demo/operation:store.put\"\n    text: the write is durable\n" +
		"    kind: waived\n    reason: enforced by a database constraint\n" +
		"    at: \"2026-08-27T00:00:00Z\"\n    by: interactive decision\n"
	if err := os.WriteFile(oldPath, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	// It is read, not ignored: the earlier judgment must survive the rename.
	rec, err := loadCoverageExceptions(testContext(t), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if rec == nil || len(rec.Exceptions) != 1 {
		t.Fatalf("a pre-rename ledger must still be read; got %+v", rec)
	}

	resetFlagsAfterTest(t, recordExceptionCmd.Flags())
	recordAWeakDecision(t, "Store honesty", "writes land")

	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("the ledger must be written under the current name: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("the pre-rename file must be gone after migration; leaving both leaves a stale ledger nobody can tell from the live one (stat err: %v)", err)
	}
	migrated, err := loadCoverageExceptions(testContext(t), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(migrated.Exceptions) != 2 {
		t.Fatalf("the pre-rename decision and the new one must both survive; got %d", len(migrated.Exceptions))
	}
}
