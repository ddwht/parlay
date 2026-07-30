// parlay-feature: parlay-tool/multi-adapter
// parlay-component: validation-mode-dispatch
//
// ValidationMode discriminates the two run-modes for validation rules:
// authoring (permissive, warning-rich) vs build (strict). Each rule
// declares its severity per mode; rules without an explicit declaration
// default to error in both modes. Authoring mode never silently passes a
// build-mode failure; build mode never downgrades errors.

package agent

// ValidationMode is the closed enum {authoring, build}.
type ValidationMode string

const (
	ModeAuthoring ValidationMode = "authoring"
	ModeBuild     ValidationMode = "build"
)

// Severity is the closed enum {warning, error}.
type Severity string

const (
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// ValidationOutcome is the structured form every multi-adapter validator
// emits. It is a superset of the legacy ValidationError shape, with explicit
// mode + severity attribution.
type ValidationOutcome struct {
	Mode     ValidationMode `json:"mode"`
	Code     string         `json:"code"`
	Severity Severity       `json:"severity"`
	Message  string         `json:"message"`
	Fix      string         `json:"fix,omitempty"`
	Context  string         `json:"context,omitempty"`
}

// ruleSeverityTable records per-mode severity for every multi-adapter rule.
// Entries default to error in both modes when absent.
var ruleSeverityTable = map[string]map[ValidationMode]Severity{
	"domain-operations-deprecated": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityError,
	},
	"capabilities-not-closed-form": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityError,
	},
	"surface-md-superseded": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},
	"surface-md-legacy-format": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},
	"testcases-source-refs-missing-legacy": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},
	// Warning in both modes, and deliberately not an error in build. Every
	// buildfile in existence carries models: because the validator used to
	// require it, so erroring would fail every project at once. The severity
	// states the direction of travel while leaving existing buildfiles valid.
	//
	// It lived as a hardcoded SeverityWarning literal at the emit site, which put
	// it outside this table and therefore outside anything that reads this table
	// — including the check that a code documented as a warning really is one.
	"buildfile-models-deprecated": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},
	"prototype-framework-deprecated": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},

	// Buildfile deep-validation rules. These previously lived in a second,
	// private table inside the check-buildfile command, so the two
	// consumers of the same finding set disagreed about the same file:
	// check-buildfile graded these four as warnings while validate --deep
	// returned every finding under "errors" with no severity field at all.
	// A CI step using validate --deep therefore treated them as hard
	// failures and could not even reconstruct the distinction. One table,
	// both consumers.
	//
	// plan-create-collision is a warning in both modes deliberately: it
	// fires whenever a planned path already exists on disk, which is the
	// normal state of every buildfile after its own code has been
	// generated. Grading it blocking makes a correct buildfile
	// un-revalidatable the moment codegen runs.
	"plan-create-collision": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},
	// An element the active adapter's widget vocabulary does not know. A
	// warning in both modes on purpose: the adapter may simply be behind the
	// surface, and blocking would make an incomplete vocabulary un-buildable.
	//
	// The key is `unknown-widget` because that is what the validator emits.
	// This table said `unknown-component-widget` — a string nothing emits — so
	// the real code missed the table, fell through RuleSeverity's
	// SeverityError default, and **blocked** the very builds this entry exists
	// to let through. Two sibling phantoms went with it:
	// `unknown-component-action` and `unknown-flow` are emitted by nothing and
	// documented in no schema.
	"unknown-widget": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},
	// No fixture is designated as the one the prototype boots from, and the
	// route suites do not settle it. A warning while authoring — a feature
	// whose testcases are still being written has nothing to designate yet —
	// and blocking at build, because the composed seed is an input to code
	// generation and there is nothing sensible to generate from an undecided
	// one. Guessing is the option this rule exists to remove.
	"composition-seed-ambiguous": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityError,
	},
}

// RuleSeverity returns the per-mode severity for a given rule code. Rules
// not in the table default to error.
func RuleSeverity(code string, mode ValidationMode) Severity {
	if perMode, ok := ruleSeverityTable[code]; ok {
		if sev, ok := perMode[mode]; ok {
			return sev
		}
	}
	return SeverityError
}

// NewOutcome builds a ValidationOutcome with severity resolved from the rule
// table for the given mode.
func NewOutcome(mode ValidationMode, code, message string) ValidationOutcome {
	return ValidationOutcome{
		Mode:     mode,
		Code:     code,
		Severity: RuleSeverity(code, mode),
		Message:  message,
	}
}
