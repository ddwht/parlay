// parlay-feature: annotations
// parlay-component: collect-annotations-probe
//
// `parlay internal collect-annotations` — the probe. It finds and reports; it
// makes no judgement about what a thread should become. That split is the
// project's shape: a CLI that emits JSON, and a skill that decides and acts.
//
// Mirrors collect-questions in placement and shape, for the same consumers.

package commands

import (
	"fmt"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
)

var collectAnnotationsAll bool

var collectAnnotationsCmd = &cobra.Command{
	Use:   "collect-annotations [@feature]",
	Short: "Collect anchored review comments and their threads (JSON output for agent consumption)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runCollectAnnotations,
}

func init() {
	collectAnnotationsCmd.Flags().BoolVar(&collectAnnotationsAll, "all", false,
		"also scan project-level files (root domain model, blueprint, adapters, adapter-set)")
}

type annotationsOutput struct {
	Feature  string                     `json:"feature,omitempty"`
	Threads  []parser.AnnotationThread  `json:"threads"`
	Findings []parser.AnnotationFinding `json:"findings"`
	Counts   annotationCounts           `json:"counts"`
	Features []annotationsOutput        `json:"features,omitempty"`
	// Errors names features whose scan failed. A probe that silently omits
	// them would report "no threads" for a feature nobody could read.
	Errors []string `json:"errors,omitempty"`
}

func runCollectAnnotations(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}

	if len(args) == 1 {
		out, err := annotationsForFeature(cfg, parser.FeatureSlug(args[0]))
		if err != nil {
			return err
		}
		return printJSON(cmd, out)
	}

	features, err := cfg.AllFeatures()
	if err != nil {
		return fmt.Errorf("cannot enumerate features: %w", err)
	}
	all := annotationsOutput{Threads: []parser.AnnotationThread{}, Findings: []parser.AnnotationFinding{}}
	for _, slug := range features {
		out, err := annotationsForFeature(cfg, slug)
		if err != nil {
			// collect-questions omits a feature it cannot read; this probe
			// must not. Its answer gates a build boundary, so a feature that
			// failed to scan has to be visibly different from one with
			// nothing to report.
			all.Errors = append(all.Errors, fmt.Sprintf("%s: %v", slug, err))
			continue
		}
		if out.Counts.total() == 0 && len(out.Findings) == 0 {
			continue
		}
		all.Features = append(all.Features, *out)
		all.Counts.Open += out.Counts.Open
		all.Counts.Answered += out.Counts.Answered
		all.Counts.Closed += out.Counts.Closed
	}

	if collectAnnotationsAll {
		project, err := collectProjectAnnotations(cfg)
		if err != nil {
			return err
		}
		for _, scan := range project {
			all.Threads = append(all.Threads, scan.Threads...)
			all.Findings = append(all.Findings, scan.Findings...)
		}
		counts := countAnnotations(project)
		all.Counts.Open += counts.Open
		all.Counts.Answered += counts.Answered
		all.Counts.Closed += counts.Closed
	}

	return printJSON(cmd, all)
}

func annotationsForFeature(cfg *config.Context, slug string) (*annotationsOutput, error) {
	scans, err := collectFeatureAnnotations(cfg, slug)
	if err != nil {
		return nil, err
	}
	refuseAnnotationsInAppliedRecords(scans)

	out := &annotationsOutput{
		Feature:  slug,
		Threads:  []parser.AnnotationThread{},
		Findings: []parser.AnnotationFinding{},
	}
	for _, scan := range scans {
		for _, thread := range scan.Threads {
			thread.Frozen = scan.Frozen
			out.Threads = append(out.Threads, thread)
		}
		out.Findings = append(out.Findings, scan.Findings...)
	}
	out.Counts = countAnnotations(scans)
	return out, nil
}
