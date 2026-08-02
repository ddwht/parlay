// parlay-feature: parlay-tool/multi-adapter
// parlay-component: cli-and-deployer-registration
//
// Multi-target project walker. Loads .parlay/adapter-set.yaml, gates on
// IsMultiTarget(), iterates non-presentation slots, and invokes the new
// validators against the right inputs (capabilities files, adapter
// supports blocks, blueprint strategy choices).
//
// This is the orchestrator the architecture's §4–§5–§9 rules need: each
// individual validator is reachable on its own, but only the walker has
// the project-level context to compose them correctly.

package agent

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/parser"
)

// ValidateProjectMultiTarget walks the supplied project root and runs every
// multi-target validation rule whose preconditions are met. Returns one
// outcome per rule violation. Presentation-only projects (no
// adapter-set.yaml, or adapter-set with only the presentation slot filled)
// short-circuit the entire walker and return zero outcomes — the legacy
// rules continue to apply via ValidateBuildfileDeepStructured.
//
// rootPath is the project root (the directory containing .parlay/).
func ValidateProjectMultiTarget(mode ValidationMode, rootPath string) []ValidationOutcome {
	var outcomes []ValidationOutcome

	// Deprecation gate: surface a warning whenever config.yaml still carries
	// the legacy prototype-framework: field. Fires regardless of adapter-set
	// state — the field is on its way out per architecture §13 and v0.3
	// removes it entirely.
	outcomes = append(outcomes, validateConfigDeprecations(mode, rootPath)...)

	asPath := filepath.Join(rootPath, ".parlay", "adapter-set.yaml")
	asContent, err := os.ReadFile(asPath)
	if err != nil {
		// No adapter-set.yaml — presentation-only project. Multi-target
		// rules do not apply, but deprecation outcomes already collected
		// above still surface.
		return outcomes
	}

	adapterSet, err := parser.ParseAdapterSetBytes(asPath, asContent)
	if err != nil {
		outcomes = append(outcomes, NewOutcome(mode, "adapter-set-invalid-yaml",
			fmt.Sprintf("%s: %v", asPath, err)))
		return outcomes
	}

	// Adapter-set structural validation runs unconditionally — even a
	// presentation-only adapter-set may have an unknown kind.
	outcomes = append(outcomes, ValidateAdapterSet(mode, asPath, asContent)...)

	// IsMultiTarget gate: presentation-only projects short-circuit every
	// backend rule below.
	if !adapterSet.IsMultiTarget() {
		return outcomes
	}

	// Walk every feature with capabilities.yaml under spec/intents/.
	intentsRoot := filepath.Join(rootPath, "spec", "intents")
	features, _ := walkCapabilitiesFiles(intentsRoot)

	// Per-non-presentation-slot supports gate.
	for slotKind, target := range adapterSet.Targets {
		if slotKind == "presentation" {
			continue
		}
		adapterPath := filepath.Join(rootPath, ".parlay", "adapters", target.Adapter+".adapter.yaml")
		adapterContent, err := os.ReadFile(adapterPath)
		if err != nil {
			outcomes = append(outcomes, NewOutcome(mode, "adapter-set-adapter-missing",
				fmt.Sprintf("targets.%s.adapter %q: %v", slotKind, target.Adapter, err)))
			continue
		}
		// Walk every loaded capabilities file against this adapter's
		// supports block.
		for _, capFile := range features {
			caps, err := parser.ParseCapabilities(capFile)
			if err != nil {
				outcomes = append(outcomes, NewOutcome(mode, "capabilities-not-closed-form",
					fmt.Sprintf("%s: %v", capFile, err)))
				continue
			}
			outcomes = append(outcomes, ValidateSupports(mode, adapterContent, caps)...)
		}
	}

	// Cross-kind link enforcement: project each feature buildfile's edges and
	// assert every one is authorized by the adapter-set links: block. This is
	// the call site that makes ValidateAdapterSetLinks reachable — the walker
	// is the only place with both the buildfiles (to extract edges) and the
	// adapter-set (to authorize them).
	buildRoot := filepath.Join(rootPath, ".parlay", "build")
	if buildEntries, err := os.ReadDir(buildRoot); err == nil {
		for _, be := range buildEntries {
			if !be.IsDir() {
				continue
			}
			bfContent, err := os.ReadFile(filepath.Join(buildRoot, be.Name(), "buildfile.yaml"))
			if err != nil {
				continue
			}
			if edges := ExtractCrossKindEdges(bfContent); len(edges) > 0 {
				outcomes = append(outcomes, ValidateAdapterSetLinks(mode, adapterSet, edges)...)
			}
		}
	}

	// Blueprint scope + strategy gate (multi-target only).
	bpPath := filepath.Join(rootPath, ".parlay", "blueprint.yaml")
	if bpContent, err := os.ReadFile(bpPath); err == nil {
		outcomes = append(outcomes, ValidateBlueprintScope(mode, bpPath, bpContent)...)
	}

	return outcomes
}

// validateConfigDeprecations surfaces deprecation warnings for legacy
// fields in .parlay/config.yaml. Currently the only deprecated field is
// prototype-framework, scheduled for removal in v0.3 in favor of
// .parlay/adapter-set.yaml.
//
// parlay-feature: parlay-tool/multi-adapter
// parlay-component: config-migration-result
func validateConfigDeprecations(mode ValidationMode, rootPath string) []ValidationOutcome {
	var outcomes []ValidationOutcome
	configPath := filepath.Join(rootPath, ".parlay", "config.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return outcomes
	}
	var shape struct {
		PrototypeFramework string `yaml:"prototype-framework"`
	}
	if err := yaml.Unmarshal(content, &shape); err != nil {
		return outcomes
	}
	if shape.PrototypeFramework != "" {
		outcomes = append(outcomes, ValidationOutcome{
			Mode:     mode,
			Code:     "prototype-framework-deprecated",
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("%s declares prototype-framework: %q which is deprecated; run `parlay migrate-config` to convert it into .parlay/adapter-set.yaml (removal scheduled for v0.3)", configPath, shape.PrototypeFramework),
			Fix:      "run `parlay migrate-config`",
		})
	}
	return outcomes
}

// walkCapabilitiesFiles returns every capabilities.yaml path under
// spec/intents/ (recursively, so nested initiatives + features are
// covered).
func walkCapabilitiesFiles(intentsRoot string) ([]string, error) {
	var out []string
	err := filepath.Walk(intentsRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(path) == "capabilities.yaml" {
			out = append(out, path)
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return out, err
}
