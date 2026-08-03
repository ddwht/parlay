// parlay-feature: parlay-tool/multi-adapter
// parlay-component: pre-codegen-support-gate-failure
//
// Pre-codegen support gate. Walks every operation in a feature's
// capabilities.yaml against each non-presentation adapter's supports:
// block. Fails before any AI invocation when a feature requires a term
// the adapter does not declare.
//
// The build-feature skill invokes this between loading capabilities and
// emitting the buildfile.

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
)

var checkSupportsCmd = &cobra.Command{
	Use:   "check-supports <@feature>",
	Short: "Validate a feature's capabilities against every adapter's supports block (JSON output for skill consumption)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCheckSupports,
}

type supportsIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

type supportsOutput struct {
	Feature string          `json:"feature"`
	Ready   bool            `json:"ready"`
	Issues  []supportsIssue `json:"issues"`
}

func runCheckSupports(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])
	out := supportsOutput{Feature: slug, Issues: []supportsIssue{}}

	// Capabilities file is optional — if absent, the gate is a no-op.
	capPath := filepath.Join(cfg.FeaturePath(slug), "capabilities.yaml")
	capContent, err := os.ReadFile(capPath)
	if err != nil {
		out.Ready = true
		return emitSupportsJSON(cmd, out)
	}
	caps, err := parser.ParseCapabilitiesBytes(capPath, capContent)
	if err != nil {
		out.Issues = append(out.Issues, supportsIssue{
			Severity: "error",
			Code:     "capabilities-not-closed-form",
			Message:  fmt.Sprintf("parse %s: %v", capPath, err),
		})
		return emitSupportsJSON(cmd, out)
	}

	// Adapter-set is optional — presentation-only projects skip the gate.
	asPath := cfg.AdapterSetPath()
	asContent, err := os.ReadFile(asPath)
	if err != nil {
		out.Ready = true
		return emitSupportsJSON(cmd, out)
	}
	adapterSet, err := parser.ParseAdapterSetBytes(asPath, asContent)
	if err != nil {
		out.Issues = append(out.Issues, supportsIssue{
			Severity: "error",
			Code:     "adapter-set-invalid-yaml",
			Message:  fmt.Sprintf("%s: %v", asPath, err),
		})
		return emitSupportsJSON(cmd, out)
	}

	// IsMultiTarget gate — presentation-only projects don't run supports.
	if !adapterSet.IsMultiTarget() {
		out.Ready = true
		return emitSupportsJSON(cmd, out)
	}

	// (a) Per-adapter shape/vocabulary validation + collect the backend
	// adapters for (b) union coverage across all filled slots.
	backendAdapters := map[string][]byte{}
	for slotKind, target := range adapterSet.Targets {
		if slotKind == "presentation" {
			continue
		}
		adapterPath := filepath.Join(cfg.AdaptersPath(), target.Adapter+".adapter.yaml")
		adapterContent, err := os.ReadFile(adapterPath)
		if err != nil {
			out.Issues = append(out.Issues, supportsIssue{
				Severity: "error",
				Code:     "adapter-set-adapter-missing",
				Message:  fmt.Sprintf("targets.%s.adapter %q: %v", slotKind, target.Adapter, err),
			})
			continue
		}
		for _, o := range agent.ValidateSupports(agent.ModeBuild, adapterContent) {
			if o.Severity == agent.SeverityError {
				out.Issues = append(out.Issues, supportsIssue{Severity: "error", Code: o.Code, Message: o.Message})
			}
		}
		backendAdapters[slotKind] = adapterContent
	}

	// (b) Union coverage: a term passes if any filled backend adapter supports it.
	for _, o := range agent.ValidateOperationsCoverage(agent.ModeBuild, backendAdapters, caps) {
		if o.Severity == agent.SeverityError {
			out.Issues = append(out.Issues, supportsIssue{Severity: "error", Code: o.Code, Message: o.Message})
		}
	}

	out.Ready = len(out.Issues) == 0
	return emitSupportsJSON(cmd, out)
}

func emitSupportsJSON(cmd *cobra.Command, out supportsOutput) error {
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	if !out.Ready {
		return NewExitCodeError(1)
	}
	return nil
}
