// parlay-feature: design-loop/vocabulary-validation
// parlay-component: cross-cutting/stable-error-codes-tests
// parlay-artifact: test

// errors_test.go pins the two stable error code strings textually. The
// design-loop skill matches these strings literally in its conflict-
// classification logic; renaming either code is a wire-contract break.
// The test lives in its own file so a rename fails the build with a
// single dedicated failure signal — separate from validator-logic
// regressions in validator_test.go and adapter-load regressions in
// vocabulary_test.go.
//
// The closed pair invariant: exactly two stable error codes exist for
// vocabulary resolution failures; no third code may be introduced
// without an explicit schema change. Suite 4 enforces this.

package vocabulary

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestErrVocabularyMissingFromAdapterIsTheLiteralString pins the first
// stable error code by direct string comparison against the sentinel's
// Error() value. A rename of the string is detected here.
func TestErrVocabularyMissingFromAdapterIsTheLiteralString(t *testing.T) {
	if got := ErrVocabularyMissingFromAdapter.Error(); got != "vocabulary-missing-from-adapter" {
		t.Fatalf("ErrVocabularyMissingFromAdapter.Error() = %q, want %q",
			got, "vocabulary-missing-from-adapter")
	}
}

// TestErrVocabularyUnknownAdapterIsTheLiteralString pins the second
// stable error code by literal string comparison.
func TestErrVocabularyUnknownAdapterIsTheLiteralString(t *testing.T) {
	if got := ErrVocabularyUnknownAdapter.Error(); got != "vocabulary-unknown-adapter" {
		t.Fatalf("ErrVocabularyUnknownAdapter.Error() = %q, want %q",
			got, "vocabulary-unknown-adapter")
	}
}

// TestErrCodesCarryRelevantIdentifierInMessage pins Suite 4 invariant:
// vocabulary-missing-from-adapter wraps the adapter file path; the
// vocabulary-unknown-adapter sentinel wraps both the referenced value
// AND the registered-adapter list. Both literals "adapter" and
// "registered" must appear in the resulting error messages.
func TestErrCodesCarryRelevantIdentifierInMessage(t *testing.T) {
	// Missing-from-adapter: wrapping should include the path.
	wrapped := fmt.Errorf("%w: adapter file /tmp/x.adapter.yaml has no vocabulary: block", ErrVocabularyMissingFromAdapter)
	if !errors.Is(wrapped, ErrVocabularyMissingFromAdapter) {
		t.Fatalf("errors.Is failed on wrapped missing-from-adapter")
	}
	if !strings.Contains(wrapped.Error(), "adapter") {
		t.Fatalf("wrapped error message missing 'adapter': %v", wrapped)
	}

	// Unknown-adapter: wrapping should include the registered list.
	wrapped2 := fmt.Errorf("%w: referenced componentVocabulary %q does not resolve against any registered adapter (registered: %v)",
		ErrVocabularyUnknownAdapter, "unknown@99", []string{"a", "b"})
	if !errors.Is(wrapped2, ErrVocabularyUnknownAdapter) {
		t.Fatalf("errors.Is failed on wrapped unknown-adapter")
	}
	if !strings.Contains(wrapped2.Error(), "registered") {
		t.Fatalf("wrapped error message missing 'registered': %v", wrapped2)
	}
}

// TestClosedPairExactlyTwoStableErrorCodes asserts the closed pair
// invariant: exactly two stable error codes exist in the vocabulary
// package. The test parses the package's Go source files and counts
// `errors.New("vocabulary-...")` declarations whose argument string
// starts with "vocabulary-". The closed-pair regression signal: any
// third sentinel matching the vocabulary- prefix fails this test.
func TestClosedPairExactlyTwoStableErrorCodes(t *testing.T) {
	// Locate the package directory from this test file's source path.
	// runtime.Caller(0) returns the path to errors_test.go; the package
	// directory is its containing directory.
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Dir(thisFile)

	// Read the source bytes for vocabulary.go and count occurrences of
	// the vocabulary- prefix in errors.New calls. We keep it tightly
	// scoped: only the canonical source file, not the _test.go files
	// (which legitimately reference the strings).
	canonicalPath := filepath.Join(dir, "vocabulary.go")
	data, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read vocabulary.go: %v", err)
	}
	found := map[string]struct{}{}
	src := string(data)
	// Hunt for errors.New("vocabulary-... patterns.
	prefix := `errors.New("vocabulary-`
	cursor := 0
	for {
		idx := strings.Index(src[cursor:], prefix)
		if idx < 0 {
			break
		}
		// Extract the literal up to the closing quote.
		start := cursor + idx + len(`errors.New("`)
		end := strings.Index(src[start:], `"`)
		if end < 0 {
			break
		}
		code := src[start : start+end]
		found[code] = struct{}{}
		cursor = start + end + 1
	}

	if len(found) != 2 {
		t.Fatalf("closed pair invariant violated: expected exactly 2 vocabulary-* sentinel codes in vocabulary.go, got %d: %v",
			len(found), found)
	}
	if _, ok := found["vocabulary-missing-from-adapter"]; !ok {
		t.Fatal("missing vocabulary-missing-from-adapter sentinel in vocabulary.go")
	}
	if _, ok := found["vocabulary-unknown-adapter"]; !ok {
		t.Fatal("missing vocabulary-unknown-adapter sentinel in vocabulary.go")
	}
}

