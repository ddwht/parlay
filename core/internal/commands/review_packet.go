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

	// Subject is what is being decided ABOUT, and it comes first.
	Subject SubjectBlock `json:"subject"`
	// History is context on how it was decided before. It comes second, and it
	// is labelled as history so it cannot be mistaken for the thing under
	// review.
	History HistoryBlock `json:"history"`

	// Question is the fixed three-way choice. It carries no default and no
	// recommendation: this is the point of the whole exercise.
	Question QuestionBlock `json:"question"`
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

type QuestionBlock struct {
	Prompt   string   `json:"prompt"`
	Outcomes []string `json:"outcomes"`
	// RationaleRequired restates that the reviewer must supply their own words.
	RationaleRequired bool `json:"rationale_required"`
	// NoDefault is emitted so a consumer cannot quietly pick one.
	NoDefault bool `json:"no_default"`
}

const historyLabel = "PRIOR DECISION — CONTEXT ONLY, NOT WHAT YOU ARE DECIDING"

// reviewOutcomes is the closed set. Exactly three, in a fixed order, with no
// element marked preferred.
var reviewOutcomes = []string{
	"still-holds",
	"no-longer-holds",
	"cannot-say",
}

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

	prompt := fmt.Sprintf("Does this waiver still hold for %s?", occ.Ref)
	if entryWide && len(reqs) > 1 {
		// Breadth is surfaced in the question itself, not only in the subject
		// block, because it is the thing a reviewer is most likely to grant
		// without noticing they granted it.
		prompt = fmt.Sprintf("Does this waiver still hold for ALL %d requirements now on %s?", len(reqs), occ.Ref)
	}

	return &ReviewPacket{
		Label: occ.Label, Fingerprint: occ.Fingerprint, Duplicate: occ.Duplicate,
		Subject: SubjectBlock{
			Ref: occ.Ref, Requirements: reqs, EntryWide: entryWide,
			Cases: cases, NothingCovers: len(cases) == 0,
		},
		History: HistoryBlock{
			Label: historyLabel, Reason: strings.TrimSpace(occ.GrantedBecause),
			Attempts: attempts,
		},
		Question: QuestionBlock{
			Prompt: prompt, Outcomes: reviewOutcomes,
			RationaleRequired: true, NoDefault: true,
		},
	}, nil
}

// readTestcases returns the feature's testcases content, or nil when there are
// none. Absence is not an error here: a legacy waiver frequently predates any
// testcases at all, and refusing to build the packet would make the entries
// most in need of review the ones that cannot be reviewed.
func readTestcases(cfg *config.Context, slug string) ([]byte, error) {
	path := filepath.Join(cfg.BuildPath(slug), "testcases.yaml")
	data, err := os.ReadFile(path)
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
