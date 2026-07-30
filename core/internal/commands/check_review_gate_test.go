// parlay-feature: parlay-tool/coverage-review
// parlay-component: review-gate
// parlay-artifact: test

// core/internal/commands is the weakest-covered package in the tree (50.3% of
// statements, against 79-85% for its neighbours) and the largest by more than
// double, so it is where an untested path is most likely to hide. These tests
// cover requiredSuiteIDs, which decides which suites the review gate demands —
// a wrong answer there either blocks a build that should pass or passes one that
// should block, and neither is visible from the outside.

package commands

import (
	"strings"
	"testing"
)

func TestRequiredSuiteIDs_PrefersIDOverName(t *testing.T) {
	got := requiredSuiteIDs([]byte(`suites:
  - name: human readable name
    id: stable-id
`))
	if len(got) != 1 || got[0] != "stable-id" {
		t.Fatalf("got %v, want [stable-id] — an explicit id must win over the display name", got)
	}
}

func TestRequiredSuiteIDs_FallsBackToName(t *testing.T) {
	got := requiredSuiteIDs([]byte(`suites:
  - name: only-a-name
`))
	if len(got) != 1 || got[0] != "only-a-name" {
		t.Fatalf("got %v, want [only-a-name]", got)
	}
}

// A suite with neither is not an identity the gate can demand, and inventing one
// would make the gate require something no suite can satisfy.
func TestRequiredSuiteIDs_SkipsSuitesWithNeither(t *testing.T) {
	got := requiredSuiteIDs([]byte(`suites:
  - name: kept
  - {}
  - id: also-kept
`))
	if len(got) != 2 {
		t.Fatalf("got %v, want exactly the two identifiable suites", got)
	}
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, "kept") || !strings.Contains(joined, "also-kept") {
		t.Fatalf("got %v, want both identifiable suites", got)
	}
}

// Order is the file's order. The gate reports missing suites, and a reordered
// report is harder to diff against the file the author is editing.
func TestRequiredSuiteIDs_PreservesFileOrder(t *testing.T) {
	got := requiredSuiteIDs([]byte(`suites:
  - id: third
  - id: first
  - id: second
`))
	want := []string{"third", "first", "second"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v (file order, not sorted)", got, want)
		}
	}
}

// Malformed YAML yields nil rather than a partial list. A partial list would make
// the gate demand a subset it happened to parse, which passes a build for the
// wrong reason — worse than reporting nothing.
func TestRequiredSuiteIDs_MalformedYAMLYieldsNil(t *testing.T) {
	if got := requiredSuiteIDs([]byte("suites: [unclosed")); got != nil {
		t.Fatalf("got %v, want nil for unparseable input", got)
	}
}

// An empty or suite-less file is not an error, it is a file with no requirements.
func TestRequiredSuiteIDs_EmptyInputIsEmptyNotError(t *testing.T) {
	for _, in := range []string{"", "suites: []", "feature: f\n"} {
		if got := requiredSuiteIDs([]byte(in)); len(got) != 0 {
			t.Errorf("input %q gave %v, want no required suites", in, got)
		}
	}
}
