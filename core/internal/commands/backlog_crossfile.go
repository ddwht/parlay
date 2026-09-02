// parlay-feature: parlay-tool/backlog-and-activity
// parlay-section: cross-cutting
// parlay-source: backlog-item

package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// CROSS-FILE backlog findings.
//
// `validate --type backlog` reads one item and can only answer questions
// that one file contains. These three ask whether a ref RESOLVES against
// the project, so they belong here — to the commands that already hold
// the whole inventory — rather than to the single-file validator. The
// schema says so; until now it said so and nothing implemented them,
// which read as a statement about where they lived rather than an
// admission that they did not exist.
//
// Why they matter: the mutation commands prevent CREATING a dangling
// ref — fold resolves its destination and requires it open, promote
// scaffolds before it records — but nothing detects one that has since
// become dangling. A deleted feature, a compacted amendment, a hand
// edit. A closed item is never revisited, so a `becomes:` that stopped
// resolving is a permanently wrong answer to "what did this become"
// that nobody would otherwise notice.
const (
	CodeBacklogFoldDangling      = "backlog-fold-dangling"
	CodeBacklogPromotionDangling = "backlog-promotion-dangling"
	CodeBacklogItemStale         = "backlog-item-stale"
	// A target nobody could READ. Distinct from dangling, because
	// "it is gone" and "we could not tell" are different claims and only
	// the first accuses the record of anything.
	//
	// Published rather than silently dropped: the tri-state was honest
	// inside the resolver and dishonest at the surface, because
	// refUnavailable produced no finding at all. A corrupt amendment
	// therefore vanished from list and review entirely — worse than
	// mislabelling it, because nothing said anything was wrong.
	CodeBacklogPromotionTargetUnavailable = "backlog-promotion-target-unavailable"
)

// staleAfter is when an open item starts being reported as stale.
//
// NINETY DAYS, and this is a default rather than a discovered constant:
// the proposal specifies "an age bucket" without naming one. A quarter
// with no disposition is the point at which "we have not decided yet"
// has become "nobody is going to", which is the graveyard this feature
// exists to prevent.
//
// It is a WARNING and never a refusal. Age is not evidence that an item
// is wrong, only that it has been waiting; a project that deliberately
// keeps long-lived ideas is not misusing the backlog.
//
// PRIOR DEFERRALS DO NOT RESET IT. A deferral records that somebody
// looked and could not decide, which is review context for the next
// reviewer — not a fresh lease. Treating it as one would let an item be
// kept permanently invisible by being repeatedly not-decided, which is
// exactly the failure mode the age signal exists to surface.
const staleAfter = 90 * 24 * time.Hour

// backlogFinding is one cross-file problem, carrying the published code
// so a consumer keys on that rather than on English.
type backlogFinding struct {
	Code    string `json:"code"`
	Item    string `json:"item"`
	Root    string `json:"root,omitempty"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
}

// refResolution is the THREE answers a ref lookup can give.
//
// Three, not two, because "not there" and "could not tell" are different
// facts and only the first is a defect in the record. Collapsing them
// meant a malformed destination file was reported as "removed or
// renamed" — a confident claim about something nobody had read — while
// the same run separately reported it as unreadable. Two contradictory
// statements about one file, from one command.
type refResolution int

const (
	refResolved refResolution = iota
	// refMissing is a POSITIVE finding of absence: we looked where it
	// would be and it is not there.
	refMissing
	// refUnavailable is the honest non-answer: something is there and
	// nobody could read it. It produces no DANGLING finding — that would
	// accuse the record of something nobody established — but it does
	// produce its own, under CodeBacklogPromotionTargetUnavailable.
	refUnavailable
)

// crossFileFindings checks every item in one root against that root.
//
// `now` is a parameter rather than time.Now() so the age rule is
// testable at an instant instead of only in a project old enough to
// trigger it. `broken` is the parser's list of items that would not
// read, so a fold into one of them is reported as unreadable rather
// than as missing.
func crossFileFindings(cfg *config.Context, root string, items []*parser.BacklogItem, broken []string, now time.Time) []backlogFinding {
	byID := make(map[string]*parser.BacklogItem, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}
	// Ids whose file EXISTS and will not parse. They are not missing —
	// something is there and nobody can read it.
	unreadable := map[string]bool{}
	for _, b := range broken {
		if i := strings.Index(b, ".yaml:"); i > 0 {
			unreadable[b[:i]] = true
		}
	}

	var out []backlogFinding
	for _, it := range items {
		add := func(code, msg, fix string) {
			out = append(out, backlogFinding{Code: code, Item: it.ID, Root: root, Message: msg, Fix: fix})
		}

		for _, ev := range it.History {
			becomes := strings.TrimSpace(ev.Becomes)
			if becomes == "" {
				continue
			}
			switch ev.Event {
			case parser.EventFolded:
				// folded names another ITEM.
				switch {
				case byID[becomes] != nil:
				case unreadable[becomes]:
					// Present and unreadable. Already reported as an
					// unreadable record; claiming it was removed would
					// contradict that in the same output.
				default:
					add(CodeBacklogFoldDangling,
						fmt.Sprintf("folded into %q, which is not an item in this root", becomes),
						"the destination was removed or renamed — record where the work actually went, or re-open this item")
				}
			case parser.EventPromoted:
				// promoted names a FEATURE, and only a feature.
				var out refOutcome
				switch featureTargetResolves(cfg, becomes) {
				case refMissing:
					out = refOutcome{refMissing, "does not exist",
						"the feature was removed or renamed — a closed item is never revisited, so this permanently answers \"what did this become\" with something that is not there"}
				case refUnavailable:
					out = refOutcome{refUnavailable, "exists and cannot be read",
						"nobody has established this link is broken — repair the path, then re-check"}
				}
				reportTargetOutcome(add, "promoted into feature", becomes, out)
			case parser.EventAmended:
				// amended names an AMENDMENT, and only an amendment.
				reportTargetOutcome(add, "amended into", becomes,
					amendmentTargetResolves(cfg, becomes, it.ID))
			}
		}

		if it.State() != parser.StateOpen {
			continue
		}
		captured, err := time.Parse(time.RFC3339, it.Captured.At)
		if err != nil {
			// An unparseable timestamp is already reported by the
			// single-file validator as backlog-timestamp-not-rfc3339.
			// Reporting it a second time here as an age problem would
			// name the same fault twice under a code that does not
			// describe it.
			continue
		}
		if age := now.Sub(captured); age >= staleAfter {
			days := int(age.Hours() / 24)
			msg := fmt.Sprintf("open and undecided for %d days", days)
			if n := len(it.Deferrals()); n > 0 {
				msg += fmt.Sprintf(" across %d recorded deferral(s)", n)
			}
			add(CodeBacklogItemStale, msg,
				"decide it, or decline it — a deferral records that somebody looked, and does not reset the clock")
		}
	}
	return out
}

// refOutcome is a resolution WITH its reason and its remedy.
//
// Carried together because they have to agree. Trigger drift used to
// return plain refMissing and then collect the generic dangling fix,
// "the amendment was removed or renamed" — directly contradicting its
// own message, which said the amendment exists and names another item.
// A finding whose two halves disagree is worse than no finding: the
// reader has to work out which half to believe.
type refOutcome struct {
	resolution refResolution
	reason     string
	fix        string
}

// THE EVENT KIND DECIDES WHICH OBJECT IS LOOKED FOR.
//
// `promoted` names a feature; `amended` names an amendment. These were
// one function that tried the whole ref as a feature first and fell back
// to an amendment, which let the WRONG OBJECT mask a dangling ref: an
// item recorded as amended into `@widget/001-filter` resolved clean
// whenever an initiative-qualified feature happened to be named
// `widget/001-filter`, whether or not the amendment existed. Two events
// that are not ambiguous should not share an ambiguous lookup.

// featureTargetResolves resolves a `promoted` target: a feature ref,
// `@feature` or `@initiative/feature`, and nothing else.
func featureTargetResolves(cfg *config.Context, ref string) refResolution {
	body := strings.TrimPrefix(strings.TrimSpace(ref), "@")
	if body == "" {
		return refMissing
	}
	return exists(cfg.FeaturePath(body))
}

// amendmentTargetResolves resolves an `amended` target: `@feature/NNN-slug`,
// live or compacted into archive/. It returns the phrase describing what
// is wrong, so the finding says which of several things happened.
//
// EXISTENCE IS NOT ENOUGH. os.Stat proves a file is there, not that it
// is a readable amendment — and a record that will not parse cannot be
// what an item became, any more than a deleted one can. The trigger is
// checked too: it was required to equal `backlog:<id>` at the moment the
// item was closed, so a trigger that no longer names it is precisely the
// post-mutation drift these cross-file checks exist to catch.
//
// Parse and I/O failures are UNAVAILABLE, never missing. Something is
// there and nobody could read it, which is not a finding of absence.
func amendmentTargetResolves(cfg *config.Context, ref, itemID string) refOutcome {
	body := strings.TrimPrefix(strings.TrimSpace(ref), "@")
	idx := strings.LastIndex(body, "/")
	if idx <= 0 || idx == len(body)-1 {
		return refOutcome{refMissing, "is not an amendment ref (@feature/NNN-slug)",
			"the recorded ref is malformed — this needs provenance repair, not an edit to either record"}
	}
	feature, slug := body[:idx], body[idx+1:]
	dir := parser.AmendmentsDir(cfg.FeaturePath(feature))

	// BOTH candidates are examined, never first-success.
	//
	// A record present in the live directory AND in archive/ is split
	// history, which the canonical ledger treats as an integrity fault.
	// Returning on the first one that parses would let a valid archived
	// copy silently mask an unreadable or conflicting live duplicate —
	// resolving clean against exactly the state that is wrong.
	type found struct {
		path      string
		amendment *parser.Amendment
	}
	var ok []found
	var unreadable []string
	for _, candidate := range []string{
		filepath.Join(dir, slug+".md"),
		// Compaction moves applied records here. Retained ledger
		// history, not a deletion.
		filepath.Join(dir, "archive", slug+".md"),
	} {
		content, err := os.ReadFile(candidate)
		if err != nil {
			if !os.IsNotExist(err) {
				unreadable = append(unreadable, fmt.Sprintf("%s (%v)", filepath.Base(candidate), err))
			}
			continue
		}
		amendment, perr := parser.ParseAmendmentBytes(candidate, content)
		if perr != nil {
			unreadable = append(unreadable, fmt.Sprintf("%s (%v)", filepath.Base(candidate), perr))
			continue
		}
		ok = append(ok, found{candidate, amendment})
	}

	if len(ok)+len(unreadable) > 1 {
		return refOutcome{refUnavailable,
			"exists in both the live and archived ledger, which is split history",
			"compaction moves a record, it does not copy one — reconcile the duplicate before this provenance link can be trusted"}
	}
	if len(unreadable) == 1 {
		return refOutcome{refUnavailable,
			"exists and cannot be read: " + unreadable[0],
			"nobody has established this link is broken — repair or restore the record, then re-check"}
	}
	if len(ok) == 0 {
		return refOutcome{refMissing, "does not exist",
			"the amendment was removed or renamed — a closed item is never revisited, so this permanently answers \"what did this become\" with something that is not there"}
	}

	want := "backlog:" + itemID
	if got := strings.TrimSpace(ok[0].amendment.Trigger); got != want {
		return refOutcome{refMissing,
			fmt.Sprintf("exists but no longer names this item (`trigger: %s`)", got),
			"the causal link changed after the item was closed. Amendments and backlog history are both append-only, so do not edit either to make them agree — this needs provenance repair with the authority that closing carried"}
	}
	return refOutcome{refResolved, "", ""}
}

// exists separates "not there" from "could not look".
//
// os.Stat's error was being read as absence for every failure, so a
// permissions change or an I/O fault became a confident claim that the
// target had been deleted.
func exists(path string) refResolution {
	if _, err := os.Stat(path); err == nil {
		return refResolved
	} else if os.IsNotExist(err) {
		return refMissing
	}
	return refUnavailable
}

// reportTargetOutcome turns a resolution into the RIGHT finding.
//
// Missing accuses the record: the target is gone. Unavailable accuses
// nothing and reports that nobody could look — which must still be
// reported, because a fault that produces no output is indistinguishable
// from a healthy project. Each carries the fix that belongs to it,
// rather than one generic remedy that contradicts half the messages it
// is attached to.
func reportTargetOutcome(add func(code, msg, fix string), verb, ref string, out refOutcome) {
	switch out.resolution {
	case refMissing:
		add(CodeBacklogPromotionDangling, fmt.Sprintf("%s %q, which %s", verb, ref, out.reason), out.fix)
	case refUnavailable:
		add(CodeBacklogPromotionTargetUnavailable, fmt.Sprintf("%s %q, which %s", verb, ref, out.reason), out.fix)
	}
}
