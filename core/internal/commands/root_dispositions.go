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
	"fmt"
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

// DispositionRecord is the operator-authored file passed via
// --dispositions: every feature in the retiring root, mapped to exactly
// one disposition.
type DispositionRecord struct {
	Dispositions []Disposition `yaml:"dispositions"`
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
// record. Term and rationale validation happen here — a term outside
// the closed set is refused naming the three accepted terms, a valid
// term with no rationale is refused — but completeness against the
// enumeration is checkDispositionCompleteness's job, because it needs
// the enumeration.
func LoadDispositionRecord(path string) (*DispositionRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read disposition record %s: %w", path, err)
	}
	var rec DispositionRecord
	if err := yaml.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("parse disposition record %s: %w", path, err)
	}
	for i, d := range rec.Dispositions {
		if strings.TrimSpace(d.Feature) == "" {
			return nil, fmt.Errorf("disposition record %s: entry %d names no feature", path, i+1)
		}
		switch d.Term {
		case dispositionDeliveredAndDeleted, dispositionBuiltButUndelivered:
			// valid, no target expected
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
