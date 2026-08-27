// parlay-feature: domain-model-editor/feature-contributions
// parlay-component: cross-cutting/contribution-impact

// The diff itself lives in internal/editor/domain, with the loader and the
// serializer — one decoder and one definition of "conflict" for the file. What
// lives here is the layer core owns: which features a proposed change reaches,
// and which of their fixtures would have to carry a new field.
//
// It is in agent rather than in the command so `parlay internal domain-impact`
// and `validate --project` answer with the same rule, which is the reasoning
// that put ComposingFixture here.

package agent

import (
	"fmt"
	"sort"

	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/ddwht/parlay/internal/editor/domain"
)

// FixtureEntity is one (feature, fixture, entity) triple present in the
// project's build state. The command collects these; nothing here reads a
// file.
type FixtureEntity struct {
	Feature string
	Fixture string
	Entity  string
}

// ProjectFacts is what the impact walk needs to know about the rest of the
// project. Passed in rather than gathered here so this stays a function of
// its inputs — the same reason the composition walk enumerates features at
// its call site.
type ProjectFacts struct {
	// EntityUsers maps an entity name to the features whose capabilities.yaml
	// names it as a subject or an output.
	EntityUsers map[string][]string
	// Fixtures is every (feature, fixture, entity) triple in the project.
	Fixtures []FixtureEntity
}

// EntityAudience is who a change to one entity reaches.
type EntityAudience struct {
	Entity string `json:"entity"`
	// Features are the OTHER features whose capabilities reference this
	// entity — the ones a change to it is a change for.
	Features []string `json:"features,omitempty"`
	// Fixtures are the existing fixtures holding records of this entity that
	// would need the newly-proposed fields. This is the concrete half: a
	// person can read it as a list of files to go and update.
	Fixtures []FixtureNeed `json:"fixtures,omitempty"`
}

// FixtureNeed names one fixture and the proposed fields its records of the
// entity do not yet carry.
type FixtureNeed struct {
	Feature string   `json:"feature"`
	Fixture string   `json:"fixture"`
	Fields  []string `json:"fields"`
}

// ContributionImpact is the whole answer for one feature: what it proposes,
// what disagrees, what was already there, and who it reaches.
type ContributionImpact struct {
	Feature   string            `json:"feature"`
	Path      string            `json:"path"`
	Additions []domain.Element  `json:"additions"`
	Conflicts []domain.Conflict `json:"conflicts"`
	Redundant []domain.Element  `json:"redundant"`
	Affects   []EntityAudience  `json:"affects"`
	// Applicable is false when the contribution conflicts. Stated rather than
	// left to be derived from len(conflicts), because "can this land" is the
	// question the loop asks and a caller getting the derivation wrong would
	// fail open.
	Applicable bool `json:"applicable"`
}

// Impact diffs a feature's contribution against the root model and works out
// who the additions reach.
func Impact(feature, path string, root, contribution domain.Model, facts ProjectFacts) ContributionImpact {
	d := domain.Diff(root, contribution)

	imp := ContributionImpact{
		Feature:    feature,
		Path:       path,
		Additions:  d.Additions,
		Conflicts:  d.Conflicts,
		Redundant:  d.Redundant,
		Applicable: !d.HasConflicts(),
	}

	// Which entities the proposal touches, and for each, the new fields
	// landing on it. A brand-new entity reaches nobody by definition — no
	// existing feature references it and no existing fixture holds one — so
	// only additions to entities that already exist produce an audience.
	newFieldsByEntity := map[string][]string{}
	var touched []string
	for _, a := range d.Additions {
		if a.Kind != domain.KindField || a.Entity == "" {
			continue
		}
		if _, seen := newFieldsByEntity[a.Entity]; !seen {
			touched = append(touched, a.Entity)
		}
		newFieldsByEntity[a.Entity] = append(newFieldsByEntity[a.Entity], a.Name)
	}
	// A conflict reaches people too — arguably more urgently, since somebody
	// is already relying on the root's description.
	for _, c := range d.Conflicts {
		if c.Kind != domain.KindField || c.Entity == "" {
			continue
		}
		if _, seen := newFieldsByEntity[c.Entity]; !seen {
			touched = append(touched, c.Entity)
			newFieldsByEntity[c.Entity] = nil
		}
	}
	sort.Strings(touched)

	for _, entity := range touched {
		audience := EntityAudience{Entity: entity}

		for _, f := range facts.EntityUsers[entity] {
			if f == feature {
				continue
			}
			audience.Features = append(audience.Features, f)
		}
		sort.Strings(audience.Features)
		audience.Features = dedupe(audience.Features)

		fields := dedupe(sortedCopy(newFieldsByEntity[entity]))
		if len(fields) > 0 {
			for _, fx := range facts.Fixtures {
				if fx.Entity != entity || fx.Feature == feature {
					continue
				}
				audience.Fixtures = append(audience.Fixtures, FixtureNeed{
					Feature: fx.Feature, Fixture: fx.Fixture, Fields: fields,
				})
			}
			sort.Slice(audience.Fixtures, func(i, j int) bool {
				if audience.Fixtures[i].Feature != audience.Fixtures[j].Feature {
					return audience.Fixtures[i].Feature < audience.Fixtures[j].Feature
				}
				return audience.Fixtures[i].Fixture < audience.Fixtures[j].Fixture
			})
		}

		if len(audience.Features) == 0 && len(audience.Fixtures) == 0 {
			continue
		}
		imp.Affects = append(imp.Affects, audience)
	}

	return imp
}

// CapabilityEntities returns the entity names a capabilities.yaml references,
// from both subject.entity and output.entity — the two places the schema
// names in one breath and the two the entity cross-reference already checks.
// An unparseable file yields nothing rather than an error: the capabilities
// validator reports that, and reporting it twice under two codes would be
// worse than reporting it once.
func CapabilityEntities(path string, content []byte) []string {
	caps, err := parser.ParseCapabilitiesBytes(path, content)
	if err != nil {
		return nil
	}
	var out []string
	for _, op := range caps.Operations {
		if op.Subject.Entity != "" {
			out = append(out, op.Subject.Entity)
		}
		if op.Output != nil && op.Output.Entity != "" {
			out = append(out, op.Output.Entity)
		}
	}
	sort.Strings(out)
	return dedupe(out)
}

// ProposedEntities maps every entity name a contribution introduces to the
// feature proposing it. It is what lets the capabilities validator tell a
// typo from an entity another feature is about to add.
//
// A name proposed by more than one feature keeps the first proposer in sorted
// feature order, so the message is stable across runs. Which of two features
// gets named matters far less than the message being reproducible.
func ProposedEntities(contributions map[string]domain.Model) map[string]string {
	features := make([]string, 0, len(contributions))
	for f := range contributions {
		features = append(features, f)
	}
	sort.Strings(features)

	out := map[string]string{}
	for _, f := range features {
		for _, e := range contributions[f].Entities {
			if _, taken := out[e.Name]; !taken {
				out[e.Name] = f
			}
		}
	}
	return out
}

// ValidateCapabilitiesWithProposals is ValidateCapabilities plus knowledge of
// what other features have proposed.
//
// `capabilities-entity-undeclared` could not tell a typo from an entity a
// sibling feature introduces, so it graded both the same way — and that is
// what forced two features in the regression run to ship placeholders for an
// entity a third was about to add. With the contributions readable, the
// second case becomes `capabilities-entity-pending`: a warning that names the
// feature proposing it, so the reference is a thing to review rather than a
// thing to work around.
func ValidateCapabilitiesWithProposals(mode ValidationMode, path string, content []byte, declaredEntities []string, proposedBy map[string]string) []ValidationOutcome {
	outcomes := ValidateCapabilities(mode, path, content, declaredEntities)
	if len(proposedBy) == 0 {
		return outcomes
	}

	referenced := CapabilityEntities(path, content)
	pending := map[string]string{}
	declared := map[string]bool{}
	for _, e := range declaredEntities {
		declared[e] = true
	}
	for _, e := range referenced {
		if declared[e] {
			continue
		}
		if by, ok := proposedBy[e]; ok {
			pending[e] = by
		}
	}
	if len(pending) == 0 {
		return outcomes
	}

	// Replace the undeclared finding for a pending entity rather than adding
	// alongside it: two findings about one reference, one blocking and one
	// not, would leave a reader unsure which applies.
	var kept []ValidationOutcome
	for _, o := range outcomes {
		if o.Code == "capabilities-entity-undeclared" && mentionsPendingEntity(o.Message, pending) {
			continue
		}
		kept = append(kept, o)
	}

	names := make([]string, 0, len(pending))
	for e := range pending {
		names = append(names, e)
	}
	sort.Strings(names)
	for _, e := range names {
		kept = append(kept, NewOutcome(mode, "capabilities-entity-pending",
			fmt.Sprintf("%s: entity %q is not in the project domain model yet, but feature %q proposes it in its contribution. "+
				"Review and accept that contribution before this feature builds.", path, e, pending[e])))
	}
	return kept
}

// mentionsPendingEntity reports whether an undeclared-entity finding is about
// one of the entities a contribution proposes. The entity name appears in the
// message quoted, which is the same form the emitting site writes.
func mentionsPendingEntity(message string, pending map[string]string) bool {
	for e := range pending {
		if containsSubstring(message, `"`+e+`"`) {
			return true
		}
	}
	return false
}

func containsSubstring(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func dedupe(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := in[:0:0]
	var last string
	for i, s := range in {
		if i > 0 && s == last {
			continue
		}
		out = append(out, s)
		last = s
	}
	return out
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
