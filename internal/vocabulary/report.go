// parlay-feature: design-loop/vocabulary-validation
// parlay-component: cross-cutting/vocabulary-validator-library

// Package vocabulary implements the closed six-check validator that compares
// a layout against an adapter-declared vocabulary. The report.go file owns
// the report data shape; the closed Severity enum; and the closed Rule enum
// for the six checks. The data types here are pure values — zero imports
// beyond the stdlib — so any caller can depend on them without pulling in
// validator or vocabulary internals.
package vocabulary

// Severity is the closed enum of admissible severity values for a report
// entry. The validator emits exactly these two values; any other string
// would be a regression of the wire contract documented in the buildfile.
type Severity string

const (
	// SeverityError marks an entry the caller MUST treat as a hard validation
	// failure. Checks 1-3 (type, property, variant) only ever emit error.
	SeverityError Severity = "error"
	// SeverityWarning marks an entry that resolves through an alias to a
	// non-canonical token (or a similar "resolves but isn't canonical"
	// classification). Checks 4-6 (spacing-token, color-token,
	// layout-container) may emit warning; checks 1-3 never do.
	SeverityWarning Severity = "warning"
)

// Rule is the closed enum of the six checks the validator runs against each
// applicable node. The string values are part of the JSON wire contract and
// are stable across vocabulary versions — callers grep on these strings to
// classify entries.
type Rule string

const (
	// RuleTypeCheck flags a node whose type does not resolve in the
	// vocabulary's component set. Error severity only.
	RuleTypeCheck Rule = "type-check"
	// RulePropertyCheck flags a node carrying a property whose name is not
	// in the resolved ComponentSpec's Properties list. Error severity only.
	RulePropertyCheck Rule = "property-check"
	// RuleVariantCheck flags a node whose variant value falls outside the
	// resolved ComponentSpec's Variants enum for that axis. Error severity
	// only.
	RuleVariantCheck Rule = "variant-check"
	// RuleSpacingTokenCheck flags spacing/padding/gap values that are raw
	// literals (error) or aliases to non-canonical tokens (warning).
	RuleSpacingTokenCheck Rule = "spacing-token-check"
	// RuleColorTokenCheck flags color values that are raw hex (error) or
	// aliases to non-canonical tokens (warning).
	RuleColorTokenCheck Rule = "color-token-check"
	// RuleLayoutContainerCheck flags layout containers whose parameter
	// names fall outside AdmissibleParameters, or whose parameter values
	// fail the matching ParameterConstraint.
	RuleLayoutContainerCheck Rule = "layout-container-check"
)

// Entry is the closed report-entry shape. The five fields are exactly
// {NodePath, Rule, Expected, Actual, Severity} — no additional fields,
// no renames. Suite 1 source-greps this declaration; Suite 4 pins the
// string values via the rule and severity constants. Any change to this
// shape is a wire-contract break and must update the documented schema.
type Entry struct {
	NodePath string   `json:"node_path" yaml:"node_path"`
	Rule     Rule     `json:"rule"      yaml:"rule"`
	Expected any      `json:"expected"  yaml:"expected"`
	Actual   any      `json:"actual"    yaml:"actual"`
	Severity Severity `json:"severity"  yaml:"severity"`
}

// Report is the structured value the validator returns. The single Entries
// field carries the full set of issues discovered during the walk; the
// validator does not short-circuit, so callers see every applicable failure
// in one pass.
type Report struct {
	Entries []Entry `json:"report" yaml:"report"`
}

// HasErrors reports whether the report contains at least one entry whose
// Severity is SeverityError. Callers use this to compute the
// in-vocabulary / out-of-vocabulary signal — the derivation lives here in
// callers, never inside the validator itself.
func (r Report) HasErrors() bool {
	for _, e := range r.Entries {
		if e.Severity == SeverityError {
			return true
		}
	}
	return false
}
