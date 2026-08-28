package commands

import "testing"

// The two ref spellings must normalize onto one another, or the check silently
// matches nothing and passes on every project while appearing to run.
func TestCanonicalFragmentRef(t *testing.T) {
	cases := map[string]string{
		"@graded/fragment:Customer Detail":   "@graded/customer-detail",
		"@graded/customer-detail":            "@graded/customer-detail",
		" @graded/fragment:Customer Detail ": "@graded/customer-detail",
		// An operation is a different subject and must not be folded into a
		// fragment ref.
		"@graded/operation:customer.archive": "@graded/operation:customer.archive",
		"":                                   "",
	}
	for in, want := range cases {
		if got := canonicalFragmentRef(in); got != want {
			t.Errorf("canonicalFragmentRef(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlanSourceMatches(t *testing.T) {
	if !planSourceMatches("component/customer-detail", "customer-detail") {
		t.Error("kind-prefixed plan source must match the bare component name")
	}
	if !planSourceMatches("customer-detail", "customer-detail") {
		t.Error("bare plan source must still match")
	}
	if planSourceMatches("component/other", "customer-detail") {
		t.Error("unrelated source must not match")
	}
}
