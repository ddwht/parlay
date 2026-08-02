package commands

// The domain model declares cardinalities and nothing ever held the
// project's own data against them. `validate --type domain-model` checks
// that `cardinality:` is one of four recognised words; that is a check on
// the spelling, not on the data. Run 3 shipped a prototype with two
// mutually exclusive records hanging off one parent, every gate passed
// because the ids differed, and it surfaced only when a person read the
// seed.
//
// The honest scope is narrow, and saying so matters more than the check
// itself — a check whose reach is overstated gets trusted further than it
// earns. Only `one-to-one` yields a constraint a scalar ref field can
// violate: at most one child may point at any given parent. `many-to-one`
// and `many-to-many` cannot be violated by counting at all, and the "one"
// side of `one-to-many` is automatic because a scalar field holds one
// value. So this checks one-to-one and nothing else.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ddwht/parlay/core/internal/config"
)

// findCardinalityViolations holds the composed records against the
// one-to-one relationships the domain model declares.
//
// It returns errors and notes separately, following the same grading this
// command already uses: a violation is a defect in the composed runtime;
// an unresolvable join is a fact about the model that a person may want to
// fix, but guessing which field realises a relationship and then failing
// on the guess would be worse than saying nothing.
func findCardinalityViolations(model *config.DomainModelArtifact, records []entityRecord) (errors, notes []compositionFinding) {
	if model == nil {
		return nil, nil
	}
	entityByName := map[string]config.DomainEntity{}
	for _, e := range model.Entities {
		entityByName[e.Name] = e
	}

	for _, rel := range model.Relationships {
		if rel.Cardinality != "one-to-one" {
			continue
		}
		child, ok := entityByName[rel.To]
		if !ok {
			// The domain-model validator already reports an endpoint
			// naming an undeclared entity. Restating it here would give
			// the same defect two codes.
			continue
		}

		field, why := realisingField(child, rel)
		if field == "" {
			notes = append(notes, compositionFinding{
				Code:   "composition-cardinality-unresolvable",
				Entity: rel.To,
				Message: fmt.Sprintf("relationship %q declares %s but %s. "+
					"Nothing links a relationship to the field that realises it, so the check infers it from `target:` — "+
					"add `relationship: %s` to the intended field on %s to settle it.",
					rel.Name, rel.Cardinality, why, rel.Name, rel.To),
				Sites: []string{rel.Name},
			})
			continue
		}

		// parent id -> the distinct children pointing at it, and where
		// each was seen. Only the composed seed counts: a scenario
		// fixture describes a state the prototype never boots into, so
		// two of them hanging off one parent are not simultaneously true.
		children := map[string]map[string]bool{}
		sites := map[string][]string{}
		for _, r := range records {
			if r.Entity != rel.To || !r.Composing {
				continue
			}
			parent, _ := r.Fields[field].(string)
			if parent == "" {
				continue
			}
			if children[parent] == nil {
				children[parent] = map[string]bool{}
			}
			children[parent][r.ID] = true
			sites[parent] = append(sites[parent], recordSite{Feature: r.Feature, Fixture: r.Fixture}.String())
		}

		var offending []string
		for parent, kids := range children {
			if len(kids) > 1 {
				offending = append(offending, parent)
			}
		}
		sort.Strings(offending)

		for _, parent := range offending {
			kids := make([]string, 0, len(children[parent]))
			for id := range children[parent] {
				kids = append(kids, id)
			}
			sort.Strings(kids)
			where := dedupeSorted(sites[parent])
			errors = append(errors, compositionFinding{
				Code:   "composition-cardinality-violated",
				Entity: rel.To,
				ID:     parent,
				Field:  field,
				Sites:  where,
				Message: fmt.Sprintf("relationship %q declares %s, but %d %s records in the composed seed point at %s %s via %s: %s. "+
					"The prototype boots with data its own model forbids.",
					rel.Name, rel.Cardinality, len(kids), rel.To, rel.From, parent, field, strings.Join(kids, ", ")),
			})
		}
	}

	sortCompositionFindings(errors)
	sortCompositionFindings(notes)
	return errors, notes
}

// realisingField returns the field on the child entity that implements the
// relationship, or "" plus a reason the join could not be settled.
//
// An explicit `relationship:` back-reference wins outright. Otherwise the
// candidates are the child's ref fields targeting the parent: exactly one
// is unambiguous; zero means nothing realises the relationship; more than
// one means the model connects the same pair twice and only the author
// knows which edge is which. Guessing in either of those cases would let
// the check misfire on a correct model, which is worse than not running.
func realisingField(child config.DomainEntity, rel config.DomainRelationship) (field, why string) {
	var explicit, byTarget []string
	for _, f := range child.Fields {
		if f.Relationship == rel.Name {
			explicit = append(explicit, f.Name)
		}
		if f.Target == rel.From {
			byTarget = append(byTarget, f.Name)
		}
	}
	sort.Strings(explicit)
	sort.Strings(byTarget)

	switch len(explicit) {
	case 1:
		return explicit[0], ""
	case 0:
		// fall through to inference
	default:
		return "", fmt.Sprintf("%d fields on %s claim `relationship: %s` (%s); exactly one field realises a relationship",
			len(explicit), child.Name, rel.Name, strings.Join(explicit, ", "))
	}

	switch len(byTarget) {
	case 1:
		return byTarget[0], ""
	case 0:
		return "", fmt.Sprintf("no field on %s targets %s, so nothing in the data realises it", child.Name, rel.From)
	default:
		return "", fmt.Sprintf("%d fields on %s target %s (%s), so which one realises it is ambiguous",
			len(byTarget), child.Name, rel.From, strings.Join(byTarget, ", "))
	}
}

func dedupeSorted(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
