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
// schema's example alternates them within one steps: list.
type caseStepShape struct {
	Action string `yaml:"action,omitempty"`
	Verify string `yaml:"verify,omitempty"`
	Target string `yaml:"target,omitempty"`
}

// validateSuiteCases checks the cases[] a suite declares: every case is named,
// and every step's action/verify term is in its closed set.
func validateSuiteCases(mode ValidationMode, path, suiteName string, cases []map[string]yaml.Node) []ValidationOutcome {
	var outcomes []ValidationOutcome
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
		for j, st := range steps {
			if st.Action != "" && !closedSetCaseActions[st.Action] {
				outcomes = append(outcomes, NewOutcome(mode, "testcases-unknown-term",
					fmt.Sprintf("%s: suite %q %s step %d action %q is outside the closed set {render, click, input, select, navigate, wait}", path, suiteName, label, j+1, st.Action)))
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
	}
	return outcomes
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

// ValidateTestcasesV2 validates the v2 shape and walks operation coverage
// against the supplied list of canonical operation refs.
func ValidateTestcasesV2(mode ValidationMode, path string, content []byte, canonicalOperations []string) []ValidationOutcome {
	var outcomes []ValidationOutcome

	var tc testcasesV2Shape
	if err := yaml.Unmarshal(content, &tc); err != nil {
		// Upstream YAML validator handles parse errors.
		return outcomes
	}

	covered := make(map[string]bool)
	for _, suite := range tc.Suites {
		// The cases vocabulary is version-independent, so this runs before the
		// discriminator check and for legacy suites too — putting it after the
		// `continue` below would have silently exempted every v1 suite, which is
		// most of what exists in projects today.
		outcomes = append(outcomes, validateSuiteCases(mode, path, suite.Name, suite.Cases)...)

		// Discriminator check.
		kind := suite.Kind
		if kind == "" {
			// Legacy v1 — load as presentation. If source_refs is empty AND no
			// intent string, surface the legacy warning.
			if len(suite.SourceRefs) == 0 {
				if suite.Intent == "" {
					outcomes = append(outcomes, NewOutcome(mode, "testcases-source-refs-missing-legacy",
						fmt.Sprintf("%s: legacy v1 suite %q has neither source_refs nor intent — auto-population would be approximate", path, suite.Name)))
				} else {
					outcomes = append(outcomes, NewOutcome(mode, "testcases-source-refs-missing-legacy",
						fmt.Sprintf("%s: legacy v1 suite %q auto-populated source_refs from intent string — regenerate v2 testcases to clear this warning", path, suite.Name)))
				}
			}
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
	for _, op := range canonicalOperations {
		if !covered[op] {
			outcomes = append(outcomes, NewOutcome(mode, "testcases-operation-uncovered",
				fmt.Sprintf("%s: operation %q has no covering kind: operation suite", path, op)))
		}
	}

	return outcomes
}
