// parlay-extends: claude-md-section-preservation/claudemd-marker-preservation

package deployer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/internal/embedded"
)

const (
	parlayMarkerBegin = "<!-- parlay:begin -->"
	parlayMarkerEnd   = "<!-- parlay:end -->"
)

// ClaudeDeployer deploys skills as .claude/skills/parlay-*/SKILL.md for Claude Code.
type ClaudeDeployer struct{}

func (d *ClaudeDeployer) Name() string { return "Claude Code" }

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

	// Write CLAUDE.md
	return writeCLAUDEmd(projectRoot, skills)
}

func writeCLAUDEmd(projectRoot string, skills []embedded.SkillEntry) error {
	var commands string
	for _, skill := range skills {
		commands += fmt.Sprintf("- `/parlay-%s` — %s\n", skill.Name, skillTitle(skill.Name))
	}

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
- **spec/intents/<feature>/** (generated, human-reviewed): surface.md, domain-model.md, *.page.md
- **spec/handoff/<feature>/** (engineering output): specification.md
- **.parlay/build/<feature>/** (tool internals): buildfile.yaml, testcases.yaml, .baseline.yaml — never user-facing
%s`, parlayMarkerBegin, commands, parlayMarkerEnd)

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
		"create-artifacts":     "Determine and create surface.md, infrastructure.md, or both",
		"create-surface":       "Generate surface from intents and dialogs (deprecated — use create-artifacts)",
		"build-feature":        "Generate buildfile and testcases",
		"generate-code":        "Generate prototype code from buildfile",
		"generate-enggspec":    "Generate engineering specification",
		"extract-domain-model": "Extract domain model from all features",
		"load-domain-model":    "Load and integrate external domain model",
		"collect-questions":    "Collect open questions from intents",
		"reference-design-spec": "Extract design spec from Figma",
		"sync":                 "Check intent-dialog coverage",
		"view-page":            "Assemble and display a page view",
		"lock-page":            "Lock a page layout into a manifest",
		"register-adapter":     "Register a framework adapter",
		"onboard":              "Onboard existing codebase and draft adapter",
	}
	if t, ok := titles[name]; ok {
		return t
	}
	return name
}
