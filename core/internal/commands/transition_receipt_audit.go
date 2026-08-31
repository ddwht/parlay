package commands

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/parser"
)

// Auditing a stored transition receipt against the amendment it claims to
// approve.
//
// This exists because the ledger's older accounting rule —
// intent-supersession-unaccounted-affect — no longer runs for records written
// in the amends_intents: vocabulary. The dispositions replaced it, and they are
// a stronger accounting, but only while something durable shows they were
// actually checked. That durable thing is the receipt.
//
// THE ADVERSARY IS A RECOMPUTED RECEIPT. The digest already catches a receipt
// whose bytes were edited: observeAppliedAuthority refuses the whole capsule.
// What it cannot catch is a receipt edited and then re-digested, because the
// digest proves a receipt is SELF-CONSISTENT and never that it describes this
// record. So every check here compares the receipt to the amendment on disk,
// which is what the digest cannot do for itself.
//
// A first version of this compared only lineage and mode — the headline. That
// left the accounting entirely unvalidated: keep `check-readiness/retire`,
// delete every consequence, re-digest, and the check passed while nothing
// showed a single retired entry had been accounted for. That is precisely the
// bypass the rule exists to close, so it is compared field by field now.
//
// NOT AUTHENTICITY, and not claimed as such. The baseline is unsigned; anyone
// who can write it can write a consistent forgery. This is consistency and
// audit validation: it makes a receipt evidence about a specific record rather
// than a well-formed object that happens to sit beside one.
//
// It also deliberately does NOT replay mutable historical state. Fingerprints
// are checked for validity, never recomputed against today's artifacts — the
// contract has moved on since the approval, and demanding it match would report
// every honest historical record as broken.

// auditEvolutionReceipt compares a stored receipt to the amendment it claims to
// approve, returning every discrepancy.
func auditEvolutionReceipt(a parser.Amendment, receipt TransitionReceipt) []string {
	var problems []string

	if receipt.Payload.Mode != transitionModeEvolve {
		return []string{fmt.Sprintf("its receipt records mode %q — a receipt from a different "+
			"ceremony does not evidence this one", receipt.Payload.Mode)}
	}

	transitions := a.IntentTransitions()

	// Counting subjects is deliberately NOT a separate check. A map keyed by
	// lineage collapses duplicates, which is how the first version of this
	// missed them — but the fix is to reject the duplicate, not to count. Every
	// way the two can differ is caught below with a message that says which
	// promise is wrong: a repeated subject, a subject for a lineage the record
	// does not change, and a lineage with no subject at all. A count check on
	// top fires a second, vaguer line on cases already reported and can fire on
	// nothing else, so it would be an unpinnable duplicate rather than a guard.

	subjectOf := map[string]*evolutionSubject{}
	for _, sub := range receipt.Payload.Evolution {
		if sub == nil {
			problems = append(problems, "its receipt holds an empty approved promise")
			continue
		}
		lineage := strings.TrimSpace(sub.Lineage)
		if lineage == "" {
			problems = append(problems, "its receipt approves a promise with no lineage")
			continue
		}
		if _, dup := subjectOf[lineage]; dup {
			problems = append(problems, fmt.Sprintf(
				"its receipt approves %q twice — one promise has one approval per record", lineage))
			continue
		}
		subjectOf[lineage] = sub
	}

	// Exceptions grouped by lineage, canonicalised, so the comparison is
	// against what the amendment actually declares rather than its spelling.
	declared := map[string]map[string]expectation{}
	for _, ex := range exceptionsOf(a) {
		lineage := strings.TrimSpace(ex.Intent)
		canon, cerr := parser.CanonicalScopeRef(ex.Ref)
		if cerr != nil {
			// Shape, reported by ValidateScopeImpact. A malformed declaration
			// cannot be the yardstick for anything.
			continue
		}
		rep := ""
		if strings.TrimSpace(ex.ReplacedBy) != "" {
			if rc, rerr := parser.CanonicalScopeRef(ex.ReplacedBy); rerr == nil {
				rep = rc
			}
		}
		if declared[lineage] == nil {
			declared[lineage] = map[string]expectation{}
		}
		declared[lineage][canon] = expectation{disposition: ex.Disposition, replacedBy: rep}
	}

	closureAsserted := a.ScopeImpact != nil && a.ScopeImpact.PreservesUnlisted

	// The canonical affects: set, which is what the Named partition means.
	affects := map[string]bool{}
	for _, raw := range a.Affects {
		if canon, cerr := parser.CanonicalScopeRef(raw); cerr == nil {
			affects[canon] = true
		}
	}

	changed := map[string]bool{}
	for _, tr := range transitions {
		changed[strings.TrimSpace(tr.Intent)] = true
	}
	// A declared exception under a lineage this record does not transition is
	// invisible to everything else here: the per-lineage audit runs over
	// TRANSITIONS, and the extra-subject sweep runs over receipt SUBJECTS, so a
	// stray declaration group belongs to neither. The operational checker
	// rejects it, but the operational checker does not run over history — and
	// this rule is the only thing standing between a re-digested receipt and a
	// consequence filed under a promise the record never touched.
	for _, lineage := range sortedLineages(declared) {
		if !changed[lineage] {
			problems = append(problems, fmt.Sprintf(
				"the record dispositions entries under %q, which it does not change — a "+
					"consequence belongs to a promise this record actually transitions", lineage))
		}
	}

	for _, tr := range transitions {
		lineage := strings.TrimSpace(tr.Intent)
		sub, ok := subjectOf[lineage]
		if !ok {
			problems = append(problems, fmt.Sprintf(
				"its receipt approves nothing for %q, which this record changes", lineage))
			continue
		}
		if sub.Mode != string(tr.Mode) {
			problems = append(problems, fmt.Sprintf(
				"its receipt approves %q as %q, but the record declares %q",
				lineage, sub.Mode, tr.Mode))
		}
		// The closure claim is mode-scoped: false for a retirement, and the
		// record's assertion for a mode that leaves a promise behind.
		wantClosure := closureAsserted && tr.Mode != parser.IntentRetire
		if sub.PreservesUnlisted != wantClosure {
			problems = append(problems, fmt.Sprintf(
				"its receipt records the closure claim for %q as %v, but the record's declaration "+
					"for a %s is %v", lineage, sub.PreservesUnlisted, tr.Mode, wantClosure))
		}
		if sub.Attestation != parser.AttestationFor(tr.Mode) {
			problems = append(problems, fmt.Sprintf(
				"its receipt records the attestation for %q as %q, but a %s asserts %q — the "+
					"receipt must say what was actually claimed",
				lineage, sub.Attestation, tr.Mode, parser.AttestationFor(tr.Mode)))
		}
		problems = append(problems, auditPromiseText(lineage, tr, sub)...)
		problems = append(problems, auditConsequences(lineage, tr.Mode, sub, declared[lineage],
			closureAsserted, affects)...)
	}

	// A subject for a lineage the record does not change is an approval over
	// something nobody declared.
	for _, lineage := range sortedSubjectLineages(subjectOf) {
		if !changed[lineage] {
			problems = append(problems, fmt.Sprintf(
				"its receipt approves %q, which this record does not change", lineage))
		}
	}

	sort.Strings(problems)
	return problems
}

// expectation is what the amendment says should have become of one entry.
type expectation struct {
	disposition string
	replacedBy  string
}

// auditConsequences compares one subject's recorded consequences to the
// exceptions the amendment declares for that lineage.
//
// This is the half that matters. Lineage and mode are the receipt's headline;
// the consequences are the accounting that replaced the ledger rule, and a
// receipt whose consequences were emptied or rewritten evidences nothing while
// still looking like an approval.
func auditConsequences(lineage string, mode parser.IntentMode, sub *evolutionSubject,
	declared map[string]expectation, closureAsserted bool, affects map[string]bool) []string {
	var problems []string
	seen := map[string]bool{}

	// The receipt's OWN captured prior inventory is the evidence that the
	// promise ever justified these entries. Without linking to it, the audit
	// accepts any valid-looking hex as a before fingerprint — which recreates
	// the plausible-absent-ref problem the pre-splice capture exists to close,
	// one layer in: disposition an arbitrary ref, synthesise a digest-shaped
	// string, re-digest, and the durable record says it was accounted for while
	// the receipt's own inventory never showed the promise supporting it.
	//
	// Self-contained and historical: this compares fields inside the receipt to
	// each other and to the amendment, never to today's artifacts.
	captured, capturedProblems := auditScopeStructure("captured", lineage, sub.ScopeBefore, affects)
	problems = append(problems, capturedProblems...)
	// Unconditionally, including a retirement whose population is empty.
	// deriveLineageScope names every scope it returns even when both partitions
	// are empty, so an unowned inventory is never something the ceremony
	// produces — and carving an exception here for the one case that reaches it
	// would suppress the shared validator exactly where the forger is freest.
	surviving, survivingProblems := auditScopeStructure("recorded surviving", lineage, sub.Scope, affects)
	problems = append(problems, survivingProblems...)
	if mode == parser.IntentRetire && len(surviving) > 0 {
		for _, ref := range sortedFingerprintKeys(surviving) {
			problems = append(problems, fmt.Sprintf(
				"its receipt records %s as still justified by %q after that promise is retired",
				ref, lineage))
		}
	}

	for _, c := range sub.Consequences {
		canon, cerr := parser.CanonicalScopeRef(c.Ref)
		if cerr != nil || canon != c.Ref {
			problems = append(problems, fmt.Sprintf(
				"its receipt records a consequence for %q under %q, which is not a canonical "+
					"contract reference", c.Ref, lineage))
			continue
		}
		if strings.TrimSpace(c.Lineage) != lineage {
			problems = append(problems, fmt.Sprintf(
				"its receipt files %s under %q while the consequence itself names %q — a "+
					"consequence belongs to the promise that justified the entry",
				canon, lineage, c.Lineage))
			continue
		}
		if seen[canon] {
			problems = append(problems, fmt.Sprintf(
				"its receipt records %s twice under %q; one entry has one fate per lineage",
				canon, lineage))
			continue
		}
		seen[canon] = true

		want, declaredHere := declared[canon]
		if !declaredHere {
			problems = append(problems, fmt.Sprintf(
				"its receipt records a consequence for %s under %q, which the record does not "+
					"disposition there", canon, lineage))
			continue
		}
		if c.Disposition != want.disposition {
			problems = append(problems, fmt.Sprintf(
				"its receipt records %s under %q as %q, but the record declares %q",
				canon, lineage, c.Disposition, want.disposition))
			continue
		}
		// A consequence with no before fingerprint records that something
		// happened to an entry without recording which entry it was, which is
		// the audit trail's whole content.
		if !validSHA256(c.BeforeFingerprint) {
			problems = append(problems, fmt.Sprintf(
				"its receipt records no usable fingerprint for %s as it was before the change, "+
					"so what was given up cannot be established", canon))
		} else if capturedFP, inCapture := captured[canon]; !inCapture {
			problems = append(problems, fmt.Sprintf(
				"its receipt records a consequence for %s under %q, but its own captured "+
					"inventory does not show that promise justifying it — a consequence is about "+
					"something the promise actually supported", canon, lineage))
		} else if capturedFP != c.BeforeFingerprint {
			problems = append(problems, fmt.Sprintf(
				"its receipt records %s as it was before the change, but that is not the entry "+
					"its own captured inventory holds — the consequence and the evidence for it "+
					"describe different things", canon))
		}
		// The after subject is derived from the disposition's own rule rather
		// than a switch here, so the two cannot drift.
		rule, known := dispositionRules[c.Disposition]
		if !known {
			problems = append(problems, fmt.Sprintf(
				"its receipt records %s under %q with disposition %q, which has no defined meaning",
				canon, lineage, c.Disposition))
			continue
		}
		// The claim is deterministic from the disposition and is what the
		// ceremony PRINTED. A receipt saying "removed" while its human-readable
		// claim says the entry survives is a durable record that reads one way
		// and means another — which is exactly why the consequence digest was
		// made to cover Claim.
		if c.Claim != rule.Claim {
			problems = append(problems, fmt.Sprintf(
				"its receipt records %s under %q as %q but describes it as %q — the stored words "+
					"and the stored disposition disagree", canon, lineage, c.Disposition, c.Claim))
		}
		switch {
		case rule.NeedsReplacement:
			if want.replacedBy == "" {
				// The record itself is malformed; ValidateScopeImpact says so.
				break
			}
			if c.AfterRef != want.replacedBy {
				problems = append(problems, fmt.Sprintf(
					"its receipt says %s was taken over by %s, but the record names %s",
					canon, orNone(c.AfterRef), want.replacedBy))
			}
			if !validSHA256(c.AfterFingerprint) {
				problems = append(problems, fmt.Sprintf(
					"its receipt does not bind the replacement for %s by fingerprint — the human "+
						"approved an entry, not an address", canon))
			}
		case rule.MustResolve:
			if c.AfterRef != canon {
				problems = append(problems, fmt.Sprintf(
					"its receipt says %s survives as %s; a surviving entry is itself",
					canon, orNone(c.AfterRef)))
			}
			if !validSHA256(c.AfterFingerprint) {
				problems = append(problems, fmt.Sprintf(
					"its receipt does not bind %s as it survives, so what was approved to remain "+
						"cannot be established", canon))
			}
			// `retained` claims THIS promise still supports the entry, so the
			// receipt's own surviving population has to show it. `revised` is
			// exempt from the PRESENCE requirement only: a revision may
			// re-source an entry to whatever justifies it now, which is a
			// legitimate outcome rather than a loss. The agreement requirement
			// below applies to both, and to every other disposition.
			if rule.MustStayAttributed {
				if _, stillHere := surviving[canon]; !stillHere {
					problems = append(problems, fmt.Sprintf(
						"its receipt says %q retained %s, but its own record of what that promise "+
							"still justifies does not include it", lineage, canon))
				}
			}
		default:
			if c.AfterRef != "" || c.AfterFingerprint != "" {
				problems = append(problems, fmt.Sprintf(
					"its receipt gives %s an after subject (%s) though it is dispositioned %q, "+
						"which says nothing takes it over", canon, orNone(c.AfterRef), c.Disposition))
			}
		}

		// Wherever a consequence's after subject ALSO appears in this lineage's
		// surviving population, the two must describe the same entry. Exemption
		// from being required to be present is not exemption from agreeing when
		// present: a receipt binding an entry to one thing while recording
		// another as surviving is internally contradictory however it got that
		// way, and both halves are individually valid digests.
		//
		// General on purpose. It covers `revised` where the entry stayed, and
		// the unusual `replaced-by` whose target is in this same lineage.
		if c.AfterRef != "" && validSHA256(c.AfterFingerprint) {
			if survivingFP, present := surviving[c.AfterRef]; present &&
				survivingFP != c.AfterFingerprint {
				problems = append(problems, fmt.Sprintf(
					"its receipt binds %s under %q to one entry and records another as surviving "+
						"— the consequence and the population disagree", c.AfterRef, lineage))
			}
		}
	}

	// The closure has to be BACKED, not merely asserted. preserves_unlisted
	// says the entries the record does not list remain supported by the changed
	// promise — so the receipt's own record of what that promise still
	// justifies must show them. Without this, a forged receipt can drop an
	// entry from its after-population and still claim the closure preserved it:
	// the same missing link as a consequence with a synthesised fingerprint,
	// one field over.
	if mode != parser.IntentRetire && closureAsserted {
		var unbacked []string
		for ref := range captured {
			if seen[ref] {
				continue // it has a consequence; the closure is not what covers it
			}
			if _, stillHere := surviving[ref]; !stillHere {
				unbacked = append(unbacked, ref)
			}
		}
		sort.Strings(unbacked)
		for _, ref := range unbacked {
			problems = append(problems, fmt.Sprintf(
				"its receipt leaves %s to the closure under %q, but its own record of what that "+
					"promise still justifies does not include it — the closure claims a survival "+
					"the receipt does not show", ref, lineage))
		}
	}

	// And the other direction: an entry in the surviving population that neither
	// carries a consequence nor is covered by a closure is in the record for no
	// stated reason.
	if !sub.PreservesUnlisted {
		var unexplained []string
		for ref := range surviving {
			if !seen[ref] {
				unexplained = append(unexplained, ref)
			}
		}
		sort.Strings(unexplained)
		for _, ref := range unexplained {
			problems = append(problems, fmt.Sprintf(
				"its receipt records %s as still justified by %q with no consequence and no "+
					"closure to cover it", ref, lineage))
		}
	}

	// Completeness against the CAPTURE, which is the population that actually
	// existed rather than the one the record chose to list.
	//
	// A retirement has no closure: the promise is over, so every entry it
	// justified owes a consequence and none can be sheltered. A living mode
	// shelters the rest only when the record asserts the closure — which is the
	// assertion preserves_unlisted exists to be.
	if len(captured) > 0 && (mode == parser.IntentRetire || !closureAsserted) {
		uncovered := make([]string, 0, len(captured))
		for ref := range captured {
			if !seen[ref] {
				uncovered = append(uncovered, ref)
			}
		}
		sort.Strings(uncovered)
		for _, ref := range uncovered {
			if mode == parser.IntentRetire {
				problems = append(problems, fmt.Sprintf(
					"its captured inventory shows %q justified %s, and its receipt records no "+
						"consequence for it — a retirement has no closure to leave it under",
					lineage, ref))
				continue
			}
			problems = append(problems, fmt.Sprintf(
				"its captured inventory shows %q justified %s, its receipt records no consequence "+
					"for it, and the record asserts no closure to cover it", lineage, ref))
		}
	}

	// Completeness the other way: an exception the receipt never accounted for.
	// This is the emptied-consequences forgery.
	missing := make([]string, 0, len(declared))
	for ref := range declared {
		if !seen[ref] {
			missing = append(missing, ref)
		}
	}
	sort.Strings(missing)
	for _, ref := range missing {
		problems = append(problems, fmt.Sprintf(
			"the record dispositions %s under %q, but its receipt records no consequence for it — "+
				"nothing shows that entry was ever accounted for", ref, lineage))
	}
	return problems
}

// auditScopeStructure validates one stored inventory and returns it as
// ref -> fingerprint.
//
// Shared by the before and after populations because they are the same kind of
// artifact and a forger has the same freedom with each. An inventory with no
// owner is not evidence that a particular promise justified anything, so the
// lineage must be present and exact — accepting an empty one was a hole of
// exactly that shape.
//
// The PARTITION is checked too, not just the membership. Named means "this
// amendment declares it changed", which is `affects:` and nothing else. Moving
// an entry across the partition changes what the durable record says the
// amendment claimed about it, while leaving every ref and fingerprint intact.
func auditScopeStructure(what, lineage string, sc lineageScope, affects map[string]bool) (map[string]string, []string) {
	var problems []string
	out := map[string]string{}
	if strings.TrimSpace(sc.Lineage) != lineage {
		problems = append(problems, fmt.Sprintf(
			"its %s inventory for %q is filed under %q — an inventory with no owner, or the "+
				"wrong one, is not evidence about this promise",
			what, lineage, sc.Lineage))
		return out, problems
	}
	named := map[string]bool{}
	for _, e := range sc.Named {
		named[e.Ref] = true
	}
	for _, e := range append(append([]scopedEntry{}, sc.Named...), sc.Unlisted...) {
		canon, cerr := parser.CanonicalScopeRef(e.Ref)
		if cerr != nil || canon != e.Ref {
			problems = append(problems, fmt.Sprintf(
				"its %s inventory for %q holds %q, which is not a canonical contract reference",
				what, lineage, e.Ref))
			continue
		}
		if _, dup := out[canon]; dup {
			problems = append(problems, fmt.Sprintf(
				"its %s inventory for %q holds %s twice", what, lineage, canon))
			continue
		}
		if !validSHA256(e.Fingerprint) {
			problems = append(problems, fmt.Sprintf(
				"its %s inventory for %q records no usable fingerprint for %s",
				what, lineage, canon))
			continue
		}
		if named[canon] != affects[canon] {
			side, want := "undeclared", "does"
			if named[canon] {
				side, want = "declared changed", "does not"
			}
			problems = append(problems, fmt.Sprintf(
				"its %s inventory for %q files %s as %s, but the amendment's affects: %s name it",
				what, lineage, canon, side, want))
		}
		out[canon] = e.Fingerprint
	}
	// An entry in both partitions is in neither: the partition is what says
	// whether the record declared it changed.
	for _, e := range sc.Unlisted {
		if named[e.Ref] {
			problems = append(problems, fmt.Sprintf(
				"its %s inventory for %q holds %s as both declared and undeclared",
				what, lineage, e.Ref))
		}
	}
	return out, problems
}

func sortedFingerprintKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "nothing"
	}
	return s
}

// auditPromiseText compares the promise the receipt says was approved to the
// one the amendment declares.
//
// Before cannot be recomputed — it is the version that was in force at the time,
// and the ledger has moved on. After can: it comes from the amendment's own
// version block through the same snapshot rule the ceremony used. And Delta must
// be internally consistent with the two, or the receipt shows the human a change
// that is not the change it records.
//
// Without this a receipt can carry the right disposition headline while lying
// about the promise text the human supposedly approved.
func auditPromiseText(lineage string, tr parser.IntentTransition, sub *evolutionSubject) []string {
	var problems []string
	if tr.Mode == parser.IntentRetire {
		empty, eerr := asStored(parser.Intent{})
		if eerr != nil {
			return []string{eerr.Error()}
		}
		if !reflect.DeepEqual(sub.After, empty) {
			problems = append(problems, fmt.Sprintf(
				"its receipt records a promise still standing after %q is retired — a retirement "+
					"leaves nothing behind", lineage))
		}
		if len(sub.Delta) > 0 {
			problems = append(problems, fmt.Sprintf(
				"its receipt records a field delta for the retirement of %q — an ending is not a "+
					"rewrite, and a delta against nothing blanks every field", lineage))
		}
		return problems
	}
	want, werr := asStored(agent.MaterialiseIntent(lineage, tr.Version))
	if werr != nil {
		return append(problems, werr.Error())
	}
	if !reflect.DeepEqual(sub.After, want) {
		problems = append(problems, fmt.Sprintf(
			"its receipt records a different promise for %q than the amendment declares — the "+
				"text the human approved is not the text on record", lineage))
		return problems
	}
	wantDelta, derr := asStored(diffVersions(sub.Before, sub.After))
	if derr != nil {
		return append(problems, derr.Error())
	}
	if !reflect.DeepEqual(sub.Delta, wantDelta) {
		problems = append(problems, fmt.Sprintf(
			"its receipt shows a change for %q that its own before and after do not describe",
			lineage))
	}
	return problems
}

// asStored returns a value as it reads back from the baseline.
//
// The receipt on disk has been through the storage encoding, which turns a nil
// slice into an empty one; a value built fresh in memory has not. Comparing the
// two directly reports every honest receipt as a mismatch — the same trap the
// transition digest hit, and the same fix: put both sides through the encoding
// before comparing them.
func asStored[T any](v T) (T, error) {
	var out T
	data, err := yaml.Marshal(v)
	if err != nil {
		return out, fmt.Errorf("its receipt could not be compared to the record: %w", err)
	}
	if err := yaml.Unmarshal(data, &out); err != nil {
		return out, fmt.Errorf("its receipt could not be compared to the record: %w", err)
	}
	return out, nil
}

func sortedLineages(m map[string]map[string]expectation) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedSubjectLineages(m map[string]*evolutionSubject) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// exceptionsOf returns the record's scope exceptions, or nothing.
func exceptionsOf(a parser.Amendment) []parser.ScopeException {
	if a.ScopeImpact == nil {
		return nil
	}
	return a.ScopeImpact.Exceptions
}
