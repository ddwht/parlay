// parlay-feature: studio-support/page-layout-field
// parlay-cross-cutting-id: layout-precheck-contract
//
// The layout-validation precheck gate shared by view-page and lock-page:
// before either command assembles or locks a page, consult the precheck
// for any layout artifact already on disk for that page name, and refuse
// to proceed on a failing verdict — matching how generate-code.skill.md
// (step 11.7) describes precheck refusals for codegen.

package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
)

// layoutPrecheckGate resolves and runs the layout-validation precheck for
// pageName before view-page/lock-page do their (legacy, fragment-based)
// assembly. A page with no layout artifact at all — neither an embedded
// ## Layout section in an existing page manifest nor a standalone
// *.layout.yaml — returns Verdict{Code: "ok"}: there is nothing to gate,
// and the existing region-based flow proceeds exactly as it did before
// this feature landed. The embedded form wins over the standalone form
// when both exist for the same page, per page.schema.md's "Precedence
// when a per-feature layout also exists" rule.
func layoutPrecheckGate(cfg *config.Context, pageName string) agent.Verdict {
	manifestPath := filepath.Join(cfg.PagesPath(), pageName+".page.md")
	if _, err := os.Stat(manifestPath); err == nil {
		page, err := parser.ParsePageFile(manifestPath)
		if err != nil {
			// Parse failure is adapter-independent — every branch
			// parseErrorToLayoutViolation covers (missing schema_version,
			// wiring, raw spacing, malformed block) doesn't need a
			// resolved adapter.
			return agent.LayoutParseErrorVerdict(manifestPath, err, nil)
		}
		if page.Layout != nil {
			adapter := resolveAgentAdapterForVocabulary(cfg, page.Layout.ComponentVocabulary)
			return agent.LayoutPrecheck(page, adapter)
		}
		// Page manifest exists but has no embedded Layout section — fall
		// through to the standalone-file check below.
	}

	layoutPath := filepath.Join(cfg.PagesPath(), pageName+".layout.yaml")
	if _, err := os.Stat(layoutPath); err == nil {
		layout, err := loadStandaloneLayout(layoutPath)
		if err != nil {
			// loadStandaloneLayout's error already came from parsing
			// layoutPath's own content (via a synthetic wrapper) —
			// translate it directly rather than re-parsing layoutPath as
			// if it were itself a page artifact (it isn't one, and
			// ParsePagePrecheck re-parsing it would either fail
			// differently or silently succeed as a layout-free page).
			return agent.LayoutParseErrorVerdict(layoutPath, err, nil)
		}
		adapter := resolveAgentAdapterForVocabulary(cfg, layout.ComponentVocabulary)
		return agent.LayoutPrecheck(&parser.Page{Name: pageName, Layout: layout}, adapter)
	}

	return agent.Verdict{Code: "ok"}
}

// resolveAgentAdapterForVocabulary scans the active root's registered
// adapters (.parlay/adapters/*.adapter.yaml) for one whose
// componentVocabulary name matches vocabName. Returns nil when none
// matches (or none is registered) — agent.LayoutPrecheck degrades
// gracefully with a nil adapter, skipping adapter-dependent checks
// rather than failing on an unrelated lookup problem.
func resolveAgentAdapterForVocabulary(cfg *config.Context, vocabName string) *agent.Adapter {
	if vocabName == "" {
		return nil
	}
	entries, err := os.ReadDir(cfg.AdaptersPath())
	if err != nil {
		return nil
	}
	const suffix = ".adapter.yaml"
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || len(name) <= len(suffix) || name[len(name)-len(suffix):] != suffix {
			continue
		}
		a, err := agent.LoadAdapterFile(filepath.Join(cfg.AdaptersPath(), name))
		if err != nil || a.ComponentVocabulary == nil {
			continue
		}
		if a.ComponentVocabulary.Name == vocabName {
			return a
		}
	}
	return nil
}

// refuseOnPrecheckVerdict surfaces a failing layout-validation precheck
// verdict verbatim — matching how generate-code.skill.md (step 11.7)
// describes precheck refusals: surface the precheck's message as-is, do
// not augment it with command-internal vocabulary, and refuse to
// proceed. The field labels below only structure the verdict's own
// fields for display; every value printed is the verdict's, untouched.
func refuseOnPrecheckVerdict(cmd *cobra.Command, verdict agent.Verdict) error {
	fmt.Fprintf(cmd.ErrOrStderr(), "precheck-refusal: layout validation failed for %s\n", verdict.File)
	fmt.Fprintf(cmd.ErrOrStderr(), "  [%s] found: %s\n", verdict.Code, verdict.Found)
	if verdict.Expected != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "  expected: %s\n", verdict.Expected)
	}
	if verdict.NodePath != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "  at: %s\n", verdict.NodePath)
	}
	if verdict.Fix != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "  fix: %s\n", verdict.Fix)
	}
	return NewExitCodeError(1)
}
