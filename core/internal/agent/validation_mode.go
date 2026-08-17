// parlay-feature: parlay-tool/multi-adapter
// parlay-component: validation-mode-dispatch
//
// ValidationMode discriminates the two run-modes for validation rules:
// authoring (permissive, warning-rich) vs build (strict). Each rule
// declares its severity per mode; rules without an explicit declaration
// default to error in both modes. Authoring mode never silently passes a
// build-mode failure; build mode never downgrades errors.

package agent

import "github.com/ddwht/parlay/core/internal/feedback"

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
	// Contested step ownership is a nudge, not a blocker — ownership still
	// resolves deterministically (deepest layer wins), so it warns in both
	// modes rather than failing the build.
	"adapter-supports-step-ambiguous-owner": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},
	"domain-operations-deprecated": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityError,
	},
	"capabilities-not-closed-form": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityError,
	},
	"testcases-source-refs-missing-legacy": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},
	// Warning in both modes for now, on the same reasoning that once
	// governed buildfile-models (since removed): every testcases.yaml in existence
	// was written before `file:` existed, so erroring would fail every
	// project at once over a fact none of them could have recorded. The
	// severity states the direction of travel while leaving them valid.
	"testcases-file-missing": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},
	// A v2 case with no criterion: records nothing about why it exists.
	// Warning in both modes while the field lands — every testcases.yaml was
	// generated before criterion-driven cases existed, so erroring would fail
	// every project at once over a fact none of them could have recorded. The
	// severity states the direction of travel; the vacuous/claims checks that
	// enforce a criterion's mechanics stay at the default error, because those
	// only fire once a case declares the fields at all.
	"testcases-case-criterion-missing": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},
	// A contract entry carries verify: criteria that no case discharges.
	// Warning in both modes while criterion-driven cases land: every
	// testcases.yaml was generated before criterion: existed, so its cases
	// cite nothing, and erroring would fail every project at once over a fact
	// none of them could have recorded. The severity states the direction of
	// travel; it graduates to error once projects have rebuilt with
	// criterion-carrying cases.
	"verify-criterion-uncovered": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},
	// Same reasoning again: required on generate, tolerated absent on
	// read, because every capabilities.yaml in existence predates the
	// field. See capabilities.schema.md's "Why `source:` exists".
	"capabilities-source-missing": {
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
	// Page-manifest references that do not resolve. Warnings in both modes:
	// a manifest listing a fragment its feature has not written yet is the
	// normal state of a page designed ahead of its features, and blocking it
	// would make the manifest unusable for the thing it exists to do.
	// view-page reports the same drift at assembly time.
	"page-fragment-unresolved": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},
	"page-has-no-fragments": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},
	// Two or more different features contribute fragments to the same
	// (page, region). A warning in both modes: two features stacking in one
	// region is legitimate when a page manifest or a supersedes: annotation
	// orders them, and blocking it would forbid the composition the tool is
	// meant to support. What it converts from silence into a named finding is
	// the "a working component never appears" mystery — the first feature's
	// assembly quietly winning over the second's.
	"surface-region-shared": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},
	// One architectural concept (a normalized Affects: value) is constrained
	// by fragments from two or more features. A warning in both modes: two
	// features holding invariants over the same concept is not by itself a
	// contradiction — it becomes one only when a single implementation cannot
	// satisfy both, which no lexical check can decide. What it converts from
	// silence into a named finding is the pair a reviewer should read together.
	"infrastructure-concept-shared": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},
	// A capabilities.yaml referencing an entity that no root model declares
	// but some feature's contribution proposes. A warning in both modes: the
	// reference is correct about where the project is going, and the only
	// thing outstanding is a review someone else has to do. Blocking it is
	// what forced two features in the regression run to ship placeholders
	// for an entity a third was about to introduce.
	"capabilities-entity-pending": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},
	// An intents.md with a feature header and no intent blocks is what
	// `parlay add-feature` writes. Refusing it while the designer is
	// still typing would make the validator useless for the artifact it
	// exists to check; refusing it at build is the point, because every
	// downstream artifact derives from those blocks.
	"no-intents": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityError,
	},
	// An amendment without acceptance bullets is legitimate for renames
	// and pure-prose changes — the field exists for behavior changes, and
	// only the author knows which kind this is. A warning in both modes
	// states the expectation without blocking the legitimate cases.
	"amendment-missing-acceptance": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},
	// A gap in the sequence numbers is what an archived-then-compacted
	// ledger legitimately looks like; a DUPLICATE sequence number is a
	// collision and stays at the default error severity under its own
	// code (amendment-out-of-sequence).
	"amendment-sequence-gap": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},
	// A later amendment edits a contract entry an earlier one also edits,
	// without naming the earlier in its supersedes:. A warning in both modes:
	// two amendments composing over the same entry is legitimate as long as an
	// ordering exists between them, and only the author knows whether this one
	// replaces the earlier or genuinely stacks on it. Naming the earlier in
	// supersedes: silences it; that is the intended resolution, not a
	// workaround.
	"amendment-scope-overlap": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},
	// A decisions: entry names an enforcing file that exists but does not
	// carry the decision id. A warning in both modes: the code still runs and
	// the buildfile still records the reason — what is missing is the link
	// from one to the other, which is a documentation gap a reader can close,
	// not a build-blocking defect. Blocking it would also fail a buildfile the
	// instant a decision is recorded ahead of the codegen pass that will write
	// the id into the file, which is the normal authoring order.
	"rationale-stranded": {
		ModeAuthoring: SeverityWarning,
		ModeBuild:     SeverityWarning,
	},
}

// unknown-turn-form is deliberately absent from this table, so it is an
// error in both modes. A half-typed turn does not reach the check — the
// pattern requires a complete speaker-and-colon shape — so what it
// catches is a finished line in the wrong form, which is a mistake at
// any stage of authoring, not an intermediate state.

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
	out := ValidationOutcome{
		Mode:     mode,
		Code:     code,
		Severity: RuleSeverity(code, mode),
		Message:  message,
	}
	// Recorded at construction rather than at each emit site, which is a
	// deliberate trade. Construction is one place and cannot be forgotten
	// when a validator is added; the emit sites are a dozen and adding the
	// thirteenth without instrumenting it is exactly how coverage rots.
	//
	// The cost is that this records outcomes PRODUCED, not outcomes
	// surfaced — a warning that a caller filters out still appears here.
	// That is the honest word for it, and for the question the log is read
	// to answer ("which rules actually fire") it is also the better one.
	//
	// `message` is deliberately NOT passed. It is rendered from a template
	// this repo already owns, so the only new information in it is the
	// interpolated values — paths, operation ids, entity names, and in a
	// few places verbatim prose out of the user's spec. The code plus the
	// emitting symbol answer the same question without any of that.
	feedback.Record(feedback.FindingData{
		Code:     code,
		Mode:     string(mode),
		Severity: string(out.Severity),
		Site:     feedback.CallerSite(1),
	})
	return out
}
