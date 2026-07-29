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

// TestVersionMismatch_RealBannerVersion checks the downstream effect: with
// the version extracted correctly, the range comparison operates on a real
// number instead of a commit hash.
func TestVersionMismatch_RealBannerVersion(t *testing.T) {
	if !versionMismatch("0.1.2") {
		t.Error("0.1.2 should be reported as older than the >=1.0.0 floor")
	}
	if versionMismatch("1.0.0") {
		t.Error("1.0.0 satisfies the floor and must not warn")
	}
	if versionMismatch("2.3.4") {
		t.Error("2.3.4 satisfies the floor and must not warn")
	}
}
