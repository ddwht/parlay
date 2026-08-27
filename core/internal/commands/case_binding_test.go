package commands

import (
	"strings"
	"testing"
)

const twoCases = `
suites:
  - name: Store honesty
    cases:
      - name: writes land
        coverage: state-only
        criterion: {ref: C1, text: "the write is durable"}
        steps: ["put a key", "read it back"]
      - name: deletes land
        coverage: state-only
        criterion: {ref: C1, text: "the write is durable"}
        steps: ["delete a key", "read it back"]
`

// Layout is not content. A reviewer's judgment survives reindentation and key
// reordering, so those must not invalidate a decision — otherwise every
// formatting pass would send real approvals back for re-review and the
// mechanism would be abandoned as noise.
func TestCaseFingerprint_IgnoresLayout(t *testing.T) {
	reordered := `
suites:
  - name: Store honesty
    cases:
      - criterion:
          text: "the write is durable"
          ref: C1
        steps:
          - "put a key"
          - "read it back"
        coverage: state-only
        name: writes land
`
	a, err := resolveCases([]byte(twoCases))
	if err != nil {
		t.Fatal(err)
	}
	b, err := resolveCases([]byte(reordered))
	if err != nil {
		t.Fatal(err)
	}
	if a[0].Fingerprint != b[0].Fingerprint {
		t.Fatalf("reindenting and reordering keys changed the fingerprint:\n%s\n%s", a[0].Fingerprint, b[0].Fingerprint)
	}
}

// The defect this exists to close: a case keeps its name, its criterion and its
// state-only marker, but its body is replaced with a different observation. The
// approval was about the old body. If the fingerprint does not move, the
// reviewer is recorded as having approved something they never saw.
func TestCaseFingerprint_MovesWhenTheObservationChanges(t *testing.T) {
	replaced := strings.Replace(twoCases,
		`steps: ["put a key", "read it back"]`,
		`steps: ["check a counter incremented"]`, 1)
	a, _ := resolveCases([]byte(twoCases))
	b, _ := resolveCases([]byte(replaced))
	if a[0].Fingerprint == b[0].Fingerprint {
		t.Fatal("replacing what the case observes left the fingerprint unchanged, so an approval of the old observation would still match")
	}
	if a[1].Fingerprint != b[1].Fingerprint {
		t.Fatal("the untouched sibling case must keep its fingerprint; editing one case must not invalidate approvals of another")
	}
}

// Dropping the state-only marker, or citing a different criterion, is also not
// the case that was approved — even with the body untouched.
func TestCaseFingerprint_MovesWhenCoverageOrCriterionChanges(t *testing.T) {
	a, _ := resolveCases([]byte(twoCases))
	for _, m := range []struct{ what, from, to string }{
		{"coverage strengthened", "coverage: state-only\n        criterion: {ref: C1", "coverage: full\n        criterion: {ref: C1"},
		{"criterion swapped", "criterion: {ref: C1, text: \"the write is durable\"}\n        steps: [\"put a key\"", "criterion: {ref: C2, text: \"the write is durable\"}\n        steps: [\"put a key\""},
	} {
		mutated := strings.Replace(twoCases, m.from, m.to, 1)
		if mutated == twoCases {
			t.Fatalf("%s: fixture mutation did not apply, so this asserts nothing", m.what)
		}
		b, err := resolveCases([]byte(mutated))
		if err != nil {
			t.Fatalf("%s: %v", m.what, err)
		}
		if a[0].Fingerprint == b[0].Fingerprint {
			t.Fatalf("%s: fingerprint unchanged", m.what)
		}
	}
}

// Two cases discharging the same criterion must be distinguishable. This is
// the shape that wedges a ledger keyed only on (ref, text).
func TestResolveCases_SiblingsOnOneCriterionAreDistinct(t *testing.T) {
	cs, err := resolveCases([]byte(twoCases))
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Fatalf("want 2 cases, got %d", len(cs))
	}
	if cs[0].Ref != cs[1].Ref {
		t.Fatal("fixture must have both cases on one criterion to be the case under test")
	}
	if cs[0].Fingerprint == cs[1].Fingerprint {
		t.Fatal("two distinct cases on one criterion share a fingerprint")
	}
}
