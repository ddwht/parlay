package agent

// parlay-feature: parlay-tool/hand-authored-units
// parlay-component: authored-unit-validation
// parlay-artifact: test

import (
	"path/filepath"
	"strings"
	"testing"
)

const validAuthoredYAML = `schema_version: 1
unit: geometry-engine
summary: "image → 3D relief mesh; six-stage pure transform"
sources:
  - "App/Sources/BlockPrintingCore/**"
tests:
  - "App/Tests/BlockPrintingCoreTests/**"
satisfies:
  - "@relief-workspace/invariant:deterministic-output"
`

func authoredPath(unit string) string {
	return filepath.Join("spec", "intents", unit, "authored.yaml")
}

func TestValidateAuthoredUnit_Clean(t *testing.T) {
	outcomes := ValidateAuthoredUnit(ModeAuthoring, authoredPath("geometry-engine"), []byte(validAuthoredYAML))
	if len(outcomes) != 0 {
		t.Errorf("expected a clean declaration to produce no outcomes, got %v", codesOf(outcomes))
	}
}

func TestValidateAuthoredUnit_UnparseableReportsOnlyYAML(t *testing.T) {
	outcomes := ValidateAuthoredUnit(ModeAuthoring, authoredPath("broken"), []byte("unit: [oops\n"))
	if len(outcomes) != 1 || outcomes[0].Code != "authored-invalid-yaml" {
		// Reporting "unit: is missing" about a file that is not YAML sends
		// the author to the wrong line, so the parse failure must suppress
		// every other rule rather than accumulate alongside them.
		t.Fatalf("expected exactly authored-invalid-yaml, got %v", codesOf(outcomes))
	}
}

func TestValidateAuthoredUnit_RequiredFields(t *testing.T) {
	outcomes := ValidateAuthoredUnit(ModeAuthoring, authoredPath("bare"), []byte("schema_version: 1\n"))
	for _, want := range []string{"authored-field-missing"} {
		if !hasOutcomeCode(outcomes, want) {
			t.Errorf("expected %s, got %v", want, codesOf(outcomes))
		}
	}
	// unit, summary and sources are each independently required, so an
	// empty declaration must name all three rather than stopping at the
	// first — an author fixing them one round-trip at a time is the
	// failure mode this check exists to prevent.
	missing := 0
	for _, o := range outcomes {
		if o.Code == "authored-field-missing" {
			missing++
		}
	}
	if missing != 3 {
		t.Errorf("expected 3 authored-field-missing outcomes (unit, summary, sources), got %d: %v", missing, codesOf(outcomes))
	}
}

func TestValidateAuthoredUnit_SlugMustMatchDirectory(t *testing.T) {
	content := strings.Replace(validAuthoredYAML, "unit: geometry-engine", "unit: mesh-engine", 1)
	outcomes := ValidateAuthoredUnit(ModeAuthoring, authoredPath("geometry-engine"), []byte(content))
	if !hasOutcomeCode(outcomes, "authored-unit-slug-mismatch") {
		t.Errorf("expected authored-unit-slug-mismatch, got %v", codesOf(outcomes))
	}
}

func TestValidateAuthoredUnit_SlugMatchesNestedLeaf(t *testing.T) {
	// An initiative-nested unit is addressed as "init/unit"; the leaf is
	// what the declaration names.
	path := filepath.Join("spec", "intents", "relief-workspace", "geometry-engine", "authored.yaml")
	outcomes := ValidateAuthoredUnit(ModeAuthoring, path, []byte(validAuthoredYAML))
	if hasOutcomeCode(outcomes, "authored-unit-slug-mismatch") {
		t.Errorf("nested unit should match on its leaf segment, got %v", codesOf(outcomes))
	}
}

func TestValidateAuthoredUnit_GlobContainment(t *testing.T) {
	cases := []struct {
		name string
		glob string
		want bool
	}{
		{"absolute", "/etc/passwd", true},
		{"home", "~/src/**", true},
		{"parent segment", "../sibling-project/src/**", true},
		{"nested parent segment", "App/../../escape/**", true},
		{"dots inside a name", "App/Sources/foo..bar/**", false},
		{"ordinary", "App/Sources/Core/**", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content := "schema_version: 1\nunit: u\nsummary: s\nsources:\n  - \"" + tc.glob + "\"\n"
			outcomes := ValidateAuthoredUnit(ModeAuthoring, authoredPath("u"), []byte(content))
			got := hasOutcomeCode(outcomes, "authored-glob-escapes-root")
			if got != tc.want {
				t.Errorf("glob %q: escapes-root = %v, want %v (outcomes: %v)", tc.glob, got, tc.want, codesOf(outcomes))
			}
		})
	}
}

func TestValidateAuthoredUnit_SchemaVersion(t *testing.T) {
	content := strings.Replace(validAuthoredYAML, "schema_version: 1", "schema_version: 99", 1)
	outcomes := ValidateAuthoredUnit(ModeAuthoring, authoredPath("geometry-engine"), []byte(content))
	if !hasOutcomeCode(outcomes, "authored-schema-version-unsupported") {
		t.Errorf("expected authored-schema-version-unsupported, got %v", codesOf(outcomes))
	}

	// An absent field unmarshals to 0, which is not a version this binary
	// reads — the same diagnostic, since "no version" and "a version I
	// cannot reach" are the same problem for the reader.
	noVersion := strings.Replace(validAuthoredYAML, "schema_version: 1\n", "", 1)
	outcomes = ValidateAuthoredUnit(ModeAuthoring, authoredPath("geometry-engine"), []byte(noVersion))
	if !hasOutcomeCode(outcomes, "authored-schema-version-unsupported") {
		t.Errorf("expected authored-schema-version-unsupported for an absent field, got %v", codesOf(outcomes))
	}
}
