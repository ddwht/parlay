// parlay-feature: studio-foundation/studio-deployer
// parlay-section: cross-cutting
// parlay-extends: studio-foundation/studio-deployer/cross-cutting/multi-agent-target-resolution

// agents.go implements the agent-surface detection and per-agent
// target-path resolution. Per dialog Q2.1 (duplicate locally), this file
// imports NO core/* package — Studio's binary stays independent of Core's
// API even when the underlying conventions overlap. Adding a new agent
// surface to parlay-core later requires a parlay-studio binary release
// that adds the surface here.
//
// Three agent surfaces are recognized today, in stable precedence order:
//
//	AgentClaude     — .claude/ directory at the project root
//	AgentCursor     — .cursor/ directory at the project root
//	AgentGenericCLI — .parlay/cli/ directory at the project root
package deployer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// AgentSurface enumerates the per-agent target conventions the deployer
// knows how to write into.
type AgentSurface int

const (
	// AgentClaude — Claude Code; skills land at .claude/skills/parlay-<slug>/SKILL.md.
	AgentClaude AgentSurface = iota
	// AgentCursor — Cursor; skills land at .cursor/agents/parlay-<slug>.md.
	AgentCursor
	// AgentGenericCLI — Parlay's headless CLI surface; skills land at
	// .parlay/cli/skills/parlay-<slug>.md.
	AgentGenericCLI
)

// String returns the lowercase enum label used in stdout, logs, and
// operator-facing error messages.
func (a AgentSurface) String() string {
	switch a {
	case AgentClaude:
		return "claude"
	case AgentCursor:
		return "cursor"
	case AgentGenericCLI:
		return "generic-cli"
	default:
		return fmt.Sprintf("unknown-agent-%d", int(a))
	}
}

// AgentTarget bundles a detected agent surface with its marker directory
// (relative to project root) and a function that resolves a per-skill
// target path. The function returns a path RELATIVE to the project root —
// the deployer joins it with the absolute project root before any disk
// operation.
type AgentTarget struct {
	Surface         AgentSurface
	MarkerPath      string
	SkillTargetPath func(slug string) string
}

// ErrNoAgentDetected is the stable sentinel returned when DetectAgentSurfaces
// finds zero agent surface markers at the project root. The wrapped error
// message names every known surface so the operator knows what to initialize.
var ErrNoAgentDetected = errors.New("studio-deployer-no-agent-detected")

// known carries the three known agent surfaces in stable precedence order.
// The order is observable in stdout summaries and in tests; changing it is
// a feature-stability concern, not an implementation detail.
var known = []agentSpec{
	{
		surface:    AgentClaude,
		markerRel:  ".claude",
		skillsDir:  ".claude/skills",
		pathFn:     func(slug string) string { return filepath.Join(".claude", "skills", slug, "SKILL.md") },
	},
	{
		surface:    AgentCursor,
		markerRel:  ".cursor",
		skillsDir:  ".cursor/agents",
		pathFn:     func(slug string) string { return filepath.Join(".cursor", "agents", slug+".md") },
	},
	{
		surface:    AgentGenericCLI,
		markerRel:  ".parlay/cli",
		skillsDir:  ".parlay/cli/skills",
		pathFn:     func(slug string) string { return filepath.Join(".parlay", "cli", "skills", slug+".md") },
	},
}

// agentSpec is the package-private definition the known table holds. The
// exported AgentTarget is built from this when a surface is detected.
type agentSpec struct {
	surface   AgentSurface
	markerRel string // relative path to the marker directory under projectRoot
	skillsDir string // relative path to the parent of all skill targets for this surface
	pathFn    func(slug string) string
}

// DetectAgentSurfaces walks the three known agent markers under
// projectRoot and returns one AgentTarget per detected surface in stable
// precedence order. A surface is "detected" when its marker directory
// (e.g. .claude/) exists; the per-skill subdirectory (.claude/skills/)
// may be absent, in which case the deployer creates it during the
// per-agent atomic write step.
//
// On zero detected surfaces, DetectAgentSurfaces returns a wrapped
// ErrNoAgentDetected whose message names all three known surfaces.
//
// DetectAgentSurfaces does NOT check for .parlay/ — that check is owned
// by subcommands.go and runs earlier (parlay-not-initialized surfaces
// before no-agent-detected).
func DetectAgentSurfaces(projectRoot string) ([]AgentTarget, error) {
	if projectRoot == "" {
		return nil, fmt.Errorf("%w: project root is empty", ErrNoAgentDetected)
	}
	var detected []AgentTarget
	for _, spec := range known {
		marker := filepath.Join(projectRoot, spec.markerRel)
		info, err := os.Stat(marker)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("DetectAgentSurfaces: stat %s: %w", marker, err)
		}
		if !info.IsDir() {
			continue
		}
		detected = append(detected, AgentTarget{
			Surface:         spec.surface,
			MarkerPath:      marker,
			SkillTargetPath: spec.pathFn,
		})
	}
	if len(detected) == 0 {
		return nil, fmt.Errorf("%w: no agent surface markers found at %s. Tried: Claude Code via .claude/, Cursor via .cursor/, Generic CLI via .parlay/cli/. Initialize one before running parlay-studio init/upgrade",
			ErrNoAgentDetected, projectRoot)
	}
	return detected, nil
}

// skillsDirFor returns the project-root-joined parent directory of a given
// surface's per-skill files (e.g. <root>/.claude/skills/). Callers use it
// to mkdir -p the parent before any per-skill atomic write and to scan
// for orphan files matching the parlay-* prefix.
func skillsDirFor(projectRoot string, surface AgentSurface) string {
	for _, spec := range known {
		if spec.surface == surface {
			return filepath.Join(projectRoot, spec.skillsDir)
		}
	}
	return ""
}
