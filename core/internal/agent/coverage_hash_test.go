// parlay-feature: parlay-tool/multi-adapter
// parlay-component: coverage-review-gate
// parlay-artifact: test

package agent

import (
	"reflect"
	"testing"
)

func TestCanonicalFormHash_OrderInvariant(t *testing.T) {
	a := []byte(`feature: f
operations:
  "@f/operation:x":
    kind: command
    errors: [validation-failed, conflict]
`)
	b := []byte(`operations:
  "@f/operation:x":
    errors: [validation-failed, conflict]
    kind: command
feature: f
`)
	hashA, err := CanonicalFormHash(a)
	if err != nil {
		t.Fatal(err)
	}
	hashB, err := CanonicalFormHash(b)
	if err != nil {
		t.Fatal(err)
	}
	if hashA != hashB {
		t.Errorf("canonical hashes differ for key-order-permuted input:\n  a: %s\n  b: %s", hashA, hashB)
	}
}

func TestCanonicalFormHash_SemanticDifferenceDifferentHash(t *testing.T) {
	a := []byte(`feature: f
operations:
  "@f/operation:x":
    kind: command
`)
	b := []byte(`feature: f
operations:
  "@f/operation:x":
    kind: query
`)
	hashA, _ := CanonicalFormHash(a)
	hashB, _ := CanonicalFormHash(b)
	if hashA == hashB {
		t.Errorf("canonical hashes should differ for semantically different input")
	}
}

func TestCanonicalFormHash_WhitespaceInsensitive(t *testing.T) {
	a := []byte(`feature: f
operations:
  "@f/operation:x":
    kind: command
`)
	b := []byte(`feature:    f
operations:
  "@f/operation:x":


    kind:    command
`)
	hashA, _ := CanonicalFormHash(a)
	hashB, _ := CanonicalFormHash(b)
	if hashA != hashB {
		t.Errorf("whitespace-only differences should not drift the hash:\n  a: %s\n  b: %s", hashA, hashB)
	}
}

func TestSuiteHashes_KeysByIdThenNameAndSkipsNeither(t *testing.T) {
	got, err := SuiteHashes([]byte(`suites:
  - name: display name
    id: stable-id
    cases: [one]
  - name: only-a-name
    cases: [two]
  - cases: [three]
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d keys, want 2 (the id-bearing and the name-only suites): %v", len(got), got)
	}
	if _, ok := got["stable-id"]; !ok {
		t.Errorf("suite with an id must be keyed by it, not its name; got %v", got)
	}
	if _, ok := got["only-a-name"]; !ok {
		t.Errorf("suite without an id must fall back to its name; got %v", got)
	}
}

func TestSuiteHashes_ChangeIsolatedToOneSuite(t *testing.T) {
	before, _ := SuiteHashes([]byte(`suites:
  - id: a
    cases: [x]
  - id: b
    cases: [y]
`))
	after, _ := SuiteHashes([]byte(`suites:
  - id: a
    cases: [x]
  - id: b
    cases: [y, z]
`))
	if before["a"] != after["a"] {
		t.Errorf("suite a was untouched but its hash moved: %s -> %s", before["a"], after["a"])
	}
	if before["b"] == after["b"] {
		t.Errorf("suite b changed but its hash did not")
	}
}

func TestPerSuiteStale(t *testing.T) {
	recorded := map[string]string{"a": "h1", "b": "h2", "c": "h3"}
	now := map[string]string{"a": "h1", "b": "h2-changed", "c": "h3"}

	// Only b changed, and only among approved suites is it reported.
	stale := PerSuiteStale(recorded, now, []string{"a", "b"})
	if !reflect.DeepEqual(stale, []string{"b"}) {
		t.Errorf("expected only [b] stale, got %v", stale)
	}

	// c also changed in `now`? No — c is unchanged. A changed-but-unapproved
	// suite is not stale; it is unapproved, reported elsewhere.
	nowWithC := map[string]string{"a": "h1", "b": "h2", "c": "h3-changed"}
	if s := PerSuiteStale(recorded, nowWithC, []string{"a", "b"}); len(s) != 0 {
		t.Errorf("a change to an unapproved suite must not be stale, got %v", s)
	}

	// An approved suite with no recorded hash counts as needing re-review.
	if s := PerSuiteStale(recorded, now, []string{"d"}); !reflect.DeepEqual(s, []string{"d"}) {
		t.Errorf("approved suite absent from recorded hashes should be stale, got %v", s)
	}

	// An old-format review (no recorded hashes) defers to the whole-file
	// fallback and reports nothing here.
	if s := PerSuiteStale(nil, now, []string{"a", "b"}); s != nil {
		t.Errorf("empty recorded hashes must yield no per-suite staleness, got %v", s)
	}
}
