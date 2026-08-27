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
)

const coverageExceptionsSchemaVersion = 1

// ExceptionKind is the closed set of things a person may excuse.
//
// Named rather than free-form because each kind is a different claim, and a
// reader deciding whether an exception still applies needs to know which was
// made. "waived" says this criterion needs no test at all; "state-only" says it
// is checked by state rather than by output; "hand-authored" says a test exists
// that parlay cannot inspect.
type ExceptionKind string

const (
	ExceptionWaived       ExceptionKind = "waived"
	ExceptionStateOnly    ExceptionKind = "state-only"
	ExceptionHandAuthored ExceptionKind = "hand-authored"
)

var exceptionKinds = map[ExceptionKind]bool{
	ExceptionWaived: true, ExceptionStateOnly: true, ExceptionHandAuthored: true,
}

// CoverageException is one criterion a person excused.
type CoverageException struct {
	// Ref is the contract entry. Text narrows it to one bullet; empty means
	// the whole entry, which is broader than preferred and reported as such.
	Ref  string `yaml:"ref"`
	Text string `yaml:"criterion_text,omitempty"`

	Kind   ExceptionKind `yaml:"kind"`
	Reason string        `yaml:"reason"`

	// TestFile and TestHash bind a hand-authored exception to the body it
	// claims covers the criterion. Without the hash the file can change under
	// an evergreen approval, which is the same staleness this record exists to
	// prevent, one level down.
	TestFile string `yaml:"test_file,omitempty"`
	TestHash string `yaml:"test_hash,omitempty"`
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

	// The state these exceptions were granted against. An exception is a
	// judgment about a specific contract; when that contract moves, the
	// judgment has not been made about the new one.
	CriteriaHash string `yaml:"criteria_hash"`

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
type ExceptionsVerdict struct {
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
func EvaluateCoverageExceptions(rec *CoverageExceptions, current []AuthorizedCriterion) ExceptionsVerdict {
	var v ExceptionsVerdict
	if rec == nil || len(rec.Exceptions) == 0 {
		return v
	}

	if hash := CriteriaHash(current); rec.CriteriaHash != hash {
		v.Blockers = append(v.Blockers, fmt.Sprintf(
			"%d coverage exception(s) were granted against a different contract (%s, now %s) — "+
				"an exception is a judgment that a specific criterion needs no test, and the criteria have moved since. "+
				"Re-review them; they are not applied in the meantime",
			len(rec.Exceptions), shortHash(rec.CriteriaHash), shortHash(hash)))
		return v
	}

	declared := map[string]bool{}
	for _, c := range current {
		declared[c.Ref] = true
	}

	for _, ex := range rec.Exceptions {
		if !declared[ex.Ref] {
			v.Blockers = append(v.Blockers, fmt.Sprintf(
				"exception for %s excuses a contract entry that no longer declares criteria", ex.Ref))
			continue
		}
		if ex.Kind == ExceptionHandAuthored && strings.TrimSpace(ex.TestFile) == "" {
			v.Blockers = append(v.Blockers, fmt.Sprintf(
				"hand-authored exception for %s names no test file — the claim is that a test parlay cannot inspect covers this, and an uninspectable test that is also unnamed is not a claim", ex.Ref))
			continue
		}
		if text := agent.CanonicalCriterionText(ex.Text); text == "" {
			// Entry-wide: accepted, because every exemption written before
			// bullet-level identity existed is this shape and none of them
			// could have recorded a text. Warned, because it excuses bullets
			// nobody considered, including ones added later.
			if v.Exempt.Entries == nil {
				v.Exempt.Entries = map[string]bool{}
			}
			v.Exempt.Entries[ex.Ref] = true
			v.Warnings = append(v.Warnings, fmt.Sprintf(
				"exception for %s is entry-wide, so it excuses every criterion on that entry including any added later — narrow it to the bullet it was meant for", ex.Ref))
			continue
		} else {
			if v.Exempt.Bullets == nil {
				v.Exempt.Bullets = map[agent.CriterionRef]bool{}
			}
			v.Exempt.Bullets[agent.CriterionRef{Ref: ex.Ref, Text: text}] = true
		}
	}
	return v
}

func shortHash(h string) string {
	h = strings.TrimPrefix(h, "sha256:")
	if len(h) > 12 {
		return h[:12]
	}
	return h
}
