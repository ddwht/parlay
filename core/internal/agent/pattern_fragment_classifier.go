// parlay-feature: parlay-tool/multi-adapter
// parlay-component: pattern-fragment-classifier
//
// Classifies residual prose fragments (the ones the operation-shaped
// extractor in `parlay migrate-capabilities` does not consume) by detected
// shape, paired with a suggested destination from the closed list.
//
// This classifier is report-only. It writes a per-feature migration report;
// it never auto-applies a suggestion to capabilities.yaml, domain-model.yaml,
// or blueprint.yaml.

package agent

import (
	"strings"
)

// PatternShape is the closed enum the classifier emits. Adding a new shape
// requires updating both this enum and `parlay migrate-capabilities`.
type PatternShape string

const (
	PatternPipeline   PatternShape = "pipeline"
	PatternRegistry   PatternShape = "registry"
	PatternDispatcher PatternShape = "dispatcher"
	PatternTraversal  PatternShape = "traversal"
	PatternResolver   PatternShape = "resolver"
	PatternValidator  PatternShape = "validator"
	PatternAspect     PatternShape = "aspect"
	PatternCache      PatternShape = "cache"
	PatternMigrator   PatternShape = "migrator"
	PatternHook       PatternShape = "hook"
	PatternHelper     PatternShape = "helper"
	PatternUnrouted   PatternShape = "unrouted"
	PatternV2Deferred PatternShape = "v2-deferred"
)

// ClassificationResult is the per-fragment outcome.
type ClassificationResult struct {
	Shape                PatternShape
	SuggestedDestination string
	Verbatim             string
}

// ClassifyPatternFragment analyzes a residual paragraph and returns its
// shape + suggested destination. The matching is keyword-driven and
// deliberately conservative — anything ambiguous lands in PatternUnrouted
// for designer review.
func ClassifyPatternFragment(text string) ClassificationResult {
	lower := strings.ToLower(text)

	// v2-deferred shapes preserved verbatim.
	if strings.Contains(lower, "subscribe") || strings.Contains(lower, "subscription") || strings.Contains(lower, "long-lived") {
		return ClassificationResult{
			Shape:                PatternV2Deferred,
			SuggestedDestination: "preserved verbatim — v2-deferred",
			Verbatim:             text,
		}
	}
	if strings.Contains(lower, "background job") || strings.Contains(lower, "scheduled") {
		return ClassificationResult{
			Shape:                PatternV2Deferred,
			SuggestedDestination: "preserved verbatim — v2-deferred",
			Verbatim:             text,
		}
	}

	// Keyword matches in priority order.
	cases := []struct {
		shape    PatternShape
		keywords []string
		dest     string
	}{
		{PatternPipeline, []string{"then", "step", "pipeline", "validate", "transform"}, "command operation in capabilities"},
		{PatternRegistry, []string{"register", "registry", "registered"}, "domain entity plus register/list/lookup operations"},
		{PatternDispatcher, []string{"dispatch", "route to", "fan out", "fan-out"}, "application-layer dispatcher"},
		{PatternTraversal, []string{"walk", "traverse", "iterate"}, "domain query operation (read-many or search)"},
		{PatternResolver, []string{"resolve", "resolver"}, "application-layer resolver"},
		{PatternValidator, []string{"validation", "validate", "must satisfy"}, "validate-input step in command operation"},
		{PatternAspect, []string{"all", "every", "each ", "for every"}, "blueprint aspect (errors, auth, state)"},
		{PatternCache, []string{"cache", "memoize"}, "blueprint data.caching"},
		{PatternMigrator, []string{"migrate", "migration"}, "migration command (migrate-* family)"},
		{PatternHook, []string{"hook", "before", "after", "trigger"}, "application-layer hook"},
		{PatternHelper, []string{"trimmer", "helper", "utility", "format"}, "adapter-level pattern (not spec-layer)"},
	}

	for _, c := range cases {
		for _, kw := range c.keywords {
			if strings.Contains(lower, kw) {
				return ClassificationResult{
					Shape:                c.shape,
					SuggestedDestination: c.dest,
					Verbatim:             text,
				}
			}
		}
	}

	return ClassificationResult{
		Shape:                PatternUnrouted,
		SuggestedDestination: "unrouted; designer review required",
		Verbatim:             text,
	}
}
