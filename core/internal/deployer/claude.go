// parlay-extends: claude-md-section-preservation/claudemd-marker-preservation
// parlay-extends: parlay-tool/parlay-loop/claude-adapter-subagent-deployment
// parlay-extends: parlay-tool/parlay-loop/parlay-loop-cli-command-registration
// parlay-extends: parlay-tool/domain-model-yaml-migration/migrate-domain-model-deployer-title
// parlay-extends: parlay-tool/create-domain-model/deployer-claude-stale-skill-cleanup

package deployer

import (
	"github.com/ddwht/parlay/core/internal/atomicfile"

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

func (d *ClaudeDeployer) Deploy(projectRoot string, skills []embedded.SkillEntry) (int, error) {
	written := 0
	for _, skill := range skills {
		skillDir := filepath.Join(projectRoot, ".claude", "skills", "parlay-"+skill.Name)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			return written, fmt.Errorf("failed to create skill directory %s: %w", skillDir, err)
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
		wrote, err := atomicfile.WriteIfChanged(skillPath, []byte(content))
		if err != nil {
			return written, fmt.Errorf("failed to write skill %s: %w", skillPath, err)
		}
		if wrote {
			written++
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
		return written, err
	}

	// Deploy subagents to .claude/agents/parlay-<name>.md
	agentWrites, err := writeClaudeAgents(projectRoot)
	if err != nil {
		return written, err
	}
	written += agentWrites

	// Write CLAUDE.md
	claudeWrote, err := writeCLAUDEmd(projectRoot, skills)
	if err != nil {
		return written, err
	}
	if claudeWrote {
		written++
	}
	return written, nil
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
		// Only prune slugs core owns. A skill in this directory that core
		// did not deploy must survive — see retiredCoreSkills. The case
		// that motivated this was parlay-design-loop, installed here by
		// the separate parlay-studio binary; that binary is gone, but the
		// rule is not about it.
		if !shouldPruneSkill(slug, wanted) {
			continue
		}
		stale := filepath.Join(skillsDir, name)
		if err := os.RemoveAll(stale); err != nil {
			return fmt.Errorf("prune stale Claude skill %s: %w", stale, err)
		}
	}
	return nil
}

func writeClaudeAgents(projectRoot string) (int, error) {
	agents, err := embedded.ReadAllAgents()
	if err != nil {
		return 0, fmt.Errorf("failed to read embedded agents: %w", err)
	}
	agentsDir := filepath.Join(projectRoot, ".claude", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create .claude/agents/: %w", err)
	}
	written := 0
	for _, a := range agents {
		path := filepath.Join(agentsDir, "parlay-"+a.Name+".md")
		wrote, err := atomicfile.WriteIfChanged(path, a.Content)
		if err != nil {
			return written, fmt.Errorf("failed to write agent %s: %w", path, err)
		}
		if wrote {
			written++
		}
	}
	return written, nil
}

func writeCLAUDEmd(projectRoot string, skills []embedded.SkillEntry) (bool, error) {
	parlaySection := parlayMarkerBegin + "\n" + renderProjectConfigBody(skills, projectRoot) + parlayMarkerEnd

	claudePath := filepath.Join(projectRoot, "CLAUDE.md")

	existing, err := os.ReadFile(claudePath)
	if err != nil {
		// No existing file — write the parlay section with markers + trailing newline.
		return atomicWrite(claudePath, []byte(parlaySection+"\n"))
	}

	content := string(existing)
	beginIdx := strings.Index(content, parlayMarkerBegin)
	endIdx := strings.Index(content, parlayMarkerEnd)

	if beginIdx >= 0 && endIdx >= 0 && endIdx > beginIdx {
		// Markers found — replace between them, preserve outside.
		above := content[:beginIdx]
		below := content[endIdx+len(parlayMarkerEnd):]
		return atomicWrite(claudePath, []byte(above+parlaySection+below))
	}

	// No markers found — existing content is user-owned.
	// Prepend parlay section with markers, append existing content below.
	fmt.Fprintf(os.Stderr, "[WARN] CLAUDE.md has no parlay markers — preserving existing content below parlay section.\n")
	merged := parlaySection + "\n\n" + content
	return atomicWrite(claudePath, []byte(merged))
}
