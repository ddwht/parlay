package embedded

import (
	"github.com/ddwht/parlay/core/internal/atomicfile"

	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
	Surface     Surface
}

// Surface says where a skill belongs on the agent surface.
//
// The distinction exists because two very different things were sharing one
// namespace. A designer opening the skill menu saw 24 entries and had to
// know which of five migrations fit their project, or which of five phases
// came next — when the tool can answer both by looking at the project. Yet
// the phase content itself is substantial and good; the problem was never
// the prose, only that every piece of it was also a menu entry.
//
// So: SurfaceCommand skills are the ones a person invokes by name.
// SurfaceModule skills are loaded by the driver or a phase subagent, which
// knows which one it needs. Same source tree, same authoring rules, same
// marker expansion — different destination.
type Surface string

const (
	// SurfaceCommand deploys to the agent's skill menu (.claude/skills/,
	// .cursor/rules/). This is the default when frontmatter omits `surface:`.
	SurfaceCommand Surface = "command"

	// SurfaceModule deploys to .parlay/modules/<name>.md, readable by any
	// agent but absent from the menu.
	SurfaceModule Surface = "module"
)

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
	"default: <id>               # advancement kinds ONLY — see below\n" +
	"resume: \"Re-enter with decision: <id>. <what is written so far>\"\n" +
	"```\n" +
	"````\n\n" +
	"**The `default:` field.** It names the one option id a driver running `--non-interactive` may take without asking. It exists so an unattended run has a defined answer rather than an inferred one, and it must be an id from your own `options:` list.\n\n" +
	"Only the two advancement kinds may carry a default: `phase-boundary` (normally `proceed`) and `override` (your recommended set). Those are decisions where one answer is the recommendation and the others are the user electing to intervene — taking the recommendation unattended is what the user asked for by passing the flag.\n\n" +
	"The other three kinds must NOT carry one, and a driver must abort rather than invent one, because on each of them every available answer is wrong in a way the user would want to know about:\n\n" +
	"- `ambiguity` — the protocol already forbids resolving one by taking the cheapest reading. A flag must not become the exception that makes it allowed.\n" +
	"- `overwrite` — one answer destroys work that may have been hand-edited; the other ships a prototype that diverges from its spec. There is no safe default, only a choice about which loss is acceptable.\n" +
	"- `failure` — the safe-looking answer proceeds past a suite that did not pass, which is the one outcome a CI run exists to prevent.\n\n" +
	"So: when you raise one of those three, omit `default:`. Adding one does not make the run smoother; it makes an unattended run take an action nobody authorized.\n\n" +
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
func parseSkillFrontmatter(content []byte) (description string, surface Surface, body []byte) {
	surface = SurfaceCommand
	const open = "---\n"
	s := string(content)
	if !strings.HasPrefix(s, open) {
		return "", surface, content
	}
	rest := s[len(open):]
	closeIdx := strings.Index(rest, "\n---")
	if closeIdx == -1 {
		return "", surface, content
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
		if v, ok := strings.CutPrefix(line, "surface:"); ok {
			// An unrecognized value falls back to command rather than
			// erroring: the failure mode of a typo'd `surface:` should be a
			// skill that shows up in the menu, not one that silently
			// vanishes from it.
			if Surface(strings.Trim(strings.TrimSpace(v), `"`)) == SurfaceModule {
				surface = SurfaceModule
			}
		}
	}
	return description, surface, []byte(after)
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
		description, surface, body := parseSkillFrontmatter(data)
		skills = append(skills, SkillEntry{
			Name:        name,
			Description: description,
			Content:     expandMarkers(body),
			Surface:     surface,
		})
	}
	return skills, nil
}

// CommandSkills returns the subset that belongs on the agent's skill menu.
func CommandSkills(all []SkillEntry) []SkillEntry {
	return filterSurface(all, SurfaceCommand)
}

// ModuleSkills returns the subset the driver and phase subagents load by
// path rather than by name.
func ModuleSkills(all []SkillEntry) []SkillEntry {
	return filterSurface(all, SurfaceModule)
}

func filterSurface(all []SkillEntry, want Surface) []SkillEntry {
	var out []SkillEntry
	for _, s := range all {
		if s.Surface == want {
			out = append(out, s)
		}
	}
	return out
}

// WriteModules materializes the module-surface skills into targetDir (by
// convention .parlay/modules/). Returns the number it actually wrote — a module
// whose on-disk copy already matches is skipped, so a re-deploy over unchanged
// sources returns 0.
//
// Modules land at the repo-level root next to the schemas, not in an
// agent-specific directory: the content is adapter-independent, and a phase
// subagent reads it by path regardless of which agent is driving. Writing
// it once here rather than once per deployer also means Claude and Cursor
// projects cannot drift to different phase instructions.
func WriteModules(targetDir string) (int, error) {
	all, err := ReadAllSkills()
	if err != nil {
		return 0, err
	}
	modules := ModuleSkills(all)
	if len(modules) == 0 {
		return 0, nil
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return 0, err
	}
	written := 0
	for _, m := range modules {
		content := fmt.Sprintf("# %s\n\n_%s_\n\n%s", m.Name, m.Description, string(m.Content))
		dst := filepath.Join(targetDir, m.Name+".md")
		wrote, err := atomicfile.WriteIfChanged(dst, []byte(content))
		if err != nil {
			return written, err
		}
		if wrote {
			written++
		}
	}
	return written, nil
}

// PruneStaleModules removes .parlay/modules/<name>.md files that no longer
// correspond to an embedded module. Without it, a module renamed or
// promoted back to a command leaves a stale copy on disk that a phase
// subagent would happily read — stale instructions being strictly worse
// than missing ones, because nothing reports them.
func PruneStaleModules(targetDir string) error {
	all, err := ReadAllSkills()
	if err != nil {
		return err
	}
	wanted := map[string]bool{}
	for _, m := range ModuleSkills(all) {
		wanted[m.Name+".md"] = true
	}
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		// Missing directory on a fresh project — nothing to prune.
		return nil
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Orphan .tmp debris. atomicfile writes through a `<target>.tmp`
		// sibling, and a crash between its creation and the rename leaves one
		// behind. For a target still in the set that self-heals — the next write
		// opens the same .tmp with O_TRUNC and renames it — but a target that
		// leaves the set is never written again, so its .tmp would sit in the
		// user's .parlay/ indefinitely. This is where that gets collected.
		//
		// The editor's deployer swept these from a manifest path set at run
		// start. There is no manifest here, and none is needed: the wanted-set
		// check below already decides ownership, and a `<name>.md.tmp` whose
		// `<name>.md` we would not deploy is ours to remove.
		if strings.HasSuffix(name, ".md.tmp") {
			if wanted[strings.TrimSuffix(name, ".tmp")] {
				continue
			}
			if err := os.Remove(filepath.Join(targetDir, name)); err != nil {
				return err
			}
			continue
		}
		if wanted[name] || !strings.HasSuffix(name, ".md") {
			continue
		}
		if err := os.Remove(filepath.Join(targetDir, name)); err != nil {
			return err
		}
	}
	return nil
}
