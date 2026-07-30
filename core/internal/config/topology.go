// parlay-section: cross-cutting
// parlay-feature: parlay-tool/multi-root
// parlay-cross-cutting: topology-validator

// Package-local topology validator. Lives in package config so it can
// reuse RootsIndex / ProjectConfig loaders without importing the
// commands layer. Read-only — never mutates configs, never auto-fixes,
// never emits side effects.
//
// Detects four structural mismatches against the config-shape model:
//   1. bare-parent              — parent has roots.yaml but no config.yaml
//   2. agent-at-child           — a child config.yaml declares ai-agent
//   3. both-have-agent          — parent and child both declare ai-agent
//   4. single-root-missing-ai-agent — a single-root config lacks ai-agent
//
// Two simultaneous mismatches return two distinct entries. Determinism:
// running the validator twice in succession returns identical results.

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// MismatchKind enumerates the structural mismatches the validator can
// detect. Stable string values are used in CLI output and JSON.
type MismatchKind string

const (
	MismatchBareParent             MismatchKind = "bare-parent"
	MismatchAgentAtChild           MismatchKind = "agent-at-child"
	MismatchBothHaveAgent          MismatchKind = "both-have-agent"
	MismatchSingleRootMissingAgent MismatchKind = "single-root-missing-ai-agent"
)

// FieldRemoval describes a field-level edit the proposed fix wants to
// apply: clearing one named field at one config path.
type FieldRemoval struct {
	File  string
	Field string
}

// FixDescriptor is a structured proposal for repairing a mismatch.
// Apply logic lives in the repair driver — this type only carries
// what the validator wants the driver to do.
type FixDescriptor struct {
	Creates       []string       // file paths to create from scratch
	Modifies      []string       // file paths to write/replace
	RemovesFields []FieldRemoval // field-level edits
	Description   string         // human-readable summary printed verbatim
}

// Mismatch is one detected topology problem, ready for rendering and
// repair. FilePaths is non-empty for every kind. Values is non-nil for
// kinds that compare ai-agent values across files.
type Mismatch struct {
	Kind        MismatchKind
	FilePaths   []string
	Values      []string
	ProposedFix FixDescriptor
}

// ScanTopology runs the topology check against the project rooted at
// `active`. Returns mismatches in deterministic order. The validator is
// read-only — it never mutates either configs or the active root.
//
// For multi-root projects (active is a parent), the parent and every
// registered child are scanned. For single-root projects, only the
// active root is scanned. Child roots invoked directly walk up to the
// parent and validate from the parent's perspective so they see the
// same mismatch set as the parent would.
func ScanTopology(active *Root) ([]Mismatch, error) {
	if active == nil {
		return nil, errors.New("nil active root")
	}

	parent := *active
	if active.Kind == RootKindChild && active.ParentPath != "" {
		parent = Root{
			Path: active.ParentPath,
			Kind: RootKindParent,
		}
	}

	// Inspect the parent's config + roots.yaml.
	parentCfgPath := filepath.Join(parent.Path, ParlayDir, ConfigFile)
	parentRootsPath := filepath.Join(parent.Path, ParlayDir, RootsIndexFile)

	parentCfg, parentCfgErr := loadProjectConfigAt(parentCfgPath)
	parentHasCfg := parentCfgErr == nil
	if parentCfgErr != nil && !errors.Is(parentCfgErr, os.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", parentCfgPath, parentCfgErr)
	}

	_, parentRootsErr := os.Stat(parentRootsPath)
	parentHasRoots := parentRootsErr == nil
	if parentRootsErr != nil && !errors.Is(parentRootsErr, os.ErrNotExist) {
		return nil, fmt.Errorf("stat %s: %w", parentRootsPath, parentRootsErr)
	}

	var mismatches []Mismatch

	switch {
	case parentHasRoots && !parentHasCfg:
		// Bare-parent topology: roots.yaml exists, but no config.yaml at parent.
		mismatches = append(mismatches, Mismatch{
			Kind:      MismatchBareParent,
			FilePaths: []string{parentCfgPath},
			ProposedFix: FixDescriptor{
				Creates:     []string{parentCfgPath},
				Description: fmt.Sprintf("create %s with ai-agent: <prompted>", parentCfgPath),
			},
		})
	case !parentHasRoots && parentHasCfg && parentCfg.AIAgent == "":
		// Single-root with no ai-agent.
		mismatches = append(mismatches, Mismatch{
			Kind:      MismatchSingleRootMissingAgent,
			FilePaths: []string{parentCfgPath},
			ProposedFix: FixDescriptor{
				Modifies:    []string{parentCfgPath},
				Description: fmt.Sprintf("prompt for ai-agent (pre-filled with detected agent if available); write to %s", parentCfgPath),
			},
		})
	}

	// Walk children if a roots.yaml exists.
	if parentHasRoots {
		idx, err := LoadRootsIndex(parent.Path)
		if err != nil {
			return nil, err
		}
		for _, child := range idx.Children {
			childCfgPath := filepath.Join(child.Path, ParlayDir, ConfigFile)
			childCfg, err := loadProjectConfigAt(childCfgPath)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return nil, fmt.Errorf("read %s: %w", childCfgPath, err)
			}

			// agent-at-child: a child config carries ai-agent.
			//
			// When BOTH parent and child carry ai-agent, the both-have-agent
			// mismatch supersedes — emit only the both-have-agent entry to
			// avoid double-counting the same root cause. Pure agent-at-child
			// fires when the parent has no ai-agent (or no parent config).
			if childCfg.AIAgent != "" {
				if parentHasCfg && parentCfg.AIAgent != "" {
					mismatches = append(mismatches, Mismatch{
						Kind:      MismatchBothHaveAgent,
						FilePaths: []string{parentCfgPath, childCfgPath},
						Values:    []string{parentCfg.AIAgent, childCfg.AIAgent},
						ProposedFix: FixDescriptor{
							RemovesFields: []FieldRemoval{
								{File: childCfgPath, Field: "ai-agent"},
							},
							Description: bothHaveAgentFixDescription(parentCfg.AIAgent, childCfg.AIAgent, childCfgPath),
						},
					})
				} else {
					mismatches = append(mismatches, Mismatch{
						Kind:      MismatchAgentAtChild,
						FilePaths: []string{childCfgPath},
						Values:    []string{childCfg.AIAgent},
						ProposedFix: FixDescriptor{
							Creates:  []string{parentCfgPath},
							Modifies: []string{parentCfgPath},
							RemovesFields: []FieldRemoval{
								{File: childCfgPath, Field: "ai-agent"},
							},
							Description: fmt.Sprintf("remove ai-agent from %s; write ai-agent: %s to %s after confirmation", childCfgPath, childCfg.AIAgent, parentCfgPath),
						},
					})
				}
			}
		}
	}

	return mismatches, nil
}

// bothHaveAgentFixDescription produces the proposed-fix line for a
// both-have-agent mismatch. Matching values get a deterministic fix
// (drop the child); conflicting values defer to user selection at the
// parent.
func bothHaveAgentFixDescription(parentVal, childVal, childPath string) string {
	if parentVal == childVal {
		return fmt.Sprintf("remove ai-agent from %s (parent's value is authoritative)", childPath)
	}
	return "user must select which value to keep at the parent; child entry is then removed"
}

// HasAIAgentField returns true when a config.yaml at the given path
// declares ai-agent (non-empty value). Returns false on missing file or
// missing field — callers needing to distinguish should stat the file
// first.
func HasAIAgentField(cfgPath string) bool {
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return false
	}
	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return false
	}
	return cfg.AIAgent != ""
}

// --- Agent-Identity Single-Source validator (cross-cutting:
//     agent-identity-single-source-validation) ---
//
// At config-load time, enforce that ai-agent is declared in exactly one
// config file per project. Detect three illegal states and hard-error
// before any command work runs.

// ErrAgentIdentityAtChild is returned when a child config.yaml carries
// ai-agent. The error message names the offending child path verbatim.
var ErrAgentIdentityAtChild = errors.New("agent identity belongs at the parent root")

// ErrAgentIdentityDuplicated is returned when both parent and child
// declare ai-agent. The error enumerates each declaring file with its
// value; matching values do NOT silently prefer either side.
var ErrAgentIdentityDuplicated = errors.New("agent identity declared at multiple levels")

// ErrAgentIdentityMissingAtParent is returned when a multi-root parent
// has no ai-agent field while children continue to operate.
var ErrAgentIdentityMissingAtParent = errors.New("no agent identity declared at parent root")

// ValidateAgentIdentitySingleSource enforces the single-source rule for
// ai-agent. Pass non-nil parent and child configs WHEN the active root
// is a child; pass nil child for single-root or parent-active calls.
//
// The four illegal states:
//  1. parent==nil, child!=nil, child.AIAgent != ""  → not multi-root,
//     single-root child shape with agent — caller must promote to
//     parent shape; we don't error here, the caller handles it.
//  2. child carries ai-agent (multi-root) → ErrAgentIdentityAtChild
//  3. parent and child both carry ai-agent → ErrAgentIdentityDuplicated
//  4. parent missing ai-agent in multi-root → ErrAgentIdentityMissingAtParent
//
// The validator never walks up past the recorded parent and never falls
// back to a child's value to satisfy the parent's missing field.
func ValidateAgentIdentitySingleSource(parent, child *ProjectConfig, parentPath, childPath string) error {
	// Single-root invocation (no child): nothing to cross-validate here.
	// The single-root-missing-ai-agent mismatch is detected by the
	// topology validator (ScanTopology) for parlay repair; this
	// load-time validator is concerned with multi-root cross-checks.
	if child == nil {
		return nil
	}

	// Multi-root: child must NOT declare ai-agent.
	if child.AIAgent != "" {
		if parent != nil && parent.AIAgent != "" {
			return fmt.Errorf("%w: %s (ai-agent: %s) and %s (ai-agent: %s) — run `parlay repair`",
				ErrAgentIdentityDuplicated,
				parentPath, parent.AIAgent,
				childPath, child.AIAgent)
		}
		return fmt.Errorf("%w: remove ai-agent from %s",
			ErrAgentIdentityAtChild, childPath)
	}

	// Multi-root with child loaded: parent MUST declare ai-agent for any
	// command that reaches this validator. (Commands that don't need
	// the agent identity simply skip calling this validator — they get
	// the per-field inheritance resolver instead.)
	if parent == nil || parent.AIAgent == "" {
		return fmt.Errorf("%w: %s has no `ai-agent` field — add `ai-agent: <Claude Code|Cursor|Generic CLI>` and re-run, or run `parlay repair`",
			ErrAgentIdentityMissingAtParent, parentPath)
	}

	return nil
}

// --- Per-field config inheritance resolver (cross-cutting:
//     per-field-config-inheritance-resolver) ---
//
// When loading a child root's effective configuration, resolve each
// field independently. ai-agent is parent-only — the resolver never
// reads it from the child and never inherits it through walk-up beyond
// the recorded parent. sdd-framework and prototype-framework are
// child-first with parent fallback. Each effective field carries its
// source file path so verbose mode can render `<field>: <value> (from
// <file>)` for direct declarations and `<field>: <value> (inherited
// from <file>)` for parent-fallback cases.

// OriginKind classifies how an effective config field reached its value.
type OriginKind string

const (
	OriginFrom          OriginKind = "from"
	OriginInheritedFrom OriginKind = "inherited-from"
	OriginNotDeclared   OriginKind = "not-declared"
)

// FieldOrigin is one resolved config field plus the file it came from
// and how it got there.
type FieldOrigin struct {
	Value      string
	SourceFile string
	Origin     OriginKind
}

// EffectiveConfig is the per-invocation resolved config for the active
// root. Each field carries its source file and origin for verbose mode.
type EffectiveConfig struct {
	AIAgent            FieldOrigin
	SDDFramework       FieldOrigin
	PrototypeFramework FieldOrigin
}

// ResolveEffectiveConfig produces the effective config for the active
// root. For single-root or parent-active calls, every field comes
// directly from the active root's config. For child-active calls,
// ai-agent is read parent-only; sdd-framework and prototype-framework
// are child-first with parent fallback. The resolver never mutates
// either config file — inheritance is a load-time decision only.
func ResolveEffectiveConfig(active *Root) (*EffectiveConfig, error) {
	if active == nil {
		return nil, errors.New("nil active root")
	}

	activeCfgPath := filepath.Join(active.Path, ParlayDir, ConfigFile)
	activeCfg, activeErr := loadProjectConfigAt(activeCfgPath)
	if activeErr != nil && !errors.Is(activeErr, os.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", activeCfgPath, activeErr)
	}

	out := &EffectiveConfig{}

	// Single-root or parent-active: everything comes from the active
	// root's config.
	if active.Kind != RootKindChild || active.ParentPath == "" {
		if activeCfg == nil {
			out.AIAgent = FieldOrigin{Origin: OriginNotDeclared}
			out.SDDFramework = FieldOrigin{Origin: OriginNotDeclared}
			out.PrototypeFramework = FieldOrigin{Origin: OriginNotDeclared}
			return out, nil
		}
		out.AIAgent = fieldOriginOrEmpty(activeCfg.AIAgent, activeCfgPath)
		out.SDDFramework = fieldOriginOrEmpty(activeCfg.SDDFramework, activeCfgPath)
		out.PrototypeFramework = fieldOriginOrEmpty(activeCfg.PrototypeFramework, activeCfgPath)
		return out, nil
	}

	// Multi-root child active: load the parent's config too.
	parentCfgPath := filepath.Join(active.ParentPath, ParlayDir, ConfigFile)
	parentCfg, parentErr := loadProjectConfigAt(parentCfgPath)
	if parentErr != nil && !errors.Is(parentErr, os.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", parentCfgPath, parentErr)
	}

	// ai-agent: parent-only.
	if parentCfg != nil && parentCfg.AIAgent != "" {
		out.AIAgent = FieldOrigin{Value: parentCfg.AIAgent, SourceFile: parentCfgPath, Origin: OriginFrom}
	} else {
		out.AIAgent = FieldOrigin{Origin: OriginNotDeclared}
	}

	// sdd-framework: child-first, parent fallback.
	out.SDDFramework = resolveChildFirst(activeCfg, parentCfg, activeCfgPath, parentCfgPath, func(c *ProjectConfig) string {
		if c == nil {
			return ""
		}
		return c.SDDFramework
	})

	// prototype-framework: child-first, parent fallback.
	out.PrototypeFramework = resolveChildFirst(activeCfg, parentCfg, activeCfgPath, parentCfgPath, func(c *ProjectConfig) string {
		if c == nil {
			return ""
		}
		return c.PrototypeFramework
	})

	// Cross-validate ai-agent single-source. We do this once the
	// effective config is assembled, so the error message has both
	// paths handy.
	if err := ValidateAgentIdentitySingleSource(parentCfg, activeCfg, parentCfgPath, activeCfgPath); err != nil {
		return nil, err
	}

	return out, nil
}

func fieldOriginOrEmpty(value, sourceFile string) FieldOrigin {
	if value == "" {
		return FieldOrigin{Origin: OriginNotDeclared}
	}
	return FieldOrigin{Value: value, SourceFile: sourceFile, Origin: OriginFrom}
}

func resolveChildFirst(child, parent *ProjectConfig, childPath, parentPath string, get func(*ProjectConfig) string) FieldOrigin {
	if v := get(child); v != "" {
		return FieldOrigin{Value: v, SourceFile: childPath, Origin: OriginFrom}
	}
	if v := get(parent); v != "" {
		return FieldOrigin{Value: v, SourceFile: parentPath, Origin: OriginInheritedFrom}
	}
	return FieldOrigin{Origin: OriginNotDeclared}
}
