// parlay-feature: parlay-tool/intent-supersession
// parlay-component: active-specification-resolver

package commands

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ddwht/parlay/core/internal/parser"
)

var activeSpecCmd = &cobra.Command{
	Use:   "active-spec <@feature>",
	Short: "Report which founding promises a feature still makes, and which have been superseded (JSON)",
	Long: `Report a feature's current promise set.

A founding intent is frozen but not immutable in force: an amendment may record
that a later decision replaces it. This command answers what that leaves — which
promises stand, which were retired and by which amendment, and whether any
retirement has been recorded but not yet applied.

Refinement reads it before proposing a change, so a change that contradicts a
founding promise is recognized as a supersession rather than written as a
contradiction. It reads the resolved view; the frozen documents themselves are
never modified.`,
	Args: cobra.ExactArgs(1),
	RunE: runActiveSpec,
}

type activeSpecIntent struct {
	Slug   string   `json:"slug"`
	Title  string   `json:"title"`
	Goal   string   `json:"goal,omitempty"`
	Verify []string `json:"verify,omitempty"`
}

type activeSpecRetired struct {
	activeSpecIntent
	ByAmendment string `json:"by_amendment"`
	Seq         int    `json:"seq"`
}

type activeSpecPending struct {
	Intent      string `json:"intent"`
	ByAmendment string `json:"by_amendment"`
	Seq         int    `json:"seq"`
}

type activeSpecOutput struct {
	Feature string             `json:"feature"`
	Active  []activeSpecIntent `json:"active"`
	// Retired carries the whole intent, not just its slug: the point of
	// supersession is that the promise stays readable as history, and a
	// consumer that only learned the slug could not show what was given up.
	Retired []activeSpecRetired `json:"retired"`
	// Pending is non-empty when a retirement is recorded but not applied. The
	// promise is still IN FORCE while this is so — the artifacts and code still
	// make it — and every advancing boundary refuses to pass.
	Pending []activeSpecPending `json:"pending"`
	Blocked bool                `json:"blocked"`
}

func runActiveSpec(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])

	res, err := resolveActiveIntents(cfg, slug)
	if err != nil {
		return fmt.Errorf("read intents for %s: %w", slug, err)
	}

	out := activeSpecOutput{
		Feature: slug,
		Active:  []activeSpecIntent{},
		Retired: []activeSpecRetired{},
		Pending: []activeSpecPending{},
		Blocked: res.HasPending(),
	}
	for _, in := range res.Active {
		out.Active = append(out.Active, activeSpecIntent{Slug: in.Slug, Title: in.Title, Goal: in.Goal, Verify: in.Verify})
	}
	for _, s := range res.Superseded {
		out.Retired = append(out.Retired, activeSpecRetired{
			activeSpecIntent: activeSpecIntent{Slug: s.Intent.Slug, Title: s.Intent.Title, Goal: s.Intent.Goal, Verify: s.Intent.Verify},
			ByAmendment:      s.ByAmendment,
			Seq:              s.Seq,
		})
	}
	for _, p := range res.Pending {
		out.Pending = append(out.Pending, activeSpecPending{Intent: p.Intent, ByAmendment: p.ByAmendment, Seq: p.Seq})
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
