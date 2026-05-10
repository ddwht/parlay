// parlay-feature: parlay-tool/multi-adapter
// parlay-component: coverage-review-authoring
//
// Walks suites in testcases.yaml, records approvals, collects exemptions,
// computes canonical-form hashes of buildfile.yaml + testcases.yaml, and
// writes .parlay/build/<feature>/coverage-review.yaml.

package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var reviewCoverageCmd = &cobra.Command{
	Use:   "review-coverage <@feature>",
	Short: "Walk suites, record approvals, write coverage-review.yaml",
	Args:  cobra.ExactArgs(1),
	RunE:  runReviewCoverage,
}

func runReviewCoverage(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])
	buildDir := cfg.BuildPath(slug)
	bfPath := filepath.Join(buildDir, "buildfile.yaml")
	tcPath := filepath.Join(buildDir, "testcases.yaml")

	bfContent, err := os.ReadFile(bfPath)
	if err != nil {
		return fmt.Errorf("read buildfile: %w", err)
	}
	tcContent, err := os.ReadFile(tcPath)
	if err != nil {
		return fmt.Errorf("read testcases: %w", err)
	}

	bfHash, err := agent.CanonicalFormHash(bfContent)
	if err != nil {
		return fmt.Errorf("hash buildfile: %w", err)
	}
	tcHash, err := agent.CanonicalFormHash(tcContent)
	if err != nil {
		return fmt.Errorf("hash testcases: %w", err)
	}

	var tc struct {
		Suites []struct {
			Name string `yaml:"name"`
		} `yaml:"suites"`
	}
	if err := yaml.Unmarshal(tcContent, &tc); err != nil {
		return fmt.Errorf("parse testcases: %w", err)
	}

	reader := bufio.NewReader(os.Stdin)
	var approved []string
	var exemptions []parser.CoverageExemption

	fmt.Fprintf(cmd.OutOrStdout(), "Reviewing coverage for %s — %d suites\n", slug, len(tc.Suites))
	for _, s := range tc.Suites {
		fmt.Fprintf(cmd.OutOrStdout(), "  approve %q? [Y/n] ", s.Name)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(strings.ToLower(line))
		if line == "" || line == "y" || line == "yes" {
			approved = append(approved, s.Name)
		} else {
			fmt.Fprint(cmd.OutOrStdout(), "    reason for exempting (free text): ")
			reason, _ := reader.ReadString('\n')
			exemptions = append(exemptions, parser.CoverageExemption{
				Suite:  s.Name,
				Item:   s.Name,
				Reason: strings.TrimSpace(reason),
			})
		}
	}

	cr := parser.CoverageReview{
		Feature:        slug,
		ReviewedAt:     time.Now().UTC().Format(time.RFC3339),
		ReviewedBy:     reviewerIdentity(),
		ReviewMethod:   "cli",
		BuildfileHash:  bfHash,
		TestcasesHash:  tcHash,
		ApprovedSuites: approved,
		Exemptions:     exemptions,
	}
	out, err := yaml.Marshal(&cr)
	if err != nil {
		return fmt.Errorf("marshal coverage-review: %w", err)
	}
	reviewPath := filepath.Join(buildDir, "coverage-review.yaml")
	if err := os.WriteFile(reviewPath, out, 0o644); err != nil {
		return fmt.Errorf("write coverage-review: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nWrote %s\n", reviewPath)
	fmt.Fprintf(cmd.OutOrStdout(), "  approved: %d / %d\n", len(approved), len(tc.Suites))
	fmt.Fprintf(cmd.OutOrStdout(), "  exemptions: %d\n", len(exemptions))
	return nil
}

func reviewerIdentity() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u := os.Getenv("LOGNAME"); u != "" {
		return u
	}
	return "cli"
}
