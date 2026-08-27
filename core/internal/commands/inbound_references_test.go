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
			name:      "a Source field is a dependency",
			body:      "**Source**: @verify-fixture/create-the-thing\n",
			refFields: []string{"**Source**:"}, want: true,
		},
		{
			// The distinction is not "structured field" but WHICH field:
			// infrastructure.md's Behavior paragraphs describe other features
			// while Source cites them.
			name:      "a Behavior sentence is not",
			body:      "**Behavior**: Mirrors what @verify-fixture/operation:x does, but for pages.\n",
			refFields: []string{"**Source**:"}, want: false,
		},
		{
			// The trailing delimiter is what keeps a feature from matching a
			// longer name that starts with it.
			name:      "a longer feature name is not this feature",
			body:      "**Source**: @verify-fixture-v2/thing\n",
			refFields: []string{"**Source**:"}, want: false,
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
			refs, _ := scanRawLines(path, "owner", "field", pattern, tc.refFields)
			got := len(refs) > 0
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
	found, _ := scanYAML(path, "consumer", "surface fragment", pattern, []string{"supersedes"}, nil)
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

// The formats a line scan cannot see. Each of these is legal YAML a generator
// or a person may write, and missing one produces a clean result that is simply
// wrong — the worst outcome for a check whose whole job is to establish that
// nothing points here.
func TestInboundReferences_StructuralFormsALineScanMisses(t *testing.T) {
	pattern, err := featureRefPattern("verify-fixture")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()

	cases := []struct {
		name, body string
		want       bool
	}{
		{
			name: "folded scalar spreads the value over lines that do not start with the key",
			body: "fragments:\n  - name: X\n    source: >-\n      @verify-fixture/create-the-thing\n",
			want: true,
		},
		{
			name: "block scalar likewise",
			body: "fragments:\n  - name: X\n    source: |-\n      @verify-fixture/create-the-thing\n",
			want: true,
		},
		{
			name: "flow mapping puts key and value inline behind a brace",
			body: "fragments: [{name: X, source: '@verify-fixture/create-the-thing'}]\n",
			want: true,
		},
		{
			// The other direction: a key that carries prose must not count,
			// however the value is written.
			name: "a non-reference key does not count even in a folded scalar",
			body: "fragments:\n  - name: X\n    notes: >-\n      behaves like @verify-fixture/operation:x\n",
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".yaml")
			if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			refs, _ := scanYAML(path, "consumer", "surface fragment", pattern, []string{"source", "supersedes"}, nil)
			got := len(refs) > 0
			if got != tc.want {
				t.Errorf("counted=%v want=%v for:\n%s", got, tc.want, tc.body)
			}
		})
	}
}

// An unparseable file is not evidence that nothing points at the feature.
func TestInboundReferences_UnparseableYAMLFallsBackRatherThanReportingClean(t *testing.T) {
	pattern, _ := featureRefPattern("verify-fixture")
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.yaml")
	if err := os.WriteFile(path, []byte("fragments: [\n  source: '@verify-fixture/thing'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	refs, fails := scanYAML(path, "consumer", "f", pattern, []string{"source"}, nil)
	if len(fails) == 0 {
		t.Error("a present artifact that will not parse must be recorded as a failure, not treated as empty")
	}
	if len(refs) == 0 {
		t.Error("the raw pass should still surface the visible token as a hint")
	}
}

// Positions a value-only allowlist cannot reach, or that mine simply omitted.
// Each was a reachable false-clean path in the P0 check.
func TestInboundReferences_PositionsAValueAllowlistMisses(t *testing.T) {
	pattern, err := featureRefPattern("verify-fixture")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()

	cases := []struct {
		name, body           string
		valueKeys, keyedMaps []string
		want                 bool
	}{
		{
			// Canonical operations are indexed BY qualified ref, so the
			// reference is the mapping key. No value allowlist can see this,
			// however many names it holds.
			name:      "an operation is a mapping key, not a value",
			body:      "operations:\n  \"@verify-fixture/operation:thing.create\":\n    kind: command\n",
			keyedMaps: []string{"operations"},
			want:      true,
		},
		{
			name:      "and under a target as well",
			body:      "targets:\n  presentation:\n    operations:\n      \"@verify-fixture/operation:x\":\n        kind: query\n",
			keyedMaps: []string{"operations"},
			want:      true,
		},
		{
			// A v1 suite cites the intent it validates.
			name:      "a testcase suite cites its intent",
			body:      "suites:\n  - name: S\n    intent: '@verify-fixture/create-the-thing'\n",
			valueKeys: []string{"intent"},
			want:      true,
		},
		{
			// A designer-confidence binding records candidates as ref: entries.
			name:      "a binding candidate is a ref",
			body:      "bindings:\n  - candidates:\n      - ref: '@verify-fixture/operation:x'\n        ai-confidence: 0.4\n",
			valueKeys: []string{"ref"},
			want:      true,
		},
		{
			// The other direction: a key not on the list still does not count,
			// so widening coverage did not widen it into prose.
			name:      "an unlisted key still does not count",
			body:      "notes: 'behaves like @verify-fixture/operation:x'\n",
			valueKeys: []string{"source"},
			want:      false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".yaml")
			if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			refs, fails := scanYAML(path, "consumer", "f", pattern, tc.valueKeys, tc.keyedMaps)
			if len(fails) != 0 {
				t.Fatalf("unexpected scan failure: %+v", fails)
			}
			if got := len(refs) > 0; got != tc.want {
				t.Errorf("counted=%v want=%v for:\n%s", got, tc.want, tc.body)
			}
		})
	}
}

// Fail-closed. A file the scan could not read leaves the answer unknown, and a
// check whose whole job is to establish that nothing points here must never
// report clean over one.
func TestInboundReferences_UnreadableIsNotEmpty(t *testing.T) {
	pattern, _ := featureRefPattern("verify-fixture")
	dir := t.TempDir()

	t.Run("an absent optional artifact is absence", func(t *testing.T) {
		refs, fails := scanYAML(filepath.Join(dir, "not-there.yaml"), "o", "f", pattern, []string{"source"}, nil)
		if len(refs) != 0 || len(fails) != 0 {
			t.Errorf("absence should be silent, got refs=%+v fails=%+v", refs, fails)
		}
	})

	t.Run("an unreadable present artifact is a failure", func(t *testing.T) {
		path := filepath.Join(dir, "locked.yaml")
		if err := os.WriteFile(path, []byte("source: '@verify-fixture/x'\n"), 0o000); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(path, 0o644)
		if os.Geteuid() == 0 {
			t.Skip("root reads anything")
		}
		_, fails := scanYAML(path, "o", "f", pattern, []string{"source"}, nil)
		if len(fails) == 0 {
			t.Error("a permission error is not absence")
		}
	})

	t.Run("a ledger that will not load is a failure", func(t *testing.T) {
		featDir := filepath.Join(dir, "feat")
		if err := os.MkdirAll(filepath.Join(featDir, "amendments"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(featDir, "amendments", "001-broken.md"),
			[]byte("---\nnot: [valid\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, fails := scanAmendments(featDir, "consumer", pattern)
		if len(fails) == 0 {
			t.Error("a malformed ledger is not a ledger with nothing in it")
		}
	})

	t.Run("Clean is false when anything could not be read", func(t *testing.T) {
		inv := Inventory{Failures: []ScanFailure{{Path: "x", Reason: "y"}}}
		if inv.Clean() {
			t.Error("unknown is not clean")
		}
	})
}
