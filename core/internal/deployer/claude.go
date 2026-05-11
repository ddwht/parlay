// parlay-extends: claude-md-section-preservation/claudemd-marker-preservation
// parlay-extends: parlay-tool/parlay-loop/claude-adapter-subagent-deployment
// parlay-extends: parlay-tool/parlay-loop/parlay-loop-cli-command-registration
// parlay-extends: studio-support/domain-model-yaml-migration/migrate-domain-model-deployer-title
// parlay-extends: parlay-tool/create-domain-model/deployer-skill-titles-map
// parlay-extends: parlay-tool/create-domain-model/deployer-claude-stale-skill-cleanup

package deployer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/core/internal/embedded"
)

const (
	parlayMarkerBegin = "<!-- parlay:begin -->"
	parlayMarkerEnd   = "<!-- parlay:end -->"
)

// ClaudeDeployer deploys skills as .claude/skills/parlay-*/SKILL.md for Claude Code.
type ClaudeDeployer struct{}

func (d *ClaudeDeployer) Name() string { return "Claude Code" }

// AgentSurfacePaths returns paths Claude Code's deployer writes at the
// repo-level root. Used by the multi-root forbidden-directory check.
func (d *ClaudeDeployer) AgentSurfacePaths() []string {
	return []string{
		".claude/skills",
		".claude/agents",
		"CLAUDE.md",
	}
}

func (d *ClaudeDeployer) Deploy(projectRoot string, skills []embedded.SkillEntry) error {
	for _, skill := range skills {
		skillDir := filepath.Join(projectRoot, ".claude", "skills", "parlay-"+skill.Name)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			return fmt.Errorf("failed to create skill directory %s: %w", skillDir, err)
		}

		// Claude Code skills use YAML frontmatter + markdown body
		content := fmt.Sprintf(`---
name: parlay-%s
description: "Parlay: %s"
---

%s`, skill.Name, skillTitle(skill.Name), string(skill.Content))

		skillPath := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write skill %s: %w", skillPath, err)
		}
	}

	// parlay-extends: parlay-tool/create-domain-model/deployer-claude-stale-skill-cleanup
	// Stale-skill cleanup pass — after the wanted skills are written
	// but before agents and CLAUDE.md are regenerated, prune any
	// .claude/skills/parlay-<old-slug>/ directory whose slug is not
	// present in the embedded skills set. The check is generic — it
	// covers this rename and any future rename without per-rename
	// special-casing. User-owned directories (not prefixed parlay-)
	// are never touched.
	wanted := make(map[string]bool, len(skills))
	for _, s := range skills {
		wanted[s.Name] = true
	}
	if err := pruneStaleClaudeSkills(projectRoot, wanted); err != nil {
		return err
	}

	// Deploy subagents to .claude/agents/parlay-<name>.md
	if err := writeClaudeAgents(projectRoot); err != nil {
		return err
	}

	// Write CLAUDE.md
	return writeCLAUDEmd(projectRoot, skills)
}

// pruneStaleClaudeSkills removes any .claude/skills/parlay-<slug>/
// directory whose slug is not present in the wanted set. The check
// only touches entries whose names start with the parlay- prefix —
// user-owned skill directories are preserved.
//
// A read error on .claude/skills (e.g. the directory does not yet
// exist on a fresh project) is non-fatal: the function returns nil
// so the deploy can continue and write the new skill set.
func pruneStaleClaudeSkills(projectRoot string, wanted map[string]bool) error {
	skillsDir := filepath.Join(projectRoot, ".claude", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		// Missing or unreadable .claude/skills/ — nothing to prune.
		return nil
	}
	const prefix = "parlay-"
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			// Non-parlay-prefixed entries are user-owned.
			continue
		}
		slug := strings.TrimPrefix(name, prefix)
		if wanted[slug] {
			continue
		}
		stale := filepath.Join(skillsDir, name)
		if err := os.RemoveAll(stale); err != nil {
			return fmt.Errorf("prune stale Claude skill %s: %w", stale, err)
		}
	}
	return nil
}

func writeClaudeAgents(projectRoot string) error {
	agents, err := embedded.ReadAllAgents()
	if err != nil {
		return fmt.Errorf("failed to read embedded agents: %w", err)
	}
	agentsDir := filepath.Join(projectRoot, ".claude", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return fmt.Errorf("failed to create .claude/agents/: %w", err)
	}
	for _, a := range agents {
		path := filepath.Join(agentsDir, "parlay-"+a.Name+".md")
		if err := os.WriteFile(path, a.Content, 0644); err != nil {
			return fmt.Errorf("failed to write agent %s: %w", path, err)
		}
	}
	return nil
}

func writeCLAUDEmd(projectRoot string, skills []embedded.SkillEntry) error {
	var commands string
	for _, skill := range skills {
		commands += fmt.Sprintf("- `/parlay-%s` — %s\n", skill.Name, skillTitle(skill.Name))
	}

	multiRootBlock := renderMultiRootSection(projectRoot)

	parlaySection := fmt.Sprintf(`%s
# Parlay Project

This project uses the Parlay intent-driven design toolkit.
All operations are available as /parlay-* slash commands.

## Available Commands

%s
## Schema Loading

Skills load schemas on-demand from .parlay/schemas/. Do not keep schema content in memory across commands.

## Interactive Questions

When a skill step says to "ask the user", "present options", or "wait for the user's response", you MUST use the AskUserQuestion tool to pause execution and collect the user's input before proceeding to the next step. Do not output the question as plain text and continue — the skill requires the user's answer to decide what to do next.

## File Ownership

Three-zone layout — strict ownership:
- **spec/intents/<feature>/** (designer-authored): intents.md, dialogs.md — ask permission before modifying
- **spec/intents/<feature>/** (generated, human-reviewed): four co-equal spec artifacts —
  - **surface.yaml** (or surface.md) — visible output, page assemblies, dialog turns
  - **capabilities.yaml** — operation-shaped backend behavior (closed-vocabulary commands and queries)
  - **infrastructure.md** — architectural prose for boundaries, probes, allowlists, dependency pins, and other concerns that do not reduce to operations
  - **domain-model.yaml** (or domain-model.md) — entities, relationships, and shared vocabulary
  - Plus *.page.md for per-page layouts. A feature picks whichever artifacts it needs; capabilities.yaml and infrastructure.md are co-equal, not stand-ins for each other.
- **spec/handoff/<feature>/** (engineering output): specification.md
- **.parlay/build/<feature>/** (tool internals): buildfile.yaml, testcases.yaml, coverage-review.yaml, .baseline.yaml — never user-facing
- **.parlay/adapter-set.yaml** (tool config, project-owned): pins adapter slot topology — multi-target projects only
%s%s`, parlayMarkerBegin, commands, multiRootBlock, parlayMarkerEnd)

	claudePath := filepath.Join(projectRoot, "CLAUDE.md")

	existing, err := os.ReadFile(claudePath)
	if err != nil {
		// No existing file — write the parlay section with markers + trailing newline.
		return os.WriteFile(claudePath, []byte(parlaySection+"\n"), 0644)
	}

	content := string(existing)
	beginIdx := strings.Index(content, parlayMarkerBegin)
	endIdx := strings.Index(content, parlayMarkerEnd)

	if beginIdx >= 0 && endIdx >= 0 && endIdx > beginIdx {
		// Markers found — replace between them, preserve outside.
		above := content[:beginIdx]
		below := content[endIdx+len(parlayMarkerEnd):]
		return os.WriteFile(claudePath, []byte(above+parlaySection+below), 0644)
	}

	// No markers found — existing content is user-owned.
	// Prepend parlay section with markers, append existing content below.
	fmt.Fprintf(os.Stderr, "[WARN] CLAUDE.md has no parlay markers — preserving existing content below parlay section.\n")
	merged := parlaySection + "\n\n" + content
	return os.WriteFile(claudePath, []byte(merged), 0644)
}

func skillTitle(name string) string {
	titles := map[string]string{
		"add-feature":          "Create a new feature",
		"scaffold-dialogs":     "Scaffold dialog templates from intents",
		"create-artifacts":     "Determine and create any subset of surface, capabilities, infrastructure, and domain-model artifacts a feature needs",
		"build-feature":        "Generate buildfile and testcases",
		"generate-code":        "Generate prototype code from buildfile",
		"generate-enggspec":    "Generate engineering specification",
		"create-domain-model":  "Create domain model from features",
		"load-domain-model":    "Load and integrate external domain model",
		"migrate-domain-model": "Convert domain-model.md to domain-model.yaml",
		"collect-questions":    "Collect open questions from intents",
		"reference-design-spec": "Extract design spec from Figma",
		"sync":                 "Check intent-dialog coverage",
		"view-page":            "Assemble and display a page view",
		"lock-page":            "Lock a page layout into a manifest",
		"register-adapter":     "Register a framework adapter",
		"onboard":              "Onboard existing codebase and draft adapter",
		"new-initiative":       "Create an empty initiative directory",
		"repair":              "Validate and reconcile the three parallel trees",
		"loop":                "Walk a feature end-to-end through the parlay design pipeline",
		// parlay-feature: parlay-tool/multi-adapter
		// parlay-component: cli-and-deployer-registration
		"migrate-config":          "Convert legacy prototype-framework into a single-target presentation adapter-set",
		"migrate-spec":            "Convert each feature's surface.md to surface.yaml",
		"migrate-capabilities":    "Move operation-shaped fragments from infrastructure.md into capabilities.yaml; retain architectural prose in place (partial migration is the success case)",
		"migrate-domain-operations": "Migrate deprecated domain-model.operations entries into per-feature capabilities.yaml stubs",
		"review-coverage":         "Walk suites, record approvals, write coverage-review.yaml",
	}
	if t, ok := titles[name]; ok {
		return t
	}
	return name
}
