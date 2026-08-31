// parlay-feature: parlay-tool/root-retirement
// parlay-component: cross-cutting/feature-disposition-preflight
// parlay-extends: parlay-tool/root-retirement/cross-cutting/re-home-target-readiness
//
// The feature disposition preflight: before a root retires, an
// operator-authored record must say what became of every feature in it.
//
// The term vocabulary is closed at three — delivered-and-deleted,
// built-but-undelivered, authority-re-homed-to (the third carrying a
// target feature ref) — and every disposition requires a non-empty
// free-text rationale. The rationale is never parsed and never consulted
// to decide anything: the same three terms are accepted whatever any
// rationale says. A term with no rationale is a classification nobody
// can check, so it is refused.
//
// Enumeration is directory-based over the retiring root's spec/intents
// tree. Nothing about whether a feature was built is derived from the
// presence, absence or emptiness of its build state: a feature carrying
// only a placeholder baseline, or an incomplete artifact subset, is
// enumerated and requires a disposition exactly like any other.
//
// This file also owns re-home target readiness: an authority-re-homed-to
// disposition is only as good as its target, so every target is
// validated before the run mutates anything — it exists (resolution
// crosses roots via the parent's index), it is active (a feature
// carrying a retirement of its own, applied or authored-and-waiting, is
// not), and it already claims the surviving work (the ownership markers
// on the surviving files must already name the target, because a target
// that will take ownership afterwards does not own the work during the
// retirement).

package commands

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// The closed disposition term set. Three members, no fourth: a delivery
// that was a removal is recorded as delivered-and-deleted with the story
// in its rationale, not with a new term.
const (
	dispositionDeliveredAndDeleted = "delivered-and-deleted"
	dispositionBuiltButUndelivered = "built-but-undelivered"
	dispositionAuthorityReHomedTo  = "authority-re-homed-to"
)

// dispositionTerms lists the accepted terms, for refusal messages.
var dispositionTerms = []string{
	dispositionDeliveredAndDeleted,
	dispositionBuiltButUndelivered,
	dispositionAuthorityReHomedTo,
}

// Disposition is one feature's entry in the record: what became of it,
// in one closed-vocabulary term, with the reasoning a person can check.
type Disposition struct {
	Feature string `yaml:"feature"`
	Term    string `yaml:"term"`
	// Target carries the receiving feature ref for
	// authority-re-homed-to; empty otherwise.
	Target    string `yaml:"target,omitempty"`
	Rationale string `yaml:"rationale"`
}

// AcknowledgedReference is one inbound finding the operator has read
// and accepted as historical prose rather than a live dependency.
//
// The sweep is deliberately fail-closed and deliberately wide: it
// reports anything that names the retiring root's namespace, because a
// check whose purpose is to establish that nothing points here cannot
// decide on its own that a particular mention is only a mention. That
// leaves the judgment where it belongs — with a person — but until now
// the person had nowhere to record it, so the only way past a finding
// they had genuinely assessed was to delete the sentence. This is the
// place to record it instead, and it is what makes the promised human
// dismissal real rather than notional.
//
// It dismisses one exact IDENTITY CLASS: every finding whose path,
// position, reference and kind — the full identity the preview reports —
// all match exactly. Findings sharing that identity (the same reference
// written twice on one line) are indistinguishable by construction, so
// one acknowledgment covers them together; there is no per-occurrence
// ordinal, and a duplicate identical entry is rejected as redundant
// rather than read as stronger authority. There is no pattern, no prefix
// and no wildcard, because an acknowledgment that covers findings the
// operator has not read is indistinguishable from switching the check
// off.
type AcknowledgedReference struct {
	// Path is the artifact holding the reference, as the preview
	// reports it — either the path relative to the project root or the
	// absolute path.
	Path string `yaml:"path"`
	// Position is the finding's position exactly as the preview
	// reports it (e.g. "line 17"). Requiring it makes an acknowledgment
	// name ONE finding: the same reference appearing twice in a file is
	// two findings, and a future identical reference added elsewhere in
	// the file is a new finding nobody has read yet.
	Position string `yaml:"position"`
	// Reference is the reference exactly as the finding carries it.
	Reference string `yaml:"reference"`
	// Kind is the finding's kind exactly as reported
	// (e.g. "path-reference"); part of the finding's identity.
	Kind string `yaml:"kind"`
	// Rationale says why this one is prose rather than dependency. As
	// with a disposition's rationale it is never parsed; requiring it
	// is what keeps the dismissal checkable by a second reader.
	Rationale string `yaml:"rationale"`
}

// DispositionRecord is the operator-authored file passed via
// --dispositions: every feature in the retiring root, mapped to exactly
// one disposition, plus any inbound findings the operator has assessed
// and accepted.
type DispositionRecord struct {
	Dispositions []Disposition `yaml:"dispositions"`
	// Acknowledged lists inbound sweep findings the operator accepts as
	// historical prose. Optional; absent means nothing is dismissed.
	Acknowledged []AcknowledgedReference `yaml:"acknowledged-references,omitempty"`
	// Path is where this record was read from, resolved. Never decoded
	// from the file — a record does not get to say where it lives — and
	// used to exempt exactly this file from the inbound sweep, which
	// would otherwise report the operator's own answers back as
	// evidence against them.
	Path string `yaml:"-"`
}

// matches reports whether this acknowledgment dismisses that finding's
// identity class: reference, position and kind must all be identical,
// and the artifact must be the same file; the path may be written
// relative to the project root or absolutely, since the preview reports
// one form and a person may reasonably copy either. Matching is
// existential over the class, not consuming per occurrence — findings
// with identical full identity are indistinguishable, so one entry
// answers them all. Nothing else matches — no prefix, no pattern, no
// wildcard, and no position-blind match that would silently cover a
// DIFFERENT line's occurrence or a future one — because an
// acknowledgment covering findings the operator has not read is
// indistinguishable from switching the check off.
func (a AcknowledgedReference) matches(parentPath string, f RootSweepFinding) bool {
	if a.Reference != f.Ref || a.Position != f.Position || a.Kind != f.Kind {
		return false
	}
	want := filepath.ToSlash(filepath.Clean(a.Path))
	if want == filepath.ToSlash(filepath.Clean(f.Path)) {
		return true
	}
	rel, err := filepath.Rel(parentPath, f.Path)
	return err == nil && want == filepath.ToSlash(rel)
}

// acknowledges reports whether the record dismisses this exact finding.
func (r *DispositionRecord) acknowledges(parentPath string, f RootSweepFinding) (AcknowledgedReference, bool) {
	if r == nil {
		return AcknowledgedReference{}, false
	}
	for _, a := range r.Acknowledged {
		if a.matches(parentPath, f) {
			return a, true
		}
	}
	return AcknowledgedReference{}, false
}

// byFeature indexes the record. Duplicates are checked by
// checkDispositionCompleteness, which sees the full list.
func (r *DispositionRecord) byFeature() map[string]Disposition {
	out := map[string]Disposition{}
	if r == nil {
		return out
	}
	for _, d := range r.Dispositions {
		out[d.Feature] = d
	}
	return out
}

// LoadDispositionRecord reads and validates the operator-authored YAML
// record, structurally closed. The document must decode with no unknown
// keys — a misspelled field is refused, never dropped — and carry
// exactly one YAML document. Term and rationale validation happen here
// too: a term outside the closed set is refused naming the three
// accepted terms, a valid term with no rationale is refused, and a term
// that names no target carrying one anyway is refused as the
// contradiction it is. Completeness against the enumeration is
// checkDispositionCompleteness's job, because it needs the enumeration.
func LoadDispositionRecord(path string) (*DispositionRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read disposition record %s: %w", path, err)
	}
	// The record is a destructive authorization, so it is read
	// structurally CLOSED: a key the shape does not define is an error,
	// not something to ignore. A silently dropped `raitonale:` or
	// `feautre:` would otherwise read as a well-formed record that
	// authorizes a deletion nobody actually wrote down.
	var rec DispositionRecord
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&rec); err != nil && err != io.EOF {
		return nil, fmt.Errorf("parse disposition record %s: %w", path, err)
	}
	// One document, and nothing after it: a second document would carry
	// dispositions this record never presents for checking.
	var extra DispositionRecord
	if err := dec.Decode(&extra); err == nil {
		return nil, fmt.Errorf("disposition record %s carries more than one YAML document — the record is one document naming every feature exactly once", path)
	} else if err != io.EOF {
		return nil, fmt.Errorf("parse disposition record %s: %w", path, err)
	}
	for i, d := range rec.Dispositions {
		if strings.TrimSpace(d.Feature) == "" {
			return nil, fmt.Errorf("disposition record %s: entry %d names no feature", path, i+1)
		}
		switch d.Term {
		case dispositionDeliveredAndDeleted, dispositionBuiltButUndelivered:
			// No target is expected, and one carried anyway is refused
			// rather than ignored: a record saying authority moved to a
			// named feature under a term that moves nothing is two
			// contradictory statements, and quietly honouring the term
			// picks one of them for the operator.
			if strings.TrimSpace(d.Target) != "" {
				return nil, fmt.Errorf("disposition record %s: %s carries term %s with target %q — only %s names a target, and a contradictory record is refused rather than resolved in the operator's stead",
					path, d.Feature, d.Term, d.Target, dispositionAuthorityReHomedTo)
			}
		case dispositionAuthorityReHomedTo:
			if strings.TrimSpace(d.Target) == "" {
				return nil, fmt.Errorf("disposition record %s: %s is %s but names no target feature",
					path, d.Feature, dispositionAuthorityReHomedTo)
			}
		default:
			return nil, fmt.Errorf("disposition record %s: %s carries term %q, which is not in the closed set — the accepted terms are %s",
				path, d.Feature, d.Term, strings.Join(dispositionTerms, ", "))
		}
		// The rationale is required but never parsed: whatever it says,
		// only the three closed terms are accepted, and nothing in the
		// rationale widens or narrows that.
		if strings.TrimSpace(d.Rationale) == "" {
			return nil, fmt.Errorf("disposition record %s: %s carries term %s with no rationale — a term with no rationale is a classification nobody can check",
				path, d.Feature, d.Term)
		}
	}
	for i, a := range rec.Acknowledged {
		if strings.TrimSpace(a.Path) == "" {
			return nil, fmt.Errorf("disposition record %s: acknowledged reference %d names no path — an acknowledgment dismisses one finding identity in one artifact, so it has to say which", path, i+1)
		}
		if strings.TrimSpace(a.Reference) == "" {
			return nil, fmt.Errorf("disposition record %s: the acknowledged reference in %s names no reference — the reference must match the finding exactly, so an empty one would dismiss whatever happened to be found there", path, a.Path)
		}
		if strings.TrimSpace(a.Position) == "" {
			return nil, fmt.Errorf("disposition record %s: the acknowledgment of %q in %s carries no position — an acknowledgment names one exact finding identity, and without its position it would also dismiss every other line's occurrence, present or future", path, a.Reference, a.Path)
		}
		if strings.TrimSpace(a.Kind) == "" {
			return nil, fmt.Errorf("disposition record %s: the acknowledgment of %q in %s carries no kind — the kind is part of the finding's identity as the preview reports it", path, a.Reference, a.Path)
		}
		if strings.TrimSpace(a.Rationale) == "" {
			return nil, fmt.Errorf("disposition record %s: the acknowledgment of %q in %s carries no rationale — a dismissal nobody can check is the thing this section exists to avoid",
				path, a.Reference, a.Path)
		}
	}
	// Exact duplicate acknowledgments are refused rather than tolerated:
	// one entry already covers the whole identity class, so a repeat adds
	// no authority and can only mislead a reader into thinking
	// multiplicity means something.
	seenAcks := map[string]int{}
	for i, a := range rec.Acknowledged {
		key := a.Path + "\x00" + a.Position + "\x00" + a.Reference + "\x00" + a.Kind
		if prev, dup := seenAcks[key]; dup {
			return nil, fmt.Errorf("disposition record %s: acknowledged reference %d duplicates entry %d (%q at %s in %s) — one acknowledgment covers every finding sharing that exact identity; repetition is not stronger authority",
				path, i+1, prev+1, a.Reference, a.Position, a.Path)
		}
		seenAcks[key] = i
	}
	// Where the record was read from, resolved, so the sweep can exempt
	// exactly this file and nothing else.
	if abs, err := filepath.Abs(path); err == nil {
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			rec.Path = filepath.Clean(resolved)
		} else {
			rec.Path = filepath.Clean(abs)
		}
	}
	return &rec, nil
}

// checkDispositionCompleteness compares the record against the
// directory-based enumeration of the retiring root's features: no
// enumerated feature omitted, no feature named that the enumeration did
// not produce, no feature named more than once.
func checkDispositionCompleteness(enumerated []string, record *DispositionRecord) []error {
	var errs []error
	seen := map[string]int{}
	for _, d := range record.Dispositions {
		seen[d.Feature]++
	}
	for f, n := range seen {
		if n > 1 {
			errs = append(errs, fmt.Errorf("disposition record names %s %d times — every feature carries exactly one disposition", f, n))
		}
	}
	enumSet := map[string]bool{}
	for _, f := range enumerated {
		enumSet[f] = true
		if _, ok := seen[f]; !ok {
			errs = append(errs, fmt.Errorf("disposition record omits %s — every enumerated feature requires a disposition", f))
		}
	}
	var extras []string
	for f := range seen {
		if !enumSet[f] {
			extras = append(extras, f)
		}
	}
	sort.Strings(extras)
	for _, f := range extras {
		errs = append(errs, fmt.Errorf("disposition record names %s, which the enumeration of the retiring root did not produce", f))
	}
	sort.Slice(errs, func(i, j int) bool { return errs[i].Error() < errs[j].Error() })
	return errs
}

// enumerateRetiringFeatures is the directory-based enumeration the
// preflight compares the record against. A package var so tests can
// instrument WHEN it is called (the preflight-ordering invariant: no
// enumeration before resolution and destination checking succeed).
var enumerateRetiringFeatures = func(rootPath string) ([]string, error) {
	if err := retirementEvent("enumerate-features"); err != nil {
		return nil, err
	}
	return config.ScanFeatureTree(filepath.Join(rootPath, config.SpecDir, config.IntentsDir))
}

// rehomeAmendmentState reports a feature's retirement state as
// check-amendments computes it: retiredBy names an applied terminal
// retirement, pending an authored-but-unapplied one. A package var so
// tests can substitute ledger states without standing up full amendment
// fixtures; the default is the real computation.
var rehomeAmendmentState = func(cfg *config.Context, slug string) (retiredBy, pending string) {
	out := computeCheckAmendments(cfg, slug)
	return out.RetiredBy, out.PendingRetirement
}

// resolveFeatureAcrossRoots finds which non-retiring root holds the
// named feature. Resolution crosses root boundaries via the parent's
// index: the parent root and every registered child except the retiring
// one are searched.
func resolveFeatureAcrossRoots(parentPath string, idx *config.RootsIndex, retiring config.Root, slug string) (*config.Context, bool) {
	roots := []config.Root{{
		Name: filepath.Base(parentPath),
		Path: parentPath,
		Kind: config.RootKindParent,
	}}
	if idx != nil {
		for _, c := range idx.Children {
			if c.Name == retiring.Name {
				continue
			}
			roots = append(roots, c)
		}
	}
	for _, r := range roots {
		cfg := config.NewContext(&config.ResolutionResult{ActiveRoot: r}, idx)
		featDir := cfg.FeaturePath(slug)
		if _, err := os.Stat(filepath.Join(featDir, "intents.md")); err == nil {
			return cfg, true
		}
	}
	return nil, false
}

// checkRehomeTargets validates every authority-re-homed-to disposition
// target before the run mutates anything: exists / active / already
// claims the surviving work, cross-root. Each refusal names the target,
// the failing condition, and for the third condition the file whose
// claim has not moved.
//
// The retirement record is never accepted as the owner of surviving
// work: only live features resolve here, and a record of something
// ending cannot own live code.
func checkRehomeTargets(parentPath string, idx *config.RootsIndex, retiring config.Root, record *DispositionRecord, sweep RootSweepResult) []error {
	var errs []error
	if record == nil {
		return nil
	}
	for _, d := range record.Dispositions {
		if d.Term != dispositionAuthorityReHomedTo {
			continue
		}
		targetSlug := parser.FeatureSlug(d.Target)

		// Condition 1: the named feature exists in the project. A
		// target naming no live feature — the retirement record
		// included — refuses: only a live feature can own surviving
		// work; a record of an ending cannot.
		targetCfg, found := resolveFeatureAcrossRoots(parentPath, idx, retiring, targetSlug)
		if !found {
			errs = append(errs, fmt.Errorf("re-home target %q for %s resolves to no live feature in the project — only a live feature can own surviving work; a record of an ending cannot",
				d.Target, d.Feature))
			continue
		}

		// Condition 2: the target is active. A feature carrying a
		// retirement of its own — applied, or authored and waiting to
		// be applied — is not active, because a record of something
		// ending cannot own live code.
		retiredBy, pending := rehomeAmendmentState(targetCfg, targetSlug)
		if retiredBy != "" {
			errs = append(errs, fmt.Errorf("re-home target %q for %s is itself retired (by %s) — a retirement record cannot own live code",
				d.Target, d.Feature, retiredBy))
			continue
		}
		if pending != "" {
			errs = append(errs, fmt.Errorf("re-home target %q for %s carries an authored but unapplied retirement (%s) — authored-and-waiting is not active",
				d.Target, d.Feature, pending))
			continue
		}

		// Condition 3 (decision: rehomed-ownership-nonblocking-at-sweep):
		// the target already claims the surviving work.
		// Every surviving file the sweep found still owned by the
		// re-homed feature must carry the target's marker too — a
		// target that will take ownership afterwards does not own the
		// work during the retirement.
		for _, finding := range sweep.Findings {
			if finding.Kind != sweepKindOwnershipMarker || finding.Feature != d.Feature {
				continue
			}
			// A finding the record acknowledges is one the operator has
			// already judged prose rather than claim — a document QUOTING
			// a marker (an inventory, an amendment) is not a file owned by
			// the retiring feature, and readiness asks exactly the
			// question the acknowledgment answered.
			if _, ok := record.acknowledges(parentPath, finding); ok {
				continue
			}
			if !fileClaimsFeature(finding.Path, targetSlug) {
				errs = append(errs, fmt.Errorf("re-home target %q for %s does not yet claim %s — the file's ownership markers still name the retiring feature, and its claim has not moved",
					d.Target, d.Feature, finding.Path))
			}
		}
	}
	return errs
}

// fileClaimsFeature reports whether the file's marker block names the
// given feature as an owner (parlay-feature: or parlay-extends:).
func fileClaimsFeature(path, slug string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		f, ok := markerFeature(line)
		if !ok {
			continue
		}
		if f == slug {
			return true
		}
	}
	return false
}
