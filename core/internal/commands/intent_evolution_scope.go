package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/parser"
)

// The downstream subjects of an intent transition.
//
// Side-by-side promise text answers "what changed". It does not answer "which
// contract entries is the human's preservation claim about", and without that
// the ceremony would approve a revision while saying nothing about the scope it
// might have stopped supporting — reopening semantic justification drift under
// a vocabulary built to make it visible.
//
// So the ceremony shows the EXACT current population of entries attributed to
// each lineage, partitioned into those this amendment declares it changes and
// those it does not, and binds that population into the approval. A count would
// not do: the population can change between preflight and confirmation, and a
// token that survives that change approves a claim about a different subject.

// scopedEntry is one contract entry the approval is about.
//
// Ref alone is NOT the subject. The human is asserting that a revised promise
// still supports this entry, so what they reviewed is its MEANING, not its
// address: an edit that rewrites an operation's summary, inputs or acceptance
// criteria while keeping its id and source produces the same ref and a
// different promise, and a token bound only to the ref would still approve it.
//
// Taken over the RESOLVED GENERIC entry — not the parsed struct, and not the
// literal yaml.Node either.
//
// Both of those are wrong in opposite directions, and the naming matters
// because a maintainer who reads "raw node" may reasonably hash the node.
// Hashing the parsed struct drops keys the parser does not model; hashing the
// node verbatim binds `<<: *defaults` while leaving the anchor target unbound.
// Decoding the item into a generic value does both jobs: unknown keys survive
// because the target is generic, and aliases and merges resolve to the values
// the parser actually sees. Fingerprinting the parsed
// struct was a live bypass rather than a future limitation: yaml.Unmarshal is
// non-strict, CapabilityOperation has no Summary field, and `summary` is
// plainly semantic contract text that today's artifacts carry — so an edit
// confined to it changed the promise, kept the ref, and left the token valid.
// "The parser does not model that meaning" does not put it outside what the
// human asserted.
//
// Raw nodes cover unknown and additive keys without the overbinding of a
// whole-file hash, which would invalidate approvals over unrelated entries and
// formatting.
type scopedEntry struct {
	Ref         string `yaml:"ref" json:"ref"`
	Fingerprint string `yaml:"fingerprint" json:"fingerprint"`
}

// refsOf renders a partition for display.
func refsOf(entries []scopedEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Ref)
	}
	return out
}

// entryFingerprint hashes a value, returning an error rather than a sentinel.
//
// A string like "unfingerprintable:..." would turn an inability to bind a
// subject INTO a comparable subject: two entries nobody could fingerprint would
// compare equal, and an approval could be given over them.
func entryFingerprint(v any) (string, error) {
	data, err := yaml.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("fingerprint an attributed entry: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// resolvedEntryFingerprints maps an entry key to the canonical fingerprint of
// its RESOLVED entry — decoded into a generic value, so aliases and merge keys
// are expanded while unknown fields survive.
//
// Hashing the item node verbatim would bind `<<: *defaults` — the alias name
// and merge syntax — while leaving the anchor target unbound, so changing that
// target changes what the entry promises and leaves the fingerprint identical.
// Decoding the item resolves merges and aliases into concrete values, which is
// what the parser sees and therefore what the human reviewed; unknown keys
// survive it because the decode target is generic.
//
// It also refuses to manufacture an ambiguous subject: a duplicate key or a
// node that is not a sequence produces an error rather than a map whose last
// writer wins. Other commands reject duplicate ids, but this ceremony does not
// run those validators, and an authority derivation may not rely on a gate it
// does not invoke.
func resolvedEntryFingerprints(path, sequenceKey, idKey string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	out := map[string]string{}
	if len(doc.Content) == 0 {
		return out, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s is not a mapping at its root", path)
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != sequenceKey {
			continue
		}
		seq := root.Content[i+1]
		if seq.Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("%s: %s is not a sequence, so its entries cannot be "+
				"enumerated", path, sequenceKey)
		}
		for _, item := range seq.Content {
			// Expand before hashing.
			var expanded map[string]any
			if derr := item.Decode(&expanded); derr != nil {
				return nil, fmt.Errorf("%s: an entry under %s cannot be resolved (%w), so what it "+
					"promises is unknown", path, sequenceKey, derr)
			}
			id, _ := expanded[idKey].(string)
			if id == "" {
				return nil, fmt.Errorf("%s: an entry under %s has no %s, so it cannot be "+
					"identified", path, sequenceKey, idKey)
			}
			if _, dup := out[id]; dup {
				return nil, fmt.Errorf("%s: %s %q appears more than once, so which entry a "+
					"reference names is ambiguous", path, idKey, id)
			}
			fp, ferr := entryFingerprint(expanded)
			if ferr != nil {
				return nil, ferr
			}
			out[id] = fp
		}
	}
	return out, nil
}

// strictArtifactPresence reports whether an artifact is genuinely absent.
//
// Lstat, and only IsNotExist counts as absence — the same rule as the
// compaction transaction marker. A dangling symlink reads as ENOENT through
// Stat while being plainly present in the directory, and "there is something
// here I cannot follow" is not "there is nothing here".
func strictArtifactPresence(path string) (bool, error) {
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("deriving the entries attributed to this promise: %s cannot be "+
			"probed (%w), so the population the approval covers is unknown", path, err)
	}
	return true, nil
}

// lineageScope is the attributed population for one lineage, partitioned.
type lineageScope struct {
	Lineage  string        `yaml:"lineage" json:"lineage"`
	Named    []scopedEntry `yaml:"named" json:"named"`
	Unlisted []scopedEntry `yaml:"unlisted" json:"unlisted"`
}

// deriveLineageScope enumerates every contract entry attributed to each lineage
// and splits it by whether this amendment's affects: names it.
func deriveLineageScope(featDir, slug string, lineages []string, affects []string) ([]lineageScope, error) {
	named := map[string]bool{}
	for _, raw := range affects {
		if ref, err := parser.ParseAmendmentRef(raw); err == nil {
			named[canonicalAmendmentRef(ref)] = true
		}
	}

	// FAIL CLOSED. An earlier version skipped an artifact it could not parse,
	// so an unreadable file holding only unlisted attributed entries produced
	// an "exact population" that was neither exact nor the population — and the
	// human approved a closure assertion over a list with a hole in it.
	//
	// Absence and malformation are different: a feature legitimately has no
	// surface or no infrastructure, but a file that exists and will not parse
	// is an unknown, and an unknown here is unapprovable.
	type entry struct {
		kind, name, source string
		fingerprint        string
	}
	var entries []entry

	unreadable := func(path string, err error) error {
		return fmt.Errorf("deriving the entries attributed to this promise: %s cannot be read "+
			"(%v), so the population the approval covers is unknown", path, err)
	}

	capsPath := filepath.Join(featDir, "capabilities.yaml")
	present, err := strictArtifactPresence(capsPath)
	if err != nil {
		return nil, err
	}
	if present {
		caps, perr := parser.ParseCapabilities(capsPath)
		if perr != nil {
			return nil, unreadable(capsPath, perr)
		}
		raw, rerr := resolvedEntryFingerprints(capsPath, "operations", "id")
		if rerr != nil {
			return nil, unreadable(capsPath, rerr)
		}
		if len(caps.Operations) != len(raw) {
			return nil, unreadable(capsPath, fmt.Errorf("%d parsed operation(s) but %d raw "+
				"entr(y/ies) — the subject cannot be derived from a document the parser and the "+
				"reader disagree about", len(caps.Operations), len(raw)))
		}
		for _, op := range caps.Operations {
			fp, ok := raw[op.ID]
			if !ok {
				return nil, unreadable(capsPath, fmt.Errorf("operation %q has no raw node", op.ID))
			}
			entries = append(entries, entry{"operation", op.ID, op.Source, fp})
		}
	}

	surfacePath := filepath.Join(featDir, "surface.yaml")
	present, err = strictArtifactPresence(surfacePath)
	if err != nil {
		return nil, err
	}
	if present {
		frags, perr := parser.ParseSurfaceFile(surfacePath)
		if perr != nil {
			return nil, unreadable(surfacePath, perr)
		}
		raw, rerr := resolvedEntryFingerprints(surfacePath, "fragments", "name")
		if rerr != nil {
			return nil, unreadable(surfacePath, rerr)
		}
		if len(frags) != len(raw) {
			return nil, unreadable(surfacePath, fmt.Errorf("%d parsed fragment(s) but %d raw "+
				"entr(y/ies)", len(frags), len(raw)))
		}
		for _, f := range frags {
			fp, ok := raw[f.Name]
			if !ok {
				return nil, unreadable(surfacePath, fmt.Errorf("fragment %q has no raw node", f.Name))
			}
			entries = append(entries, entry{"surface", parser.Slugify(f.Name), f.Source, fp})
		}
	}

	infraPath := filepath.Join(featDir, "infrastructure.md")
	present, err = strictArtifactPresence(infraPath)
	if err != nil {
		return nil, err
	}
	if present {
		_, _, frags, perr := readFragments(infraPath)
		if perr != nil {
			return nil, unreadable(infraPath, perr)
		}
		for _, f := range frags {
			// Heading AND body, explicitly. Today `body` is the verbatim block
			// INCLUDING the heading line, so the heading is already covered —
			// naming it here is belt-and-braces against that changing, not a
			// fix for a live gap. Said plainly rather than implying this
			// closed a hole it did not.
			fp, ferr := entryFingerprint(struct{ Heading, Body string }{f.heading, f.body})
			if ferr != nil {
				return nil, ferr
			}
			entries = append(entries, entry{"infrastructure", parser.Slugify(f.heading),
				infraFragmentSource(f.body), fp})
		}
	}

	// A duplicate ref anywhere makes "the exact population" ambiguous, whatever
	// produced it.
	seenRef := map[string]bool{}
	for _, e := range entries {
		ref := fmt.Sprintf("@%s/%s:%s", slug, e.kind, e.name)
		if seenRef[ref] {
			return nil, fmt.Errorf("deriving the entries attributed to this promise: %s appears "+
				"more than once, so the population the approval covers is ambiguous", ref)
		}
		seenRef[ref] = true
	}

	out := make([]lineageScope, 0, len(lineages))
	sorted := append([]string(nil), lineages...)
	sort.Strings(sorted)
	for _, lineage := range sorted {
		sc := lineageScope{Lineage: lineage}
		for _, e := range entries {
			if e.source == "" || !sourceNamesIntent(e.source, slug, lineage) {
				continue
			}
			se := scopedEntry{Ref: fmt.Sprintf("@%s/%s:%s", slug, e.kind, e.name), Fingerprint: e.fingerprint}
			if named[se.Ref] {
				sc.Named = append(sc.Named, se)
			} else {
				sc.Unlisted = append(sc.Unlisted, se)
			}
		}
		sort.Slice(sc.Named, func(i, j int) bool { return sc.Named[i].Ref < sc.Named[j].Ref })
		sort.Slice(sc.Unlisted, func(i, j int) bool { return sc.Unlisted[i].Ref < sc.Unlisted[j].Ref })
		out = append(out, sc)
	}
	return out, nil
}

// scopeInventoryDigest is a canonical hash of the derived population.
//
// Attribution comes from MUTABLE contract artifacts rather than from the
// authority capsule, so the capsule comparison the confirmed run already does
// says nothing about it. Without binding this, a contract edit between preflight
// and confirmation would change the subject the human's claim was about while
// leaving the token valid.
func scopeInventoryDigest(scopes []lineageScope) string {
	h := sha256.New()
	for _, sc := range scopes {
		fmt.Fprintf(h, "lineage\x00%s\n", sc.Lineage)
		for _, e := range sc.Named {
			fmt.Fprintf(h, "named\x00%s\x00%s\n", e.Ref, e.Fingerprint)
		}
		for _, e := range sc.Unlisted {
			fmt.Fprintf(h, "unlisted\x00%s\x00%s\n", e.Ref, e.Fingerprint)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// fieldDelta is one field's before/after, rendered so a cleared field is
// visible rather than merely absent.
type fieldDelta struct {
	Field  string `yaml:"field" json:"field"`
	Before string `yaml:"before" json:"before"`
	After  string `yaml:"after" json:"after"`
}

// diffVersions renders a field-aware delta over the complete snapshot.
//
// Field-aware rather than two YAML blocks, because the whole hazard of snapshot
// semantics is that an OMITTED field is cleared: printing two documents makes
// the absence of a line the thing a reviewer has to notice, which is exactly
// the thing reviewers do not notice.
func diffVersions(before, after parser.Intent) []fieldDelta {
	list := func(v []string) string {
		if len(v) == 0 {
			return "(none)"
		}
		return strings.Join(v, "; ")
	}
	text := func(v string) string {
		if strings.TrimSpace(v) == "" {
			return "(none)"
		}
		return v
	}
	candidates := []fieldDelta{
		{"title", text(before.Title), text(after.Title)},
		{"goal", text(before.Goal), text(after.Goal)},
		{"persona", text(before.Persona), text(after.Persona)},
		{"priority", text(before.Priority), text(after.Priority)},
		{"context", text(before.Context), text(after.Context)},
		{"action", text(before.Action), text(after.Action)},
		{"objects", list(before.Objects), list(after.Objects)},
		{"constraints", list(before.Constraints), list(after.Constraints)},
		{"verify", list(before.Verify), list(after.Verify)},
		{"questions", list(before.Questions), list(after.Questions)},
	}
	var out []fieldDelta
	for _, d := range candidates {
		if d.Before != d.After {
			out = append(out, d)
		}
	}
	return out
}
