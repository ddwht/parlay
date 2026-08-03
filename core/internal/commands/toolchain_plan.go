// parlay-feature: parlay-tool/multi-adapter
// parlay-component: adapter-toolchain-runtime
//
// toolchain-plan surfaces the external skills and MCP servers an adapter's
// Section-10 `toolchain:` block declares for a pipeline phase, as JSON the code
// phase consumes. parlay never invokes the tools itself — the code agent does,
// calling the host agent's own tools by name; this command is the surfacing
// half (the agent never parses adapter YAML), mirroring how scaffold-plan
// surfaces the file plan and scan-generated surfaces markers.
//
// Read-only and deterministic, like scaffold-plan.

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var toolchainPlanCmd = &cobra.Command{
	Use:   "toolchain-plan @<feature>",
	Short: "Emit the external skills/MCP servers an adapter's toolchain declares for a phase (JSON)",
	Args:  cobra.ExactArgs(1),
	RunE:  runToolchainPlan,
}

var (
	toolchainPlanPhase string
	toolchainPlanStage string
)

func init() {
	toolchainPlanCmd.Flags().StringVar(&toolchainPlanPhase, "phase", "code", "pipeline phase to collect entries for")
	toolchainPlanCmd.Flags().StringVar(&toolchainPlanStage, "stage", "", "optional stage filter: pre-emit | post-emit")
}

// toolchainPlanEntry is one resolved skill or MCP server, flattened for the
// agent. `target` is the adapter-set kind for multi-target projects, "" for
// single-target. `required` resolves the *bool (nil → false).
type toolchainPlanEntry struct {
	Kind        string   `json:"kind"` // "skill" | "mcp"
	Target      string   `json:"target,omitempty"`
	Adapter     string   `json:"adapter"`
	Name        string   `json:"name"`
	Invoke      string   `json:"invoke,omitempty"`
	Server      string   `json:"server,omitempty"`
	Tools       []string `json:"tools,omitempty"`
	Source      string   `json:"source,omitempty"`
	Phase       []string `json:"phase"`
	Stage       string   `json:"stage"`
	Authority   string   `json:"authority"`
	Required    bool     `json:"required"`
	ReadSet     []string `json:"read_set,omitempty"`
	WriteSet    []string `json:"write_set,omitempty"`
	OwnsMarkers string   `json:"owns_markers,omitempty"`
	Preserves   []string `json:"preserves,omitempty"`
	Fallback    string   `json:"fallback,omitempty"`
}

type toolchainPlanOutput struct {
	Feature string               `json:"feature"`
	Phase   string               `json:"phase"`
	Stage   string               `json:"stage,omitempty"`
	Entries []toolchainPlanEntry `json:"entries"`
}

func runToolchainPlan(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	if !agent.ValidToolchainPhase(toolchainPlanPhase) {
		return fmt.Errorf("unknown --phase %q: must be one of {intents, dialogs, artifacts, build, code}", toolchainPlanPhase)
	}
	if toolchainPlanStage != "" && toolchainPlanStage != "pre-emit" && toolchainPlanStage != "post-emit" {
		return fmt.Errorf("unknown --stage %q: must be pre-emit or post-emit", toolchainPlanStage)
	}

	slug := parser.FeatureSlug(args[0])
	bfPath := filepath.Join(cfg.BuildPath(slug), "buildfile.yaml")
	data, err := os.ReadFile(bfPath)
	if err != nil {
		return fmt.Errorf("feature %s: %w", slug, err)
	}
	var bf struct {
		AdapterSet string `yaml:"adapter-set"`
	}
	if err := yaml.Unmarshal(data, &bf); err != nil {
		return fmt.Errorf("parse %s: %w", bfPath, err)
	}

	out := toolchainPlanOutput{
		Feature: slug,
		Phase:   toolchainPlanPhase,
		Stage:   toolchainPlanStage,
		Entries: []toolchainPlanEntry{},
	}

	if bf.AdapterSet != "" {
		// Multi-target: each filled slot's adapter carries its own toolchain.
		adapters, as, err := adaptersForProject(cfg)
		if err != nil {
			return err
		}
		for kind, ad := range adapters {
			if ad.Toolchain == nil {
				continue
			}
			adapterBase := as.Targets[kind].Adapter + ".adapter.yaml"
			out.Entries = append(out.Entries,
				collectToolchainEntries(ad.Toolchain, kind, adapterBase, toolchainPlanPhase, toolchainPlanStage)...)
		}
	} else {
		// Single-target: the one adapter under .parlay/adapters/.
		adapterPath := firstAdapterFile(cfg.AdaptersPath())
		if adapterPath != "" {
			adData, err := os.ReadFile(adapterPath)
			if err != nil {
				return err
			}
			var ad adapterForPlan
			if err := yaml.Unmarshal(adData, &ad); err != nil {
				return fmt.Errorf("parse adapter %s: %w", adapterPath, err)
			}
			if ad.Toolchain != nil {
				out.Entries = append(out.Entries,
					collectToolchainEntries(ad.Toolchain, "", filepath.Base(adapterPath), toolchainPlanPhase, toolchainPlanStage)...)
			}
		}
	}

	out.Entries = dedupToolchainEntries(out.Entries)
	sortToolchainEntries(out.Entries)

	buf, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(buf))
	return nil
}

// collectToolchainEntries flattens one adapter's toolchain into plan entries
// matching the phase (and stage, if given).
func collectToolchainEntries(tc *agent.Toolchain, target, adapter, phase, stage string) []toolchainPlanEntry {
	var out []toolchainPlanEntry
	add := func(e agent.ToolchainEntry, kind string) {
		if !slices.Contains(e.Phase, phase) {
			return
		}
		if stage != "" && e.Stage != stage {
			return
		}
		out = append(out, toolchainPlanEntry{
			Kind:        kind,
			Target:      target,
			Adapter:     adapter,
			Name:        e.Name(),
			Invoke:      e.Invoke,
			Server:      e.Server,
			Tools:       e.Tools,
			Source:      e.Source,
			Phase:       e.Phase,
			Stage:       e.Stage,
			Authority:   e.Authority,
			Required:    e.Required != nil && *e.Required,
			ReadSet:     e.ReadSet,
			WriteSet:    e.WriteSet,
			OwnsMarkers: e.OwnsMarkers,
			Preserves:   e.Preserves,
			Fallback:    e.Fallback,
		})
	}
	for _, e := range tc.Skills {
		add(e, "skill")
	}
	for _, e := range tc.MCP {
		add(e, "mcp")
	}
	return out
}

func dedupToolchainEntries(in []toolchainPlanEntry) []toolchainPlanEntry {
	seen := map[string]bool{}
	out := []toolchainPlanEntry{}
	for _, e := range in {
		key := e.Target + "\x00" + e.Kind + "\x00" + e.Name + "\x00" + e.Stage
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}
	return out
}

// sortToolchainEntries orders deterministically: stage (pre-emit < post-emit),
// then target (alpha; "" single-target first), then kind (mcp < skill), then
// name. Stage-first lets the code phase consume pre-emit and post-emit groups
// in emission-relevant order.
func sortToolchainEntries(e []toolchainPlanEntry) {
	stageRank := func(s string) int {
		switch s {
		case "pre-emit":
			return 0
		case "post-emit":
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(e, func(i, j int) bool {
		a, b := e[i], e[j]
		if ra, rb := stageRank(a.Stage), stageRank(b.Stage); ra != rb {
			return ra < rb
		}
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Name < b.Name
	})
}
