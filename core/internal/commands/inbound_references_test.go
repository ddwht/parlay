// parlay-feature: parlay-tool/feature-retirement
// parlay-component: inbound-reference-inventory
// parlay-artifact: test

package commands

import (
	"os"
	"path/filepath"
	"testing"
)

// What counts as pointing at a feature is a closed set of positions, and the
// boundary matters in both directions: a missed reference retires something out
// from under a dependent, and a false one blocks a retirement on a sentence.
func TestInboundReferences_WhatCountsAndWhatDoesNot(t *testing.T) {
	dir := t.TempDir()
	pattern, err := featureRefPattern("verify-fixture")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, body string
		refFields  []string
		want       bool
	}{
		{
			name: "a Source field is a dependency",
			body: "**Source**: @verify-fixture/create-the-thing\n",
			refFields: []string{"**Source**:"}, want: true,
		},
		{
			// The distinction is not "structured field" but WHICH field:
			// infrastructure.md's Behavior paragraphs describe other features
			// while Source cites them.
			name: "a Behavior sentence is not",
			body: "**Behavior**: Mirrors what @verify-fixture/operation:x does, but for pages.\n",
			refFields: []string{"**Source**:"}, want: false,
		},
		{
			name: "a yaml source ref is a dependency",
			body: "source: '@verify-fixture/create-the-thing'\n",
			want: true,
		},
		{
			// The trailing delimiter is what keeps a feature from matching a
			// longer name that starts with it.
			name: "a longer feature name is not this feature",
			body: "source: '@verify-fixture-v2/thing'\n",
			want: false,
		},
		{
			name: "a comment is not a dependency",
			body: "# see @verify-fixture/operation:x for the pattern\n",
			want: false,
		},
		{
			// trigger: records what prompted a change. Provenance, not need.
			name: "trigger provenance is not a dependency",
			body: "trigger: \"@verify-fixture needed this\"\n",
			want: false,
		},
		{
			name: "indented trigger provenance is not either",
			body: "  trigger: \"@verify-fixture needed this\"\n",
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".txt")
			if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			got := len(scanFileForRefs(path, "owner", "field", pattern, tc.refFields)) > 0
			if got != tc.want {
				t.Errorf("counted=%v want=%v for %q", got, tc.want, tc.body)
			}
		})
	}
}

func TestInboundReferences_ReportsWhereItFoundThings(t *testing.T) {
	// A refusal has to be verifiable without repeating the scan, so a finding
	// carries the owning artifact, the position and the reference itself.
	dir := t.TempDir()
	pattern, _ := featureRefPattern("verify-fixture")
	path := filepath.Join(dir, "surface.yaml")
	if err := os.WriteFile(path, []byte("fragments:\n    - name: X\n      supersedes: '@verify-fixture/thing-list'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	found := scanFileForRefs(path, "consumer", "surface fragment", pattern, nil)
	if len(found) != 1 {
		t.Fatalf("expected one finding, got %+v", found)
	}
	f := found[0]
	if f.Owner != "consumer" || f.Path != path || f.Field == "" || f.Ref == "" {
		t.Errorf("finding is missing context needed to verify it: %+v", f)
	}
	if f.Ref != "@verify-fixture/thing-list" {
		t.Errorf("the report should show the reference as written, got %q", f.Ref)
	}
}
