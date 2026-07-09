// parlay-feature: parlay-tool/schema-consolidation
// parlay-component: vocabulary-block-parity-check
//
// CheckVocabularyBlockParity is the cheap consistency net described in
// vocabulary.schema.md's "Cross-block parity check" section: it does NOT
// derive the vocabulary: block from componentVocabulary:/tokens: (that
// full derivation is deferred — see the same schema section for why),
// it only flags when an adapter author has edited one side and forgotten
// the other. Deliberately reads the adapter file directly rather than
// importing studio/pkg/vocabulary, so this check has no cross-module
// dependency.

package agent

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// CheckVocabularyBlockParity reads the adapter file at path and compares
// its componentVocabulary:/tokens: blocks (the authoritative declaration)
// against its vocabulary: block (the Design Loop's derivation target),
// per the field-name equivalence table in vocabulary.schema.md. Returns
// one outcome per name present on one side and missing on the other.
//
// Returns (nil, nil) when either block is absent entirely — an adapter
// that hasn't adopted one of the two sections yet has nothing to compare,
// and that's not a parity failure (see adapter.schema.md: both sections
// are optional).
func CheckVocabularyBlockParity(mode ValidationMode, path string) ([]ValidationOutcome, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read adapter file %s: %w", path, err)
	}
	var a deepAdapter
	if err := yaml.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("parse adapter file %s: %w", path, err)
	}
	if a.ComponentVocabulary == nil || a.Vocabulary == nil {
		return nil, nil
	}

	var outcomes []ValidationOutcome

	cvNames := map[string]bool{}
	for _, c := range a.ComponentVocabulary.Components {
		cvNames[c.Type] = true
	}
	vocabNames := map[string]bool{}
	for _, c := range a.Vocabulary.Components {
		vocabNames[c.Name] = true
	}
	outcomes = append(outcomes, diffNameSets(mode, path, "componentVocabulary.components", cvNames, "vocabulary.components", vocabNames)...)

	if a.Tokens != nil {
		spacingNames := map[string]bool{}
		for _, t := range a.Tokens.Spacing {
			spacingNames[t.Name] = true
		}
		vocabSpacing := map[string]bool{}
		for _, n := range a.Vocabulary.SpacingTokens {
			vocabSpacing[n] = true
		}
		outcomes = append(outcomes, diffNameSets(mode, path, "tokens.spacing", spacingNames, "vocabulary.spacing_tokens", vocabSpacing)...)

		colorNames := map[string]bool{}
		for _, t := range a.Tokens.Color {
			colorNames[t.Name] = true
		}
		vocabColor := map[string]bool{}
		for _, n := range a.Vocabulary.ColorTokens {
			vocabColor[n] = true
		}
		outcomes = append(outcomes, diffNameSets(mode, path, "tokens.color", colorNames, "vocabulary.color_tokens", vocabColor)...)
	}

	return outcomes, nil
}

// diffNameSets reports every name present in exactly one of the two sets,
// in both directions, each as a vocabulary-block-parity-drift outcome.
func diffNameSets(mode ValidationMode, path, aLabel string, aNames map[string]bool, bLabel string, bNames map[string]bool) []ValidationOutcome {
	var outcomes []ValidationOutcome
	for name := range aNames {
		if !bNames[name] {
			outcomes = append(outcomes, NewOutcome(mode, "vocabulary-block-parity-drift",
				fmt.Sprintf("%s: %s declares %q but %s has no matching entry", path, aLabel, name, bLabel)))
		}
	}
	for name := range bNames {
		if !aNames[name] {
			outcomes = append(outcomes, NewOutcome(mode, "vocabulary-block-parity-drift",
				fmt.Sprintf("%s: %s declares %q but %s has no matching entry", path, bLabel, name, aLabel)))
		}
	}
	return outcomes
}
