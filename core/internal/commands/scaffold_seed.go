package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// scaffoldSeedCmd computes the composed runtime seed — the one dataset the
// whole prototype boots from.
//
// Before this, every feature emitted its own fixtures and the app booted from
// whichever service happened to hydrate first. Three things followed, and all
// three were visible to anyone using the prototype: the data you saw depended
// on which screen you entered from; a report could read "submitted" on one
// page and "rejected" on another; and keeping the features agreeing was
// manual work that nothing preserved across a rebuild.
//
// The seed is DERIVED, not authored. Deriving it is what makes coherence
// checkable rather than a thing someone remembered to do: the union is well
// defined exactly when the contributing fixtures agree, so a contradiction
// stops the derivation instead of silently picking a winner.
//
// Like scaffold-plan, this computes and prints — it writes nothing. The
// adapter's paths.seed template says where the file goes and the generating
// agent writes it in whatever shape the framework wants; parlay knows the
// data, not the rendering.
var scaffoldSeedCmd = &cobra.Command{
	Use:   "scaffold-seed",
	Short: "Compute the composed runtime seed from every feature's composing fixture (JSON output)",
	Args:  cobra.NoArgs,
	RunE:  runScaffoldSeed,
}

// seedRecord is one entity row in the composed seed.
type seedRecord struct {
	Entity string                 `json:"entity"`
	ID     string                 `json:"id"`
	Fields map[string]interface{} `json:"fields"`
	// From names every feature whose composing fixture contributed to this
	// record, so a reader can see which features share it.
	From []string `json:"from"`
}

type seedOutput struct {
	// Contributors maps each feature to the fixture it contributed.
	Contributors map[string]string `json:"contributors"`
	Records      []seedRecord      `json:"records"`
	Findings     []seedFinding     `json:"findings,omitempty"`
	// Derivable is false when any finding blocks: a seed is either the union
	// of agreeing fixtures or it does not exist. There is no partial seed.
	Derivable bool `json:"derivable"`
}

type seedFinding struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Sites   []string `json:"sites,omitempty"`
}

// seedBuildfile is the buildfile subset the seed derivation reads.
type seedBuildfile struct {
	Fixtures map[string]struct {
		// Composes marks this fixture as the one the app boots from.
		//
		// This is the one part of the seed that is not derivable, and it is
		// deliberately a single declared bit rather than a heuristic. A
		// feature has several fixtures and they are SUPPOSED to disagree —
		// an empty state and a populated one are different scenarios of the
		// same ids, which is the documented intra-feature rule. Unioning all
		// of them would manufacture exactly the contradictions this command
		// exists to detect.
		Composes bool                                `yaml:"composes"`
		Data     map[string][]map[string]interface{} `yaml:"data"`
	} `yaml:"fixtures"`
}

func runScaffoldSeed(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	features, err := cfg.AllFeatures()
	if err != nil {
		return fmt.Errorf("enumerate features: %w", err)
	}

	out := deriveSeed(cfg, features)
	buf, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(cmd.OutOrStdout(), string(buf))
	if !out.Derivable {
		return NewExitCodeError(1)
	}
	return nil
}

// deriveSeed computes the composed seed. Split from the command so it is
// testable without a cobra invocation.
func deriveSeed(cfg *config.Context, features []string) seedOutput {
	out := seedOutput{Contributors: map[string]string{}}

	// value → the features that assert it, per (entity, id, field).
	type cell struct {
		entity, id, field string
	}
	claims := map[cell]map[string][]string{}
	order := []cell{}
	seenCell := map[cell]bool{}
	recordFields := map[string]map[string]interface{}{} // "entity\x00id" → fields
	recordFrom := map[string]map[string]bool{}
	recordOrder := []string{}
	seenRecord := map[string]bool{}

	for _, slug := range features {
		bfPath := filepath.Join(cfg.BuildPath(slug), "buildfile.yaml")
		data, err := os.ReadFile(bfPath)
		if err != nil {
			// Unbuilt features contribute nothing. check-composition reports
			// them; the seed just has less to work with.
			continue
		}
		var bf seedBuildfile
		if err := yaml.Unmarshal(data, &bf); err != nil {
			out.Findings = append(out.Findings, seedFinding{
				Code:    "composition-buildfile-unreadable",
				Message: fmt.Sprintf("%s: cannot parse buildfile.yaml: %v", slug, err),
				Sites:   []string{slug},
			})
			continue
		}
		if len(bf.Fixtures) == 0 {
			continue
		}

		name, finding := pickComposingFixture(cfg, slug, bf)
		if finding != nil {
			out.Findings = append(out.Findings, *finding)
			continue
		}
		out.Contributors[slug] = name

		fx := bf.Fixtures[name]
		for entity, rows := range fx.Data {
			for _, row := range rows {
				id, _ := row["id"].(string)
				if id == "" {
					continue
				}
				key := entity + "\x00" + id
				if !seenRecord[key] {
					seenRecord[key] = true
					recordOrder = append(recordOrder, key)
					recordFields[key] = map[string]interface{}{}
					recordFrom[key] = map[string]bool{}
				}
				recordFrom[key][slug] = true
				for field, v := range row {
					if field == "id" {
						continue
					}
					recordFields[key][field] = v
					if !isScalar(v) {
						// Non-scalars legitimately differ by what each feature
						// needed to load, so they are merged without being
						// compared — the same rule check-composition applies.
						continue
					}
					c := cell{entity, id, field}
					if !seenCell[c] {
						seenCell[c] = true
						order = append(order, c)
					}
					if claims[c] == nil {
						claims[c] = map[string][]string{}
					}
					val := fmt.Sprint(v)
					claims[c][val] = append(claims[c][val], slug)
				}
			}
		}
	}

	// A contradiction stops the derivation. No merge policy, no
	// last-writer-wins: silently reconciling would hide exactly the class of
	// defect the composed seed exists to make impossible.
	for _, c := range order {
		values := claims[c]
		if len(values) < 2 {
			continue
		}
		var parts []string
		var sites []string
		for val, feats := range values {
			sort.Strings(feats)
			parts = append(parts, fmt.Sprintf("%q in %s", val, strings.Join(feats, ", ")))
			sites = append(sites, feats...)
		}
		sort.Strings(parts)
		sort.Strings(sites)
		out.Findings = append(out.Findings, seedFinding{
			Code: "composition-fixture-contradiction",
			Message: fmt.Sprintf("%s %s has conflicting %s across the composing fixtures: %s. "+
				"There is one runtime, so one of these has to be wrong.",
				c.entity, c.id, c.field, strings.Join(parts, "; ")),
			Sites: sites,
		})
	}

	// Canonical output: records sorted by entity then id, fields emitted by
	// sorted key. Two runs over the same inputs must produce byte-identical
	// output or every consumer sees phantom drift — the same reasoning
	// scaffold-plan records for plan rows.
	sort.Strings(recordOrder)
	for _, key := range recordOrder {
		entity, id := splitRecordKey(key)
		from := make([]string, 0, len(recordFrom[key]))
		for f := range recordFrom[key] {
			from = append(from, f)
		}
		sort.Strings(from)
		out.Records = append(out.Records, seedRecord{
			Entity: entity,
			ID:     id,
			Fields: recordFields[key],
			From:   from,
		})
	}

	sort.Slice(out.Findings, func(i, j int) bool { return out.Findings[i].Message < out.Findings[j].Message })
	out.Derivable = len(out.Findings) == 0

	// A seed either exists or it does not. There is no partial one.
	//
	// The records were still being emitted alongside derivable:false, and those
	// records embodied exactly the last-writer-wins this design forbids — the
	// contradicting field held whichever contributor sorted last. Any consumer
	// reading `records` without first checking `derivable` would have gotten a
	// silently reconciled seed, which is the defect the refusal exists to
	// prevent, reintroduced one field over.
	//
	// Withholding them is what makes the refusal load-bearing rather than
	// advisory.
	if !out.Derivable {
		out.Records = nil
	}
	return out
}

// pickComposingFixture wraps agent.ComposingFixture, which owns the rule.
// The designation is validation logic — `validate --project` reports when it
// cannot be determined — and a second copy here is how the derivation and the
// validator would come to disagree about which fixture is the real one.
func pickComposingFixture(cfg *config.Context, slug string, bf seedBuildfile) (string, *seedFinding) {
	name, ambiguity := agent.ComposingFixture(cfg.BuildPath(slug))
	if ambiguity != "" {
		return "", &seedFinding{
			Code:    "composition-seed-ambiguous",
			Message: fmt.Sprintf("%s: %s", slug, ambiguity),
			Sites:   []string{slug},
		}
	}
	return name, nil
}

func splitRecordKey(key string) (string, string) {
	for i := 0; i < len(key); i++ {
		if key[i] == 0 {
			return key[:i], key[i+1:]
		}
	}
	return key, ""
}
