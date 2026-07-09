// parlay-extends: claude-md-section-preservation/claudemd-marker-preservation
// parlay-extends: parlay-tool/parlay-loop/claude-adapter-subagent-deployment
// parlay-extends: parlay-tool/parlay-loop/parlay-loop-cli-command-registration
// parlay-extends: studio-support/domain-model-yaml-migration/migrate-domain-model-deployer-title
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

		// Claude Code skills use YAML frontmatter + markdown body. The
		// description is sourced from the skill's own frontmatter
		// (embedded.ReadAllSkills parses it) — not re-declared here.
		content := fmt.Sprintf(`---
name: parlay-%s
description: "Parlay: %s"
---

%s`, skill.Name, skill.Description, string(skill.Content))

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
	parlaySection := parlayMarkerBegin + "\n" + renderProjectConfigBody(skills, projectRoot) + parlayMarkerEnd

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
