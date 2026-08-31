// parlay-feature: parlay-tool/domain-document-api
// parlay-component: cross-cutting/contribution-diff-and-apply

// A feature that needs something the project model lacks writes it into the
// feature's own contribution — spec/intents/<feature>/domain-model.yaml —
// rather than editing the root model. The root file stays the source of
// truth: a contribution is a proposal, and nothing lands until someone
// accepts it.
//
// Both the diff and the merge live here, in the package that already owns
// loading, migration and deterministic serialization of this file. That
// placement is the point. Two decoders over one file is the failure this
// codebase keeps producing — fixtureBuildfile against seedBuildfile,
// RepoRoot() against Root.Path, the map-vs-list decode that meant no project
// ever derived a models plan row. There is one writer for domain-model.yaml
// and one definition of what counts as a conflict, and every caller goes
// through them.

package domainmodel

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// ElementKind names what part of the model an element is. The values match
// the element-path grammar domain-model.schema.md documents.
type ElementKind string

const (
	KindEnum         ElementKind = "enum"
	KindEnumValue    ElementKind = "enum-value"
	KindEntity       ElementKind = "entity"
	KindField        ElementKind = "field"
	KindRelationship ElementKind = "relationship"
)

// Element is one thing a contribution proposes, identified by the same
// element path the domain-model findings use — so a person can move between
// an impact report and a validation finding without translating.
type Element struct {
	Kind ElementKind `json:"kind"`
	Path string      `json:"path"`
	// Entity is set for field-kind elements, so a caller can group by the
	// entity a change touches without parsing the path.
	Entity  string `json:"entity,omitempty"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

// Conflict is an element the root already describes differently. It carries
// both descriptions because "these disagree" is not actionable on its own.
type Conflict struct {
	Element
	Root     string `json:"root"`
	Proposed string `json:"proposed"`
}

// Delta is what a contribution would do to the root model.
type Delta struct {
	Additions []Element  `json:"additions"`
	Conflicts []Conflict `json:"conflicts"`
	Redundant []Element  `json:"redundant"`
}

// HasConflicts reports whether the contribution can be applied at all.
func (d Delta) HasConflicts() bool { return len(d.Conflicts) > 0 }

// Diff compares a contribution against the root model.
//
// Additive-only by construction: an element the root lacks is an addition, an
// element it already has identically is redundant, and an element it
// describes differently is a conflict. A contribution never removes anything
// and never rewrites anything — the third case is a question for a person,
// not a merge.
//
// Iteration follows the contribution's own declaration order, so two runs
// over the same bytes produce the same report.
func Diff(root, contribution Model) Delta {
	var d Delta

	rootEnums := map[string]Enum{}
	for _, e := range root.Enums {
		rootEnums[e.Name] = e
	}
	rootEntities := map[string]Entity{}
	for _, e := range root.Entities {
		rootEntities[e.Name] = e
	}
	rootRels := map[string]Relationship{}
	for _, r := range root.Relationships {
		rootRels[r.Name] = r
	}

	for _, en := range contribution.Enums {
		existing, present := rootEnums[en.Name]
		if !present {
			d.Additions = append(d.Additions, Element{
				Kind: KindEnum, Path: "enums." + en.Name, Name: en.Name,
				Summary: fmt.Sprintf("new enum with %d value(s): %s", len(en.Values), enumValueList(en)),
			})
			continue
		}
		// The enum exists. Its values are compared one at a time, because a
		// feature adding a value to an existing enum is the common case and
		// treating the whole enum as one unit would report it as a conflict.
		byValue := map[string]EnumValue{}
		for _, v := range existing.Values {
			byValue[v.Value] = v
		}
		for _, v := range en.Values {
			path := "enums." + en.Name + ".values." + v.Value
			cur, ok := byValue[v.Value]
			switch {
			case !ok:
				d.Additions = append(d.Additions, Element{
					Kind: KindEnumValue, Path: path, Name: v.Value,
					Summary: fmt.Sprintf("new value on existing enum %s", en.Name),
				})
			case cur == v:
				d.Redundant = append(d.Redundant, Element{
					Kind: KindEnumValue, Path: path, Name: v.Value,
					Summary: "already declared identically",
				})
			default:
				d.Conflicts = append(d.Conflicts, Conflict{
					Element: Element{
						Kind: KindEnumValue, Path: path, Name: v.Value,
						Summary: "described differently by the root model",
					},
					Root:     describeEnumValue(cur),
					Proposed: describeEnumValue(v),
				})
			}
		}
	}

	for _, ent := range contribution.Entities {
		existing, present := rootEntities[ent.Name]
		if !present {
			d.Additions = append(d.Additions, Element{
				Kind: KindEntity, Path: "entities." + ent.Name, Name: ent.Name,
				Summary: fmt.Sprintf("new entity with %d field(s): %s", len(ent.Fields), fieldNameList(ent)),
			})
			continue
		}
		byName := map[string]Field{}
		for _, f := range existing.Fields {
			byName[f.Name] = f
		}
		for _, f := range ent.Fields {
			path := "entities." + ent.Name + ".fields." + f.Name
			cur, ok := byName[f.Name]
			switch {
			case !ok:
				d.Additions = append(d.Additions, Element{
					Kind: KindField, Path: path, Entity: ent.Name, Name: f.Name,
					Summary: fmt.Sprintf("new field on existing entity %s: %s", ent.Name, describeField(f)),
				})
			case cur == f:
				d.Redundant = append(d.Redundant, Element{
					Kind: KindField, Path: path, Entity: ent.Name, Name: f.Name,
					Summary: "already declared identically",
				})
			default:
				d.Conflicts = append(d.Conflicts, Conflict{
					Element: Element{
						Kind: KindField, Path: path, Entity: ent.Name, Name: f.Name,
						Summary: "described differently by the root model",
					},
					Root:     describeField(cur),
					Proposed: describeField(f),
				})
			}
		}
	}

	for _, rel := range contribution.Relationships {
		path := "relationships." + rel.Name
		cur, ok := rootRels[rel.Name]
		switch {
		case !ok:
			d.Additions = append(d.Additions, Element{
				Kind: KindRelationship, Path: path, Name: rel.Name,
				Summary: fmt.Sprintf("new relationship: %s", describeRelationship(rel)),
			})
		case cur == rel:
			d.Redundant = append(d.Redundant, Element{
				Kind: KindRelationship, Path: path, Name: rel.Name,
				Summary: "already declared identically",
			})
		default:
			d.Conflicts = append(d.Conflicts, Conflict{
				Element: Element{
					Kind: KindRelationship, Path: path, Name: rel.Name,
					Summary: "described differently by the root model",
				},
				Root:     describeRelationship(cur),
				Proposed: describeRelationship(rel),
			})
		}
	}

	return d
}

// ErrContributionConflicts is returned by Merge and ApplyContribution when
// the contribution describes something the root already describes
// differently. There is no auto-merge and no last-writer-wins: which of two
// descriptions is right is a design question, and answering it by picking one
// is how a model quietly stops meaning what its author thought.
type ErrContributionConflicts struct {
	Conflicts []Conflict
}

func (e *ErrContributionConflicts) Error() string {
	paths := make([]string, 0, len(e.Conflicts))
	for _, c := range e.Conflicts {
		paths = append(paths, fmt.Sprintf("%s (root: %s; proposed: %s)", c.Path, c.Root, c.Proposed))
	}
	return fmt.Sprintf("contribution conflicts with the root model at %d place(s): %s",
		len(e.Conflicts), strings.Join(paths, "; "))
}

// Merge returns the root model with the contribution's additions folded in.
// It refuses outright when the contribution conflicts, and it never modifies
// the model it was given.
//
// New elements are appended in the contribution's declaration order rather
// than sorted into place: the root file's existing order is a human ordering
// and re-sorting it would turn an accepted proposal into a whole-file diff.
func Merge(root, contribution Model) (Model, error) {
	d := Diff(root, contribution)
	if d.HasConflicts() {
		return Model{}, &ErrContributionConflicts{Conflicts: d.Conflicts}
	}

	merged := root
	merged.Enums = append([]Enum(nil), root.Enums...)
	merged.Entities = append([]Entity(nil), root.Entities...)
	merged.Relationships = append([]Relationship(nil), root.Relationships...)

	enumAt := map[string]int{}
	for i, e := range merged.Enums {
		enumAt[e.Name] = i
	}
	for _, en := range contribution.Enums {
		i, ok := enumAt[en.Name]
		if !ok {
			merged.Enums = append(merged.Enums, en)
			enumAt[en.Name] = len(merged.Enums) - 1
			continue
		}
		have := map[string]bool{}
		for _, v := range merged.Enums[i].Values {
			have[v.Value] = true
		}
		for _, v := range en.Values {
			if !have[v.Value] {
				merged.Enums[i].Values = append(merged.Enums[i].Values, v)
			}
		}
	}

	entityAt := map[string]int{}
	for i, e := range merged.Entities {
		entityAt[e.Name] = i
	}
	for _, ent := range contribution.Entities {
		i, ok := entityAt[ent.Name]
		if !ok {
			merged.Entities = append(merged.Entities, ent)
			entityAt[ent.Name] = len(merged.Entities) - 1
			continue
		}
		have := map[string]bool{}
		for _, f := range merged.Entities[i].Fields {
			have[f.Name] = true
		}
		for _, f := range ent.Fields {
			if !have[f.Name] {
				merged.Entities[i].Fields = append(merged.Entities[i].Fields, f)
			}
		}
	}

	relPresent := map[string]bool{}
	for _, r := range merged.Relationships {
		relPresent[r.Name] = true
	}
	for _, r := range contribution.Relationships {
		if !relPresent[r.Name] {
			merged.Relationships = append(merged.Relationships, r)
			relPresent[r.Name] = true
		}
	}

	return merged, nil
}

// ApplyContribution merges a contribution into the root model on disk and
// writes it through the same compare-and-swap save path every other write
// takes —
// atomic write, deterministic serialization, deprecated-operations block
// carried through byte-for-byte. It writes nothing when the contribution
// conflicts.
//
// The etag is read and presented in one call rather than taken from a
// caller: an apply is not a user editing a draft they loaded minutes ago, it
// is a merge computed from the file as it is right now. A concurrent write
// between the read and the save still fails the compare-and-swap.
func ApplyContribution(ctx context.Context, root string, contribution Model) (Etag, error) {
	current, etag, err := Load(ctx, root)
	if err != nil {
		return "", err
	}
	merged, err := Merge(current, contribution)
	if err != nil {
		return "", err
	}
	return Save(ctx, root, merged, etag)
}

func describeField(f Field) string {
	parts := []string{"type: " + f.Type}
	if f.Target != "" {
		parts = append(parts, "target: "+f.Target)
	}
	if f.Enum != "" {
		parts = append(parts, "enum: "+f.Enum)
	}
	if f.Relationship != "" {
		parts = append(parts, "relationship: "+f.Relationship)
	}
	if f.Required {
		parts = append(parts, "required")
	}
	return strings.Join(parts, ", ")
}

func describeEnumValue(v EnumValue) string {
	parts := []string{v.Value}
	if v.Label != "" {
		parts = append(parts, "label: "+v.Label)
	}
	if v.Tone != "" {
		parts = append(parts, "tone: "+v.Tone)
	}
	return strings.Join(parts, ", ")
}

func describeRelationship(r Relationship) string {
	return fmt.Sprintf("%s -> %s (%s)", r.From, r.To, r.Cardinality)
}

func enumValueList(e Enum) string {
	names := make([]string, 0, len(e.Values))
	for _, v := range e.Values {
		names = append(names, v.Value)
	}
	return strings.Join(names, ", ")
}

func fieldNameList(e Entity) string {
	names := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		names = append(names, f.Name)
	}
	return strings.Join(names, ", ")
}

// EntityNames returns the entity names a model declares, sorted. Callers use
// it to answer "does this entity exist yet" without reaching into the model
// shape themselves.
func EntityNames(m Model) []string {
	names := make([]string, 0, len(m.Entities))
	for _, e := range m.Entities {
		names = append(names, e.Name)
	}
	sort.Strings(names)
	return names
}
