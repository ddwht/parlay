package agent

// parlay-feature: infrastructure-layer
// parlay-component: portability-lint
// parlay-artifact: test

import (
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
)

func specificFragment(name, justification string) parser.InfraFragment {
	return parser.InfraFragment{
		Name: name,
		// Implementation vocabulary on both linted fields: a language
		// keyword, a qualified path, and a file extension.
		Affects:              "internal/config/config.go",
		Behavior:             "the func ClassifyDir(path string) returns a DirClass",
		Source:               "@parlay-tool/classify-directories",
		DeliberatelySpecific: justification,
	}
}

// The lint's whole reason for warning rather than forbidding is that
// specificity is sometimes correct. This asserts it still fires by default,
// so the suppression below is meaningful.
func TestPortabilityLintFiresWithoutJustification(t *testing.T) {
	warnings := lintPortability([]parser.InfraFragment{specificFragment("Directory classification", "")})
	if len(warnings) == 0 {
		t.Fatal("expected portability warnings for implementation vocabulary")
	}
}

// Once refinements promote into infrastructure.md, justified specificity
// stops being rare. A warning that fires forever on every promoted fragment
// is worse than none: people learn to scroll past the category, taking the
// accidental specificity the lint exists to catch with it.
func TestDeliberateSpecificitySuppressesTheLint(t *testing.T) {
	frag := specificFragment("Directory classification",
		"the constraint is about this package by name; restating it in domain terms would describe a different rule")
	if warnings := lintPortability([]parser.InfraFragment{frag}); len(warnings) != 0 {
		t.Errorf("a justified fragment must not be linted, got %d warning(s): %+v", len(warnings), warnings)
	}
}

// The justification is the mechanism, not decoration. A bare marker would
// be a mute switch; only a claim someone can disagree with earns silence.
func TestEmptyJustificationDoesNotSuppress(t *testing.T) {
	for _, blank := range []string{"", "   ", "\t"} {
		frag := specificFragment("Directory classification", blank)
		if warnings := lintPortability([]parser.InfraFragment{frag}); len(warnings) == 0 {
			t.Errorf("marker %q suppressed the lint with no justification", blank)
		}
	}
}

// Suppression is per fragment, not per file — one justified fragment must
// not silence its neighbours.
func TestSuppressionDoesNotLeakAcrossFragments(t *testing.T) {
	justified := specificFragment("Justified", "named on purpose")
	unjustified := specificFragment("Unjustified", "")

	warnings := lintPortability([]parser.InfraFragment{justified, unjustified})
	if len(warnings) == 0 {
		t.Fatal("the unjustified fragment must still be linted")
	}
	for _, w := range warnings {
		if w.Fragment == "Justified" {
			t.Errorf("suppression leaked to the justified fragment: %+v", w)
		}
	}
}
