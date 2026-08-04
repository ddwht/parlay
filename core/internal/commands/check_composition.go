package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// checkCompositionCmd validates the runtime the features compose into,
// rather than each feature against its own spec.
//
// This is the gap the ExpenseFlow regression run was built around finding.
// Every artifact validated cleanly in isolation and the composed application
// was still incoherent: four features declared four fixtures for the same
// report and disagreed about its status, the login form offered a persona no
// fixture defined, and a rejected report carried no approval record. Every
// one of those passed `check-buildfile`, `validate --deep`, and 483 testcase
// assertions, because nothing ever looked at two features at once.
//
// build-feature's unit of work is the component and nothing owns the
// composition. This command owns it.
var checkCompositionCmd = &cobra.Command{
	Use:   "check-composition",
	Short: "Validate cross-feature fixture coherence — the composed runtime (JSON output)",
	Args:  cobra.NoArgs,
	RunE:  runCheckComposition,
}

// entityRecord is one fixture row, remembered with where it came from so a
// disagreement can name both sides.
type entityRecord struct {
	Entity  string
	ID      string
	Fields  map[string]interface{}
	Feature string
	Fixture string
	// Composing says whether this record's fixture is the one its feature
	// contributes to the composed runtime seed — `composes: true`, or the
	// designation agent.ComposingFixture infers from the route suites. A
	// record from any other fixture describes a scenario the running
	// prototype never boots into, so two such records cannot both be on
	// screen and cannot contradict each other in the sense this command
	// exists to catch. Undesignated counts as non-composing: a feature
	// mid-authoring, or one whose designation is ambiguous, degrades to a
	// note rather than to a failure or to silence.
	Composing bool
}

// recordSite is where a value came from, keeping feature and fixture as
// separate fields rather than one "feature/fixture" string. The string form
// is for display only; nothing derives the feature back out of it. See
// spansMultipleFeatures for why that distinction is load-bearing.
type recordSite struct {
	Feature   string
	Fixture   string
	Composing bool
}

func (s recordSite) String() string { return s.Feature + "/" + s.Fixture }

type compositionFinding struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Entity  string   `json:"entity,omitempty"`
	ID      string   `json:"id,omitempty"`
	Field   string   `json:"field,omitempty"`
	Sites   []string `json:"sites,omitempty"`
}

type compositionOutput struct {
	// Features are the features that actually contributed fixture records.
	Features []string `json:"features"`
	// Examined is how many features the walk considered — every feature the
	// canonical enumeration returns, built or not. Reported separately from
	// Features so a caller can see when the two disagree: a coherent verdict
	// over a subset is not a coherent verdict over the project, and the
	// previous single-level walk returned exactly that with nothing to
	// indicate it. Compare against `parlay status`, whose feature count is
	// deliberately the same number — it lists hand-authored units beneath
	// its features but does not add them in.
	//
	// Units are absent here for the same reason, and their absence is
	// correct rather than an omission to repair: the canonical enumeration
	// returns features, a unit produces no buildfile and therefore no
	// fixture records, and there is no coherence question to ask about it.
	// Before units were classified, one appeared here as a permanent
	// composition-feature-unbuilt note — a by-design fact reported forever
	// as a coverage gap, which is how a notes list stops being read.
	Examined int                  `json:"features_examined"`
	Records  int                  `json:"fixture_records"`
	Findings []compositionFinding `json:"findings"`
	// Notes carry facts about coverage rather than incoherence — chiefly a
	// feature that has intents but no buildfile. They are reported because
	// the previous walk's silence about them is what let a half-built project
	// look identical to a complete one, but they deliberately do NOT flip
	// Coherent: an unbuilt feature contributes nothing to the composed
	// runtime, so it cannot make the features that DID contribute disagree.
	// Folding it into the verdict would make this command fail on every
	// project mid-build, which is the normal state during a pipeline run.
	Notes    []compositionFinding `json:"notes,omitempty"`
	Coherent bool                 `json:"coherent"`
}

type fixtureBuildfile struct {
	Feature  string `yaml:"feature"`
	Fixtures map[string]struct {
		Data map[string][]map[string]interface{} `yaml:"data"`
	} `yaml:"fixtures"`
}

// collectFixtureRecords reads every feature's buildfile and flattens its
// fixture data into one list of records.
//
// The feature list is passed in rather than discovered here. It used to be
// discovered, by a single-level os.ReadDir over the build root, and that
// silently dropped every initiative-scoped feature: `approvals/review-queue`
// has no buildfile at depth 1, so the walk `continue`d past the initiative
// directory and never descended. The command then reported `coherent: true`
// having examined half the project — the exact failure mode a coherence gate
// exists to prevent. Enumeration now happens at the call site, against the
// same canonical helper `status` and `diff` use, so the two cannot drift
// apart again without the guard test noticing.
func collectFixtureRecords(cfg *config.Context, features []string) ([]entityRecord, []string, []compositionFinding) {
	var records []entityRecord
	var contributing []string
	var findings []compositionFinding

	for _, slug := range features {
		path := filepath.Join(cfg.BuildPath(slug), "buildfile.yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			// A feature with intents but no buildfile has not been built. It
			// contributes nothing to the composed runtime, which is a fact
			// about coverage worth reporting rather than an absence to pass
			// over in silence — the previous walk simply skipped it, so a
			// half-built project and a fully-built one looked identical.
			findings = append(findings, compositionFinding{
				Code: "composition-feature-unbuilt",
				Message: fmt.Sprintf("%s has no buildfile, so it contributes nothing to the composed runtime. "+
					"Coherence was checked over the remaining features only.", slug),
				Sites: []string{slug},
			})
			continue
		}
		var bf fixtureBuildfile
		if err := yaml.Unmarshal(data, &bf); err != nil {
			findings = append(findings, compositionFinding{
				Code:    "composition-buildfile-unreadable",
				Message: fmt.Sprintf("%s: cannot parse buildfile.yaml: %v", slug, err),
				Sites:   []string{slug},
			})
			continue
		}
		// The slug is authoritative. The buildfile's own `feature:` field is
		// advisory — it may carry the bare directory name on a nested feature
		// (see buildfile.schema.md's `feature:` rule), and using it would
		// reintroduce the ambiguity this fix removes.
		contributing = append(contributing, slug)

		// Which of this feature's fixtures reaches the composed runtime.
		// Asked of agent.ComposingFixture — already exported, and already
		// what `scaffold-seed` and `validate --project` consult — rather
		// than decoded again here. fixtureBuildfile above has no
		// `composes` field at all, and that blind spot is the whole
		// defect: two decoders over one file meant scaffold-seed and this
		// command could return opposite verdicts under the same code
		// string. An ambiguous or absent designation yields "", so every
		// fixture in that feature counts as non-composing.
		composingFixture, _ := agent.ComposingFixture(cfg.BuildPath(slug))

		for fxName, fx := range bf.Fixtures {
			for entity, rows := range fx.Data {
				for _, row := range rows {
					id, _ := row["id"].(string)
					if id == "" {
						continue
					}
					records = append(records, entityRecord{
						Entity: entity, ID: id, Fields: row,
						Feature: slug, Fixture: fxName,
						Composing: composingFixture != "" && fxName == composingFixture,
					})
				}
			}
		}
	}
	sort.Strings(contributing)
	sort.Slice(findings, func(i, j int) bool { return findings[i].Message < findings[j].Message })
	return records, contributing, findings
}

// findContradictions reports the same entity id carrying different values
// for the same field in different features.
//
// This is P5-2. "Hamburg training" was `submitted` in two features and
// `rejected` in a third. Each feature's own tests passed against its own
// fixture, so the contradiction was invisible per-feature and obvious the
// moment a user clicked from one page to another.
//
// It returns two slices because the finding has two strengths. The check
// already ignores disagreements *within* one feature, on the grounds that
// alternative scenarios are supposed to differ — an empty draft and a
// submitted report are two states of the same id, and flagging that would
// bury the real finding. `composes: true` extends the same reasoning
// across features: a fixture that never reaches the composed seed
// describes a scenario the prototype never boots into, so it cannot
// coexist on screen with anything. Reporting it as a contradiction is
// what forced a run-3 build agent to renumber a scenario fixture for no
// reason.
//
// The two strengths are decided by PARTITIONING the group first, not by
// classifying it and then demoting. The demoting form was R4-16: a single
// `allComposing` flag was computed across every site in the group, spanning
// all values, so one non-composing scenario fixture anywhere in the group
// silently downgraded a genuine disagreement between two *composing*
// fixtures sitting beside it. The error the whole command exists to raise
// became a note because unrelated data was nearby.
//
// Partitioning also makes this agree with `scaffold-seed` by construction
// rather than by anyone keeping the two in step. agent.ComposingFixture
// returns at most one fixture per feature (see collectFixtureRecords), so
// every Composing site for a given feature comes from that one fixture, and
// therefore two distinct values among composing sites necessarily span two
// or more features. The composing-only comparison below is thus equivalent
// to deriveSeed's contradiction loop, and spansMultipleFeatures is
// redundant over that subset — it is kept as the outer guard because the
// NOTE still needs it.
//
// Both findings can fire for the same cell, which is correct: two composing
// fixtures disagreeing and a scenario fixture diverging are two true
// statements about the same field.
func findContradictions(records []entityRecord) (errors, notes []compositionFinding) {
	type key struct{ entity, id, field string }
	seen := map[key]map[string][]recordSite{} // value -> sites

	for _, r := range records {
		for field, v := range r.Fields {
			if field == "id" {
				continue
			}
			// Only scalars: comparing nested structures across features
			// produces noise about shapes that legitimately differ by what
			// each feature needed to load.
			if !isScalar(v) {
				continue
			}
			k := key{r.Entity, r.ID, field}
			if seen[k] == nil {
				seen[k] = map[string][]recordSite{}
			}
			val := fmt.Sprint(v)
			seen[k][val] = append(seen[k][val], recordSite{
				Feature: r.Feature, Fixture: r.Fixture, Composing: r.Composing,
			})
		}
	}

	for k, values := range seen {
		if len(values) < 2 {
			continue
		}
		// Only across features. Within one feature, fixtures that disagree
		// are the normal way to express alternative scenarios — an empty
		// draft and a submitted report are supposed to be different states
		// of the same id, and flagging that would bury the real finding in
		// noise. Two features disagreeing is different: nothing reconciles
		// them, and a user navigating between the two pages sees both.
		if !spansMultipleFeatures(values) {
			continue
		}
		// Partition first. The composing subset is the composed runtime: a
		// disagreement inside it is a disagreement a user actually sees.
		composing := map[string][]recordSite{}
		anyNonComposing := false
		for val, s := range values {
			for _, site := range s {
				if site.Composing {
					composing[val] = append(composing[val], site)
				} else {
					anyNonComposing = true
				}
			}
		}

		contradicted := len(composing) >= 2
		if contradicted {
			// Named over the composing sites ONLY. A refusal has to name the
			// specific conflict, and a scenario fixture that merely sits in
			// the same group is not part of it.
			parts, sites := describeValues(composing)
			errors = append(errors, compositionFinding{
				Code:   "composition-fixture-contradiction",
				Entity: k.entity, ID: k.id, Field: k.field,
				Sites: sites,
				Message: fmt.Sprintf("%s %s has conflicting %s across features: %s. "+
					"Each feature's tests pass against its own fixture; a user navigating between them sees both values.",
					k.entity, k.id, k.field, strings.Join(parts, "; ")),
			})
		}

		if anyNonComposing {
			parts, sites := describeValues(values)
			message := fmt.Sprintf("%s %s carries different %s values across features: %s. "+
				"At least one side is not the fixture its feature composes into the runtime seed, "+
				"so the two never coexist in the running prototype — reported rather than failed. "+
				"Mark a fixture `composes: true` if it is meant to reach the composed runtime.",
				k.entity, k.id, k.field, strings.Join(parts, "; "))
			if contradicted {
				// Do not tell the reader this was "reported rather than
				// failed" when the same cell just failed. The note is the
				// residue: the sides the running prototype never holds.
				message = fmt.Sprintf("%s %s carries different %s values across features: %s. "+
					"The disagreement between the composing fixtures is failed separately as "+
					"composition-fixture-contradiction; this note covers the remaining sides, which are not "+
					"the fixture their feature composes into the runtime seed and so never reach the running prototype. "+
					"Mark a fixture `composes: true` if it is meant to reach the composed runtime.",
					k.entity, k.id, k.field, strings.Join(parts, "; "))
			}
			notes = append(notes, compositionFinding{
				Code:   "composition-scenario-fixture-divergence",
				Entity: k.entity, ID: k.id, Field: k.field,
				Sites:   sites,
				Message: message,
			})
		}
	}
	sortCompositionFindings(errors)
	sortCompositionFindings(notes)
	return errors, notes
}

// describeValues renders a value→sites group into the two forms a finding
// needs: the quoted "value in feature/fixture" clauses its message reads
// out, and the flat site list it carries in JSON. Both are sorted, so the
// output does not depend on map iteration order.
//
// It takes the group as an argument rather than reading the enclosing one,
// because the contradiction and the divergence are now rendered over
// different subsets of the same cell.
func describeValues(values map[string][]recordSite) (parts, sites []string) {
	for val, s := range values {
		labels := make([]string, 0, len(s))
		for _, site := range s {
			labels = append(labels, site.String())
		}
		sort.Strings(labels)
		parts = append(parts, fmt.Sprintf("%q in %s", val, strings.Join(labels, ", ")))
		sites = append(sites, labels...)
	}
	sort.Strings(parts)
	sort.Strings(sites)
	return parts, sites
}

// sortCompositionFindings gives both grades of contradiction the same
// stable order, so two runs over the same project produce byte-identical
// JSON regardless of map iteration order.
func sortCompositionFindings(out []compositionFinding) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Entity != out[j].Entity {
			return out[i].Entity < out[j].Entity
		}
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Field < out[j].Field
	})
}

// findDanglingReferences reports a field whose value is shaped like another
// record's id but matches no record in any feature's fixtures.
//
// This is P4-2. The login form offered persona `emp-2` while the fixture
// defined `emp-3`, so the account-lockout flow it existed to demonstrate
// could not be reached — the credentials never matched a record. Nothing
// caught it because the login component and the fixture live in different
// features.
func findDanglingReferences(records []entityRecord) []compositionFinding {
	known := map[string]bool{}
	prefixes := map[string]bool{}
	for _, r := range records {
		known[r.ID] = true
		if i := strings.Index(r.ID, "-"); i > 0 {
			prefixes[r.ID[:i]] = true
		}
	}

	type dangle struct{ value, field string }
	sites := map[dangle][]string{}
	for _, r := range records {
		for field, v := range r.Fields {
			if field == "id" {
				continue
			}
			s, ok := v.(string)
			if !ok || known[s] {
				continue
			}
			// Only values that look like ids of a kind some fixture does
			// define. Without the prefix test every description string
			// would be a candidate.
			i := strings.Index(s, "-")
			if i <= 0 || !prefixes[s[:i]] {
				continue
			}
			d := dangle{s, field}
			sites[d] = append(sites[d], fmt.Sprintf("%s/%s (%s %s)", r.Feature, r.Fixture, r.Entity, r.ID))
		}
	}

	var out []compositionFinding
	for d, s := range sites {
		sort.Strings(s)
		out = append(out, compositionFinding{
			Code:  "composition-dangling-reference",
			ID:    d.value,
			Field: d.field,
			Sites: s,
			Message: fmt.Sprintf("%s references %q, which no fixture in any feature defines. "+
				"The flow that depends on it cannot be reached at runtime.", d.field, d.value),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Field < out[j].Field
	})
	return out
}

func isScalar(v interface{}) bool {
	switch v.(type) {
	case string, int, int64, float64, bool, nil:
		return true
	}
	return false
}

func runCheckComposition(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}

	// The canonical enumeration — the same one `status` and `diff` use. Doing
	// it here rather than inside the collector keeps the traversal in one
	// place across the whole codebase; a second hand-rolled walk is what
	// produced the bug this replaces.
	all, err := cfg.AllFeatures()
	if err != nil {
		return fmt.Errorf("enumerate features: %w", err)
	}

	records, contributing, notes := collectFixtureRecords(cfg, all)

	contradictions, divergences := findContradictions(records)
	notes = append(notes, divergences...)
	findings := append(contradictions, findDanglingReferences(records)...)

	// The composed seed against the model's own declared cardinalities.
	// This runs here rather than in the domain-model validator because
	// that validator is a pure function of one file's bytes — threading
	// fixtures into it would break the property that makes it usable on
	// stdin and in the editor. This command already has the context, the
	// walk and the composed records.
	//
	// A project with no domain model is a normal state, not a failure:
	// the check simply has nothing to hold the data against.
	if model, err := cfg.LoadDomainModel(); err == nil {
		cardErrs, cardNotes := findCardinalityViolations(model, records)
		findings = append(findings, cardErrs...)
		notes = append(notes, cardNotes...)
	}

	// Cross-feature flow assertions. Whether an unsatisfiable one is an error
	// or a note depends on whether the framework has a shared runtime at all,
	// which only the adapter can say — so the store path is read here and
	// passed down rather than assumed.
	storePath, warnOnly := sharedStorePath(cfg)
	flowFindings := findUnsatisfiableFlows(cfg, all, storePath)
	if warnOnly {
		// The adapter declares no store. A CLI has no shared runtime between
		// invocations and a static generator has none at all, so this is a
		// fact about the framework rather than a defect in the spec — it is
		// reported, loudly, without failing a build that could not have
		// succeeded differently.
		notes = append(notes, flowFindings...)
	} else {
		findings = append(findings, flowFindings...)
	}

	out := compositionOutput{
		Features: contributing,
		Examined: len(all),
		Records:  len(records),
		Findings: findings,
		Notes:    notes,
		Coherent: len(findings) == 0,
	}
	buf, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(cmd.OutOrStdout(), string(buf))
	if !out.Coherent {
		return NewExitCodeError(1)
	}
	return nil
}

// spansMultipleFeatures reports whether two different values for the same
// field come from two different features.
//
// The feature is carried on the site rather than recovered from a formatted
// string. It used to be recovered with strings.SplitN(site, "/", 2)[0] over a
// "feature/fixture" label, which is correct only while feature slugs contain
// no slash. Qualified identifiers do: "approvals/review-queue/three-reports"
// split that way yields "approvals", so both features under one initiative
// collapsed into a single name and a genuine disagreement between
// review-queue and approval-history compared equal and was suppressed. That
// bug was latent until the walk above started reading nested features at all,
// which is why the two fixes belong in one change.
func spansMultipleFeatures(values map[string][]recordSite) bool {
	featuresFor := func(sites []recordSite) map[string]bool {
		out := map[string]bool{}
		for _, s := range sites {
			out[s.Feature] = true
		}
		return out
	}
	var sets []map[string]bool
	for _, sites := range values {
		sets = append(sets, featuresFor(sites))
	}
	// Distinct values must be attributable to distinct features: some value
	// has to appear in a feature where another value does not.
	for i := range sets {
		for j := range sets {
			if i == j {
				continue
			}
			for f := range sets[i] {
				if !sets[j][f] {
					return true
				}
			}
		}
	}
	return false
}

// sharedStorePath returns the adapter's declared shared-store path, source-
// root-relative, and whether its absence should downgrade flow findings to
// notes.
//
// Absence is not an error — adapter.schema.md says so outright, and five of
// the seven bundled adapters declare no paths.store template. What absence
// means here is narrower and worth stating: the project has no mechanism to
// carry domain state across a feature boundary, so a cross-feature assertion
// is unsatisfiable for a reason nobody can fix by writing better code.
func sharedStorePath(cfg *config.Context) (string, bool) {
	adapterPath := presentationAdapterFile(cfg)
	if adapterPath == "" {
		return "", true
	}
	data, err := os.ReadFile(adapterPath)
	if err != nil {
		return "", true
	}
	var ad adapterForPlan
	if yaml.Unmarshal(data, &ad) != nil {
		return "", true
	}
	if ad.FileConventions.Paths.Store == "" {
		return "", true
	}
	return path.Join(ad.FileConventions.SourceRoot, ad.FileConventions.Paths.Store), false
}
