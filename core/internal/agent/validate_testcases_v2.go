// parlay-feature: parlay-tool/multi-adapter
// parlay-component: testcases-v2
//
// Validates testcases.yaml v2 — the discriminated-suite-kind shape with
// {presentation, operation} kinds, source_refs[] requirement, and
// operation-coverage walker.

package agent

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// SuiteKind is the closed discriminator over v2 suite kinds.
type SuiteKind string

const (
	SuiteKindPresentation SuiteKind = "presentation"
	SuiteKindOperation    SuiteKind = "operation"
)

var validSuiteKinds = map[SuiteKind]bool{
	SuiteKindPresentation: true,
	SuiteKindOperation:    true,
}

// closedSetCaseActions and closedSetCaseVerbs are the step vocabularies
// testcases.schema.md documents for cases[].steps[]. Both were closed sets on
// paper and unchecked in practice: Cases decoded into []map[string]yaml.Node and
// nothing looked inside, so a suite could specify `action: hover` and validate
// cleanly. The agent generating tests from it then has to guess whether the term
// means something, which is the failure this closes — an unknown verb is not
// obviously wrong to a reader, only to the runner.
var closedSetCaseActions = map[string]bool{
	"render": true, "click": true, "input": true,
	"select": true, "navigate": true, "wait": true,
	// appears asserts that a component reached the renderer at a given depth —
	// the composition-visibility vocabulary behind Theme 1's E. The level lives
	// in the step's value:, drawn from closedSetAppearsLevels.
	"appears": true,
}

// closedSetAppearsLevels is the depth an `appears` step asserts to: mounted (a
// mount point exists), output (the component produced output), content (the
// declared content reached the renderer — e.g. a rendered row or triangle
// count). Pixels are deliberately out of scope. The store was correct in every
// composition defect these levels exist to catch, so no state assertion could
// have caught them; only a render-level fact can.
var closedSetAppearsLevels = map[string]bool{
	"mounted": true, "output": true, "content": true,
}

// closedSetCoverage is the honesty marker on a case: full when the criterion
// compiled to an assertion at its own altitude, state-only when a display-shaped
// criterion could only compile to a store-level assertion because the adapter
// cannot deliver `appears` yet. Stamping the downgrade is part E — a weaker
// claim the coverage reviewer sees instead of a silent one.
var closedSetCoverage = map[string]bool{
	"full": true, "state-only": true,
}

// The set is the union of two lists testcases.schema.md carries, and the union
// is deliberate.
//
// The inline example under "Suite structure" shows nine verifies; the
// Verifications table under "Step types" lists twelve. They overlap but neither
// contains the other — the example has hidden and disabled, the table has class,
// file-exists, file-content, directory-exists and error. The generator emits from
// both: real testcases.yaml files in a project use hidden (example-only) and
// error, file-exists, file-content (table-only) in the same file.
//
// Taking the example's list alone, which is what this first did, rejected seven
// steps in a single real generated file. Taking the table's alone would reject
// every `verify: hidden`. So the check accepts the union, and the schema's two
// lists have been reconciled to match it — the drift between them is what made
// the shorter list look authoritative.
var closedSetCaseVerbs = map[string]bool{
	// Rendering and content.
	"element": true, "text": true, "count": true, "class": true,
	// Visibility and interactivity, in both polarities the two lists use.
	"visible": true, "hidden": true, "enabled": true, "disabled": true,
	// Model and navigation.
	"state": true, "route": true,
	// Filesystem and failure, for adapters whose output is files rather than a
	// rendered DOM.
	"file-exists": true, "file-content": true, "directory-exists": true,
	"error": true,
}

// closedSetVerbList renders the accepted verifies for diagnostics, sorted so the
// message is stable across runs.
var closedSetVerbList = sortedSetKeys(closedSetCaseVerbs)

// sortedSetKeys returns a set's keys joined for display.
func sortedSetKeys(set map[string]bool) string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

// caseStepShape decodes the two mutually-exclusive step forms. A step is either
// an action (something the test does) or a verify (something it asserts); the
// schema's example alternates them within one steps: list. Value and Expected
// were previously dropped on the floor — the loose decode read only the verb and
// its target — so nothing could hold a case's mechanics against its declaration.
type caseStepShape struct {
	Action   string `yaml:"action,omitempty"`
	Verify   string `yaml:"verify,omitempty"`
	Target   string `yaml:"target,omitempty"`
	Value    string `yaml:"value,omitempty"`
	Expected string `yaml:"expected,omitempty"`
}

// caseCriterion is the reason a case exists: the verify: entry it discharges,
// with the criterion text pinned so a later contract edit shows as drift.
type caseCriterion struct {
	Ref  string `yaml:"ref"`
	Text string `yaml:"text,omitempty"`
}

// validateSuiteCases checks the cases[] a suite declares. Beyond naming and step
// vocabulary it holds each case's declared mechanics against its steps: a case
// that says it exercises a target must have a step that touches it (vacuous
// otherwise), and a case's assertions must read only targets it declares it
// observes (claims-unmet otherwise). isV2 gates the criterion warning — legacy
// v1 suites predate the field and are already flagged by their own legacy code.
//
// The second return is every criterion ref the suite's cases discharge — the
// set the criterion-coverage walker holds the contract's verify: entries
// against. It is collected here rather than re-decoded in the caller because the
// criterion node is already decoded for the missing-ref check.
func validateSuiteCases(mode ValidationMode, path, suiteName string, isV2 bool, cases []map[string]yaml.Node) ([]ValidationOutcome, []string) {
	var outcomes []ValidationOutcome
	var criterionRefs []string
	for i, c := range cases {
		label := fmt.Sprintf("case %d", i+1)
		if nameNode, ok := c["name"]; ok {
			var name string
			if err := nameNode.Decode(&name); err == nil && strings.TrimSpace(name) != "" {
				label = fmt.Sprintf("case %q", name)
			} else {
				outcomes = append(outcomes, NewOutcome(mode, "testcases-case-unnamed",
					fmt.Sprintf("%s: suite %q %s has an empty name — the name is what a failing test reports, so an unnamed case fails anonymously", path, suiteName, label)))
			}
		} else {
			outcomes = append(outcomes, NewOutcome(mode, "testcases-case-unnamed",
				fmt.Sprintf("%s: suite %q %s declares no name", path, suiteName, label)))
		}

		// A v2 case with no criterion records nothing about why it exists.
		// Warning while the field lands — every testcases.yaml predates it.
		if isV2 {
			var crit caseCriterion
			if node, ok := c["criterion"]; !ok {
				outcomes = append(outcomes, NewOutcome(mode, "testcases-case-criterion-missing",
					fmt.Sprintf("%s: suite %q %s declares no criterion: — nothing records which verify: entry it discharges; regenerate with `parlay build-feature` to derive it", path, suiteName, label)))
			} else if err := node.Decode(&crit); err != nil || strings.TrimSpace(crit.Ref) == "" {
				outcomes = append(outcomes, NewOutcome(mode, "testcases-case-criterion-missing",
					fmt.Sprintf("%s: suite %q %s has a criterion: with no ref — the ref cites the @feature/kind:name verify: entry the case discharges", path, suiteName, label)))
			} else {
				criterionRefs = append(criterionRefs, strings.TrimSpace(crit.Ref))
			}
		}

		// Declared mechanics: exercises are the targets steps must mutate,
		// observes the targets expectations may read. Optional, but held
		// against the steps once present.
		var exercises, observes []string
		if node, ok := c["exercises"]; ok {
			_ = node.Decode(&exercises)
		}
		if node, ok := c["observes"]; ok {
			_ = node.Decode(&observes)
		}

		// coverage: is the state-only honesty marker. When present it must name
		// a known altitude; an unknown value hides whether the claim was
		// downgraded, which is the exact thing the marker exists to make visible.
		if node, ok := c["coverage"]; ok {
			var coverage string
			if err := node.Decode(&coverage); err == nil {
				coverage = strings.TrimSpace(coverage)
				if coverage != "" && !closedSetCoverage[coverage] {
					outcomes = append(outcomes, NewOutcome(mode, "testcases-coverage-unknown",
						fmt.Sprintf("%s: suite %q %s declares coverage %q, outside {full, state-only}", path, suiteName, label, coverage)))
				}
			}
		}

		stepsNode, ok := c["steps"]
		if !ok {
			continue
		}
		var steps []caseStepShape
		if err := stepsNode.Decode(&steps); err != nil {
			// Shape problems belong to the YAML validator; this check is about
			// vocabulary, and guessing at a malformed list would report the
			// same problem twice in different words.
			continue
		}
		stepTargets := make(map[string]bool)
		for j, st := range steps {
			if t := strings.TrimSpace(st.Target); t != "" {
				stepTargets[t] = true
			}
			if st.Action != "" && !closedSetCaseActions[st.Action] {
				outcomes = append(outcomes, NewOutcome(mode, "testcases-unknown-term",
					fmt.Sprintf("%s: suite %q %s step %d action %q is outside the closed set {render, click, input, select, navigate, wait, appears}", path, suiteName, label, j+1, st.Action)))
			}
			// An appears step names its depth in value:; an unknown or missing
			// level makes the assertion unrunnable — the runner cannot decide
			// what "appears" means without knowing whether it must reach a
			// mount point, an output, or content.
			if st.Action == "appears" {
				level := strings.TrimSpace(st.Value)
				if level == "" || !closedSetAppearsLevels[level] {
					outcomes = append(outcomes, NewOutcome(mode, "testcases-appears-level-unknown",
						fmt.Sprintf("%s: suite %q %s step %d appears value %q is outside the level set {mounted, output, content}", path, suiteName, label, j+1, st.Value)))
				}
			}
			if st.Verify != "" && !closedSetCaseVerbs[st.Verify] {
				outcomes = append(outcomes, NewOutcome(mode, "testcases-unknown-term",
					fmt.Sprintf("%s: suite %q %s step %d verify %q is outside the closed set {%s}", path, suiteName, label, j+1, st.Verify, closedSetVerbList)))
			}
			if st.Action == "" && st.Verify == "" {
				outcomes = append(outcomes, NewOutcome(mode, "testcases-unknown-term",
					fmt.Sprintf("%s: suite %q %s step %d declares neither action: nor verify: — a step either does something or asserts something", path, suiteName, label, j+1)))
			}
		}

		// Vacuity: a case that names things it exercises but touches none of
		// them in any step asserts on nothing it claims to.
		if len(exercises) > 0 {
			touched := false
			var missing []string
			for _, ex := range exercises {
				ex = strings.TrimSpace(ex)
				if ex == "" {
					continue
				}
				if stepTargets[ex] {
					touched = true
				} else {
					missing = append(missing, ex)
				}
			}
			if !touched {
				outcomes = append(outcomes, NewOutcome(mode, "testcases-case-vacuous",
					fmt.Sprintf("%s: suite %q %s declares exercises: %v but no step targets any of them — the case acts on nothing it claims to", path, suiteName, label, missing)))
			}
		}

		// Claims: an expectation may only read a target the case declares it
		// observes. Checked when observes: is declared; without it there is
		// nothing to hold the assertions against.
		if len(observes) > 0 {
			observed := make(map[string]bool, len(observes))
			for _, o := range observes {
				observed[strings.TrimSpace(o)] = true
			}
			for j, st := range steps {
				if st.Verify == "" {
					continue
				}
				t := strings.TrimSpace(st.Target)
				if t == "" {
					continue
				}
				if !observed[t] {
					outcomes = append(outcomes, NewOutcome(mode, "testcases-case-claims-unmet",
						fmt.Sprintf("%s: suite %q %s step %d asserts on %q, which is outside its declared observes: %v — the assertion reads something the case does not admit it observes", path, suiteName, label, j+1, t, observes)))
				}
			}
		}
	}
	return outcomes, criterionRefs
}

type testcasesV2Shape struct {
	SchemaVersion int            `yaml:"schema_version"`
	Feature       string         `yaml:"feature"`
	Suites        []suiteV2Shape `yaml:"suites"`
}

type suiteV2Shape struct {
	Name string `yaml:"name"`
	// File is where this suite's generated code goes — the plan row
	// scaffold-plan derived from file-conventions.paths.test. Optional on
	// read so every testcases.yaml written before the field existed keeps
	// loading; required on generate, which is the build phase's job to
	// satisfy and generate-code's to refuse without.
	File       string                 `yaml:"file,omitempty"`
	Kind       string                 `yaml:"kind,omitempty"` // v2 only
	Component  string                 `yaml:"component,omitempty"`
	Operation  string                 `yaml:"operation,omitempty"` // v2 operation suites
	Intent     string                 `yaml:"intent,omitempty"`    // v1 legacy
	SourceRefs []string               `yaml:"source_refs,omitempty"`
	Output     map[string]yaml.Node   `yaml:"output,omitempty"`
	Cases      []map[string]yaml.Node `yaml:"cases,omitempty"`
}

// TestcasesV2Input carries everything ValidateTestcasesV2 needs beyond the run
// mode. The two coverage inputs are derived by the caller from the feature's
// contract artifacts, and both are empty when no such artifact resolves — a
// feature with no capabilities.yaml or surface.yaml is a normal state, and an
// empty input disables only its own walker rather than reporting everything as
// covered. Mirrors CoverageReviewInputs: mode stays a separate positional arg,
// the rest travels in one struct so a new coverage source does not re-shape
// every call site.
type TestcasesV2Input struct {
	Path    string
	Content []byte
	// CanonicalOperations are the @<feature>/operation:<id> refs the coverage
	// walker holds operation-suite coverage against — every operation the
	// feature's capabilities.yaml declares.
	CanonicalOperations []string
	// Criteria are the contract entries carrying a verify: list, each as its
	// @<feature>/<kind>:<name> ref. Every one must be discharged by a case whose
	// criterion.ref matches it, or excused in ExemptCriteria; the criterion
	// walker fires verify-criterion-uncovered for each that is neither.
	Criteria []string
	// ExemptCriteria are refs a human review (coverage-review.yaml) has excused
	// from needing a covering case. A ref present here is never reported
	// uncovered.
	ExemptCriteria map[string]bool
}

// ValidateTestcasesV2 validates the v2 shape, walks operation coverage against
// the supplied canonical operation refs, and walks criterion coverage against
// the supplied contract criteria.
func ValidateTestcasesV2(mode ValidationMode, in TestcasesV2Input) []ValidationOutcome {
	path := in.Path
	var outcomes []ValidationOutcome

	var tc testcasesV2Shape
	if err := yaml.Unmarshal(in.Content, &tc); err != nil {
		// Upstream YAML validator handles parse errors.
		return outcomes
	}

	covered := make(map[string]bool)
	criteriaCovered := make(map[string]bool)
	for _, suite := range tc.Suites {
		// The cases vocabulary is version-independent, so this runs before the
		// discriminator check and for legacy suites too — putting it after the
		// `continue` below would have silently exempted every v1 suite, which is
		// most of what exists in projects today.
		caseOutcomes, refs := validateSuiteCases(mode, path, suite.Name, suite.Kind != "", suite.Cases)
		outcomes = append(outcomes, caseOutcomes...)
		for _, ref := range refs {
			criteriaCovered[ref] = true
		}

		// Discriminator check. The v1 shape (no kind:) stopped being
		// accepted in v0.3 — its policy has always been regenerate, so the
		// fix is one build-feature run, and continuing to half-load it as
		// presentation kept a shim alive that no gate ever exercised.
		kind := suite.Kind
		if kind == "" {
			outcomes = append(outcomes, NewOutcome(mode, "testcases-v1-unsupported",
				fmt.Sprintf("%s: suite %q has no kind: — the v1 testcases shape was removed in v0.3; re-run /parlay-build-feature to regenerate the v2 form", path, suite.Name)))
			continue
		}

		if !validSuiteKinds[SuiteKind(kind)] {
			outcomes = append(outcomes, NewOutcome(mode, "testcases-suite-kind-unknown",
				fmt.Sprintf("%s: suite %q kind %q is outside the v2 set {presentation, operation}", path, suite.Name, kind)))
			continue
		}

		// New v2 suites must declare source_refs.
		if len(suite.SourceRefs) == 0 {
			outcomes = append(outcomes, NewOutcome(mode, "testcases-source-refs-missing",
				fmt.Sprintf("%s: v2 suite %q declares no source_refs (every v2 suite must cite at least one)", path, suite.Name)))
		}

		// And a v2 suite must say where its code goes. Checked on v2 only:
		// every legacy v1 suite predates the field, and erroring on those
		// would fail every project that has not rebuilt yet over a fact
		// they could not have recorded.
		//
		// Warning rather than error while the field lands, for the reason
		// buildfile.schema.md gives about `models:` — every testcases.yaml
		// in existence was written without it, so erroring would fail them
		// all at once. The severity states the direction of travel.
		if strings.TrimSpace(suite.File) == "" {
			outcomes = append(outcomes, NewOutcome(mode, "testcases-file-missing",
				fmt.Sprintf("%s: v2 suite %q declares no file: — generate-code would have to invent a path, which is how two components' tests end up in two places; rebuild with `parlay build-feature` to populate it from the plan", path, suite.Name)))
		}

		if SuiteKind(kind) == SuiteKindOperation {
			if suite.Operation == "" {
				outcomes = append(outcomes, NewOutcome(mode, "testcases-operation-shape-mismatch",
					fmt.Sprintf("%s: operation suite %q has no operation: reference", path, suite.Name)))
				continue
			}
			covered[suite.Operation] = true
		}
	}

	// Coverage walker — every canonical operation must have a covering
	// operation suite.
	for _, op := range in.CanonicalOperations {
		if !covered[op] {
			outcomes = append(outcomes, NewOutcome(mode, "testcases-operation-uncovered",
				fmt.Sprintf("%s: operation %q has no covering kind: operation suite", path, op)))
		}
	}

	// Criterion walker — every contract entry carrying a verify: list must be
	// discharged by a case that cites it, or excused by an explicit exemption.
	// This is the complement of the operation walker: that one asks whether a
	// suite exists per operation, this one asks whether a case exists per stated
	// acceptance criterion. Warning severity while the field lands — every
	// testcases.yaml predates criterion:, so its cases cite nothing yet and an
	// error would fail every project at once over a fact none could have
	// recorded.
	for _, ref := range in.Criteria {
		if criteriaCovered[ref] || in.ExemptCriteria[ref] {
			continue
		}
		outcomes = append(outcomes, NewOutcome(mode, "verify-criterion-uncovered",
			fmt.Sprintf("%s: contract entry %q carries verify: criteria but no case discharges it — a case must cite it in criterion.ref, or coverage-review.yaml must exempt it", path, ref)))
	}

	return outcomes
}
