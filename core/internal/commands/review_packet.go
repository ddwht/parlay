package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
)

// A review packet is everything a person needs to answer one stranded
// judgment, assembled in the order they must see it.
//
// Ordering could have been left to the skill's prose — "render the subject,
// then the history". A lint could then check only that those two phrases appear
// in that order in an instruction file, which the agent can satisfy while
// rendering them the other way round. That check would read as enforcement
// while proving almost nothing, and for a flow whose entire purpose is that a
// person forms a real judgment, that is the wrong trade.
//
// So the packet is built here, ordered here, and tested here. The skill
// presents it and asks the question; it does not decide what the reviewer sees
// first.
type ReviewPacket struct {
	Label       string `json:"label"`
	Fingerprint string `json:"fingerprint"`
	Duplicate   int    `json:"duplicate"`

	// Display is the evidence, rendered, subject first and history second. The
	// skill presents it verbatim and does not assemble its own.
	Display string `json:"display"`

	// Decision is the interaction contract: the question and the closed set of
	// options, with no default and no recommendation. The skill maps it
	// straight into the chooser.
	//
	// Evidence and interaction are separated deliberately. Folding the options
	// into the rendered prose would show the reviewer the same three choices
	// twice — once in the packet, once in the chooser — and would make the
	// formatter responsible for that duplication while inviting later callers
	// to parse choices back out of prose.
	Decision DecisionContract `json:"decision"`

	// The structured evidence Display is built from. Deliberately absent from
	// the JSON: a consumer that could read these could render its own packet,
	// and two renderings of one occurrence is exactly the divergence Display
	// exists to prevent.
	subject SubjectBlock
	history HistoryBlock
}

// DecisionContract is what the chooser is built from.
type DecisionContract struct {
	Question string         `json:"question"`
	Options  []ReviewOption `json:"options"`
	// RationaleRequired: the reviewer supplies their own words AFTER choosing.
	// It is a second authority-bearing question, never sourced from an option
	// description — a description the reviewer merely accepted is not a reason
	// they gave.
	RationaleRequired bool `json:"rationale_required"`
	// NoDefault is stated rather than implied by absence, so a consumer cannot
	// treat "no default given" as licence to pick one.
	NoDefault bool `json:"no_default"`
}

// ReviewOption is one outcome. ID is stable and maps to the command that
// records it; Label and Description are what a person reads.
type ReviewOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type SubjectBlock struct {
	Ref string `json:"ref"`
	// Requirements is what the contract says TODAY. A legacy entry naming only
	// a ref may now cover several — re-confirming it would grant a waiver over
	// all of them, so all of them are shown.
	Requirements []string `json:"requirements"`
	// EntryWide marks the shape above, where breadth is the thing most likely
	// to be granted by accident.
	EntryWide bool `json:"entry_wide"`
	// Cases are what currently observes those requirements.
	Cases []string `json:"cases"`
	// NothingCovers is stated explicitly rather than implied by an empty list.
	// A waiver frequently exists PRECISELY because nothing covers the
	// requirement, so silence here would be read as "no information" when it is
	// in fact the most important fact on the page.
	NothingCovers bool `json:"nothing_covers"`
}

type HistoryBlock struct {
	// Label is fixed text. It is not advisory phrasing the caller may soften.
	Label  string `json:"label"`
	Reason string `json:"reason"`
	Suite  string `json:"suite,omitempty"`
	// Attempts are earlier reviewers who could not decide.
	Attempts []string `json:"prior_attempts,omitempty"`
}

const historyLabel = "PRIOR DECISION — CONTEXT ONLY, NOT WHAT YOU ARE DECIDING"

// Outcome IDs are stable and map to the command that records each. They are
// not display text: a person reads the label.
const (
	OutcomeReconfirm = "reconfirm"
	OutcomeDrop      = "drop"
	OutcomeDefer     = "defer"
)

// reviewOutcomeIDs is the closed set. Exactly three, fixed order, none preferred.
var reviewOutcomeIDs = []string{OutcomeReconfirm, OutcomeDrop, OutcomeDefer}

// BuildReviewPacket assembles one occurrence for review.
func BuildReviewPacket(cfg *config.Context, slug string, occ MigrationOccurrence) (*ReviewPacket, error) {
	current, err := CurrentCriteria(cfg, slug)
	if err != nil {
		return nil, fmt.Errorf("read the requirements this decision would be about: %w", err)
	}

	wantText := agent.CanonicalCriterionText(occ.Text)
	entryWide := strings.TrimSpace(occ.Text) == ""

	var reqs []string
	for _, c := range current {
		if c.Ref != occ.Ref {
			continue
		}
		if entryWide || agent.CanonicalCriterionText(c.Text) == wantText {
			reqs = append(reqs, c.Text)
		}
	}

	cases, cErr := casesObserving(cfg, slug, occ.Ref, reqs)
	if cErr != nil {
		return nil, cErr
	}

	var attempts []string
	for _, a := range occ.Attempts {
		attempts = append(attempts, fmt.Sprintf("%s (%s): %s", a.By, a.At, strings.TrimSpace(a.Reason)))
	}

	subject := SubjectBlock{
		Ref: occ.Ref, Requirements: reqs, EntryWide: entryWide,
		Cases: cases, NothingCovers: len(cases) == 0,
	}
	history := HistoryBlock{
		Label: historyLabel, Reason: strings.TrimSpace(occ.GrantedBecause),
		Attempts: attempts,
	}

	question := fmt.Sprintf("Does this waiver still hold for %s?", occ.Ref)
	reconfirmDesc := "The requirement still genuinely needs no test. You will write your own reason."
	if entryWide && len(reqs) > 1 {
		// Breadth belongs in the decision, not only in the evidence. This is
		// the thing a reviewer is most likely to grant without noticing they
		// granted it, so it appears where they are actually choosing.
		question = fmt.Sprintf("Does this waiver still hold for ALL %d requirements now on %s?", len(reqs), occ.Ref)
		reconfirmDesc = fmt.Sprintf("Waives ALL %d requirements listed above, not just one.", len(reqs))
	}

	return &ReviewPacket{
		Label: occ.Label, Fingerprint: occ.Fingerprint, Duplicate: occ.Duplicate,
		Display: renderReviewDisplay(subject, history),
		Decision: DecisionContract{
			Question: question,
			Options: []ReviewOption{
				{ID: OutcomeReconfirm, Label: "Still holds", Description: reconfirmDesc},
				{ID: OutcomeDrop, Label: "No longer holds", Description: "Withdraw it. The requirement goes back to needing a test."},
				{ID: OutcomeDefer, Label: "I cannot say", Description: "Records that you looked. Does NOT answer it; the boundary keeps reporting it."},
			},
			RationaleRequired: true, NoDefault: true,
		},
		subject: subject, history: history,
	}, nil
}

// renderReviewDisplay writes the evidence in the order it must be read.
//
// Subject first is the whole point: the reviewer decides about what the
// contract says NOW, and the previous reasoning is context they weigh, not the
// thing under review. Rendering here rather than instructing a caller to render
// in this order is what makes the ordering a testable property of an artifact
// instead of a promise about prose.
func renderReviewDisplay(s SubjectBlock, h HistoryBlock) string {
	var b strings.Builder

	fmt.Fprintf(&b, "## What you are deciding about\n\n%s\n\n", s.Ref)
	if s.EntryWide && len(s.Requirements) > 1 {
		fmt.Fprintf(&b, "This waiver covers ALL %d requirements below.\n\n", len(s.Requirements))
	}
	b.WriteString("**Requirements as they read today**\n\n")
	if len(s.Requirements) == 0 {
		b.WriteString("- (the contract no longer declares any requirement here)\n")
	}
	for _, r := range s.Requirements {
		fmt.Fprintf(&b, "- %s\n", r)
	}

	b.WriteString("\n**What currently observes them**\n\n")
	if s.NothingCovers {
		// Stated, never left as an empty list. This is frequently the reason
		// the waiver exists, so rendering silence would hide the most
		// important fact on the page.
		b.WriteString("- NOTHING covers this today. If you re-confirm, it stays untested.\n")
	}
	for _, c := range s.Cases {
		fmt.Fprintf(&b, "- %s\n", c)
	}

	fmt.Fprintf(&b, "\n## %s\n\n", h.Label)
	if h.Reason != "" {
		fmt.Fprintf(&b, "It was originally waived because: %s\n", h.Reason)
	}
	for _, a := range h.Attempts {
		fmt.Fprintf(&b, "\n- Earlier review, no decision reached — %s\n", a)
	}
	return b.String()
}

// readTestcases returns the feature's testcases content, or nil when there are
// none. Absence is not an error: a legacy waiver frequently predates any
// testcases at all, and refusing to build the packet would make the entries
// most in need of review the ones that cannot be reviewed.
func readTestcases(cfg *config.Context, slug string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(cfg.BuildPath(slug), "testcases.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read testcases: %w", err)
	}
	return data, nil
}

// casesObserving reports the cases that currently discharge any of reqs on ref.
func casesObserving(cfg *config.Context, slug, ref string, reqs []string) ([]string, error) {
	content, err := readTestcases(cfg, slug)
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, nil
	}
	resolved, err := resolveCases(content)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, r := range reqs {
		want[agent.CanonicalCriterionText(r)] = true
	}
	var out []string
	for _, c := range resolved {
		if c.Ref != ref {
			continue
		}
		if len(want) > 0 && !want[agent.CanonicalCriterionText(c.Text)] {
			continue
		}
		note := c.Coverage
		if note == "" {
			note = "unmarked"
		}
		out = append(out, fmt.Sprintf("suite %q case %q (%s)", c.Suite, c.Name, note))
	}
	return out, nil
}
