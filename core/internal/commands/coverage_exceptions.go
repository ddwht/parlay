// parlay-feature: parlay-tool/criterion-authority
// parlay-component: coverage-exception-ledger
//
// What a person deliberately excused, and against which state.
//
// This replaces the exemption half of coverage-review.yaml. The approval half
// is gone — approving suite NAMES proved someone answered, never that they saw
// anything — but exemptions were always the real content: a person saying "this
// criterion genuinely needs no test", which no walker can decide.
//
// The property this adds is freshness. Today validate.go folds exemptions into
// ExemptCriteria without reading either hash, so nothing binds an exemption to
// the artifacts it was granted against; the ONLY thing enforcing that was the
// blanket gate, which is being removed. Without this, removing that gate would
// silently convert every recorded exemption into a permanent unconditional
// waiver — aimed precisely at the criteria a person once said needed no test.
// That is a strictly worse failure than the one being fixed, and it would be
// invisible.

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

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/atomicfile"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

const coverageExceptionsSchemaVersion = 1

// ExceptionKind is the closed set of things a person may excuse.
//
// `waived` and `state-only` are supported; `hand-authored` is RESERVED.
//
// state-only is not an exemption and is not treated as one: it excuses nothing.
// It records a person accepting that ONE case observes its criterion more
// weakly than the criterion states — "the archive button is disabled" checked by
// reading the stored flag rather than the rendered control. The criterion is
// still covered; the claim is weaker.
//
// It needs a decision because nothing else can see it. The suite review that
// used to catch a weakened observation is gone; criterion approval happens
// before testcases exist, so it cannot; and the case cites its criterion
// correctly, so every mechanical walk passes. Without a recorded decision an
// agent could substitute a weaker check for the one that was approved and
// proceed — including unattended, where a warning simply advances.
//
// hand-authored is refused for a sharper reason: it SUCCEEDED, producing a real
// exemption for any contained regular file whose hash matched. That is a false
// coverage path rather than deferred richness — existence and a stable
// fingerprint establish that something is there and unchanged, never that it is
// a test, still less that it tests THIS criterion. Honouring it means
// reconciling with the authored-unit model that already exists for exactly this
// purpose: a declared unit, its `tests:` globs, and the criteria its
// `satisfies:` claims. Inventing a second vocabulary for external coverage
// beside that one is how two answers to the same question start disagreeing.
//
// waived-only is a coherent release. The others are RESERVED: named so a
// ledger using one gets a remedy rather than a parse error, refused so neither
// can excuse anything, and carrying no fields or helpers of their own. An
// unsupported kind that ships validation machinery nothing reaches is the
// orphaned-leaf shape this release spent itself eliminating — the path checks
// and body hashing that used to sit here were reachable only from their own
// tests.
type ExceptionKind string

const (
	ExceptionWaived       ExceptionKind = "waived"
	ExceptionStateOnly    ExceptionKind = "state-only"
	ExceptionHandAuthored ExceptionKind = "hand-authored"
)

// exceptionKinds are the values a ledger may NAME. Whether a kind is honoured
// is a separate question, answered per kind below.
var exceptionKinds = map[ExceptionKind]bool{
	ExceptionWaived: true, ExceptionStateOnly: true, ExceptionHandAuthored: true,
}

// Accepts reports whether a decision covers this case's downgrade.
//
// fingerprint is the case's content as it stands now. A decision carrying a
// hash accepts only the content it was granted against; one carrying none
// predates the binding and accepts nothing, so it surfaces for re-confirmation
// rather than silently continuing to excuse a case nobody can prove is the one
// that was reviewed.
func (d DowngradeDecision) AcceptsCase(ref, text, suite, caseName, fingerprint string) bool {
	if !d.Accepts(ref, text, suite, caseName) {
		return false
	}
	return d.CaseHash != "" && d.CaseHash == fingerprint
}

func (d DowngradeDecision) Accepts(ref, text, suite, caseName string) bool {
	return d.Ref == ref && d.Text == agent.CanonicalCriterionText(text) &&
		d.Suite == suite && d.Case == caseName
}

// CoverageException is one criterion a person excused.
type CoverageException struct {
	// Ref is the contract entry. Text narrows it to one bullet; empty means
	// the whole entry, which is broader than preferred and reported as such.
	Ref  string `yaml:"ref"`
	Text string `yaml:"criterion_text,omitempty"`

	Kind   ExceptionKind `yaml:"kind"`
	Reason string        `yaml:"reason"`

	// At and By belong to THIS decision. They used to live at file level and
	// were overwritten on every append, so recording a second person's
	// judgment rewrote the first one as theirs — an audit trail that
	// misattributes is worse than none.
	At string `yaml:"at,omitempty"`
	By string `yaml:"by,omitempty"`

	// EntryHash binds an ENTRY-WIDE exception to the set of bullets that entry
	// carried when it was granted, so adding a bullet invalidates it. A
	// bullet-specific exception needs no such field: its (ref, text) IS its
	// binding, and it is valid exactly while that bullet is still declared.
	EntryHash string `yaml:"entry_hash,omitempty"`

	// Suite and Case identify the downgraded case a `state-only` decision
	// accepts. A downgrade is a judgment about ONE case observing ONE criterion
	// more weakly than it states, so the decision binds to that case: accepting
	// "checking the store is honest here" says nothing about a different case
	// doing the same thing for a different reason.
	Suite string `yaml:"suite,omitempty"`
	Case  string `yaml:"case,omitempty"`

	// CaseHash binds the decision to what that case actually observed when it
	// was approved. Suite and Case are NAMES, and a name survives its body
	// being replaced wholesale: a different observation, for different
	// reasons, still citing this criterion and still marked state-only, keeps
	// matching a decision keyed on names alone. The reviewer is then recorded
	// as having approved something they never saw. Empty means the decision is
	// PRE-BINDING — an old-format entry in this ledger, not a stranded
	// coverage-review exemption — and must be re-confirmed.
	CaseHash string `yaml:"case_hash,omitempty"`
}

// RetiredDecision is a withdrawn approval, carrying both the original
// judgment and the decision to withdraw it — one without the other tells only
// half the story.
type RetiredDecision struct {
	Ref   string        `yaml:"ref"`
	Text  string        `yaml:"text,omitempty"`
	Kind  ExceptionKind `yaml:"kind"`
	Suite string        `yaml:"suite,omitempty"`
	Case  string        `yaml:"case,omitempty"`

	OriginalReason string `yaml:"original_reason,omitempty"`
	OriginalBy     string `yaml:"original_by,omitempty"`
	OriginalAt     string `yaml:"original_at,omitempty"`

	Reason string `yaml:"reason"`
	By     string `yaml:"by"`
	At     string `yaml:"at"`
}

// CoverageExceptions is the per-feature ledger.
type CoverageExceptions struct {
	SchemaVersion int    `yaml:"schema_version"`
	Feature       string `yaml:"feature"`

	// GrantedAt is when this ledger was first opened. Per-decision attribution
	// lives on each entry: a file-level "granted by" is a single slot that
	// every later append overwrites, which silently reassigns earlier
	// judgments to whoever recorded the most recent one.
	GrantedAt string `yaml:"granted_at"`

	// CriteriaHash is the whole standard at grant time, kept as audit context
	// only. It is deliberately NOT the binding: an exception is a localized
	// claim about one criterion, and binding it to the whole feature would let
	// an unrelated operation reword force re-review of a waived presentation
	// bullet. Criteria authority approves the entire standard; this approves
	// one thing being weaker or absent, and the durable binding should match
	// the claim actually granted.
	CriteriaHash string `yaml:"criteria_hash,omitempty"`

	Exceptions []CoverageException `yaml:"exceptions"`

	// ReconciledLegacy records what became of each exemption that was
	// stranded in the retired coverage-review.yaml.
	//
	// Needed because the stranded check used to fire only while this file did
	// not exist, so the first fresh decision made every OTHER stranded
	// judgment disappear from the blocker — migrating one exemption silently
	// abandoned the rest. Dispositions are per legacy entry, so the check can
	// keep firing until every one has been either re-recorded or deliberately
	// dropped.
	// RetiredDecisions are approvals withdrawn because what they were about
	// changed or went away. They are kept rather than deleted: an approval
	// that silently vanishes is indistinguishable from one nobody made, and
	// "who decided this no longer applies, and why" has to stay answerable.
	RetiredDecisions []RetiredDecision `yaml:"retired_decisions,omitempty"`

	// LegacyFileHash is the retired review these dispositions answer. A
	// disposition names a position in that file, so an edited file invalidates
	// every one of them rather than silently repointing them.
	LegacyFileHash string `yaml:"legacy_file_hash,omitempty"`

	ReconciledLegacy []LegacyDisposition `yaml:"reconciled_legacy,omitempty"`

	// DeferredLegacy are review ATTEMPTS, not answers. They are kept separate
	// from ReconciledLegacy on purpose: only that list satisfies
	// reconciliation, so a deferral cannot unblock anything no matter how many
	// accumulate. Treating uncertainty as a completed outcome would silently
	// withdraw a possibly load-bearing waiver, which is the failure this whole
	// reconciliation exists to prevent.
	DeferredLegacy []LegacyDeferral `yaml:"deferred_legacy,omitempty"`
}

// LegacyDeferral is one person looking at a stranded judgment and being unable
// to say whether it still holds.
//
// "I cannot say" and "this no longer applies" are different statements, and
// only the second is a decision. Recording the attempt anyway means the next
// reviewer sees that somebody already looked, and who, rather than
// rediscovering the entry cold — while the entry itself stays unanswered.
//
// The series is preserved rather than overwritten. Two people independently
// unable to decide is a materially different fact from one attempt replaced
// twice, and it is the kind of fact that should make somebody ask why.
type LegacyDeferral struct {
	Ref         string `yaml:"ref"`
	Text        string `yaml:"criterion_text,omitempty"`
	Fingerprint string `yaml:"fingerprint"`
	Duplicate   int    `yaml:"duplicate,omitempty"`
	// SourceHash is the version of the legacy file this attempt was made
	// against. The same person deferring again after the file changed is
	// looking at a different subject, and that attempt is new.
	SourceHash string `yaml:"source_hash"`
	Reason     string `yaml:"reason"`
	At         string `yaml:"at"`
	By         string `yaml:"by"`
}

// sameAttempt reports whether two deferrals record the same act of review.
//
// Time is deliberately NOT compared. An atomic write can succeed while its
// response is lost, and the retry that follows carries a new clock reading for
// the same act — comparing timestamps would turn transport uncertainty into a
// duplicate entry, and refusing it outright would turn it into operator
// repair. What identifies the attempt is who looked, at which occurrence, in
// which version of the file, and what they said.
func (d LegacyDeferral) sameAttempt(o LegacyDeferral) bool {
	return d.Fingerprint == o.Fingerprint && d.Duplicate == o.Duplicate &&
		d.SourceHash == o.SourceHash &&
		strings.TrimSpace(d.By) == strings.TrimSpace(o.By) &&
		strings.TrimSpace(d.Reason) == strings.TrimSpace(o.Reason)
}

// LegacyDisposition is one stranded exemption, answered.
//
// "Dropped" is a decision and is recorded as one. A judgment abandoned without
// a trace is indistinguishable from one nobody noticed, which is the failure
// this whole reconciliation exists to prevent.
type LegacyDisposition struct {
	Ref  string `yaml:"ref"`
	Text string `yaml:"criterion_text,omitempty"`
	// Fingerprint identifies the EXACT legacy entry this answers, over that
	// entry's whole content — suite, item, criterion text and reason. Two
	// exemptions may share a ref and criterion text while recording different
	// judgments for different reasons; keyed on (ref, text) alone, answering
	// one marked BOTH answered, so an unreviewed legacy judgment passed as
	// reconciled.
	//
	// Content rather than position, because reordering the file must not
	// change which judgment a disposition answered.
	Fingerprint string `yaml:"fingerprint"`
	// Duplicate distinguishes entries whose content is identical in every
	// field, where the fingerprint alone cannot. It is a 0-based index among
	// those copies, so one disposition cannot answer two of them.
	Duplicate int `yaml:"duplicate,omitempty"`
	// Disposition is "recorded" or "dropped".
	Disposition string `yaml:"disposition"`
	Reason      string `yaml:"reason"`
	At          string `yaml:"at"`
	By          string `yaml:"by"`
}

// legacyKey identifies a stranded exemption across the two files.
func legacyKey(ref, text string) string {
	return ref + "\x00" + agent.CanonicalCriterionText(text)
}

// legacyExemptionFingerprint identifies one EXACT legacy entry by its whole
// content. Reason is included deliberately: two exemptions on the same bullet
// recording different justifications are two judgments, and answering one says
// nothing about the other.
func legacyExemptionFingerprint(ex parser.CoverageExemption) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		ex.Suite, ex.Item,
		agent.CanonicalCriterionText(ex.CriterionText),
		strings.TrimSpace(ex.Reason),
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

// legacyDispositionKey pairs the content fingerprint with the index among
// byte-identical copies.
func legacyDispositionKey(fp string, duplicate int) string {
	return fmt.Sprintf("%s#%d", fp, duplicate)
}

// legacyFileHash fingerprints the retired review, so dispositions recorded
// against it can be shown to be about the file that is actually there. Without
// it, positions recorded yesterday are read against a file edited since, and a
// disposition silently comes to answer a different entry than the one somebody
// judged.
func legacyFileHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// The file is named for what it holds. It began as a list of waivers — "this
// criterion needs no test" — and "exceptions" described that. It now also holds
// approvals that a criterion IS discharged but observed weakly, which waive
// nothing; decisions withdrawn but kept for the audit trail; and answers to
// judgments inherited from the retired coverage review. Only the first group is
// an exception, and a name that describes a quarter of a file's contents sends
// every later reader looking for the wrong thing.
func coverageDecisionsPath(cfg *config.Context, slug string) string {
	return filepath.Join(cfg.BuildPath(slug), "coverage-decisions.yaml")
}

// legacyCoverageDecisionsPath is the pre-rename name. It is read, never
// written: a project that has one gets it migrated on the next write rather
// than being told to go and rename a file itself.
func legacyCoverageDecisionsPath(cfg *config.Context, slug string) string {
	return filepath.Join(cfg.BuildPath(slug), "coverage-exceptions.yaml")
}

func loadCoverageExceptions(cfg *config.Context, slug string) (*CoverageExceptions, error) {
	data, err := os.ReadFile(coverageDecisionsPath(cfg, slug))
	if err != nil && os.IsNotExist(err) {
		data, err = os.ReadFile(legacyCoverageDecisionsPath(cfg, slug))
	}
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var rec CoverageExceptions
	if err := yaml.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("invalid coverage-decisions file: %w", err)
	}
	if rec.SchemaVersion != coverageExceptionsSchemaVersion {
		return nil, fmt.Errorf("coverage-decisions schema_version %d is not supported (expected %d)", rec.SchemaVersion, coverageExceptionsSchemaVersion)
	}
	if rec.Feature != slug {
		return nil, fmt.Errorf("coverage-decisions names feature %q but was read for %q", rec.Feature, slug)
	}
	for i, ex := range rec.Exceptions {
		if strings.TrimSpace(ex.Ref) == "" {
			return nil, fmt.Errorf("exception %d names no ref", i+1)
		}
		if !exceptionKinds[ex.Kind] {
			return nil, fmt.Errorf("exception %d has kind %q, outside {waived, state-only, hand-authored}", i+1, ex.Kind)
		}
		if strings.TrimSpace(ex.Reason) == "" {
			return nil, fmt.Errorf("exception %d for %s records no reason — an exception nobody can review later is not one", i+1, ex.Ref)
		}
		if strings.TrimSpace(ex.By) == "" || strings.TrimSpace(ex.At) == "" {
			return nil, fmt.Errorf("exception %d for %s is missing what decided it or when — attribution is per decision, and a ledger that cannot say who made which one, or in what order, cannot be audited", i+1, ex.Ref)
		}
	}
	for i, d := range rec.ReconciledLegacy {
		switch d.Disposition {
		case "recorded", "dropped":
		default:
			return nil, fmt.Errorf("legacy disposition %d has %q, outside {recorded, dropped}", i+1, d.Disposition)
		}
		if strings.TrimSpace(d.Reason) == "" || strings.TrimSpace(d.By) == "" || strings.TrimSpace(d.At) == "" {
			return nil, fmt.Errorf("legacy disposition %d for %s is missing why, what decided it, or when — a judgment answered without those is answered in name only", i+1, d.Ref)
		}
	}
	for i, d := range rec.DeferredLegacy {
		if strings.TrimSpace(d.Reason) == "" || strings.TrimSpace(d.By) == "" || strings.TrimSpace(d.At) == "" {
			return nil, fmt.Errorf("deferral %d for %s is missing why, who, or when — an attempt nobody can attribute tells the next reviewer nothing they did not already know", i+1, d.Ref)
		}
		if strings.TrimSpace(d.SourceHash) == "" {
			return nil, fmt.Errorf("deferral %d for %s records no source version, so it cannot be told apart from an attempt against a different version of the legacy file", i+1, d.Ref)
		}
	}
	return &rec, nil
}

func saveCoverageExceptions(cfg *config.Context, slug string, rec *CoverageExceptions) error {
	rec.SchemaVersion = coverageExceptionsSchemaVersion
	rec.Feature = slug
	data, err := yaml.Marshal(rec)
	if err != nil {
		return err
	}
	path := coverageDecisionsPath(cfg, slug)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := atomicfile.WriteAtomic(path, data); err != nil {
		return err
	}
	// A pre-rename file is removed only AFTER the new one is safely written,
	// so an interrupted migration leaves the old ledger intact rather than
	// neither. Leaving both would be worse than either alone: two files, one
	// stale, and nothing saying which the tool reads.
	old := legacyCoverageDecisionsPath(cfg, slug)
	if old != path {
		if err := os.Remove(old); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("the decisions were written to %s but the pre-rename %s could not be removed, and leaving both would leave a stale ledger nobody can tell from the live one: %w", path, old, err)
		}
	}
	return nil
}

// ExceptionsVerdict reports what the ledger excuses, and what is wrong with it.
// DowngradeDecision is a person accepting one case's weaker observation.
type DowngradeDecision struct {
	Ref, Text   string
	Suite, Case string
	CaseHash    string
}

type ExceptionsVerdict struct {
	// AcceptedDowngrades are the weakened observations somebody approved.
	// Carried separately from Exempt because they excuse nothing: the
	// criterion IS discharged, by a case that observes it more weakly.
	AcceptedDowngrades []DowngradeDecision
	// Exempt is the set to hand the criterion walker. Empty when the ledger is
	// stale: a judgment about a different contract excuses nothing here.
	Exempt agent.ExemptedCriteria
	// Blockers are reasons the ledger cannot be honoured.
	Blockers []string
	// Warnings are honoured but worth saying.
	Warnings []string
}

// EvaluateCoverageExceptions binds a ledger to the state it was granted against.
//
// A stale ledger BLOCKS and excuses nothing. Dropping its entries quietly would
// turn each waiver back into an uncovered criterion, which under warning
// severities may still proceed — so freshness would be advisory, which is the
// opposite of the point. Refusing is what makes it a real check: a person said
// this criterion needs no test, about a contract that has since changed, and
// only a person can say whether that judgment survives.
func EvaluateCoverageExceptions(root string, rec *CoverageExceptions, current []AuthorizedCriterion) ExceptionsVerdict {
	var v ExceptionsVerdict
	if rec == nil || len(rec.Exceptions) == 0 {
		return v
	}

	// Bullets actually declared right now, and the per-entry bullet sets an
	// entry-wide exception is bound to.
	declared := map[agent.CriterionRef]bool{}
	byEntry := map[string][]AuthorizedCriterion{}
	for _, c := range current {
		declared[agent.CriterionRef{Ref: c.Ref, Text: agent.CanonicalCriterionText(c.Text)}] = true
		byEntry[c.Ref] = append(byEntry[c.Ref], c)
	}

	// A waiver's identity is the criterion: one criterion cannot be excused
	// twice. A state-only decision's identity includes the case, because
	// several cases may each observe one criterion weakly and each needs its
	// own judgment. Keying both on (ref, text) alone wedges the ledger the
	// moment a second case discharges a criterion a first one already
	// downgraded — a legitimate shape, refused as a duplicate.
	type claimKey struct{ ref, text, suite, caseName string }
	claimed := map[claimKey]bool{}
	for _, ex := range rec.Exceptions {
		text := agent.CanonicalCriterionText(ex.Text)
		key := agent.CriterionRef{Ref: ex.Ref, Text: text}
		ck := claimKey{ref: ex.Ref, text: text}
		if ex.Kind == ExceptionStateOnly {
			ck.suite, ck.caseName = ex.Suite, ex.Case
		}

		// Two exceptions claiming the same thing is an authoring defect: they
		// cannot be reviewed independently and one silently shadows the other.
		if claimed[ck] {
			v.Blockers = append(v.Blockers, fmt.Sprintf("%s is excused twice — one claim shadows the other and neither can be reviewed on its own", describeClaim(ex)))
			continue
		}
		claimed[ck] = true

		bullets, entryExists := byEntry[ex.Ref]
		if !entryExists {
			v.Blockers = append(v.Blockers, fmt.Sprintf(
				"exception for %s excuses a contract entry that no longer declares criteria", ex.Ref))
			continue
		}

		if text == "" {
			// Entry-wide: bound to the bullet SET, so adding one invalidates
			// it. Accepted at all because every exemption written before
			// bullet identity is this shape and none could have recorded a
			// text.
			want := entryBulletsHash(bullets)
			if ex.EntryHash == "" {
				v.Blockers = append(v.Blockers, fmt.Sprintf(
					"entry-wide exception for %s records no entry_hash — it would then excuse bullets added after it was granted, which nobody judged", ex.Ref))
				continue
			}
			if ex.EntryHash != want {
				v.Blockers = append(v.Blockers, fmt.Sprintf(
					"entry-wide exception for %s was granted against a different set of bullets on that entry (%s, now %s) — re-review it",
					ex.Ref, shortHash(ex.EntryHash), shortHash(want)))
				continue
			}
			if v.Exempt.Entries == nil {
				v.Exempt.Entries = map[string]bool{}
			}
			v.Exempt.Entries[ex.Ref] = true
			v.Warnings = append(v.Warnings, fmt.Sprintf(
				"exception for %s is entry-wide, so it excuses every bullet on that entry — narrow it to the one it was meant for", ex.Ref))
			continue
		}

		// Bullet-specific: the (ref, text) pair IS the binding. Membership is
		// checked exactly rather than by entry, because a typo or a fabricated
		// text would otherwise become an inert exemption plus an uncovered
		// warning — a broken ledger that reads as a working one.
		if !declared[key] {
			v.Blockers = append(v.Blockers, fmt.Sprintf(
				"exception for %s names a criterion that entry does not declare: %q — a text that matches no bullet excuses nothing and looks like it does", ex.Ref, ex.Text))
			continue
		}

		if ex.Kind == ExceptionStateOnly {
			if strings.TrimSpace(ex.Suite) == "" || strings.TrimSpace(ex.Case) == "" {
				v.Blockers = append(v.Blockers, fmt.Sprintf(
					"state-only decision for %s names no suite:/case: — a downgrade is a judgment about one case observing one criterion weakly, and one that names no case accepts every weakening of that criterion including ones nobody saw", ex.Ref))
				continue
			}
			// Recorded, and deliberately NOT added to Exempt: the criterion is
			// covered by a real case. What this authorizes is the weaker
			// observation, which the readiness walk resolves against the
			// testcases.
			v.AcceptedDowngrades = append(v.AcceptedDowngrades, DowngradeDecision{
				Ref: ex.Ref, Text: text, Suite: ex.Suite, Case: ex.Case, CaseHash: ex.CaseHash,
			})
			continue
		}

		if ex.Kind == ExceptionHandAuthored {
			v.Blockers = append(v.Blockers, fmt.Sprintf(
				"exception for %s is kind: hand-authored, which is RESERVED and not implemented — declare the test as an authored unit and let its satisfies: carry the claim, or use kind: waived if the criterion genuinely needs no generated test", ex.Ref))
			continue
		}

		if v.Exempt.Bullets == nil {
			v.Exempt.Bullets = map[agent.CriterionRef]bool{}
		}
		v.Exempt.Bullets[key] = true
	}
	return v
}

// entryBulletsHash fingerprints every bullet currently on one contract entry.
func entryBulletsHash(bullets []AuthorizedCriterion) string {
	return CriteriaHash(bullets)
}

func describeClaim(ex CoverageException) string {
	if strings.TrimSpace(ex.Text) == "" {
		return ex.Ref + " (entry-wide)"
	}
	return fmt.Sprintf("%s — %q", ex.Ref, ex.Text)
}

func shortHash(h string) string {
	h = strings.TrimPrefix(h, "sha256:")
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// CheckCoverageExceptions is the production entry point: read the ledger, read
// the standard, and report what the ledger excuses and what is wrong with it.
//
// One function every caller shares. The gate had been reading only the excused
// SET while discarding every blocker and every error, so a stale ledger
// excused nothing and said nothing — precisely the drop-and-proceed behaviour
// the freshness rule exists to prevent, with a comment above it claiming the
// opposite.
func CheckCoverageExceptions(cfg *config.Context, slug string) ExceptionsVerdict {
	rec, err := loadCoverageExceptions(cfg, slug)
	if err != nil {
		// A ledger that will not load is not a feature with nothing excused.
		return ExceptionsVerdict{Blockers: []string{
			fmt.Sprintf("coverage exceptions for %s cannot be read: %v — an unreadable ledger is not an empty one", slug, err),
		}}
	}
	// Stranded legacy judgments are checked ALWAYS, not only while this file is
	// absent. Gating on absence meant the first fresh decision created the file
	// and every other stranded exemption vanished from the blocker — migrating
	// one silently abandoned the rest.
	stranded, unreadable := strandedLegacyExemptions(cfg, slug, rec)
	if unreadable != "" {
		return ExceptionsVerdict{Blockers: []string{fmt.Sprintf(
			"%s has a coverage-review.yaml that cannot be read (%s) — it may hold exemptions nobody can now recover, and the gate that used to block on this is gone. "+
				"Repair or delete it deliberately", slug, unreadable)}}
	}
	if len(stranded) > 0 {
		return ExceptionsVerdict{Blockers: []string{fmt.Sprintf(
			"%s has %d exemption(s) in the retired coverage-review.yaml that nothing has answered: %s. "+
				"Those are no longer read, so leaving them is quietly dropping judgments somebody made. "+
				"Re-record each one that still holds, or drop it deliberately — `parlay internal migrate-coverage-exceptions @%s` lists them",
			slug, len(stranded), strings.Join(stranded, "; "), slug)}}
	}

	if rec == nil {
		return ExceptionsVerdict{}
	}
	current, err := CurrentCriteria(cfg, slug)
	if err != nil {
		return ExceptionsVerdict{Blockers: []string{
			fmt.Sprintf("cannot establish which criteria %s is graded against, so its exceptions cannot be checked: %v", slug, err),
		}}
	}
	return EvaluateCoverageExceptions(cfg.Root.Path, rec, current)
}

// strandedLegacyExemptions counts exemptions in a retired coverage-review.yaml
// that nothing reads any more.
//
// Presence of the legacy file alone is ignored: most carry only suite
// approvals, which were the half that proved nothing and are gone on purpose.
// Only recorded exemptions are stranded, because only they were load-bearing.
func strandedLegacyExemptions(cfg *config.Context, slug string, rec *CoverageExceptions) (stranded []string, unreadable string) {
	path := filepath.Join(cfg.BuildPath(slug), "coverage-review.yaml")
	if _, statErr := os.Stat(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return nil, "" // no legacy file: nothing stranded
		}
		return nil, statErr.Error()
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, readErr.Error()
	}
	legacy, err := parser.ParseCoverageReviewBytes(path, content)
	if err != nil {
		// Present and unreadable is NOT absent. Collapsing the two meant a
		// malformed legacy review — which may well contain exemptions nobody
		// can now read — was treated exactly like a feature that never had
		// any, and the blanket gate being removed is the last thing that would
		// have blocked it.
		return nil, err.Error()
	}
	if legacy == nil {
		return nil, ""
	}

	// Dispositions answer entries in ONE version of the legacy file. If that
	// file changed after they were written, they no longer demonstrably answer
	// what is there now — an entry may have been added, or one somebody
	// judged may have been reworded into a different judgment. Refusing the
	// whole set is right: silently keeping them would let an edit to the
	// legacy file retire a reconciliation nobody redid.
	hash := legacyFileHash(content)
	if rec != nil && len(rec.ReconciledLegacy) > 0 && rec.LegacyFileHash != "" && rec.LegacyFileHash != hash {
		return []string{fmt.Sprintf(
			"the retired coverage review at %s changed after its exemptions were reconciled, so the %d recorded disposition(s) no longer demonstrably answer what it now contains. Re-run migrate-coverage-exceptions against the current file",
			path, len(rec.ReconciledLegacy))}, ""
	}

	answered := map[string]bool{}
	if rec != nil {
		for _, d := range rec.ReconciledLegacy {
			answered[legacyDispositionKey(d.Fingerprint, d.Duplicate)] = true
		}
	}
	// Deferrals do NOT go into `answered`. They are attempts, and an attempt
	// that unblocked the boundary would be an answer wearing a softer word.
	// They only change how an unanswered entry is DESCRIBED.
	deferrals := map[string][]LegacyDeferral{}
	if rec != nil {
		for _, d := range rec.DeferredLegacy {
			k := legacyDispositionKey(d.Fingerprint, d.Duplicate)
			deferrals[k] = append(deferrals[k], d)
		}
	}

	seen := map[string]int{}
	for _, ex := range legacy.Exemptions {
		if ex.Item == "" {
			continue
		}
		fp := legacyExemptionFingerprint(ex)
		dup := seen[fp]
		seen[fp]++
		key := legacyDispositionKey(fp, dup)
		if answered[key] {
			continue
		}
		desc := describeStrandedOccurrence(ex.Item, ex.CriterionText, ex.Reason, fp)
		if tries := deferrals[key]; len(tries) > 0 {
			latest := tries[len(tries)-1]
			noun := "review"
			if len(tries) > 1 {
				noun = "reviews"
			}
			desc = fmt.Sprintf("%s [%d %s, none conclusive; most recently %s: %q]",
				desc, len(tries), noun, latest.By, strings.TrimSpace(latest.Reason))
		}
		stranded = append(stranded, desc)
	}
	sort.Strings(stranded)
	return stranded, ""
}

func describeStranded(ref, text string) string {
	if strings.TrimSpace(text) == "" {
		return ref + " (entry-wide)"
	}
	return fmt.Sprintf("%s — %q", ref, text)
}

// describeStrandedOccurrence names ONE entry in a way a reader can act on.
//
// Ref and criterion text are not enough. Two exemptions may carry both and
// still be different judgments recorded for different reasons — that is exactly
// why identity is a content fingerprint. Rendered on ref and text alone, those
// two appear in the report as one line repeated, so a reviewer answering "the
// first one" has no way to know which they answered, and no way to see that a
// second is still waiting. The reason is what distinguishes them to a person;
// the fingerprint is what distinguishes them to the writer.
func describeStrandedOccurrence(ref, text, reason, fingerprint string) string {
	out := describeStranded(ref, text)
	if r := strings.TrimSpace(reason); r != "" {
		out += fmt.Sprintf(" [granted because: %s]", r)
	}
	if len(fingerprint) >= 8 {
		out += " #" + fingerprint[:8]
	}
	return out
}
