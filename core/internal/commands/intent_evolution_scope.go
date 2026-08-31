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

	"github.com/ddwht/parlay/core/internal/config"
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
	return resolvedEntryFingerprintsFrom(path, data, sequenceKey, idKey)
}

// resolvedEntryFingerprintsFrom is the same derivation over bytes ALREADY IN
// HAND, so a caller that also hashes the file can do both from one read.
//
// Two reads of a path produce two byte states, and a caller comparing a
// whole-file verdict against per-entry verdicts derived separately can report
// a file as changed while its entries all read stable, or the reverse. These
// are advisory rather than authority — but the refine walkthrough now tells an
// agent to treat the ref-by-ref comparison as decisive, and a decisive
// comparison should not be assembled from two moments.
func resolvedEntryFingerprintsFrom(path string, data []byte, sequenceKey, idKey string) (map[string]string, error) {
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
// contractEntry is one entry anywhere in the feature's contract, with the
// promises that justify it.
type contractEntry struct {
	Ref         string
	Fingerprint string
	Sources     string
}

// deriveContractIndex enumerates the WHOLE contract, not one lineage's slice.
//
// Existence is a question about the contract; attribution is a question about a
// promise. An entry the splice re-sourced elsewhere has left a lineage's
// population and is still very much present, so anything asking "does this
// still exist" must read this rather than a scope.
func deriveContractIndex(featDir, slug string) (map[string]contractEntry, error) {
	entries, err := enumerateContractEntries(featDir, slug)
	if err != nil {
		return nil, err
	}
	out := make(map[string]contractEntry, len(entries))
	for _, e := range entries {
		if _, dup := out[e.Ref]; dup {
			return nil, fmt.Errorf("deriving the entries attributed to this promise: %s appears "+
				"more than once, so the population the approval covers is ambiguous", e.Ref)
		}
		out[e.Ref] = e
	}
	return out, nil
}

func deriveLineageScope(featDir, slug string, lineages []string, affects []string) ([]lineageScope, error) {
	named := map[string]bool{}
	for _, raw := range affects {
		if ref, err := parser.ParseAmendmentRef(raw); err == nil {
			named[canonicalAmendmentRef(ref)] = true
		}
	}

	entries, err := enumerateContractEntries(featDir, slug)
	if err != nil {
		return nil, err
	}

	// A duplicate ref anywhere makes "the exact population" ambiguous, whatever
	// produced it.
	seenRef := map[string]bool{}
	for _, e := range entries {
		ref := e.Ref
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
			if e.Sources == "" || !sourceNamesIntent(e.Sources, slug, lineage) {
				continue
			}
			se := scopedEntry{Ref: e.Ref, Fingerprint: e.Fingerprint}
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
func enumerateContractEntries(featDir, slug string) ([]contractEntry, error) {

	// enumerateContractEntries reads every contract entry the feature declares.
	// FAIL CLOSED. An earlier version skipped an artifact it could not parse,
	// so an unreadable file holding only unlisted attributed entries produced
	// an "exact population" that was neither exact nor the population — and the
	// human approved a closure assertion over a list with a hole in it.
	//
	// Absence and malformation are different: a feature legitimately has no
	// surface or no infrastructure, but a file that exists and will not parse
	// is an unknown, and an unknown here is unapprovable.
	var entries []contractEntry

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
			entries = append(entries, contractEntry{Ref: fmt.Sprintf("@%s/operation:%s", slug, op.ID), Fingerprint: fp, Sources: op.Source})
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
			entries = append(entries, contractEntry{Ref: fmt.Sprintf("@%s/surface:%s", slug, parser.Slugify(f.Name)), Fingerprint: fp, Sources: f.Source})
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
			entries = append(entries, contractEntry{
				Ref:         fmt.Sprintf("@%s/infrastructure:%s", slug, parser.Slugify(f.heading)),
				Fingerprint: fp, Sources: infraFragmentSource(f.body)})
		}
	}

	return entries, nil
}

// --- Contract resolution across the whole ref grammar ---

// contractResolver resolves a canonical contract ref — any feature, any kind,
// including root-scoped domain entities — to the entry it names and that
// entry's exact semantic fingerprint.
//
// The scope grammar is deliberately cross-feature. `affects:` has always been
// able to name another feature's operation or a domain entity, and
// `replaced_by` accepts the same grammar because it is the same grammar.
// Resolving it against ONE feature's index made every legal cross-feature or
// domain replacement look nonexistent: a designer writing a true statement was
// told it does not resolve against the contract, which is both wrong and
// unactionable. There were two honest options — resolve the whole grammar, or
// reject in the schema the part the applier cannot honour. Accepting syntax
// that can never be satisfied was the one option not available.
//
// Cross-feature replacement is also the ORDINARY shape of a narrowing. Work
// that stops being promised here rarely evaporates; it moves to whatever
// already owns that ground, and that owner is frequently in another feature.
//
// Fail closed throughout: a target artifact that exists and will not parse is
// an error, never a miss. "Does not resolve" must mean the entry is absent, not
// that the reader gave up.
type contractResolver struct {
	cfg     *config.Context
	feature map[string]map[string]contractEntry
	domain  map[string]string
	domainL bool
}

func newContractResolver(cfg *config.Context) *contractResolver {
	return &contractResolver{cfg: cfg, feature: map[string]map[string]contractEntry{}}
}

// resolve returns the entry a canonical ref names. The bool distinguishes
// absence from failure; both callers must keep them apart, because an
// unreadable artifact is not evidence that a replacement is missing.
func (r *contractResolver) resolve(canonicalRef string) (contractEntry, bool, error) {
	ref, err := parser.ParseAmendmentRef(canonicalRef)
	if err != nil {
		return contractEntry{}, false, fmt.Errorf("%s is not a contract reference: %w",
			canonicalRef, err)
	}
	if ref.Kind == "domain" {
		fps, derr := r.domainEntities()
		if derr != nil {
			return contractEntry{}, false, derr
		}
		fp, ok := fps[ref.Name]
		if !ok {
			return contractEntry{}, false, nil
		}
		return contractEntry{Ref: canonicalAmendmentRef(ref), Fingerprint: fp}, true, nil
	}
	idx, ok := r.feature[ref.Feature]
	if !ok {
		featDir := r.cfg.FeaturePath(ref.Feature)
		// The same absence-versus-failure split the resolver enforces one layer
		// up, applied at the directory probe. Folding every stat error into "no
		// spec directory" is fail-closed but tells a designer their reference
		// names a feature that does not exist, when what actually happened is
		// that the probe could not read it. Lstat, so a dangling symlink is a
		// thing that cannot be read rather than a thing that is not there.
		st, serr := os.Lstat(featDir)
		switch {
		case serr != nil && os.IsNotExist(serr):
			return contractEntry{}, false, fmt.Errorf(
				"%s names feature %q, which has no spec directory, so the reference cannot be "+
					"resolved", canonicalRef, ref.Feature)
		case serr != nil:
			// Defence in depth, not a pinned guard: with Lstat the only other
			// reachable error is an unsearchable parent, and spec/intents is
			// the parent of the asking feature too, so this feature's own
			// derivation fails first and reports that instead. It stays
			// because folding an unknown into "no such feature" is the wrong
			// answer whenever it does become reachable.
			return contractEntry{}, false, fmt.Errorf(
				"%s names feature %q, whose spec directory cannot be read (%v), so whether the "+
					"reference resolves is unknown", canonicalRef, ref.Feature, serr)
		case !st.IsDir():
			return contractEntry{}, false, fmt.Errorf(
				"%s names feature %q, but %s is not a directory, so the reference cannot be "+
					"resolved", canonicalRef, ref.Feature, featDir)
		}
		built, berr := deriveContractIndex(featDir, ref.Feature)
		if berr != nil {
			return contractEntry{}, false, berr
		}
		r.feature[ref.Feature] = built
		idx = built
	}
	e, found := idx[canonicalAmendmentRef(ref)]
	return e, found, nil
}

// domainEntities fingerprints the root domain model's entities.
//
// The model is root-scoped: the ref's feature part records who is asking, and
// the entity resolves against the active root — the same rule resolveAmendmentRef
// applies to `affects:`. The fingerprint comes from the raw node for the same
// reason it does everywhere else here: a parsed struct silently drops the fields
// the schema has not modelled yet, and an edit confined to one of those is
// exactly the change an approval token must not miss.
func (r *contractResolver) domainEntities() (map[string]string, error) {
	if r.domainL {
		return r.domain, nil
	}
	path := r.cfg.DomainModelPath()
	present, err := strictArtifactPresence(path)
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, fmt.Errorf("this record names a domain entity, but the root has no domain "+
			"model at %s, so the reference cannot be resolved", path)
	}
	raw, rerr := resolvedEntryFingerprints(path, "entities", "name")
	if rerr != nil {
		return nil, fmt.Errorf("deriving the domain entities: %s cannot be read (%v), so the "+
			"reference cannot be resolved", path, rerr)
	}
	// Parsed from the SAME BYTES, deliberately not through LoadDomainModel:
	// that caches the artifact on the context, so at the commit recheck it can
	// hand back the model as it was before a lock-boundary edit while the raw
	// reader above sees the edited file. An agreement check between a stale
	// read and a fresh one is not an agreement check.
	data, derr := os.ReadFile(path)
	if derr != nil {
		return nil, fmt.Errorf("deriving the domain entities: %s cannot be read (%v)", path, derr)
	}
	var dm config.DomainModelArtifact
	if uerr := yaml.Unmarshal(data, &dm); uerr != nil {
		return nil, fmt.Errorf("deriving the domain entities: %s cannot be parsed (%v)", path, uerr)
	}
	// The parser and the raw reader must agree about the document, for the same
	// reason the capability and surface readers must. Note this is belt and
	// braces rather than a load-bearing guard: resolvedEntryFingerprints
	// already refuses a duplicated or unkeyable entry, so there is no reachable
	// document that reaches here disagreeing. It stays as a consistency
	// assertion against a future loosening of that reader, and is not counted
	// among the tested guards.
	if len(dm.Entities) != len(raw) {
		return nil, fmt.Errorf("deriving the domain entities: %d parsed but %d raw — the subject "+
			"cannot be derived from a document the parser and the reader disagree about",
			len(dm.Entities), len(raw))
	}
	for _, e := range dm.Entities {
		if _, ok := raw[e.Name]; !ok {
			return nil, fmt.Errorf("deriving the domain entities: %q has no raw node", e.Name)
		}
	}
	r.domain, r.domainL = raw, true
	return r.domain, nil
}
