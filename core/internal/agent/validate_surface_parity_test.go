package agent

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
)

// TestValidateSurfaceAgreesWithTheParser is the regression for a validator
// that accepted input the parser rejected.
//
// notes: is free prose, which makes it the most colon-prone field in the
// artifact set. An ordinary note containing ": " makes YAML resolve that list
// item to a map rather than a string. validateSurfaceYAML decoded into a
// private struct that did not declare notes: at all, and yaml.v3 ignores
// undeclared keys — so the validator saw nothing wrong and returned OK, both
// plain and --deep, on a file the build stage could not read. The author
// found out one phase later, as surface-not-readable.
//
// Asserting the two AGREE, rather than asserting a specific message, is the
// point: it holds for every field either side gains later.
func TestValidateSurfaceAgreesWithTheParser(t *testing.T) {
	const unparseable = `feature: repro
fragments:
  - name: Frag
    shows: message
    source: "@repro/some-intent"
    page: somepage
    region: main
    order: 10
    notes:
      - This note has a colon: and it silently becomes a map.
`
	const clean = `feature: repro
fragments:
  - name: Frag
    shows: message
    source: "@repro/some-intent"
    page: somepage
    region: main
    order: 10
    notes:
      - This note has a colon — and stays a string.
`
	for _, tc := range []struct {
		name       string
		content    string
		wantErrors bool
	}{
		{"colon in a note makes it a map", unparseable, true},
		{"the same note without the colon", clean, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "surface.yaml")

			validateErr := ValidateSurface(path, []byte(tc.content))
			_, parseErr := parser.LoadSurfaceYAMLBytes(path, []byte(tc.content))

			if (validateErr != nil) != (parseErr != nil) {
				t.Fatalf("validator and parser disagree: validate=%v parse=%v", validateErr, parseErr)
			}
			if tc.wantErrors && validateErr == nil {
				t.Fatal("want an error for a file the parser rejects, got nil")
			}
			if !tc.wantErrors && validateErr != nil {
				t.Fatalf("want no error, got %v", validateErr)
			}
		})
	}
}

// The two dispatches must route the same extension to the same reader. They
// used to differ — the validator matched .yaml/.yml case-insensitively while
// ParseSurfaceFile matched only lowercase .yaml, so a surface.yml got the
// YAML validator and the markdown parser, which reported it as having no
// fragments rather than as misrouted.
func TestSurfaceExtensionDispatchAgrees(t *testing.T) {
	const yamlDoc = `feature: repro
fragments:
  - name: Frag
    shows: message
    source: "@repro/i"
    page: p
`
	for _, name := range []string{"surface.yaml", "surface.yml", "surface.YAML", "surface.YML"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			if err := ValidateSurface(path, []byte(yamlDoc)); err != nil {
				t.Fatalf("validator rejected valid YAML at %s: %v", name, err)
			}
			// The markdown branch would complain about missing "## " headings.
			if err := ValidateSurface(path, []byte(yamlDoc)); err != nil && strings.Contains(err.Error(), "## ") {
				t.Errorf("%s was routed to the markdown probe", name)
			}
		})
	}
}
