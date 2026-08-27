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
// The second return is every criterion the suite's cases discharge, as (ref,
// text) pairs — the set the criterion-coverage walker holds the contract's
// verify: bullets against. It is collected here rather than re-decoded in the
// caller because the criterion node is already decoded for the missing-ref
// check.
func validateSuiteCases(mode ValidationMode, revs ArtifactRevisions, path, suiteName, suiteKind string, cases []map[string]yaml.Node) ([]ValidationOutcome, []CriterionRef) {
	var outcomes []ValidationOutcome
	var criterionRefs []CriterionRef
	isV2 := suiteKind != ""
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
		var citedRef string
		if isV2 {
			var crit caseCriterion
			if node, ok := c["criterion"]; !ok {
				outcomes = append(outcomes, NewGraduatedOutcome(mode, revs, "testcases-case-criterion-missing",
					fmt.Sprintf("%s: suite %q %s declares no criterion: — nothing records which verify: entry it discharges; regenerate with `parlay build-feature` to derive it", path, suiteName, label)))
			} else if err := node.Decode(&crit); err != nil || strings.TrimSpace(crit.Ref) == "" {
				outcomes = append(outcomes, NewGraduatedOutcome(mode, revs, "testcases-case-criterion-missing",
					fmt.Sprintf("%s: suite %q %s has a criterion: with no ref — the ref cites the @feature/kind:name verify: entry the case discharges", path, suiteName, label)))
			} else {
				citedRef = strings.TrimSpace(crit.Ref)
				criterionRefs = append(criterionRefs, CriterionRef{
					Ref:  citedRef,
					Text: CanonicalCriterionText(crit.Text),
				})
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

		// Cross-kind citation: a presentation case may cite an operation's
		// criterion, but only when it actually invokes that operation.
		//
		// The suite's kind need not equal its criterion's owner — an operation
		// criterion can legitimately be observed end-to-end through the UI. What
		// this stops is the operation ref used as a SUBSTITUTE for a display
		// claim the fragment never stated, which is the tempting move when a
		// fragment carries no verify: at all.
		//
		// Only invocation is mechanizable. Whether the criterion is
		// contract-shaped, and so suitable for the presentation case citing it,
		// needs classification metadata that criteria do not carry; that half
		// stays an authoring rule in build-feature and a job for review.
		//
		// Membership is exact and deliberately not routed through exercises:.
		// The vacuity walker below fires only when NO step targets ANY declared
		// exercise, so listing the operation there proves nothing about the
		// steps.
		if SuiteKind(suiteKind) == SuiteKindPresentation && strings.Contains(citedRef, "/operation:") && !stepTargets[citedRef] {
			outcomes = append(outcomes, NewGraduatedOutcome(mode, revs, "testcases-cross-kind-criterion-unexercised",
				fmt.Sprintf("%s: presentation suite %q %s cites the operation criterion %q but no step targets that operation — a presentation case may discharge an operation's criterion only by invoking it, not as a stand-in for a display criterion the fragment never stated", path, suiteName, label, citedRef)))
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

// CriterionRef is one acceptance criterion's identity: the contract entry that
// declares it, plus the bullet's own text.
//
// The ref alone is NOT an identity. testcasesCoverageInputs used to append one
// ref per criterion-BEARING entry, so an operation with five verify: bullets
// contributed a single ref and one case citing it marked all five discharged.
// Two shipped claims were false as a result: build-feature's "cases come 1:1
// from verify: entries (coverage)" was unenforceable, and testcases.schema.md's
// promise that criterion.text "pins the criterion's wording so a later edit to
// the contract shows up as drift here" was untrue — the field was decoded and
// never read.
//
// Identity is the (ref, text) pair rather than an index or hash appended to the
// ref, because the contract those fields already state is that a wording edit
// invalidates the case citing the old wording. An indexed id would survive a
// re-wording, which is precisely the drift this is supposed to surface. It also
// needs no new syntax: both halves already exist and build-feature already
// writes them.
type CriterionRef struct {
	Ref  string
	Text string
}

// CanonicalCriterionText normalizes a criterion bullet for identity comparison.
//
// Deliberately narrow: surrounding whitespace and line endings only. It does
// NOT lowercase, collapse internal whitespace, or strip punctuation, because
// each of those can merge materially distinct claims — a spec that
// distinguishes a field name from a value by capitalization alone is unusual
// but not wrong, and a validator that silently unifies two such bullets reports
// coverage the tests do not have.
func CanonicalCriterionText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimSpace(s)
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
	// ContractResolved reports that the feature's contract artifacts were found
	// and read, whatever they turned out to contain.
	//
	// Separate from len(Criteria) > 0, and the distinction is the whole point of
	// this change. "No contract resolved" and "contract resolved and entirely
	// vacant" both yield zero criteria, but they are opposite situations: in the
	// first, a case's citations cannot be judged and reporting them as unknown
	// would be an artifact of the missing input; in the second — the state the
	// criteria-presence walker exists to surface — a case citing anything is
	// citing something the contract does not declare, which is exactly what
	// wants reporting.
	ContractResolved bool
	// Criteria are the individual verify: bullets the feature's contract
	// declares, each carrying the entry it came from and its own text. Every one
	// must be discharged by a case whose criterion (ref AND text) matches it, or
	// excused in ExemptCriteria; the criterion walker fires
	// verify-criterion-uncovered for each that is neither.
	Criteria []CriterionRef

	// Revisions carry the declared schema_version of the artifacts in play, so
	// the transitional diagnostics can graduate on a file a current generator
	// produced while still only warning on one that predates the field.
	Revisions ArtifactRevisions
	// ExemptCriteria are criteria a person deliberately excused, from the
	// feature's coverage-decisions.yaml, which has
	// excused from needing a covering case.
	//
	// An exemption naming a ref and a text excuses exactly that bullet. An
	// exemption naming only a ref is read as entry-wide — which is how every
	// exemption written before bullet-level coverage existed has to be read,
	// since none of them could have recorded a text.
	ExemptCriteria ExemptedCriteria
}

// ExemptedCriteria answers whether a given criterion is excused, honoring both
// the bullet-specific and the legacy entry-wide form.
type ExemptedCriteria struct {
	// Entries excuses every criterion on the named ref.
	Entries map[string]bool
	// Bullets excuses one (ref, canonical text) pair.
	Bullets map[CriterionRef]bool
}

// Excuses reports whether an exemption covers this criterion.
func (e ExemptedCriteria) Excuses(c CriterionRef) bool {
	if e.Entries[c.Ref] {
		return true
	}
	return e.Bullets[CriterionRef{Ref: c.Ref, Text: CanonicalCriterionText(c.Text)}]
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

	// The file's own declared revision is what graduates the transitional
	// diagnostics. It was decoded and never read until now — v1-versus-v2 is
	// discriminated by per-suite kind: — so the promised "graduates once
	// projects have rebuilt" had no way to arrive. A caller may also supply it
	// (the criteria come from other artifacts, and their revisions with them);
	// the file wins for its own.
	if tc.SchemaVersion > in.Revisions.Testcases {
		in.Revisions.Testcases = tc.SchemaVersion
	}

	covered := make(map[string]bool)
	// Cited criteria, keyed by the (ref, canonical text) identity.
	criteriaCovered := make(map[CriterionRef]bool)
	for _, suite := range tc.Suites {
		// The cases vocabulary is version-independent, so this runs before the
		// discriminator check and for legacy suites too — putting it after the
		// `continue` below would have silently exempted every v1 suite, which is
		// most of what exists in projects today.
		caseOutcomes, refs := validateSuiteCases(mode, in.Revisions, path, suite.Name, suite.Kind, suite.Cases)
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
			outcomes = append(outcomes, NewGraduatedOutcome(mode, in.Revisions, "testcases-file-missing",
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

	outcomes = append(outcomes, walkCriterionCoverage(mode, path, in, criteriaCovered)...)

	return outcomes
}

// walkCriterionCoverage holds the contract's verify: bullets against what the
// cases cite, at bullet granularity.
//
// Three distinct failures, where there used to be one. The old walker compared
// ref to ref and so could only ever answer "does any case mention this entry":
//
//   - a case cites a ref no contract entry declares — a miscitation. Nothing
//     reported this: an unknown ref simply failed to mark anything covered, so
//     it read as a coverage gap on some other entry.
//   - a case cites a known entry with a text that matches none of its current
//     bullets — drift, or a fabricated criterion. This is the check
//     testcases.schema.md has always described criterion.text as performing.
//   - a declared bullet no case discharges — the real uncovered criterion, now
//     counted per bullet rather than per entry.
func walkCriterionCoverage(mode ValidationMode, path string, in TestcasesV2Input, criteriaCovered map[CriterionRef]bool) []ValidationOutcome {
	var outcomes []ValidationOutcome

	// Every criterion the contract declares, indexed for the citation checks.
	//
	// Two identical bullets on one entry are indistinguishable under text
	// identity and carry no distinct meaning, so they are reported as the
	// authoring defect they are rather than given an invented index to tell
	// them apart. Reported once per duplicated pair, not once per occurrence.
	declaredRefs := make(map[string]bool)
	declaredPairs := make(map[CriterionRef]bool)
	dupeReported := make(map[CriterionRef]bool)
	for _, c := range in.Criteria {
		key := CriterionRef{Ref: c.Ref, Text: CanonicalCriterionText(c.Text)}
		declaredRefs[c.Ref] = true
		if declaredPairs[key] && !dupeReported[key] {
			dupeReported[key] = true
			outcomes = append(outcomes, NewOutcome(mode, "verify-criterion-duplicate",
				fmt.Sprintf("%s: contract entry %q declares the criterion %q more than once — two identical bullets assert the same thing and cannot be discharged separately; drop the duplicate or reword it into a distinct claim", path, c.Ref, key.Text)))
		}
		declaredPairs[key] = true
	}

	// The two citation-side failures. Sorted so the report is stable: map
	// iteration order would otherwise reorder findings between runs.
	var cited []CriterionRef
	for c := range criteriaCovered {
		cited = append(cited, c)
	}
	sort.Slice(cited, func(i, j int) bool {
		if cited[i].Ref != cited[j].Ref {
			return cited[i].Ref < cited[j].Ref
		}
		return cited[i].Text < cited[j].Text
	})
	// Citation checks run whenever the contract was READ, not whenever it turned
	// out to be non-empty. A resolved-but-vacant contract is the state this
	// whole change exists to make visible, and a case citing criteria that
	// contract does not declare is a miscitation there just as much as anywhere.
	//
	// textlessEntries are entries some case cites without a text. Their bullets
	// are deliberately NOT also reported uncovered: the citation genuinely
	// discharges nothing, but the cause is already named once by
	// testcases-criterion-text-missing, and repeating it per bullet turns one
	// actionable fact into N+1 warnings. Every testcases.yaml written before
	// criterion identity is in exactly this state, so that multiplier lands on
	// precisely the projects with the least to gain from it. Nothing is hidden
	// permanently: once the rebuild writes texts, any real gap surfaces.
	textlessEntries := make(map[string]bool)
	for _, c := range cited {
		if !in.ContractResolved {
			break
		}
		switch {
		case !declaredRefs[c.Ref]:
			outcomes = append(outcomes, NewGraduatedOutcome(mode, in.Revisions, "testcases-criterion-ref-unknown",
				fmt.Sprintf("%s: a case cites criterion.ref %q, which no contract entry declares — check the ref against capabilities.yaml and surface.yaml", path, c.Ref)))
		case c.Text == "":
			textlessEntries[c.Ref] = true
			outcomes = append(outcomes, NewGraduatedOutcome(mode, in.Revisions, "testcases-criterion-text-missing",
				fmt.Sprintf("%s: a case cites %q with no criterion.text — the text pins which of that entry's verify: bullets the case discharges, and without it coverage cannot be counted per criterion", path, c.Ref)))
		case !declaredPairs[c]:
			outcomes = append(outcomes, NewGraduatedOutcome(mode, in.Revisions, "testcases-criterion-text-drift",
				fmt.Sprintf("%s: a case cites %q with criterion.text %q, which matches none of that entry's current verify: bullets — either the contract was reworded after the case was written, or the criterion was invented", path, c.Ref, c.Text)))
		}
	}

	// The coverage side. Warning severity while the field lands — every
	// testcases.yaml predates criterion:, so its cases cite nothing yet and an
	// error would fail every project at once over a fact none could have
	// recorded.
	reportedUncovered := make(map[CriterionRef]bool)
	for _, c := range in.Criteria {
		key := CriterionRef{Ref: c.Ref, Text: CanonicalCriterionText(c.Text)}
		if criteriaCovered[key] || in.ExemptCriteria.Excuses(c) || reportedUncovered[key] {
			continue
		}
		if textlessEntries[c.Ref] {
			continue
		}
		reportedUncovered[key] = true
		outcomes = append(outcomes, NewGraduatedOutcome(mode, in.Revisions, "verify-criterion-uncovered",
			fmt.Sprintf("%s: criterion %q on contract entry %q has no case discharging it — a case must cite both in criterion.{ref,text}, or coverage-decisions.yaml must excuse it", path, c.Text, c.Ref)))
	}

	return outcomes
}
