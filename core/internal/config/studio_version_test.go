package config

import "testing"

// TestLooksLikeVersion_BannerShapes covers the real parlay-studio banner,
// which carries trailing commit metadata. Taking the last whitespace field
// selected "ddwht)" as the version — an unparseable token with an unbalanced
// paren — and printed it in a warning on every parlay invocation.
func TestLooksLikeVersion_BannerShapes(t *testing.T) {
	yes := []string{"0.1.2", "1.4.0", "1.2.0-rc.1", "2.0.0+build7", "10.0.1"}
	no := []string{"", "parlay-studio", "(commit", "ddwht)", "commit", "v"}
	for _, s := range yes {
		if !looksLikeVersion(s) {
			t.Errorf("looksLikeVersion(%q) = false, want true", s)
		}
	}
	for _, s := range no {
		if looksLikeVersion(s) {
			t.Errorf("looksLikeVersion(%q) = true, want false", s)
		}
	}
}

// TestVersionMismatch_RealBannerVersion checks the downstream effect of the
// banner fix: with the version extracted correctly, the range comparison
// operates on a real number instead of a commit hash.
//
// It no longer asserts that 0.1.2 mismatches — the floor moved to >=0.1.0
// because the old ">=1.0.0" was unsatisfiable by every released Studio. What
// still matters here, and is what this test was really about, is that a
// commit-hash token is recognized as garbage rather than silently accepted.
func TestVersionMismatch_RealBannerVersion(t *testing.T) {
	for _, ok := range []string{"0.1.2", "1.0.0", "2.3.4", "1-rc.1"} {
		if versionMismatch(ok) {
			t.Errorf("%s satisfies the >=0.1.0 floor and must not warn", ok)
		}
	}
	// The token the old last-field parse used to select. Garbage must still
	// warn, or lowering the floor would have thrown away the P0-1 signal.
	for _, garbage := range []string{"ddwht)", "abc", "commit"} {
		if !versionMismatch(garbage) {
			t.Errorf("versionMismatch(%q) = false — an unparseable probe result must still warn", garbage)
		}
	}
}
