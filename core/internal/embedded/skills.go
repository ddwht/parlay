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

// decisionProtocolMarker is the placeholder a phase module drops in place
// of the "## Asking the user" section. Phase modules run inside the
// parlay-loop subagents, where the interactive tool does not exist — a
// prompt written there is silently skipped and the phase then answers its
// own question. The protocol out is to stop and hand a structured decision
// back to the driver, which owns all user interaction.
//
// Like activeRootExpansion, this has exactly one source: the block's shape
// is a wire format the driver matches on, so five hand-maintained copies
// would drift into five dialects.
const decisionProtocolMarker = "<!-- parlay:expand-decision-protocol -->"

const decisionProtocolExpansion = "## Asking the user\n\n" +
	"This skill runs as a **phase module** — normally inside a parlay-loop subagent, where no interactive tool exists. A question asked there is written into a transcript nobody reads, and you then answer it yourself; that is not a confirmation, it is a decision made on the user's behalf. So do not prompt. **Stop and return a decision request** as your final output. The driver prompts and resumes you with the chosen `id`, with your context intact, so you continue exactly where you stopped.\n\n" +
	"````\n" +
	"```yaml parlay-decision\n" +
	"kind: phase-boundary        # phase-boundary | override | overwrite | failure | ambiguity\n" +
	"phase: <the phase you are in>\n" +
	"question: \"<the one question, in the user's terms>\"\n" +
	"context: |\n" +
	"  <what you found, and what is already on disk>\n" +
	"options:\n" +
	"  - id: <slug>\n" +
	"    label: \"<what the user picks>\"\n" +
	"    detail: \"<the consequence, when it isn't obvious>\"\n" +
	"resume: \"Re-enter with decision: <id>. <what is written so far>\"\n" +
	"```\n" +
	"````\n\n" +
	"Leave the filesystem coherent before you stop — a decision is a pause, not a half-write. If you genuinely cannot pause at that point, take the option that preserves the user's work, never the one that destroys it, and say so in your report.\n\n" +
	"Two things not to do: never narrow the options to spare the user a question, and never resolve an ambiguity by taking the reading that is cheapest to implement. Both turn a decision the user should own into one you made quietly."

const activeRootExpansion = "## Active root\n\n" +
	"Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:\n\n" +
	"- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.\n" +
	"- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.\n\n" +
	"When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{\"kind\":\"ambiguity\",...}`), the root cannot be guessed — the candidates are real projects, and picking one writes into the wrong tree.\n\n" +
	"Who resolves it depends on where you are running. If you own the user interaction — the loop driver, or a skill the user invoked directly — prompt with the listed candidate roots and re-invoke with `--root <chosen>`. If you are a **phase module** running inside a subagent, you have no interactive tool: return an `ambiguity` decision request listing the candidates as options and let the driver ask."

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
	content = bytes.ReplaceAll(content, []byte(decisionProtocolMarker), []byte(decisionProtocolExpansion))
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
