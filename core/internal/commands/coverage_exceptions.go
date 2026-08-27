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
	"fmt"
	"os"
	"path/filepath"
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
}

// CoverageExceptions is the per-feature ledger.
type CoverageExceptions struct {
	SchemaVersion int    `yaml:"schema_version"`
	Feature       string `yaml:"feature"`

	// GrantedAt and GrantedBy record when and by what. GrantedBy is attribution,
	// not proof: environment identity cannot establish that a person exercised
	// judgment, which is why the old artifact recorded a background process as
	// a reviewer. It is written only when something was actually declined.
	GrantedAt string `yaml:"granted_at"`
	GrantedBy string `yaml:"granted_by,omitempty"`

	// CriteriaHash is the whole standard at grant time, kept as audit context
	// only. It is deliberately NOT the binding: an exception is a localized
	// claim about one criterion, and binding it to the whole feature would let
	// an unrelated operation reword force re-review of a waived presentation
	// bullet. Criteria authority approves the entire standard; this approves
	// one thing being weaker or absent, and the durable binding should match
	// the claim actually granted.
	CriteriaHash string `yaml:"criteria_hash,omitempty"`

	Exceptions []CoverageException `yaml:"exceptions"`
}

func coverageExceptionsPath(cfg *config.Context, slug string) string {
	return filepath.Join(cfg.BuildPath(slug), "coverage-exceptions.yaml")
}

func loadCoverageExceptions(cfg *config.Context, slug string) (*CoverageExceptions, error) {
	data, err := os.ReadFile(coverageExceptionsPath(cfg, slug))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var rec CoverageExceptions
	if err := yaml.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("invalid coverage-exceptions file: %w", err)
	}
	if rec.SchemaVersion != coverageExceptionsSchemaVersion {
		return nil, fmt.Errorf("coverage-exceptions schema_version %d is not supported (expected %d)", rec.SchemaVersion, coverageExceptionsSchemaVersion)
	}
	if rec.Feature != slug {
		return nil, fmt.Errorf("coverage-exceptions names feature %q but was read for %q", rec.Feature, slug)
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
	path := coverageExceptionsPath(cfg, slug)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomicfile.WriteAtomic(path, data)
}

// ExceptionsVerdict reports what the ledger excuses, and what is wrong with it.
// DowngradeDecision is a person accepting one case's weaker observation.
type DowngradeDecision struct {
	Ref, Text   string
	Suite, Case string
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

	claimed := map[agent.CriterionRef]bool{}
	for _, ex := range rec.Exceptions {
		text := agent.CanonicalCriterionText(ex.Text)
		key := agent.CriterionRef{Ref: ex.Ref, Text: text}

		// Two exceptions claiming the same thing is an authoring defect: they
		// cannot be reviewed independently and one silently shadows the other.
		if claimed[key] {
			v.Blockers = append(v.Blockers, fmt.Sprintf("%s is excused twice — one claim shadows the other and neither can be reviewed on its own", describeClaim(ex)))
			continue
		}
		claimed[key] = true

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
				Ref: ex.Ref, Text: text, Suite: ex.Suite, Case: ex.Case,
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
	if rec == nil {
		// No new ledger. A legacy coverage-review.yaml carrying exemptions is
		// not nothing: validate.go used to read them, and it no longer does,
		// so staying silent here would let a person's recorded judgment
		// evaporate into uncovered warnings that a build may still pass. Fail
		// closed with the remedy rather than migrating silently — which of
		// those judgments still applies is exactly what nobody but their
		// author can say.
		stranded, unreadable := strandedLegacyExemptions(cfg, slug)
		if unreadable != "" {
			return ExceptionsVerdict{Blockers: []string{fmt.Sprintf(
				"%s has a coverage-review.yaml that cannot be read (%s) — it may hold exemptions nobody can now recover, and the gate that used to block on this is gone. "+
					"Repair or delete it deliberately", slug, unreadable)}}
		}
		if stranded > 0 {
			return ExceptionsVerdict{Blockers: []string{fmt.Sprintf(
				"%s has %d exemption(s) recorded in the retired coverage-review.yaml and no coverage-exceptions.yaml — "+
					"those are no longer read, so leaving this alone would quietly drop judgments somebody made. "+
					"Migrate them, or delete them deliberately if they no longer apply", slug, stranded)}}
		}
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
func strandedLegacyExemptions(cfg *config.Context, slug string) (count int, unreadable string) {
	path := filepath.Join(cfg.BuildPath(slug), "coverage-review.yaml")
	if _, statErr := os.Stat(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return 0, "" // no legacy file: nothing stranded
		}
		return 0, statErr.Error()
	}
	legacy, err := parser.ParseCoverageReview(path)
	if err != nil {
		// Present and unreadable is NOT absent. Collapsing the two meant a
		// malformed legacy review — which may well contain exemptions nobody
		// can now read — was treated exactly like a feature that never had
		// any, and the blanket gate being removed is the last thing that would
		// have blocked it.
		return 0, err.Error()
	}
	if legacy == nil {
		return 0, ""
	}
	return len(legacy.Exemptions), ""
}
