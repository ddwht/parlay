// parlay-feature: parlay-tool/domain-document-api
// parlay-component: cross-cutting/compare-and-swap-save
// parlay-artifact: test

package domainmodel

import (
	"strings"
	"testing"
)

// TestComputeEtagDeterministic asserts identical bytes always produce an
// identical token and different bytes produce different tokens — the property
// the compare-and-swap guard relies on.
func TestComputeEtagDeterministic(t *testing.T) {
	a := computeEtag([]byte("schema_version: 1\n"))
	b := computeEtag([]byte("schema_version: 1\n"))
	if a != b {
		t.Fatalf("identical bytes produced different etags: %q vs %q", a, b)
	}
	if a == computeEtag([]byte("schema_version: 2\n")) {
		t.Fatal("different bytes produced the same etag")
	}
	if !strings.HasPrefix(string(a), "sha256:") {
		t.Fatalf("etag %q is not sha256-prefixed", a)
	}
}

// TestSentinelEmptyDistinct asserts the empty-model sentinel is the fixed
// literal "empty" and never collides with a real content hash (which is always
// sha256-prefixed).
func TestSentinelEmptyDistinct(t *testing.T) {
	if SentinelEmpty != "empty" {
		t.Fatalf("SentinelEmpty = %q, want %q", SentinelEmpty, "empty")
	}
	if strings.HasPrefix(string(SentinelEmpty), "sha256:") {
		t.Fatal("sentinel must not look like a content hash")
	}
}
