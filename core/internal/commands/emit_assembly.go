// parlay-feature: parlay-tool/page-assembly-derivation
// parlay-component: assembly-emitter
//
// Serializes the assembly derivation, so the schema's "no author in the loop"
// stops being a claim about an LLM's discipline and becomes a fact.
//
// The emitter and the validator are the SAME derivation used two ways: this
// command calls DeriveAssemblySuite and writes what it returns; readiness
// calls it and diffs what is on disk against it. A separate implementation
// here would recreate, one level up, exactly the disagreement this work
// removes — a validator rejecting what the emitter writes.
package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/spf13/cobra"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/parser"
)

var (
	emitAssemblyJSON  bool
	emitAssemblyWrite bool
)

func init() {
	emitAssemblyCmd.Flags().BoolVar(&emitAssemblyJSON, "json", false, "emit machine-readable JSON")
	emitAssemblyCmd.Flags().BoolVar(&emitAssemblyWrite, "write", false,
		"merge the derived suites into testcases.yaml, replacing any existing origin: page-assembly suites")
}

var emitAssemblyCmd = &cobra.Command{
	Use:   "emit-assembly <@feature>",
	Short: "Emit the derived per-page assembly suites for a feature",
	Long: `Compute the per-page assembly suites this feature's composed pages require.

The suites are DERIVED, never authored: every declared component asserts it
reaches the renderer, every component carrying actions: asserts it is
hit-reachable, and every fragment marked interactive: false asserts it is NOT
hit-reachable. Nothing is invented and nothing cites a contract criterion — an
assembly assertion is a composition fact and discharges none.

Derivation runs over the RESOLVED composition: supersession is applied across
every surface in the project before the result is filtered to this feature, so
a fragment another feature has retired produces no assertions.

Assertions the presentation adapter cannot execute are reported as capability
debt rather than emitted as weakened cases.`,
	Args: cobra.ExactArgs(1),
	RunE: runEmitAssembly,
}

// emittedCase is a real testcases case, not an inventory row: it carries the
// derivation identity AND the executable steps, because those are what the
// validator diffs. An emitter that produced only identity left the mechanics
// to be invented downstream, which is the defect it exists to remove.
type emittedCase struct {
	Name       string               `yaml:"name" json:"name"`
	Derivation emittedDerivation    `yaml:"derivation" json:"derivation"`
	Exercises  []string             `yaml:"exercises" json:"exercises"`
	Observes   []string             `yaml:"observes" json:"observes"`
	Steps      []agent.AssemblyStep `yaml:"steps" json:"steps"`
}

type emittedDerivation struct {
	Kind      string `yaml:"kind" json:"kind"`
	Page      string `yaml:"page" json:"page"`
	Subject   string `yaml:"subject" json:"subject"`
	Assertion string `yaml:"assertion" json:"assertion"`
}

type emittedAssertion struct {
	Page      string `yaml:"page" json:"page"`
	Subject   string `yaml:"subject" json:"subject"`
	Assertion string `yaml:"assertion" json:"assertion"`
	Ref       string `yaml:"ref,omitempty" json:"ref,omitempty"`
	Needs     string `yaml:"needs_capability" json:"needs_capability"`
}

type emittedSuite struct {
	Name              string             `yaml:"name" json:"name"`
	Kind              string             `yaml:"kind" json:"kind"`
	Scope             string             `yaml:"scope" json:"scope"`
	Origin            string             `yaml:"origin" json:"origin"`
	Page              string             `yaml:"page" json:"page"`
	File              string             `yaml:"file" json:"file"`
	SourceRefs        []string           `yaml:"source_refs" json:"source_refs"`
	Cases             []emittedCase      `yaml:"cases" json:"cases"`
	PendingAssertions []emittedAssertion `yaml:"pending_assertions,omitempty" json:"pending_assertions,omitempty"`
}

type emitAssemblyOutput struct {
	Feature string         `json:"feature"`
	Suites  []emittedSuite `json:"suites"`
}

func runEmitAssembly(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])

	expected, blockers := expectedAssemblySuites(cfg, slug)
	if len(blockers) > 0 {
		cmd.SilenceUsage = true
		for _, b := range blockers {
			fmt.Fprintf(cmd.ErrOrStderr(), "  %s\n", b)
		}
		return fmt.Errorf("cannot derive assembly suites for %s", slug)
	}
	out := emitAssemblyOutput{Feature: slug}

	for _, page := range sortedAssemblyPages(expected) {
		out.Suites = append(out.Suites, buildEmittedSuite(page, expected[page]))
	}

	if emitAssemblyWrite {
		path := filepath.Join(cfg.BuildPath(slug), "testcases.yaml")
		if err := writeAssemblySuites(path, out.Suites); err != nil {
			cmd.SilenceUsage = true
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Wrote %d derived assembly suite(s) into %s.\n", len(out.Suites), path)
		return nil
	}

	if emitAssemblyJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	if len(out.Suites) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "%s contributes no active fragment to any page — no assembly suite is derived.\n", slug)
		return nil
	}
	for _, s := range out.Suites {
		fmt.Fprintf(cmd.OutOrStdout(), "%s (page %s, origin %s)\n", s.Name, s.Page, s.Origin)
		for _, c := range s.Cases {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s %s  (%d steps)\n", c.Derivation.Subject, c.Derivation.Assertion, len(c.Steps))
		}
		for _, p := range s.PendingAssertions {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s %s  — capability debt, needs %q\n", p.Subject, p.Assertion, p.Needs)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}
	return nil
}

func sortedAssemblyPages(m map[string]agent.AssemblySuite) []string {
	out := make([]string, 0, len(m))
	for p := range m {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// writeAssemblySuites merges the derived suites into testcases.yaml.
//
// This is what makes "no author in the loop" true rather than aspirational.
// Printing the suites and asking a model to transcribe them leaves the
// mechanics to be retyped, and a transcription error is exactly the class of
// defect the validator then has to catch — a correct label over a wrong test.
// Writing them deterministically removes the step where that can happen.
//
// Replace, not append: every `origin: page-assembly` suite is regenerated, so
// a suite for a page the feature no longer reaches disappears rather than
// lingering as an orphan. Authored suites are preserved untouched — this
// command owns exactly the suites it derives and nothing else.
func writeAssemblySuites(path string, suites []emittedSuite) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return fmt.Errorf("%s is not a YAML document", path)
	}
	root := doc.Content[0]

	var suitesNode *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "suites" {
			suitesNode = root.Content[i+1]
			break
		}
	}
	if suitesNode == nil {
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "suites"},
			&yaml.Node{Kind: yaml.SequenceNode})
		suitesNode = root.Content[len(root.Content)-1]
	}

	// Drop the suites this command owns; keep every authored one.
	kept := make([]*yaml.Node, 0, len(suitesNode.Content))
	for _, n := range suitesNode.Content {
		derived := false
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == "origin" && n.Content[i+1].Value == agent.AssemblyKindPageAssembly {
				derived = true
				break
			}
		}
		if !derived {
			kept = append(kept, n)
		}
	}

	for _, es := range suites {
		var node yaml.Node
		if err := node.Encode(es); err != nil {
			return fmt.Errorf("encode suite %s: %w", es.Name, err)
		}
		kept = append(kept, &node)
	}
	suitesNode.Content = kept

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("serialize %s: %w", path, err)
	}
	return os.WriteFile(path, out, 0o644)
}

// buildEmittedSuite turns one derived AssemblySuite into the serializable
// shape. Extracted so the emitter and the round-trip test build it the same
// way — a test that constructed its own would prove the test agrees with
// itself, not that the writer agrees with the validator.
func buildEmittedSuite(page string, suite agent.AssemblySuite) emittedSuite {
	es := emittedSuite{
		Name:   parser.Slugify(page) + "-page-assembly",
		Kind:   "presentation",
		Scope:  "route",
		Origin: agent.AssemblyKindPageAssembly,
		Page:   page,
		File:   fmt.Sprintf("src/%s.assembly.test", parser.Slugify(page)),
		Cases:  []emittedCase{},
	}

	// Every v2+ suite must cite at least one source_ref. For a derived suite
	// those are the fragments it was derived FROM — computed, like everything
	// else here, rather than chosen.
	seenRef := map[string]bool{}
	for _, a := range append(append([]agent.AssemblyAssertion{}, suite.Supported...), suite.Pending...) {
		if a.Ref != "" && !seenRef[a.Ref] {
			seenRef[a.Ref] = true
			es.SourceRefs = append(es.SourceRefs, a.Ref)
		}
	}
	sort.Strings(es.SourceRefs)

	for _, a := range suite.Supported {
		es.Cases = append(es.Cases, emittedCase{
			Name: fmt.Sprintf("%s %s", a.Subject, a.Kind),
			Derivation: emittedDerivation{
				Kind: agent.AssemblyKindPageAssembly, Page: a.Page,
				Subject: a.Subject, Assertion: a.Kind,
			},
			Exercises: []string{a.Subject},
			Observes:  []string{a.Subject},
			Steps:     a.Steps(),
		})
	}
	for _, a := range suite.Pending {
		es.PendingAssertions = append(es.PendingAssertions, emittedAssertion{
			Page: a.Page, Subject: a.Subject, Assertion: a.Kind, Ref: a.Ref,
			Needs: a.RequiredCapability,
		})
	}
	return es
}
