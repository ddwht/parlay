package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// Applied-authority preflight (WP1).
//
// buildBaseline stamps LastAppliedAmendment as the MAXIMUM sequence in the
// ledger, on the assumption that a save only ever follows a green build "at
// which point the ledger is by definition fully applied". That assumption is
// false for a --partial save, and the consequence is not a missing feature but
// a forged one: a pending governance record swept past the marker has its
// promises withdrawn with no confirmation shown, AND has its hash written into
// sources.amendments — manufacturing the very evidence a trusted-applied check
// would later read as proof the withdrawal was authorised.
//
// This is the guard, not the cure. WP2 removes the inference from
// buildBaseline itself; until then the primitive stays dangerous and this
// membrane keeps it unreachable. Being deliberately redundant with WP2 is the
// point — a safety property worth having is worth having twice.
//
// It is READ-ONLY and runs before the first stage-1 write, before the project
// baseline and code-hashes are touched, and before the emitted manifest is
// consumed. Ordering matters as much as the checks: the run that must be
// refused currently succeeds far enough to consume the manifest and create
// project state, destroying the inputs its own retry would need.

// ---------------------------------------------------------------------
// The applied-authority capsule (WP2).
// ---------------------------------------------------------------------

// appliedAuthority is the evidence half of a feature's baseline: the marker
// saying how far the ledger was applied, AND the per-amendment hashes proving
// which records were honoured to get there. The two travel together because
// either alone is forgeable — a marker with no hashes cannot be checked, and
// hashes with no marker say nothing about what is in force.
type appliedAuthority struct {
	Through int
	Hashes  map[string]string
	// Outputless records which of those records were blessed on a confirmed
	// output-less claim, keyed by exact amendment filename.
	Outputless map[string]bool
	// Receipts record how each was authorised, keyed by exact amendment
	// filename. See TransitionReceipt.
	Receipts map[string]TransitionReceipt
}

// authorityMode says what a save may do to that capsule.
type authorityMode int

const (
	// authorityPreserve copies the observed capsule through unchanged.
	//
	// It is the ZERO VALUE deliberately. A caller that says nothing about
	// authority must not thereby grant it, which is precisely the failure WP0
	// reproduced: buildBaseline granted authority to every caller by saying
	// nothing at all.
	authorityPreserve authorityMode = iota
	// authorityAdvance appends exactly the newly proven records.
	authorityAdvance
)

// authorityOp is the per-feature operation this invocation is entitled to
// perform. Modelled as an operation rather than a target sequence because an
// integer loses the two things that matter: whether N was OBSERVED prior
// authority or NEWLY GRANTED authority, and the evidence map that makes it
// checkable.
type authorityOp struct {
	Mode  authorityMode
	Prior appliedAuthority
	// To is the sequence this run proved. Meaningful only under
	// authorityAdvance, and always greater than Prior.Through.
	To int
	// Outputless marks this advance as resting on a confirmed output-less
	// claim, so the method is recorded alongside the evidence.
	Outputless bool
	// Newly are the records in (Prior.Through, To] whose hashes may be
	// appended. Records at or below Prior.Through are NEVER rehashed: doing so
	// would silently re-bless an edit to an amendment already honoured, which
	// is a write-once violation, and would mint fresh trusted evidence for it.
	Newly []parser.Amendment
}

// validate checks the operation is internally coherent.
//
// buildBaselineWithAuthority is an authority boundary callable from outside
// the planner, so it cannot assume its input was well-formed. Note what is
// deliberately NOT required: integer contiguity. A compacted ledger has
// legitimate sequence gaps, so the invariant is strictly increasing and above
// the prior marker, not consecutive.
func (op authorityOp) validate() error {
	switch op.Mode {
	case authorityPreserve:
		// One canonical representation: preserve carries no advance payload.
		if op.Outputless {
			return fmt.Errorf("a preserving operation is marked output-less — the method " +
				"describes an advance, and preserve advances nothing")
		}
		if op.To != 0 || len(op.Newly) != 0 {
			return fmt.Errorf("a preserving operation carries an advance payload (to=%d, %d record(s)) — "+
				"preserve and advance must not be expressible at once", op.To, len(op.Newly))
		}
		return nil
	case authorityAdvance:
		if len(op.Newly) == 0 {
			return fmt.Errorf("an advance to %d names no records — authority is granted per record, "+
				"so an advance with no evidence is not an advance", op.To)
		}
		prev := op.Prior.Through
		for _, a := range op.Newly {
			if a.Seq <= prev {
				return fmt.Errorf("advance records are not strictly increasing above the prior "+
					"marker: %s is at or below %d", amendmentIdentity(a), prev)
			}
			prev = a.Seq
		}
		if last := op.Newly[len(op.Newly)-1].Seq; op.To != last {
			return fmt.Errorf("an advance to %d whose last authorised record is %d would move the "+
				"marker past a record nobody proved", op.To, last)
		}
		return nil
	default:
		return fmt.Errorf("unknown authority mode %d — an unrecognised mode must fail, not "+
			"happen to preserve", op.Mode)
	}
}

// observeAppliedAuthority reads the capsule fail-closed.
//
// A missing baseline is genuinely zero authority. Anything unreadable is
// UNKNOWN authority, and a save must not overwrite state it could not
// inspect — lastAppliedAmendment folds both cases to 0, which would make a
// corrupt baseline read as "nothing applied" and the whole ledger as pending.
func observeAppliedAuthority(cfg *config.Context, slug string) (appliedAuthority, error) {
	return observeAppliedAuthorityAt(baselinePath(cfg, slug), slug)
}

// observeAppliedAuthorityAt is the path-based form, so callers that work from a
// root path rather than a resolved Context (the migrations) share one reader
// and one set of receipt checks.
func observeAppliedAuthorityAt(path, slug string) (appliedAuthority, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return appliedAuthority{}, nil
	}
	if err != nil {
		return appliedAuthority{}, fmt.Errorf("read %s: %w", path, err)
	}
	var baseline Baseline
	if err := yaml.Unmarshal(data, &baseline); err != nil {
		return appliedAuthority{}, fmt.Errorf("parse %s: %w", path, err)
	}
	out := appliedAuthority{Through: baseline.LastAppliedAmendment}
	if baseline.Sources != nil && len(baseline.Sources.Amendments) > 0 {
		out.Hashes = make(map[string]string, len(baseline.Sources.Amendments))
		for k, v := range baseline.Sources.Amendments {
			out.Hashes[k] = v
		}
	}
	if len(baseline.OutputlessAmendments) > 0 {
		out.Outputless = make(map[string]bool, len(baseline.OutputlessAmendments))
		for k, v := range baseline.OutputlessAmendments {
			out.Outputless[k] = v
		}
	}
	if len(baseline.TransitionReceipts) > 0 {
		out.Receipts = make(map[string]TransitionReceipt, len(baseline.TransitionReceipts))
		for k, v := range baseline.TransitionReceipts {
			// Validate on the way IN. A validator nothing calls protects
			// nothing: without this, a payload could be mutated while its
			// stored digest stayed put, both observations would accept it,
			// sameAuthority would compare equal digests, and every writer
			// would copy the inconsistent receipt forward.
			if err := v.validateAgainstCapsule(k, slug, baseline); err != nil {
				return appliedAuthority{}, fmt.Errorf("the transition receipt for %s is not "+
					"sound: %w", k, err)
			}
			out.Receipts[k] = v
		}
	}
	return out, nil
}

// sameAuthority reports whether two observations describe the same authority.
func sameAuthority(a, b appliedAuthority) bool {
	if a.Through != b.Through || len(a.Hashes) != len(b.Hashes) ||
		len(a.Outputless) != len(b.Outputless) || len(a.Receipts) != len(b.Receipts) {
		return false
	}
	for k, v := range a.Hashes {
		if b.Hashes[k] != v {
			return false
		}
	}
	for k, v := range a.Outputless {
		if b.Outputless[k] != v {
			return false
		}
	}
	for k, v := range a.Receipts {
		other, ok := b.Receipts[k]
		if !ok || other.Digest != v.Digest {
			return false
		}
	}
	return true
}

// withVerifiedAuthority is the authority-storage transaction boundary.
//
// Every production path that writes a feature baseline's authority fields goes
// through here. A cooperative lock protects only an invariant every cooperating
// writer shares, so a lock held by one command and skipped by the others is not
// exclusion — it is the same race with a different opponent. Observation,
// validation against that observation, and replacement must all sit inside it.
//
// fn receives the capsule as re-observed UNDER the lock. It is proven equal to
// what the caller planned against, so a concurrent writer's marker, hashes,
// output-less evidence or receipts cannot be overwritten from a stale read —
// atomic rename prevents a torn file, not a lost update.
//
// Not re-entrant: flock blocks a second acquisition from the same process, and
// nothing here nests. Callers that already hold it use the locked layer.
func withVerifiedAuthority(cfg *config.Context, slug string, planned appliedAuthority, fn func(current appliedAuthority) error) error {
	return withVerifiedAuthorityAt(cfg.BuildPath(slug), baselinePath(cfg, slug), slug, planned, fn)
}

// withVerifiedAuthorityAt is the path-based form. Same boundary, same checks.
func withVerifiedAuthorityAt(dir, blPath, slug string, planned appliedAuthority, fn func(current appliedAuthority) error) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create build dir for %s: %w", slug, err)
	}
	lock := flock.New(filepath.Join(dir, authorityLockName))
	// Test hook: fires immediately before acquisition, so a test can interleave
	// deterministically with a writer that is about to block on the lock.
	// Nil in production.
	if authorityLockAttemptHook != nil {
		authorityLockAttemptHook(slug)
	}
	ctx, cancel := context.WithTimeout(context.Background(), authorityLockWait)
	defer cancel()
	locked, err := lock.TryLockContext(ctx, authorityLockRetry)
	if err != nil || !locked {
		return fmt.Errorf("%w: another process is writing %s's applied authority. Refusing "+
			"rather than racing it", errAuthorityConflict, slug)
	}
	defer func() { _ = lock.Unlock() }()

	current, err := observeAppliedAuthorityAt(blPath, slug)
	if err != nil {
		return fmt.Errorf("re-read applied authority for %s under lock: %w", slug, err)
	}
	if !sameAuthority(planned, current) {
		return fmt.Errorf("%w: %s's applied authority changed while this operation was preparing, "+
			"so what it planned no longer describes the feature. Nothing was written — re-run "+
			"against the current state", errAuthorityConflict, slug)
	}
	return fn(current)
}

const authorityLockName = ".authority.lock"

// errAuthorityConflict marks a refusal to race, so a caller can tell it apart
// from a feature that simply could not be baselined.
var errAuthorityConflict = errors.New("applied-authority conflict")

var (
	authorityLockWait  = 5 * time.Second
	authorityLockRetry = 25 * time.Millisecond

	// authorityLockAttemptHook is a test seam. Production leaves it nil.
	authorityLockAttemptHook func(slug string)
)

// preserveAuthority is the safe operation: change nothing.
func preserveAuthority(prior appliedAuthority) authorityOp {
	return authorityOp{Mode: authorityPreserve, Prior: prior}
}

// advanceAuthority grants exactly the records in (prior.Through, to].
func advanceAuthority(prior appliedAuthority, to int, newly []parser.Amendment) authorityOp {
	return authorityOp{Mode: authorityAdvance, Prior: prior, To: to, Newly: newly}
}

// advanceOutputless is the same grant, recorded as resting on a confirmed
// output-less claim. A separate constructor so the method is a deliberate
// choice at the planning site rather than a field someone might forget.
func advanceOutputless(prior appliedAuthority, to int, newly []parser.Amendment) authorityOp {
	op := advanceAuthority(prior, to, newly)
	op.Outputless = true
	return op
}

// authorityInference names the non-proof a caller is willing to stand behind.
//
// Hoisted to the caller on purpose. buildBaseline and saveBuildStateForFeature
// are general-purpose and directly invocable, and neither should inherit "a
// non-partial run means the whole tail was applied" merely by existing.
type authorityInference int

const (
	// noInference is the zero value: nothing is granted that was not proved.
	noInference authorityInference = iota
	// fullGreenBuildInference is the ONE surviving inference in this codebase:
	// a full save is generate-code's final step, which regenerates from the
	// amended artifacts, so a splice-only tail is applied by the time it runs.
	//
	// Be honest about what this is. It is an assertion by the workflow, not a
	// machine proof — `save-build-state` remains directly invocable, so
	// hoisting this out of buildBaseline makes the inference explicit and
	// non-inheritable, NOT proven. Its basis is the emitted-manifest
	// requirement plus the workflow contract, and it stays temporary until the
	// later proof work replaces it. This constant is the grep target for that.
	fullGreenBuildInference
)

// appliedPlan is the single decision about what this invocation will change.
//
// Bless is the sole mutation set, and Ops carries the authority operation for
// each member. The guard and the writer consume the SAME plan, so a future
// change cannot widen the write loop while silently leaving the guard narrow —
// which is what made the original sweep possible.
type appliedPlan struct {
	Bless map[string]bool
	Ops   map[string]authorityOp
}

// authorityClass names how a pending record may lawfully be applied. The
// classification is by CONTENT, not by shape alone: it decides which proof is
// owed, never whether a proof can be skipped.
type authorityClass int

const (
	// classSplice carries affects: only. A refine splices it and the
	// re-baseline records it applied.
	classSplice authorityClass = iota
	// classGovernance supersedes a founding intent or retires the feature and
	// carries no affects:. Only `apply-governance --confirm` may move it,
	// because the promise list that command prints is what the user approves.
	classGovernance
	// classCombined carries BOTH. The schema permits it (affects: is required
	// "unless supersedes_intents: is non-empty" — may be empty, not must), and
	// no single path can apply it: apply-governance refuses anything with
	// affects:, while the splice path would advance the intent supersession
	// with no promise list ever shown.
	classCombined
	// classInvalid carries neither affects: nor any governance field. The
	// schema requires one or the other, so such a record declares no work and
	// no decision — there is nothing to prove applied, and "nothing to prove"
	// must never read as "proven".
	classInvalid
)

func classifyAmendment(a parser.Amendment) authorityClass {
	governance := len(a.SupersedesIntents) > 0 || a.RetiresFeature
	splice := len(a.Affects) > 0
	switch {
	case governance && splice:
		return classCombined
	case governance:
		return classGovernance
	case splice:
		return classSplice
	default:
		return classInvalid
	}
}

// journalReachedTested checks the journal is the exact ordered pipeline prefix
// through "tested".
//
// NextRefineStep treats Completed as a SET, which is the right question for
// "where do I resume" and the wrong one for "what did this run prove". A
// malformed list holding all six steps, or the right steps out of order,
// returns "" from it and would otherwise pass as proof. Authority needs the
// ordered prefix.
func journalReachedTested(j *refineJournal) error {
	want := refineJournalSteps[:5] // through "tested"; "re-baselined" is this save
	n := len(j.Completed)
	for i := 0; i < n && i < len(want); i++ {
		if j.Completed[i] != want[i] {
			return fmt.Errorf("its completed steps are not the refinement pipeline in order — "+
				"position %d is %q where %q was owed", i+1, j.Completed[i], want[i])
		}
	}
	if n < len(want) {
		return fmt.Errorf("it stopped before %q", want[n])
	}
	if n > len(want) {
		return fmt.Errorf("it is already marked %q while the record is still unapplied — a "+
			"finished refinement clears its journal, so this pairing is inconsistent evidence "+
			"rather than stronger evidence", refineJournalSteps[len(want)])
	}
	return nil
}

// amendmentIdentity is the full on-disk identity of a record, which is what a
// refusal must name: an operator needs to know WHICH decision is still owed,
// not merely that something was refused.
func amendmentIdentity(a parser.Amendment) string {
	return fmt.Sprintf("%03d-%s", a.Seq, a.FileSlug)
}

// planAppliedAuthority decides, once, what this invocation may change.
//
// It refuses rather than plans when a feature's tail cannot be proven. Being
// read-only and running before the first write is half the contract; the other
// half is that its output IS the write loop's instruction list, so guard and
// writer cannot drift apart.
//
// Scope is the features whose baselines this invocation will mutate — under
// --partial the emitted set. Widening it would recreate the over-refusal the
// guard exists to prevent: an untouched feature carrying a pending governance
// decision must not block a save that cannot advance its marker.
func planAppliedAuthority(cfg *config.Context, features []string, partial bool, emittedFeatures map[string]bool, inference authorityInference, outputless outputlessClaim) (*appliedPlan, error) {
	plan := &appliedPlan{Bless: map[string]bool{}, Ops: map[string]authorityOp{}}
	var refusals []string

	for _, slug := range features {
		// Membership. The emitted set decides it, EXCEPT for one explicitly
		// named and confirmed output-less feature, which cannot appear there
		// because it emitted nothing. Note what does not happen: the slug is
		// never inserted into emittedFeatures. That set is provenance and
		// reporting, and synthesising an emission for a feature that wrote no
		// files would be a lie told to every consumer of it.
		outputlessHere := outputless.made() && outputless.Feature == slug
		if partial && !emittedFeatures[slug] && !outputlessHere {
			continue
		}
		plan.Bless[slug] = true

		// An unfinished compaction means the ledger may be half-moved.
		// Recording authority over that state would bless something nobody
		// intended, and recovery is about to undo it.
		inFlight, inFlightErr := compactionInFlight(cfg, slug)
		if inFlightErr != nil {
			return nil, fmt.Errorf("applied-authority preflight: %s: %w", slug, inFlightErr)
		}
		if inFlight {
			refusals = append(refusals, fmt.Sprintf("%s has an interrupted compaction still in "+
				"flight, so its ledger may be half-moved. Run `parlay internal compact @%s` to "+
				"recover before recording any authority over it", slug, slug))
			continue
		}

		prior, err := observeAppliedAuthority(cfg, slug)
		if err != nil {
			return nil, fmt.Errorf("applied-authority preflight: %s: %w — the marker and the "+
				"stored authority evidence cannot be read, and a save must not overwrite state "+
				"it could not inspect", slug, err)
		}

		amendments, err := parser.LoadFeatureAmendments(cfg.FeaturePath(slug))
		if err != nil {
			// Fail closed. An unreadable ledger is not an empty one, and a
			// save is the moment authority gets recorded.
			return nil, fmt.Errorf("applied-authority preflight: read ledger for %s: %w", slug, err)
		}

		var pending []parser.Amendment
		for _, a := range amendments {
			if a.Seq > prior.Through {
				pending = append(pending, a)
			}
		}
		if len(pending) == 0 {
			// Nothing proven to advance to: the capsule travels through
			// untouched. Not "re-derive the same number" — the hashes must
			// survive byte-for-byte too.
			plan.Ops[slug] = preserveAuthority(prior)
			continue
		}
		authorized, reasons := proveTail(cfg, slug, partial, pending, inference)
		if outputlessHere {
			// The output-less path proves its tail with exactly the evidence a
			// code-emitting refine must produce. Governance and combined
			// records are refused before this point and stay refused:
			// --confirm-outputless asserts "this feature owes no generated
			// code", which is not, and can never be, confirmation of a
			// promise list.
			authorized, reasons = nil, outputlessTailProven(cfg, slug, pending)
			if len(reasons) == 0 {
				authorized = pending[:1]
			}
			if govReasons := refusedGovernanceRecords(slug, pending); len(govReasons) > 0 {
				authorized, reasons = nil, govReasons
			}
		}
		if len(reasons) > 0 {
			refusals = append(refusals, reasons...)
			continue
		}
		if len(authorized) == 0 {
			// No proof and no stated reason is a planner bug, not a licence.
			refusals = append(refusals, fmt.Sprintf("%s: nothing was authorised and no reason "+
				"was given — refusing rather than guessing", slug))
			continue
		}
		// The grant is exactly what the proof returned, and the marker moves
		// to that slice's last record — never to the pending maximum.
		to := authorized[len(authorized)-1].Seq
		op := advanceAuthority(prior, to, authorized)
		if outputlessHere {
			op = advanceOutputless(prior, to, authorized)
		}
		if err := op.validate(); err != nil {
			return nil, fmt.Errorf("applied-authority preflight: %s: %w", slug, err)
		}
		plan.Ops[slug] = op
	}

	if len(refusals) > 0 {
		sort.Strings(refusals)
		return nil, fmt.Errorf("this save would record decisions it cannot prove were applied, "+
			"so it has written nothing:\n  - %s", strings.Join(refusals, "\n  - "))
	}
	return plan, nil
}

// proveTail returns the records this run is AUTHORISED to record applied, or
// every reason it is not.
//
// Returning the exact slice matters. The planner used to read "no reasons"
// as "therefore every pending record was proven" and then take the maximum
// itself — but absence of refusal is not proof, and a future proof mode would
// have inherited whole-tail advancement by default. Each branch now states
// exactly what it authorises.
//
// It reports ALL of them rather than the first: a tail holding both a
// governance record and a combined one would otherwise hide the second until
// the next run, and the operator's question is "what is still owed", not
// "what tripped first".
func proveTail(cfg *config.Context, slug string, partial bool, pending []parser.Amendment, inference authorityInference) ([]parser.Amendment, []string) {
	if reasons := refusedGovernanceRecords(slug, pending); len(reasons) > 0 {
		return nil, reasons
	}
	return proveSpliceTail(cfg, slug, partial, pending, inference)
}

// refusedGovernanceRecords names every pending record no save may apply.
//
// Split out so no later branch can reach the splice logic without passing
// through it. The output-less path in particular must not be able to
// downgrade a combined record to a splice.
func refusedGovernanceRecords(slug string, pending []parser.Amendment) []string {
	var reasons []string

	// Governance, combined and invalid records are refused on BOTH paths, and
	// refused even when they are the only pending record. No journal, no green
	// build and no emission is evidence that a promise-withdrawal was approved.
	var governance, combined, invalid []string
	for _, a := range pending {
		switch classifyAmendment(a) {
		case classGovernance:
			governance = append(governance, amendmentIdentity(a))
		case classCombined:
			combined = append(combined, amendmentIdentity(a))
		case classInvalid:
			invalid = append(invalid, amendmentIdentity(a))
		}
	}
	if len(governance) > 0 {
		reasons = append(reasons, fmt.Sprintf("%s: %s withdraws founding promises and is still "+
			"unapplied. Only `parlay internal apply-governance @%s --confirm` may move it — the "+
			"promise list that command prints is what you are approving, and a save prints "+
			"nothing. Apply it there, then re-run this save",
			slug, joinNames(governance), slug))
	}
	if len(combined) > 0 {
		// NOT "split it into two amendments". That advice was in this message
		// and it is false: the accounting rule is per-amendment, so a
		// governance-only half immediately trips
		// intent-supersession-unaccounted-affect for every entry sourced to the
		// retiring promise. There is no split that satisfies both rules, which
		// is exactly why this transition needed an applier of its own.
		reasons = append(reasons, fmt.Sprintf("%s: %s carries both affects: and "+
			"supersedes_intents:, so it has a splice to record AND promises to withdraw. A save "+
			"records build evidence and approves nothing, so it will not apply it. Run "+
			"`parlay internal apply-amendment @%s`, which shows the promises that would end and "+
			"applies both halves together once you approve them",
			slug, joinNames(combined), slug))
	}
	if len(invalid) > 0 {
		reasons = append(reasons, fmt.Sprintf("%s: %s declares neither affects: nor a governance "+
			"field, so there is nothing it could have applied. A record that proves nothing must "+
			"not be recorded as proven",
			slug, joinNames(invalid)))
	}
	return reasons
}

func proveSpliceTail(cfg *config.Context, slug string, partial bool, pending []parser.Amendment, inference authorityInference) ([]parser.Amendment, []string) {
	// Splice-only tail.
	if !partial {
		// The full path carries no per-record proof. It advances only when the
		// CALLER explicitly stands behind the workflow assertion — see
		// fullGreenBuildInference, which is the single grep target for the one
		// inference this codebase still permits.
		if inference == fullGreenBuildInference {
			// This assertion deliberately covers the WHOLE splice-only tail:
			// generate-code regenerated from the amended artifacts, so every
			// one of them is in the emitted output. Stated here, in the branch
			// that supplies the authority, rather than inferred downstream.
			return pending, nil
		}
		return nil, []string{fmt.Sprintf("%s: %s %s unapplied and this caller asserts no proof "+
			"that they were applied. A save records authority; it does not assume it",
			slug, joinNames(identities(pending)), plural(len(pending), "is", "are"))}
	}

	// Under --partial the proof is the refine journal: the machine half of the
	// run that did the splicing. It authorises exactly one record, and the
	// output-less path in outputless.go reuses this same evidence — that path
	// relaxes what counts as MEMBERSHIP, never what counts as a completed
	// refinement.
	if reasons := proveTailJournal(cfg, slug, pending); len(reasons) > 0 {
		return nil, reasons
	}
	return pending[:1], nil
}

// proveTailJournal checks the refine journal proves exactly this tail.
func proveTailJournal(cfg *config.Context, slug string, pending []parser.Amendment) []string {
	journal, err := loadRefineJournal(cfg, slug)
	if err != nil {
		return []string{fmt.Sprintf("%s: the refine journal cannot be read (%v), so this run "+
			"cannot show which record it applied", slug, err)}
	}
	if journal == nil {
		return []string{fmt.Sprintf("%s: %s %s unapplied, and this run has no refine journal to "+
			"show which of them it applied. A partial save advances the marker, so it must prove "+
			"what it advanced past",
			slug, joinNames(identities(pending)), plural(len(pending), "is", "are"))}
	}
	if journal.Feature != slug {
		// Refuse an anonymous journal as well as a foreign one. Feature exists
		// on the type precisely so a journal read out of context is
		// self-describing; a record with it missing is malformed, and
		// grandfathering malformed evidence into an authority grant is the
		// same mistake as inferring authority in the first place.
		named := fmt.Sprintf("names %q", journal.Feature)
		if journal.Feature == "" {
			named = "names no feature at all"
		}
		return []string{fmt.Sprintf("%s: the refine journal here %s, so it is not evidence "+
			"about this feature", slug, named)}
	}
	if len(pending) > 1 {
		var unaccounted []string
		for _, a := range pending {
			if a.Seq != journal.Amendment {
				unaccounted = append(unaccounted, amendmentIdentity(a))
			}
		}
		return []string{fmt.Sprintf("%s: the refine journal accounts for amendment %d, but %s %s "+
			"also unapplied. A save advances the marker past EVERY record below it, so an "+
			"unaccounted one would be recorded applied without ever being applied",
			slug, journal.Amendment, joinNames(unaccounted), plural(len(unaccounted), "is", "are"))}
	}
	if pending[0].Seq != journal.Amendment {
		return []string{fmt.Sprintf("%s: the refine journal accounts for amendment %d, but the "+
			"unapplied record is %s", slug, journal.Amendment, amendmentIdentity(pending[0]))}
	}
	if err := journalReachedTested(journal); err != nil {
		return []string{fmt.Sprintf("%s: the refinement of %s cannot be shown complete — %v. "+
			"Blessing output that was never tested is the one thing the build state must not do",
			slug, amendmentIdentity(pending[0]), err)}
	}
	return nil
}

func identities(as []parser.Amendment) []string {
	out := make([]string, 0, len(as))
	for _, a := range as {
		out = append(out, amendmentIdentity(a))
	}
	return out
}
