package embedded

import (
	"bytes"
	"embed"
	"io/fs"
	"strings"
)

//go:embed skills/*.skill.md
var skillsFS embed.FS

// SkillEntry holds a skill's name, its frontmatter description, and its
// body content (frontmatter stripped, deploy-time markers expanded).
type SkillEntry struct {
	Name        string
	Description string
	Content     []byte
}

// activeRootMarker is the placeholder a skill author drops in place of
// the full "## Active root" section (right after the existing
// `<!-- parlay:active-root-aware -->` signal comment). ReadAllSkills
// expands it into activeRootExpansion below, so the multi-paragraph
// explanation of active-root resolution has exactly one source instead
// of being retyped — and occasionally rewrapped or reworded — in every
// skill that needs it.
const activeRootMarker = "<!-- parlay:expand-active-root -->"

const activeRootExpansion = "## Active root\n\n" +
	"Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:\n\n" +
	"- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.\n" +
	"- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.\n\n" +
	"When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{\"kind\":\"ambiguity\",...}`), re-prompt the user via AskUserQuestion with the listed candidate roots, then re-invoke with `--root <chosen>`."

// coEqualArtifactsMarker expands to the canonical one-clause definition
// of the four co-equal spec artifacts. Skills that need to name the set
// drop this marker in rather than re-deriving the definition in their
// own words each time they mention it.
const coEqualArtifactsMarker = "<!-- parlay:expand-co-equal-artifacts -->"

const coEqualArtifactsExpansion = "the four spec artifacts are co-equal — `surface.yaml` (or legacy `surface.md`), `capabilities.yaml`, `infrastructure.md`, and the project's `domain-model.yaml` — none is a stand-in for another"

// expandMarkers replaces the compact placeholder markers embedded skill
// authors use with their canonical expansions. Applied once, here, at
// the point every deployer (Claude, Cursor, Generic) reads skill
// content — so all three see identical expanded prose without
// re-implementing the substitution or risking the source text drifting
// between skills that both need it.
func expandMarkers(content []byte) []byte {
	content = bytes.ReplaceAll(content, []byte(activeRootMarker), []byte(activeRootExpansion))
	content = bytes.ReplaceAll(content, []byte(coEqualArtifactsMarker), []byte(coEqualArtifactsExpansion))
	return content
}

// parseSkillFrontmatter splits a raw skill source file into its
// frontmatter description and body. Every embedded skill source starts
// with a `---\nname: ...\ndescription: "..."\n---\n\n` block — this is
// the single source deployers read a skill's human-facing description
// from, instead of each deployer (or a hand-maintained title map)
// re-declaring it. A file with no leading `---` frontmatter block
// returns an empty description and the content unchanged, so malformed
// input degrades gracefully rather than panicking.
func parseSkillFrontmatter(content []byte) (description string, body []byte) {
	const open = "---\n"
	s := string(content)
	if !strings.HasPrefix(s, open) {
		return "", content
	}
	rest := s[len(open):]
	closeIdx := strings.Index(rest, "\n---")
	if closeIdx == -1 {
		return "", content
	}
	frontmatter := rest[:closeIdx]
	after := rest[closeIdx+len("\n---"):]
	after = strings.TrimPrefix(after, "\n") // rest of the closing "---" line
	after = strings.TrimPrefix(after, "\n") // the blank line separating frontmatter from body

	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "description:"); ok {
			description = strings.Trim(strings.TrimSpace(v), `"`)
		}
	}
	return description, []byte(after)
}

// ReadAllSkills returns all embedded skill files: name, frontmatter
// description, and body with deploy-time markers expanded (see
// expandMarkers).
func ReadAllSkills() ([]SkillEntry, error) {
	entries, err := fs.ReadDir(skillsFS, "skills")
	if err != nil {
		return nil, err
	}

	var skills []SkillEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := skillsFS.ReadFile("skills/" + entry.Name())
		if err != nil {
			return nil, err
		}
		// Strip .skill.md suffix to get the skill name
		name := entry.Name()
		if len(name) > 9 {
			name = name[:len(name)-9] // remove ".skill.md"
		}
		description, body := parseSkillFrontmatter(data)
		skills = append(skills, SkillEntry{Name: name, Description: description, Content: expandMarkers(body)})
	}
	return skills, nil
}
