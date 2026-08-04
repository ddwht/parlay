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

// reviewCoverageExempt collects repeated --exempt flags.
//
// The command had no flags at all, so the only way to exempt a term was to
// sit through the interactive walk and decline the suite — which meant
// there was no way to do it from CI, from a script, or from an agent, and
// no way to exempt a single term of a suite whose other terms are fine.
var reviewCoverageExempt []string

func init() {
	reviewCoverageCmd.Flags().StringArrayVar(&reviewCoverageExempt, "exempt", nil,
		"Pre-record an exemption as <suite>:<item>=<reason>; repeatable. Suites left with no unexempted term are not prompted for.")
}

// parsedExemption is one --exempt value.
type parsedExemption struct {
	Suite  string
	Item   string
	Reason string
}

// parseExemptFlags parses <suite>:<item>=<reason>.
//
// Split on the FIRST "=" and the first ":" before it: a reason is free
// text and routinely contains both characters ("covered by engine: see
// ADR-4"), while a suite id and an item id contain neither by their own
// vocabularies. Splitting on the last separator instead would silently
// truncate reasons.
func parseExemptFlags(values []string) ([]parsedExemption, error) {
	var out []parsedExemption
	for _, v := range values {
		eq := strings.Index(v, "=")
		if eq < 0 {
			return nil, fmt.Errorf("--exempt %q: missing =<reason>; the form is <suite>:<item>=<reason>, and an exemption without a reason is not reviewable", v)
		}
		lhs, reason := v[:eq], strings.TrimSpace(v[eq+1:])
		colon := strings.Index(lhs, ":")
		if colon < 0 {
			return nil, fmt.Errorf("--exempt %q: missing :<item>; the form is <suite>:<item>=<reason>", v)
		}
		suite := strings.TrimSpace(lhs[:colon])
		item := strings.TrimSpace(lhs[colon+1:])
		if suite == "" || item == "" || reason == "" {
			return nil, fmt.Errorf("--exempt %q: suite, item and reason must all be non-empty", v)
		}
		out = append(out, parsedExemption{Suite: suite, Item: item, Reason: reason})
	}
	return out, nil
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

	// source_refs is read as well as name, because an exemption is keyed
	// on the TERM a suite was supposed to cover, not on the suite. See the
	// exemption loop below.
	var tc struct {
		Suites []struct {
			Name       string   `yaml:"name"`
			SourceRefs []string `yaml:"source_refs"`
		} `yaml:"suites"`
	}
	if err := yaml.Unmarshal(tcContent, &tc); err != nil {
		return fmt.Errorf("parse testcases: %w", err)
	}

	preExempt, err := parseExemptFlags(reviewCoverageExempt)
	if err != nil {
		return err
	}
	// Terms exempted up front, keyed suite -> item, so the walk can skip a
	// suite whose every term is already accounted for rather than asking
	// the reviewer to re-decide something they passed on the command line.
	exemptedTerms := map[string]map[string]bool{}
	for _, e := range preExempt {
		if exemptedTerms[e.Suite] == nil {
			exemptedTerms[e.Suite] = map[string]bool{}
		}
		exemptedTerms[e.Suite][e.Item] = true
	}

	reader := bufio.NewReader(os.Stdin)
	var approved []string
	var exemptions []parser.CoverageExemption

	for _, e := range preExempt {
		exemptions = append(exemptions, parser.CoverageExemption{
			Suite: e.Suite, Item: e.Item, Reason: e.Reason,
		})
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Reviewing coverage for %s — %d suites\n", slug, len(tc.Suites))
	for _, s := range tc.Suites {
		if suiteFullyExempted(s.Name, s.SourceRefs, exemptedTerms) {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s — exempted on the command line, not prompted\n", s.Name)
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  approve %q? [Y/n] ", s.Name)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(strings.ToLower(line))
		if line == "" || line == "y" || line == "yes" {
			approved = append(approved, s.Name)
		} else {
			fmt.Fprint(cmd.OutOrStdout(), "    reason for exempting (free text): ")
			reason, _ := reader.ReadString('\n')
			exemptions = append(exemptions, exemptionsForSuite(s.Name, s.SourceRefs, strings.TrimSpace(reason))...)
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

// exemptionsForSuite turns "the reviewer declined this suite" into the
// exemption records the gate can actually consult — one per term the suite
// was supposed to cover.
//
// This used to record a single entry with Item set to the SUITE name. The
// validator keys `exempted` on the covered term (an operation id, an error
// code) and compares it against RequiredCoverage, which never contains a
// suite name — so a CLI-recorded exemption could not satisfy
// coverage-review-uncovered under any circumstances. The reviewer answered
// the prompt, the file recorded their answer, and the gate went on
// reporting the term as uncovered with no way to discharge it.
//
// A suite with no source_refs falls back to the suite name, which is no
// worse than the old behaviour and keeps legacy v1 testcases reviewable:
// those carry no source_refs at all, so there is no term to key on.
func exemptionsForSuite(suiteName string, sourceRefs []string, reason string) []parser.CoverageExemption {
	terms := sourceRefs
	if len(terms) == 0 {
		terms = []string{suiteName}
	}
	out := make([]parser.CoverageExemption, 0, len(terms))
	for _, term := range terms {
		out = append(out, parser.CoverageExemption{
			Suite:  suiteName,
			Item:   term,
			Reason: reason,
		})
	}
	return out
}

// suiteFullyExempted reports whether every term a suite covers already has
// a command-line exemption.
//
// Every term, not any: a suite covering three operations where one was
// exempted still needs a decision about the other two, and skipping the
// prompt would silently leave them unapproved. A suite with no source_refs
// is matched on its own name, the same fallback exemptionsForSuite uses.
func suiteFullyExempted(suiteName string, sourceRefs []string, exempted map[string]map[string]bool) bool {
	byItem := exempted[suiteName]
	if len(byItem) == 0 {
		return false
	}
	terms := sourceRefs
	if len(terms) == 0 {
		terms = []string{suiteName}
	}
	for _, t := range terms {
		if !byItem[t] {
			return false
		}
	}
	return true
}
