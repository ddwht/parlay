// parlay-feature: parlay-tool/multi-adapter
// parlay-component: codegen-read-set-and-layer-pipeline

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// The plan: allowlist was enforced by nothing.
//
// generate-code.skill.md instructs the agent to write only paths declared in
// plan.creates / plan.modifies, and that instruction was the whole of the
// enforcement — a codegen pass that wrote elsewhere produced no diagnostic, ever.
// The obvious fix, intercepting the writes, is not available: parlay does not
// perform codegen's I/O. The agent does, with its own tools, and there is nothing
// for Go to hook.
//
// What IS available is evidence. Codegen's writes are recorded in
// .code-hashes.yaml — that sidecar exists so verify-generated can detect
// hand-edits — and the buildfile carries the plan. Comparing the two turns "never
// enforced" into "detected on the next check", which is a weaker guarantee than
// interception and a much stronger one than prose.
//
// The read-set has no equivalent and is not attempted here. Nothing records which
// files codegen read, so auditing reads would require the agent to self-report a
// read log — a guard that trusts the subject it is guarding, which is worse than
// no guard because it looks like one.

var checkWriteSetCmd = &cobra.Command{
	Use:   "check-write-set [@feature]",
	Short: "Check generated files against the buildfile's plan: allowlist (JSON output)",
	Long: `Compare every file recorded in .code-hashes.yaml against the plan.creates
and plan.modifies entries of the buildfile that claims it.

This is an audit, not a gate: codegen has already run. A file reported here was
written outside the plan its feature declared, which means either the plan is
incomplete or the emission went somewhere it should not have.

With no @feature, checks every feature that has a buildfile.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCheckWriteSet,
}

// writeSetFinding is one file written outside every declared plan.
type writeSetFinding struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

type writeSetOutput struct {
	OK       bool              `json:"ok"`
	Checked  int               `json:"checked"`
	Declared int               `json:"declared"`
	Findings []writeSetFinding `json:"findings"`
	// Exempt counts tracked files the plan is not expected to declare. Reported
	// rather than silently dropped: a check that quietly excuses a third of what
	// it examines should say so, or the "checked" number overstates its reach.
	Exempt int `json:"exempt"`
	// Skipped explains a non-failure that produced no comparison, so an
	// all-clear cannot be confused with "there was nothing to compare".
	Skipped string `json:"skipped,omitempty"`
}

func runCheckWriteSet(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}

	hashes, err := loadProjectCodeHashes(cfg)
	if err != nil {
		return fmt.Errorf("read code-hashes: %w", err)
	}
	if hashes == nil || len(hashes.Files) == 0 {
		// No generation has been recorded. Reporting ok:true here would be
		// indistinguishable from "every written file was declared", so say which
		// one it is.
		return emitWriteSetJSON(cmd, &writeSetOutput{
			OK:       true,
			Findings: []writeSetFinding{},
			Skipped:  "no generated files are recorded in .code-hashes.yaml; nothing has been generated yet, so there is nothing to compare against the plan",
		})
	}

	declared, features, err := declaredPlanPaths(cfg, args)
	if err != nil {
		return err
	}
	if len(features) == 0 {
		return emitWriteSetJSON(cmd, &writeSetOutput{
			OK:       true,
			Findings: []writeSetFinding{},
			Skipped:  "no buildfile declares a plan: section; the allowlist is empty by absence rather than by policy, so no file can be judged against it",
		})
	}

	out := &writeSetOutput{
		OK:       true,
		Checked:  len(hashes.Files),
		Declared: len(declared),
		Findings: []writeSetFinding{},
	}

	// A mutating toolchain tool may write within its declared write-set; those
	// writes are authorized by the tool contract, not the plan, so admit them.
	writeSetRegions := toolchainWriteSetRegions(cfg)

	paths := make([]string, 0, len(hashes.Files))
	for p := range hashes.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, p := range paths {
		if declared[normalizeWriteSetPath(p)] {
			continue
		}
		if reason := exemptFromPlan(cfg, p, hashes.Files[p].Component); reason != "" {
			out.Exempt++
			continue
		}
		if withinAnyRegion(normalizeWriteSetPath(p), writeSetRegions) {
			out.Exempt++
			continue
		}
		out.OK = false
		out.Findings = append(out.Findings, writeSetFinding{
			Path: p,
			Code: "codegen-wrote-outside-plan",
			Message: fmt.Sprintf("%s is tracked as generated but no buildfile plan declares it (checked %d declared path(s) across %d feature(s))",
				p, len(declared), len(features)),
			Fix: "either add the path to the owning feature's plan.creates or plan.modifies and regenerate, or remove the file if the emission was unintended",
		})
	}

	if err := emitWriteSetJSON(cmd, out); err != nil {
		return err
	}
	if !out.OK {
		return NewExitCodeError(1)
	}
	return nil
}

// declaredPlanPaths unions plan.creates and plan.modifies across the requested
// features, returning the normalized path set and the features it read.
//
// Deletes are deliberately excluded: a deleted path is not something codegen
// wrote, and .code-hashes.yaml should not be tracking it. Cross-cutting
// target-creates ARE included — they are declared emissions, just declared in a
// different section, and treating them as undeclared would report the tool's own
// documented mechanism as a violation.
func declaredPlanPaths(cfg *config.Context, args []string) (map[string]bool, []string, error) {
	declared := map[string]bool{}
	var features []string

	buildfiles, err := writeSetBuildfiles(cfg, args)
	if err != nil {
		return nil, nil, err
	}
	for _, bf := range buildfiles {
		data, err := os.ReadFile(bf)
		if err != nil {
			// An unreadable buildfile contributes no declared paths, which would
			// make every file it owns look undeclared. Refuse instead of
			// reporting a flood: the first version skipped quietly here, and
			// breaking one buildfile's YAML turned eleven correctly-declared
			// files into violations. One real problem must not surface as many
			// false ones.
			return nil, nil, fmt.Errorf("read buildfile %s: %w — cannot judge the write-set against a plan that will not load", bf, err)
		}
		var shape struct {
			Plan *struct {
				Creates []struct {
					Path string `yaml:"path"`
				} `yaml:"creates"`
				Modifies []struct {
					Path string `yaml:"path"`
				} `yaml:"modifies"`
			} `yaml:"plan"`
			CrossCutting []struct {
				TargetFiles   []string `yaml:"target-files"`
				TargetCreates []string `yaml:"target-creates"`
			} `yaml:"cross-cutting"`
		}
		if err := yaml.Unmarshal(data, &shape); err != nil {
			// Same reasoning as the read failure above.
			return nil, nil, fmt.Errorf("parse buildfile %s: %w — cannot judge the write-set against a plan that will not load", bf, err)
		}
		counted := false
		if shape.Plan != nil {
			for _, e := range shape.Plan.Creates {
				if e.Path != "" {
					declared[normalizeWriteSetPath(e.Path)] = true
					counted = true
				}
			}
			for _, e := range shape.Plan.Modifies {
				if e.Path != "" {
					declared[normalizeWriteSetPath(e.Path)] = true
					counted = true
				}
			}
		}
		for _, cc := range shape.CrossCutting {
			for _, p := range append(cc.TargetFiles, cc.TargetCreates...) {
				if p != "" {
					declared[normalizeWriteSetPath(p)] = true
					counted = true
				}
			}
		}
		if counted {
			features = append(features, bf)
		}
	}
	return declared, features, nil
}

// toolchainWriteSetRegions resolves the directory prefixes that active
// code-phase MUTATING toolchain tools may write into. Each entry's write-set
// glob is reduced to its directory region; for a multi-target project it is
// rebased under the entry's target root (the same root override the plan uses),
// since the adapter authors write-set relative to its own source-root but the
// files land under the adapter-set root.
//
// Coarse by design: admission is by write-set REGION, not per-tool attribution
// — .code-hashes.yaml records no writer identity, so a file inside a declared
// mutating write-set is authorized by the tool contract, full stop. This is the
// runtime half of the write-set contract (ValidateToolchain enforces the
// declaration at registration; this admits the emission).
func toolchainWriteSetRegions(cfg *config.Context) []string {
	var regions []string
	addEntry := func(e agent.ToolchainEntry, targetRoot string) {
		if e.Authority != "mutating" || !slices.Contains(e.Phase, "code") {
			return
		}
		for _, g := range e.WriteSet {
			region := writeSetRegion(g)
			if region == "" {
				continue
			}
			if targetRoot != "" {
				region = normalizeWriteSetPath(path.Join(targetRoot, region))
			}
			regions = append(regions, region)
		}
	}

	// Multi-target: each filled slot's adapter, rebased under its target root.
	if as, err := parser.ParseAdapterSet(cfg.AdapterSetPath()); err == nil && as.IsMultiTarget() {
		if adapters, _, err := adaptersForProject(cfg); err == nil {
			for kind, ad := range adapters {
				if ad.Toolchain != nil {
					root := as.Targets[kind].Root
					for _, e := range ad.Toolchain.MCP {
						addEntry(e, root)
					}
					for _, e := range ad.Toolchain.Skills {
						addEntry(e, root)
					}
				}
			}
			return regions
		}
	}

	// Single-target: the one adapter; write-set globs used as authored (already
	// bound to the adapter's source-root).
	if adapterPath := firstAdapterFile(cfg.AdaptersPath()); adapterPath != "" {
		if data, err := os.ReadFile(adapterPath); err == nil {
			var ad adapterForPlan
			if yaml.Unmarshal(data, &ad) == nil && ad.Toolchain != nil {
				for _, e := range ad.Toolchain.MCP {
					addEntry(e, "")
				}
				for _, e := range ad.Toolchain.Skills {
					addEntry(e, "")
				}
			}
		}
	}
	return regions
}

// writeSetRegion reduces a write-set glob to its directory prefix:
// "src/**" → "src", "src/app/**" → "src/app", "src" → "src".
func writeSetRegion(glob string) string {
	g := strings.TrimSpace(glob)
	g = strings.TrimPrefix(g, "./")
	for _, suf := range []string{"/**", "/*", "/", "**", "*"} {
		g = strings.TrimSuffix(g, suf)
	}
	return strings.TrimSuffix(g, "/")
}

// withinAnyRegion reports whether a normalized path falls inside any region.
func withinAnyRegion(p string, regions []string) bool {
	for _, r := range regions {
		if r == "" {
			continue
		}
		if p == r || strings.HasPrefix(p, r+"/") {
			return true
		}
	}
	return false
}

// writeSetBuildfiles resolves which buildfiles to read: one feature's when named,
// every feature's otherwise.
func writeSetBuildfiles(cfg *config.Context, args []string) ([]string, error) {
	if len(args) == 1 {
		slug := strings.TrimPrefix(args[0], "@")
		return []string{filepath.Join(cfg.BuildPath(slug), "buildfile.yaml")}, nil
	}
	var out []string
	root := filepath.Join(cfg.Root.Path, config.ParlayDir, "build")
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && filepath.Base(p) == "buildfile.yaml" {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// normalizeWriteSetPath makes plan paths and code-hashes keys comparable. Both
// are repo-relative in practice, but slash direction and "./" prefixes have
// differed between writers, and a comparison that fails on those reports every
// file as undeclared — an audit that cries wolf is switched off.
func normalizeWriteSetPath(p string) string {
	clean := filepath.ToSlash(filepath.Clean(p))
	return strings.TrimPrefix(clean, "./")
}

func emitWriteSetJSON(cmd *cobra.Command, out *writeSetOutput) error {
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}

// exemptFromPlan reports why a tracked file is not expected to appear in any
// plan:, or "" when it is expected and its absence is a finding.
//
// Both exemptions are documented behaviour, not conveniences. Running the check
// without them reported 31 of 82 tracked files in a real project — every one of
// them legitimate — which is the shape of an audit that gets switched off rather
// than acted on.
//
//  1. Test files. generate-code.skill.md step 15 emits them from testcases.yaml,
//     separately from the plan-driven emission and at whatever location the test
//     framework expects. The skill identifies them by a `parlay-artifact: test`
//     marker, so that marker is what is checked here — not a filename suffix,
//     which would be a guess about one framework's convention.
//
//  2. Project scaffold. .code-hashes.yaml records a per-file `component`
//     attribution, and files belonging to no component carry an empty one: the
//     app shell, routing, auth guards, shared fixtures. They are project-level
//     emissions that no feature's plan can declare, because no feature owns them.
//
// What remains after both is the set the plan: allowlist is actually about —
// component-attributed implementation files.
func exemptFromPlan(cfg *config.Context, relPath, component string) string {
	if component == "" {
		return "project scaffold: no component attribution, so no feature plan can declare it"
	}
	data, err := os.ReadFile(filepath.Join(cfg.Root.Path, relPath))
	if err != nil {
		// Unreadable is not exempt. A tracked file that cannot be read is its own
		// problem, and treating it as excused would hide both.
		return ""
	}
	if strings.Contains(string(data), "parlay-artifact: test") {
		return "test file: emitted by the test-generation step, not by the plan"
	}
	return ""
}
