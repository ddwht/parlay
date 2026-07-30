// parlay-feature: studio-foundation/studio-deployer
// parlay-section: cross-cutting
// parlay-extends: studio-foundation/studio-deployer/cross-cutting/embedded-source-and-deployer-subcommands

// Package embedded owns the //go:embed directives that bake the Studio source
// surface into the parlay-studio binary at link time. The directory layout
// mirrors core/internal/embedded/: subdirectories (currently only skills/)
// each have a sibling embed.FS value. Future Studio features may add
// agents/, schemas/, adapters/ trees as additional embed.FS values.
//
// The exposed surface is intentionally small:
//
//	Skills           — the embedded skills directory tree
//	ReadSkill(slug)  — reads skills/<slug>.skill.md and validates frontmatter
//	ListSkills()     — returns embedded skill slugs in deterministic order
//
// ReadSkill enforces a defense-in-depth frontmatter shape check on every
// read: the source MUST begin with a "---" line, contain a "name:" key, and
// contain a "description:" key. A malformed source fails the call with the
// stable code studio-embedded-skill-frontmatter-invalid even when the
// binary was built with the malformed source (the check is also enforced at
// build time via embed_test.go).
package embedded

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// Skills carries every Studio skill source embedded into the binary at link
// time. The directive matches skills/*.skill.md inside this package.
//
//go:embed skills/*.skill.md
var Skills embed.FS

// ErrFrontmatterInvalid is the stable sentinel for malformed skill sources.
// Operator-facing messages wrap it with %w so callers can errors.Is against
// the sentinel; the stable code string is the wrapped error's identity.
var ErrFrontmatterInvalid = errors.New("studio-embedded-skill-frontmatter-invalid")

// skillsDir is the relative directory inside the embed.FS where skill
// sources live. The literal is shared with the //go:embed directive above
// so a typo on either side surfaces at compile time (the embed.FS would
// resolve to an empty tree).
const skillsDir = "skills"

// skillFileSuffix names the on-disk suffix every embedded skill source
// carries. The slug returned by ListSkills strips this suffix.
const skillFileSuffix = ".skill.md"

// ReadSkill returns the embedded source bytes for skills/<slug>.skill.md.
//
// The slug is the skill identity without the .skill.md suffix (e.g.
// "parlay-design-loop"). On a non-existent skill, ReadSkill returns a
// wrapped fs.ErrNotExist; callers can errors.Is(err, fs.ErrNotExist) to
// distinguish "the skill isn't shipped" from "the skill is malformed".
//
// On a malformed source (missing frontmatter, missing name key, or missing
// description key), ReadSkill returns a wrapped ErrFrontmatterInvalid.
// The shape check runs on every read so the failure surfaces at deployer
// invocation time even when the binary was built with a malformed source.
func ReadSkill(slug string) ([]byte, error) {
	return readSkill(Skills, slug)
}

// ListSkills returns the slugs of every embedded skill in deterministic
// (lexicographic) order. The order is observable downstream — the deployer
// derives its manifest by iterating ListSkills × DetectAgentSurfaces — so
// determinism here propagates to determinism in the manifest.
func ListSkills() ([]string, error) {
	return listSkills(Skills)
}

// readSkill is the embed.FS-agnostic implementation of ReadSkill. It is
// exposed package-private so embed_test.go can drive a handcrafted overlay
// embed.FS against the same shape check without mutating the production
// Skills value.
func readSkill(fsys fs.FS, slug string) ([]byte, error) {
	name := path.Join(skillsDir, slug+skillFileSuffix)
	content, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("embedded.ReadSkill(%q): %w", slug, err)
	}
	if err := validateFrontmatter(content); err != nil {
		return nil, fmt.Errorf("embedded.ReadSkill(%q): %w", slug, err)
	}
	return content, nil
}

// listSkills is the embed.FS-agnostic implementation of ListSkills.
func listSkills(fsys fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(fsys, skillsDir)
	if err != nil {
		return nil, fmt.Errorf("embedded.ListSkills: %w", err)
	}
	slugs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, skillFileSuffix) {
			continue
		}
		slugs = append(slugs, strings.TrimSuffix(name, skillFileSuffix))
	}
	sort.Strings(slugs)
	return slugs, nil
}

// validateFrontmatter enforces the skill-source shape contract: the source
// MUST begin with a "---" line, MUST contain a "name:" key inside the
// opening frontmatter block, and MUST contain a "description:" key. A
// failure returns a wrapped ErrFrontmatterInvalid; success returns nil.
//
// The check is deliberately structural — not a full YAML parse — so it
// remains cheap enough to run on every ReadSkill call and the failure
// mode is the same whether the source is malformed YAML or simply missing
// the required keys.
func validateFrontmatter(content []byte) error {
	// The frontmatter block is the leading "---\n" ... "\n---\n" pair.
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return fmt.Errorf("%w: source must begin with a '---' frontmatter delimiter", ErrFrontmatterInvalid)
	}
	closeIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closeIdx = i
			break
		}
	}
	if closeIdx == -1 {
		return fmt.Errorf("%w: source's frontmatter block is not terminated by a '---' line", ErrFrontmatterInvalid)
	}
	hasName := false
	hasDescription := false
	for _, line := range lines[1:closeIdx] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "name:") {
			hasName = true
		}
		if strings.HasPrefix(trimmed, "description:") {
			hasDescription = true
		}
	}
	if !hasName {
		return fmt.Errorf("%w: frontmatter block is missing a 'name:' key", ErrFrontmatterInvalid)
	}
	if !hasDescription {
		return fmt.Errorf("%w: frontmatter block is missing a 'description:' key", ErrFrontmatterInvalid)
	}
	return nil
}
