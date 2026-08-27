package commands

import (
	"strings"
	"testing"
)

const downgradeFixture = `
suites:
  - name: Store honesty
    cases:
      - name: writes land
        coverage: state-only
        criterion: {ref: C1, text: "the write is durable"}
        steps: ["put a key", "read it back"]
`

func fingerprintOf(t *testing.T, content string, i int) string {
	t.Helper()
	cs, err := resolveCases([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	return cs[i].Fingerprint
}

func findingCodes(out []string) string { return strings.Join(out, "\n") }

// A decision recorded against the case as it stands excuses it, and says
// nothing else. This is the baseline the other cases here are deviations from.
func TestUnapprovedDowngrades_ContentBoundDecisionApproves(t *testing.T) {
	d := DowngradeDecision{
		Ref: "C1", Text: "the write is durable",
		Suite: "Store honesty", Case: "writes land",
		CaseHash: fingerprintOf(t, downgradeFixture, 0),
	}
	out := unapprovedDowngrades("tc.yaml", []byte(downgradeFixture), []DowngradeDecision{d})
	if len(out) != 0 {
		t.Fatalf("a decision bound to this exact case must excuse it; got:\n%s", findingCodes(out))
	}
}

// The defect: the case keeps its name, criterion and state-only marker, but
// observes something else entirely. A name-keyed decision still matched. It
// must not.
func TestUnapprovedDowngrades_ReplacedBodyIsNotStillApproved(t *testing.T) {
	granted := fingerprintOf(t, downgradeFixture, 0)
	replaced := strings.Replace(downgradeFixture,
		`steps: ["put a key", "read it back"]`,
		`steps: ["check an unrelated counter"]`, 1)
	if replaced == downgradeFixture {
		t.Fatal("fixture mutation did not apply")
	}
	d := DowngradeDecision{
		Ref: "C1", Text: "the write is durable",
		Suite: "Store honesty", Case: "writes land", CaseHash: granted,
	}
	out := unapprovedDowngrades("tc.yaml", []byte(replaced), []DowngradeDecision{d})
	if !strings.Contains(findingCodes(out), "downgrade-approval-stale") {
		t.Fatalf("a rewritten case must send its approval back for re-review; got:\n%s", findingCodes(out))
	}
}

// A decision with no hash predates the binding. It must surface rather than
// continue to excuse a case nobody can prove it was about.
func TestUnapprovedDowngrades_UnboundDecisionMustBeReconfirmed(t *testing.T) {
	d := DowngradeDecision{
		Ref: "C1", Text: "the write is durable",
		Suite: "Store honesty", Case: "writes land",
	}
	out := unapprovedDowngrades("tc.yaml", []byte(downgradeFixture), []DowngradeDecision{d})
	if !strings.Contains(findingCodes(out), "downgrade-approval-stale") {
		t.Fatalf("an unbound legacy decision must be re-confirmed, not honoured; got:\n%s", findingCodes(out))
	}
	if !strings.Contains(findingCodes(out), "predates content binding") {
		t.Fatalf("the reason must distinguish 'never bound' from 'drifted'; got:\n%s", findingCodes(out))
	}
}

// An approval standing over a case that is gone, renamed, or strengthened to
// full coverage looks like diligence in the ledger while excusing nothing.
func TestUnapprovedDowngrades_OrphanedDecisionIsReported(t *testing.T) {
	for _, tc := range []struct{ what, content string }{
		{"case removed", "suites:\n  - name: Store honesty\n    cases: []\n"},
		{"strengthened to full", strings.Replace(downgradeFixture, "coverage: state-only", "coverage: full", 1)},
	} {
		d := DowngradeDecision{
			Ref: "C1", Text: "the write is durable",
			Suite: "Store honesty", Case: "writes land", CaseHash: "whatever",
		}
		out := unapprovedDowngrades("tc.yaml", []byte(tc.content), []DowngradeDecision{d})
		if !strings.Contains(findingCodes(out), "downgrade-approval-orphaned") {
			t.Fatalf("%s: must report the stranded approval; got:\n%s", tc.what, findingCodes(out))
		}
	}
}

// Two cases may each observe one criterion weakly, for different reasons. A
// decision about one must not excuse the other, and approving one must not
// require approving both.
func TestUnapprovedDowngrades_SiblingCasesJudgedSeparately(t *testing.T) {
	two := downgradeFixture + `      - name: deletes land
        coverage: state-only
        criterion: {ref: C1, text: "the write is durable"}
        steps: ["delete a key", "read it back"]
`
	d := DowngradeDecision{
		Ref: "C1", Text: "the write is durable",
		Suite: "Store honesty", Case: "writes land",
		CaseHash: fingerprintOf(t, two, 0),
	}
	out := unapprovedDowngrades("tc.yaml", []byte(two), []DowngradeDecision{d})
	joined := findingCodes(out)
	if !strings.Contains(joined, "deletes land") {
		t.Fatalf("the unapproved sibling must still be reported; got:\n%s", joined)
	}
	if strings.Contains(joined, "writes land") {
		t.Fatalf("the approved case must not be reported; got:\n%s", joined)
	}
}
