package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
}

type compositionFinding struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Entity  string   `json:"entity,omitempty"`
	ID      string   `json:"id,omitempty"`
	Field   string   `json:"field,omitempty"`
	Sites   []string `json:"sites,omitempty"`
}

type compositionOutput struct {
	Features []string             `json:"features"`
	Records  int                  `json:"fixture_records"`
	Findings []compositionFinding `json:"findings"`
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
func collectFixtureRecords(buildDir string) ([]entityRecord, []string, error) {
	entries, err := os.ReadDir(buildDir)
	if err != nil {
		return nil, nil, err
	}
	var records []entityRecord
	var features []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		path := filepath.Join(buildDir, e.Name(), "buildfile.yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var bf fixtureBuildfile
		if err := yaml.Unmarshal(data, &bf); err != nil {
			continue
		}
		feature := bf.Feature
		if feature == "" {
			feature = e.Name()
		}
		features = append(features, feature)

		for fxName, fx := range bf.Fixtures {
			for entity, rows := range fx.Data {
				for _, row := range rows {
					id, _ := row["id"].(string)
					if id == "" {
						continue
					}
					records = append(records, entityRecord{
						Entity: entity, ID: id, Fields: row,
						Feature: feature, Fixture: fxName,
					})
				}
			}
		}
	}
	sort.Strings(features)
	return records, features, nil
}

// findContradictions reports the same entity id carrying different values
// for the same field in different features.
//
// This is P5-2. "Hamburg training" was `submitted` in two features and
// `rejected` in a third. Each feature's own tests passed against its own
// fixture, so the contradiction was invisible per-feature and obvious the
// moment a user clicked from one page to another.
func findContradictions(records []entityRecord) []compositionFinding {
	type key struct{ entity, id, field string }
	seen := map[key]map[string][]string{} // value -> sites

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
				seen[k] = map[string][]string{}
			}
			val := fmt.Sprint(v)
			site := fmt.Sprintf("%s/%s", r.Feature, r.Fixture)
			seen[k][val] = append(seen[k][val], site)
		}
	}

	var out []compositionFinding
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
		var parts []string
		var sites []string
		for val, s := range values {
			sort.Strings(s)
			parts = append(parts, fmt.Sprintf("%q in %s", val, strings.Join(s, ", ")))
			sites = append(sites, s...)
		}
		sort.Strings(parts)
		sort.Strings(sites)
		out = append(out, compositionFinding{
			Code:   "composition-fixture-contradiction",
			Entity: k.entity, ID: k.id, Field: k.field,
			Sites: sites,
			Message: fmt.Sprintf("%s %s has conflicting %s across features: %s. "+
				"Each feature's tests pass against its own fixture; a user navigating between them sees both values.",
				k.entity, k.id, k.field, strings.Join(parts, "; ")),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Entity != out[j].Entity {
			return out[i].Entity < out[j].Entity
		}
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Field < out[j].Field
	})
	return out
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
	buildDir := filepath.Dir(cfg.BuildPath("_probe"))

	records, features, err := collectFixtureRecords(buildDir)
	if err != nil {
		return fmt.Errorf("read build state under %s: %w", buildDir, err)
	}

	findings := append(findContradictions(records), findDanglingReferences(records)...)
	out := compositionOutput{
		Features: features,
		Records:  len(records),
		Findings: findings,
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
// field come from two different features. Sites are "feature/fixture", so
// the feature is the part before the slash.
func spansMultipleFeatures(values map[string][]string) bool {
	featuresFor := func(sites []string) map[string]bool {
		out := map[string]bool{}
		for _, s := range sites {
			out[strings.SplitN(s, "/", 2)[0]] = true
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
