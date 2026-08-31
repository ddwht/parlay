package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// Consequence accounting for transitions that take support away.
//
// Stage 1b could apply only the preservation form — an extend, or a revise
// whose exceptions list is empty — because a transition that withdraws support
// from a contract entry has consequences, and there was nothing to collect
// them. Approving a loss whose consequences nobody gathered would have been the
// original bypass in a new verb.
//
// This is that collection. Each exception carries a disposition, and each
// disposition has an OPERATIONAL meaning the tool can check against the
// artifacts — not a label whose interpretation is left to the reader. What the
// tool cannot check remains the human's, and is still said out loud.

// dispositionRule states what a disposition claims and what can be verified.
type dispositionRule struct {
	// MustResolve says the entry is expected to still exist afterwards.
	MustResolve bool
	// MustBeNamed says the entry must appear in the record's affects:, because
	// the claim asserts the splice changed it.
	MustBeNamed bool
	// MustStayAttributed says the entry must still be justified by THIS
	// lineage afterwards. Existence is not enough: an entry re-sourced to
	// another promise still exists while no longer being supported by this
	// one, which is precisely what `retained` claims.
	MustStayAttributed bool
	// NeedsReplacement says the claim names where the promise went.
	NeedsReplacement bool
	// Claim is what the author is asserting, in their words.
	Claim string
}

// dispositionRules gives each disposition an exact operational meaning.
//
// `retained` was refused outright in Stage 1b precisely because it had none:
// it could have meant unchanged, changed-but-still-supported, or specially
// reviewed, and a second undefined spelling of preservation beside
// preserves_unlisted is worse than no spelling. It gets one here.
var dispositionRules = map[string]dispositionRule{
	"retained": {
		MustResolve: true, MustStayAttributed: true,
		Claim: "this entry survives the transition and the changed promise still supports it",
	},
	"revised": {
		MustResolve: true, MustBeNamed: true,
		// Deliberately NOT MustStayAttributed. A revision may re-source an
		// entry to the promise that now justifies it, and that is a legitimate
		// outcome rather than a loss — said explicitly so the retained rule is
		// not applied here by accident.
		Claim: "this entry survives and this record changed it; it may now be justified elsewhere",
	},
	"removed": {
		Claim: "this entry is gone from the contract, and nothing takes it over",
	},
	"replaced-by": {
		// Same reasoning as removed: the entry is gone, so it cannot be an
		// affects: ref. Where its work went is the disposition's whole content.
		NeedsReplacement: true,
		Claim:            "this entry is gone, and the named entry carries its work now",
	},
}

// scopeConsequences is the checked result of a record's exception list.
type scopeConsequences struct {
	// ByLineage are the structured results, owned by the lineage they belong
	// to. Per-lineage rather than pooled: one exception on lineage A must not
	// satisfy lineage B's obligations, and B's receipt must not carry A's fate.
	//
	// Structured rather than prose. A []string of claims records that
	// something was said; it does not record WHICH subject, in what state,
	// with which replacement — so a later reader could not audit the decision,
	// and the digest would cover a sentence rather than the facts it asserts.
	ByLineage map[string][]ConsequenceReceipt
	Problems  []string
}

// ConsequenceReceipt is the durable, auditable result of one declared
// consequence: which entry, as it was, what became of it, and — where the
// disposition names one — the replacement that carries its work now, bound by
// its own fingerprint.
type ConsequenceReceipt struct {
	Lineage           string `yaml:"lineage" json:"lineage"`
	Ref               string `yaml:"ref" json:"ref"`
	BeforeFingerprint string `yaml:"before-fingerprint" json:"before_fingerprint"`
	Disposition       string `yaml:"disposition" json:"disposition"`
	Claim             string `yaml:"claim" json:"claim"`
	// AfterRef and AfterFingerprint bind the entry as it stands now — the
	// surviving entry for retained and revised (even if re-sourced), or the
	// replacement for replaced-by. Taken from the whole-contract index rather
	// than from a lineage scope, because the subject may have left the scope.
	AfterRef         string `yaml:"after-ref,omitempty" json:"after_ref,omitempty"`
	AfterFingerprint string `yaml:"after-fingerprint,omitempty" json:"after_fingerprint,omitempty"`
}

// checkScopeConsequences verifies every declared exception against three
// DIFFERENT predicates, which an earlier version conflated into one.
//
//   - EXISTENCE — does the ref resolve against the contract now? Answered by
//     the resolver, over the whole contract, not by the attributed population.
//     An earlier version used attribution as existence, so an entry that
//     existed but belonged to another promise could be declared removed, and a
//     legitimate replacement elsewhere in the contract was rejected.
//   - ATTRIBUTION — was it justified by this lineage? Answered BEFORE the
//     splice, because a removed entry is invisible afterwards.
//   - DECLARED MUTATION — does affects: name it? Provenance, never existence.
func checkScopeConsequences(cfg *config.Context, slug string, record parser.Amendment, before, after []lineageScope, narrowing map[string]bool) scopeConsequences {
	out := scopeConsequences{ByLineage: map[string][]ConsequenceReceipt{}}
	// Existence is answered by the resolver over the whole ref grammar, not by
	// one feature's index: `replaced_by` and a disposition subject both accept
	// cross-feature and domain refs, and a legal ref that the checker cannot
	// reach must not be reported as an absent one.
	resolver := newContractResolver(cfg)
	// This feature's own contract is still derived eagerly, even when no
	// exception happens to resolve against it. If it cannot be read, the
	// attribution and survivor sets compared below are already built on an
	// unknown, and there is nothing here worth reporting except that.
	if _, ierr := deriveContractIndex(cfg.FeaturePath(slug), slug); ierr != nil {
		out.Problems = append(out.Problems, ierr.Error())
		return out
	}
	si := record.ScopeImpact
	if si == nil {
		return out
	}

	// Attribution, from the PRE-SPLICE inventory.
	attributedTo := map[string]map[string]bool{}
	for _, sc := range before {
		set := map[string]bool{}
		for _, e := range append(append([]scopedEntry{}, sc.Named...), sc.Unlisted...) {
			set[e.Ref] = true
		}
		attributedTo[sc.Lineage] = set
	}
	// Survivors, from the post-splice inventory.
	survives := map[string]map[string]bool{}
	for _, sc := range after {
		set := map[string]bool{}
		for _, e := range append(append([]scopedEntry{}, sc.Named...), sc.Unlisted...) {
			set[e.Ref] = true
		}
		survives[sc.Lineage] = set
	}
	named := map[string]bool{}
	for _, raw := range record.Affects {
		if canon, err := parser.CanonicalScopeRef(raw); err == nil {
			named[canon] = true
		}
	}
	lineageOfRecord := map[string]bool{}
	for _, tr := range record.IntentTransitions() {
		lineageOfRecord[tr.Intent] = true
	}

	// EXISTENCE is answered over the WHOLE contract, not a lineage's slice —
	// and the whole contract now means every feature the ref grammar reaches.
	// A resolution FAILURE is not absence: it is reported as its own problem,
	// because "your replacement does not exist" is a different and much more
	// misleading statement than "that artifact could not be read".
	resolved := map[string]contractEntry{}
	unreachable := map[string]bool{}
	exists := func(canon string) bool {
		if _, done := resolved[canon]; done {
			return true
		}
		if unreachable[canon] {
			return false
		}
		e, ok, rerr := resolver.resolve(canon)
		if rerr != nil {
			out.Problems = append(out.Problems, rerr.Error())
			unreachable[canon] = true
			return false
		}
		if ok {
			resolved[canon] = e
		}
		return ok
	}

	dispositioned := map[string]map[string]bool{}
	for _, ex := range si.Exceptions {
		canon, cerr := parser.CanonicalScopeRef(ex.Ref)
		if cerr != nil {
			continue // shape, reported by ValidateScopeImpact
		}
		rule, known := dispositionRules[ex.Disposition]
		if !known {
			out.Problems = append(out.Problems, fmt.Sprintf(
				"%s declares disposition %q, which has no defined meaning", canon, ex.Disposition))
			continue
		}
		lineage := strings.TrimSpace(ex.Intent)
		if !lineageOfRecord[lineage] {
			out.Problems = append(out.Problems, fmt.Sprintf(
				"%s is dispositioned under lineage %q, which this record does not change",
				canon, lineage))
			continue
		}
		// The subject must be one this promise ACTUALLY justified, established
		// before the splice. Without this any plausible absent ref could be
		// declared removed and the record would be evidence only that somebody
		// claimed a consequence.
		if !attributedTo[lineage][canon] {
			out.Problems = append(out.Problems, fmt.Sprintf(
				"%s is dispositioned under %q, but that promise did not justify it before this "+
					"change — a consequence is about something the promise actually supported",
				canon, lineage))
			continue
		}
		if dispositioned[lineage] == nil {
			dispositioned[lineage] = map[string]bool{}
		}
		dispositioned[lineage][canon] = true

		present := exists(canon)
		if rule.MustResolve && !present && !unreachable[canon] {
			out.Problems = append(out.Problems, fmt.Sprintf(
				"%s is dispositioned %q, which says it survives, but it is not in the contract",
				canon, ex.Disposition))
		}
		if !rule.MustResolve && present {
			out.Problems = append(out.Problems, fmt.Sprintf(
				"%s is dispositioned %q, which says it is gone, but it is still in the contract — "+
					"the splice and the declaration disagree", canon, ex.Disposition))
		}
		if rule.MustStayAttributed && !survives[lineage][canon] {
			out.Problems = append(out.Problems, fmt.Sprintf(
				"%s is dispositioned %q under %q, which claims that promise still supports it — "+
					"but it is no longer justified by that promise. Existing somewhere is not the "+
					"same as being supported here", canon, ex.Disposition, lineage))
		}
		if rule.MustBeNamed && !named[canon] {
			out.Problems = append(out.Problems, fmt.Sprintf(
				"%s is dispositioned %q, which says this record changed it, but affects: does not "+
					"name it", canon, ex.Disposition))
		}
		if rule.NeedsReplacement {
			rep := strings.TrimSpace(ex.ReplacedBy)
			if rep == "" {
				out.Problems = append(out.Problems, fmt.Sprintf(
					"%s is dispositioned %q but names no replacement — the disposition's whole "+
						"content is where the work went", canon, ex.Disposition))
			} else if repCanon, rerr := parser.CanonicalScopeRef(rep); rerr != nil ||
				(!exists(repCanon) && !unreachable[repCanon]) {
				// Resolution, never affects: membership. A replacement created
				// by this splice is already on disk when the ceremony runs, so
				// it can and must resolve; accepting a mere claim to touch it
				// would let a replacement that does not exist send a later
				// reader nowhere.
				out.Problems = append(out.Problems, fmt.Sprintf(
					"%s is replaced by %s, which does not resolve against the contract", canon, rep))
			}
		} else if strings.TrimSpace(ex.ReplacedBy) != "" {
			out.Problems = append(out.Problems, fmt.Sprintf(
				"%s is dispositioned %q but names a replacement; only replaced-by carries one",
				canon, ex.Disposition))
		}
		receipt := ConsequenceReceipt{
			Lineage: lineage, Ref: canon, Disposition: ex.Disposition, Claim: rule.Claim,
		}
		for _, sc := range before {
			if sc.Lineage != lineage {
				continue
			}
			for _, e := range append(append([]scopedEntry{}, sc.Named...), sc.Unlisted...) {
				if e.Ref == canon {
					receipt.BeforeFingerprint = e.Fingerprint
				}
			}
		}
		switch {
		case rule.NeedsReplacement:
			if repCanon, rerr := parser.CanonicalScopeRef(ex.ReplacedBy); rerr == nil {
				if e, ok := resolved[repCanon]; ok {
					receipt.AfterRef, receipt.AfterFingerprint = e.Ref, e.Fingerprint
				}
			}
		case rule.MustResolve:
			if e, ok := resolved[canon]; ok {
				receipt.AfterRef, receipt.AfterFingerprint = e.Ref, e.Fingerprint
			}
		}
		out.ByLineage[lineage] = append(out.ByLineage[lineage], receipt)
	}

	// COMPLETENESS, per lineage: every entry the promise justified before must
	// either survive under the closure or carry exactly one disposition.
	for lineage, was := range attributedTo {
		for ref := range was {
			if survives[lineage][ref] || dispositioned[lineage][ref] {
				continue
			}
			out.Problems = append(out.Problems, fmt.Sprintf(
				"%s was justified by %q and is gone from its population, but the record declares "+
					"no consequence for it — a disappearance nobody accounted for is exactly what "+
					"this accounting exists to catch", ref, lineage))
		}
	}

	// A narrowing that loses nothing is a revision. Checked PER LINEAGE: a
	// consequence on one narrowed lineage says nothing about another.
	for lineage := range narrowing {
		if len(dispositioned[lineage]) == 0 {
			out.Problems = append(out.Problems, fmt.Sprintf(
				"%s is narrowed but the record declares no consequence for it — a narrowing that "+
					"takes nothing away is a revision, and should say so", lineage))
		}
	}

	sort.Strings(out.Problems)
	for k := range out.ByLineage {
		sort.Slice(out.ByLineage[k], func(i, j int) bool {
			return out.ByLineage[k][i].Ref < out.ByLineage[k][j].Ref
		})
	}
	return out
}

func scopeBeforeFor(scopes []lineageScope, lineage string) lineageScope {
	for _, sc := range scopes {
		if sc.Lineage == lineage {
			return sc
		}
	}
	return lineageScope{Lineage: lineage}
}

// captureScopeBefore records the attributed inventory before the splice.
//
// Called when the journal stamps amendment-written: the record exists, so its
// lineages are known, and no artifact has been mutated yet. It is the only
// moment at which the prior subject population can be observed at all — after
// the splice a removed entry is simply gone, and "what this promise used to
// justify" becomes unanswerable.
func captureScopeBefore(cfg *config.Context, slug string, journal *refineJournal, seq int) error {
	featDir := cfg.FeaturePath(slug)
	amendments, err := parser.LoadFeatureAmendments(featDir)
	if err != nil {
		return fmt.Errorf("read ledger: %w", err)
	}
	var record *parser.Amendment
	for i := range amendments {
		if amendments[i].Seq == seq {
			record = &amendments[i]
		}
	}
	if record == nil {
		// Hard error. Stamping amendment-written for a sequence with no
		// amendment contradicts the step itself, and letting the run proceed
		// into the splice with no inventory means the problem surfaces at
		// apply time — after the very state needed to recover it is gone.
		return fmt.Errorf("amendment %d does not exist in %s's ledger, so there is nothing to "+
			"record and nothing to capture", seq, slug)
	}
	var lineages []string
	for _, tr := range record.IntentTransitions() {
		if tr.Mode.CarriesNewText() {
			lineages = append(lineages, tr.Intent)
		}
	}
	if len(lineages) == 0 {
		return nil // a found, valid record that changes no promise text
	}
	scopes, err := deriveLineageScope(featDir, slug, lineages, record.Affects)
	if err != nil {
		return fmt.Errorf("capture what this promise justifies before the splice: %w", err)
	}
	hash, ok := hashWholeFile(record.Path)
	if !ok {
		return fmt.Errorf("hash %s so the captured inventory can be tied to it",
			filepath.Base(record.Path))
	}
	sort.Strings(lineages)
	journal.ScopeBefore = scopes
	journal.ScopeBeforeAmendment = seq
	journal.ScopeBeforeFile = filepath.Base(record.Path)
	journal.ScopeBeforeHash = hash
	journal.ScopeBeforeLineages = lineages
	journal.ScopeBeforeDigest = scopeInventoryDigest(scopes)
	return nil
}

// validSHA256 reports whether s is a full hex-encoded SHA-256 digest.
//
// A length check is not a validity check: sixty-four `z` characters are exactly
// as long as a digest and are not one. At an authority boundary the comment and
// the code have to agree, and "a full valid digest" has to mean decodable.
func validSHA256(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == sha256.Size
}

// validateCapturedInventory checks the captured evidence is internally
// coherent before anything relies on it.
//
// Metadata matching is not enough. A malformed or partially written journal can
// omit a lineage, duplicate one, carry a foreign one, repeat a ref or leave a
// fingerprint empty — and completeness would then iterate only what survived,
// silently comparing against a population smaller than the one that existed.
// That is worst for `revise`, which has no per-lineage "must declare something"
// fallback to notice the gap.
//
// Not signed authenticity, and not claimed as such: this detects corruption and
// makes the capture an internally coherent artifact rather than metadata beside
// an unchecked payload.
func validateCapturedInventory(journal *refineJournal, lineages []string) error {
	want := map[string]bool{}
	for _, l := range lineages {
		want[l] = true
	}
	seenLineage := map[string]bool{}
	for _, sc := range journal.ScopeBefore {
		if strings.TrimSpace(sc.Lineage) == "" {
			return fmt.Errorf("the captured inventory holds a scope with no lineage")
		}
		if seenLineage[sc.Lineage] {
			return fmt.Errorf("the captured inventory holds %q twice", sc.Lineage)
		}
		seenLineage[sc.Lineage] = true
		if !want[sc.Lineage] {
			return fmt.Errorf("the captured inventory holds %q, which this record does not change",
				sc.Lineage)
		}
		seenRef := map[string]bool{}
		for _, e := range append(append([]scopedEntry{}, sc.Named...), sc.Unlisted...) {
			canon, cerr := parser.CanonicalScopeRef(e.Ref)
			if cerr != nil || canon != e.Ref {
				return fmt.Errorf("the captured inventory holds %q for %q, which is not a "+
					"canonical contract reference", e.Ref, sc.Lineage)
			}
			if seenRef[e.Ref] {
				return fmt.Errorf("the captured inventory holds %s twice under %q — the same ref "+
					"across DIFFERENT lineages is legitimate, twice within one is not",
					e.Ref, sc.Lineage)
			}
			seenRef[e.Ref] = true
			if !validSHA256(e.Fingerprint) {
				return fmt.Errorf("the captured inventory records no usable fingerprint for %s, "+
					"so what that entry meant cannot be established", e.Ref)
			}
		}
	}
	for l := range want {
		if !seenLineage[l] {
			return fmt.Errorf("the captured inventory has no entry for %q, so what that promise "+
				"justified before this change is unknown", l)
		}
	}
	if journal.ScopeBeforeDigest != "" &&
		journal.ScopeBeforeDigest != scopeInventoryDigest(journal.ScopeBefore) {
		return fmt.Errorf("the captured inventory does not match the digest recorded with it, so " +
			"it has been altered or partially written since capture")
	}
	if journal.ScopeBeforeDigest == "" {
		return fmt.Errorf("the captured inventory carries no digest, so its integrity cannot be " +
			"established")
	}
	return nil
}

// scopeCaptureMatches reports whether a captured inventory is evidence about
// this exact record.
func scopeCaptureMatches(journal *refineJournal, record parser.Amendment, lineages []string) error {
	if journal.ScopeBeforeAmendment != record.Seq {
		return fmt.Errorf("the journal's captured scope is for amendment %d, not this one",
			journal.ScopeBeforeAmendment)
	}
	if journal.ScopeBeforeFile != filepath.Base(record.Path) {
		return fmt.Errorf("the journal's captured scope is for %q, not %q",
			journal.ScopeBeforeFile, filepath.Base(record.Path))
	}
	hash, ok := hashWholeFile(record.Path)
	if !ok {
		return fmt.Errorf("%s cannot be hashed, so the capture cannot be tied to it",
			filepath.Base(record.Path))
	}
	if journal.ScopeBeforeHash != hash {
		return fmt.Errorf("the record changed after its scope was captured, so the captured " +
			"inventory is evidence about bytes that are no longer there")
	}
	want := append([]string(nil), lineages...)
	sort.Strings(want)
	if strings.Join(want, ",") != strings.Join(journal.ScopeBeforeLineages, ",") {
		return fmt.Errorf("the capture covers lineages %v but this record changes %v",
			journal.ScopeBeforeLineages, want)
	}
	return validateCapturedInventory(journal, want)
}
