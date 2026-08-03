// parlay-feature: parlay-tool/multi-adapter
// parlay-component: multi-target-buildfile-schema
//
// scaffold-operations derives, per operation, which backend layer OWNS each
// step — the layer whose adapter lists it in its supports block. Ownership is
// what lets codegen split responsibility across targets (each target
// implements its owned steps and calls downstream-owned steps across an
// authorized link) and what the cross-kind edge extractor reads to derive
// edges. Owner-kind only: the mechanism (prisma.<entity>.create, the controller
// method) stays in the adapter conventions, applied by codegen.
//
// Deterministic and side-effect free, like scaffold-plan.

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// opKindRank orders adapter kinds from UI down to storage. Ties in step
// ownership break toward the deepest layer (highest rank) — the layer that
// implements the mechanism owns it; shallower layers orchestrate.
var opKindRank = map[string]int{
	"presentation": 0,
	"transport":    1,
	"application":  2,
	"persistence":  3,
}

var scaffoldOperationsCmd = &cobra.Command{
	Use:   "scaffold-operations @<feature>",
	Short: "Derive per-step layer ownership for a feature's operations (JSON output)",
	Args:  cobra.ExactArgs(1),
	RunE:  runScaffoldOperations,
}

type scaffoldOperationsOutput struct {
	Feature    string                        `json:"feature"`
	Operations map[string]operationOwnership `json:"operations"`
}

// operationOwnership records, for one operation, which steps each backend kind
// owns. A step owned by no filled backend layer is omitted here — the supports
// gate (ValidateOperationsCoverage) reports that as a coverage error.
type operationOwnership struct {
	Owns map[string][]string `json:"owns"`
}

func runScaffoldOperations(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])
	bfPath := filepath.Join(cfg.BuildPath(slug), "buildfile.yaml")
	data, err := os.ReadFile(bfPath)
	if err != nil {
		return fmt.Errorf("feature %s: %w", slug, err)
	}

	var bf struct {
		AdapterSet string `yaml:"adapter-set"`
		Operations map[string]struct {
			Steps []struct {
				Type string `yaml:"type"`
			} `yaml:"steps"`
		} `yaml:"operations"`
	}
	if err := yaml.Unmarshal(data, &bf); err != nil {
		return fmt.Errorf("parse %s: %w", bfPath, err)
	}
	if bf.AdapterSet == "" {
		return fmt.Errorf("feature %s: not a multi-target buildfile (no adapter-set:)", slug)
	}

	// Load each backend adapter's supported steps.
	as, err := parser.ParseAdapterSet(cfg.AdapterSetPath())
	if err != nil {
		return err
	}
	kindSteps := map[string]map[string]bool{}
	for kind, target := range as.Targets {
		if kind == "presentation" {
			continue
		}
		p := filepath.Join(cfg.AdaptersPath(), target.Adapter+".adapter.yaml")
		adData, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("adapter for target %s (%q): %w", kind, target.Adapter, err)
		}
		var s struct {
			Supports struct {
				Steps []string `yaml:"steps"`
			} `yaml:"supports"`
		}
		if err := yaml.Unmarshal(adData, &s); err != nil {
			return fmt.Errorf("parse adapter %s: %w", p, err)
		}
		set := map[string]bool{}
		for _, st := range s.Supports.Steps {
			set[st] = true
		}
		kindSteps[kind] = set
	}

	out := scaffoldOperationsOutput{Feature: slug, Operations: map[string]operationOwnership{}}
	for opRef, op := range bf.Operations {
		owns := map[string][]string{}
		for _, step := range op.Steps {
			if step.Type == "" {
				continue
			}
			owner := ownerKind(step.Type, kindSteps)
			if owner == "" {
				continue // uncovered — reported by the supports gate, not here
			}
			owns[owner] = append(owns[owner], step.Type)
		}
		for k := range owns {
			sort.Strings(owns[k])
		}
		out.Operations[opRef] = operationOwnership{Owns: owns}
	}

	buf, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(cmd.OutOrStdout(), string(buf))
	return nil
}

// ownerKind returns the backend kind that owns a step: the one whose adapter
// lists it, breaking ties toward the deepest layer (highest opKindRank). ""
// when no filled backend adapter lists the step.
func ownerKind(step string, kindSteps map[string]map[string]bool) string {
	owner := ""
	best := -1
	for kind, set := range kindSteps {
		if set[step] && opKindRank[kind] > best {
			best = opKindRank[kind]
			owner = kind
		}
	}
	return owner
}
