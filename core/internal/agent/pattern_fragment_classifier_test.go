// parlay-feature: parlay-tool/multi-adapter
// parlay-component: pattern-fragment-classifier
// parlay-artifact: test

package agent

import "testing"

func TestClassifyPatternFragment_Registry(t *testing.T) {
	r := ClassifyPatternFragment("A router registry that lists registered handlers and supports lookup by name.")
	if r.Shape != PatternRegistry {
		t.Errorf("got shape %q, want registry", r.Shape)
	}
	if r.SuggestedDestination == "" {
		t.Error("missing suggested destination")
	}
}

func TestClassifyPatternFragment_Pipeline(t *testing.T) {
	r := ClassifyPatternFragment("Validate text length, then trim whitespace, then enforce uniqueness.")
	if r.Shape != PatternPipeline {
		t.Errorf("got shape %q, want pipeline", r.Shape)
	}
}

func TestClassifyPatternFragment_V2Deferred(t *testing.T) {
	r := ClassifyPatternFragment("Subscribe to long-lived task updates from the database.")
	if r.Shape != PatternV2Deferred {
		t.Errorf("got shape %q, want v2-deferred", r.Shape)
	}
}

func TestClassifyPatternFragment_Helper(t *testing.T) {
	r := ClassifyPatternFragment("A string trimmer helper that normalizes whitespace.")
	if r.Shape != PatternHelper {
		t.Errorf("got shape %q, want helper", r.Shape)
	}
}

func TestClassifyPatternFragment_Unrouted(t *testing.T) {
	r := ClassifyPatternFragment("Unrelated prose that does not match any closed shape.")
	if r.Shape != PatternUnrouted {
		t.Errorf("got shape %q, want unrouted", r.Shape)
	}
}
