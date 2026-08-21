// parlay-feature: parlay-tool/phase-gates
// parlay-component: unapplied-amendments-readiness
// parlay-artifact: test

package commands

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// amendmentFixtureBody is the minimal well-formed amendment used by the
// readiness/gate ledger tests. affects: names an operation that need not
// resolve — the unapplied-tail computation is by sequence, not resolvability.
func amendmentFixtureBody(slug string) string {
	return "---\namendment: " + slug + "\ndate: 2026-08-13\naffects: [\"@my-feature/operation:x\"]\n---\n## Change\nchanged\n## Acceptance\n- ok\n"
}

func readinessHasCode(issues []readinessIssue, code string) *readinessIssue {
	for i := range issues {
		if issues[i].Code == code {
			return &issues[i]
		}
	}
	return nil
}

func writeRefineJournalFixture(t *testing.T, slug string, j refineJournal) {
	t.Helper()
	cfg := testContext(t)
	path := refineJournalPath(cfg, slug)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(j)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// setupUnappliedTailFeature lays down my-feature with a saved baseline whose
// last-applied-amendment is 0 and a single amendment 001 — so the ledger tail
// {001} is unapplied. Returns the feature dir.
func setupUnappliedTailFeature(t *testing.T) string {
	t.Helper()
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	writeAmendment(t, featureDir, "001-first.md", amendmentFixtureBody("first"))
	saveLedgerBaseline(t, featureDir, nil) // LastAppliedAmendment defaults to 0
	return featureDir
}

func TestReadinessUnapplied_TailBlocks(t *testing.T) {
	featureDir := setupUnappliedTailFeature(t)
	issues := checkUnappliedAmendments(testContext(t), "my-feature", featureDir)
	iss := readinessHasCode(issues, "unapplied-amendments")
	if iss == nil {
		t.Fatalf("expected unapplied-amendments issue; got %+v", issues)
	}
	if iss.Severity != "error" {
		t.Errorf("unapplied-amendments must be an error; got %q", iss.Severity)
	}
}

func TestReadinessUnapplied_FullyAppliedIsClean(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	writeAmendment(t, featureDir, "001-first.md", amendmentFixtureBody("first"))
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 1
		b.Sources.Amendments = map[string]string{}
		if hash, ok := hashWholeFile(filepath.Join(featureDir, "amendments", "001-first.md")); ok {
			b.Sources.Amendments["001-first.md"] = hash
		}
	})
	issues := checkUnappliedAmendments(testContext(t), "my-feature", featureDir)
	if iss := readinessHasCode(issues, "unapplied-amendments"); iss != nil {
		t.Errorf("a fully-applied ledger must not report unapplied-amendments; got %+v", issues)
	}
}

func TestReadinessUnapplied_NoLedgerIsClean(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	saveLedgerBaseline(t, featureDir, nil) // no amendments/ at all
	issues := checkUnappliedAmendments(testContext(t), "my-feature", featureDir)
	if iss := readinessHasCode(issues, "unapplied-amendments"); iss != nil {
		t.Errorf("a feature with no ledger must not report unapplied-amendments; got %+v", issues)
	}
}

func TestReadinessUnapplied_ActiveJournalSuppresses(t *testing.T) {
	featureDir := setupUnappliedTailFeature(t)
	// WS1 rule: ANY active refine journal suppresses the readiness error — the
	// tail is being applied right now. (The gate, WS2b, is more precise.)
	writeRefineJournalFixture(t, "my-feature", refineJournal{
		Feature:   "my-feature",
		Amendment: 1,
		Completed: []string{"amendment-written"},
	})
	issues := checkUnappliedAmendments(testContext(t), "my-feature", featureDir)
	if iss := readinessHasCode(issues, "unapplied-amendments"); iss != nil {
		t.Errorf("an in-flight refine journal must suppress the readiness error; got %+v", issues)
	}
}
