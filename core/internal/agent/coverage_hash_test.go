// parlay-feature: parlay-tool/multi-adapter
// parlay-component: coverage-review-gate
// parlay-artifact: test

package agent

import "testing"

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
