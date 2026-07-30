// parlay-feature: studio-foundation/studio-deployer
// parlay-section: cross-cutting
// parlay-artifact: test
// parlay-extends: studio-foundation/studio-deployer/cross-cutting/embedded-source-and-deployer-subcommands

package embedded

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

// TestSkillsEmbedNonEmpty asserts the build-time //go:embed directive
// resolved to a non-empty tree containing parlay-design-loop. This is the
// manifest-derivation invariant: the embedded tree IS the source-of-truth
// the deployer fans out from.
func TestSkillsEmbedNonEmpty(t *testing.T) {
	slugs, err := ListSkills()
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(slugs) == 0 {
		t.Fatalf("ListSkills returned an empty slice; expected at least parlay-design-loop")
	}
	foundDesignLoop := false
	for _, s := range slugs {
		if s == "parlay-design-loop" {
			foundDesignLoop = true
			break
		}
	}
	if !foundDesignLoop {
		t.Fatalf("ListSkills did not return parlay-design-loop; got %v", slugs)
	}
}

// TestListSkillsDeterministic asserts ListSkills returns slugs in
// lexicographic order so downstream manifest derivation is deterministic.
func TestListSkillsDeterministic(t *testing.T) {
	first, err := ListSkills()
	if err != nil {
		t.Fatalf("ListSkills first call: %v", err)
	}
	second, err := ListSkills()
	if err != nil {
		t.Fatalf("ListSkills second call: %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("ListSkills returned different lengths on repeated calls: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("ListSkills not deterministic at index %d: %q vs %q", i, first[i], second[i])
		}
	}
	for i := 1; i < len(first); i++ {
		if first[i-1] >= first[i] {
			t.Fatalf("ListSkills not lexicographically ordered: %q !< %q", first[i-1], first[i])
		}
	}
}

// TestReadSkillValidFrontmatter asserts the production parlay-design-loop
// source contains the required frontmatter shape: "---" delimiters, "name:
// parlay-design-loop", and a "description:" key.
func TestReadSkillValidFrontmatter(t *testing.T) {
	content, err := ReadSkill("parlay-design-loop")
	if err != nil {
		t.Fatalf("ReadSkill(parlay-design-loop): %v", err)
	}
	s := string(content)
	lines := strings.Split(s, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		t.Fatalf("source does not begin with '---'; first line = %q", lines[0])
	}
	if !strings.Contains(s, "name: parlay-design-loop") {
		t.Fatalf("source does not contain 'name: parlay-design-loop'")
	}
	if !strings.Contains(s, "description:") {
		t.Fatalf("source does not contain a 'description:' key")
	}
}

// TestReadSkillMissingReturnsNotExist asserts that a non-existent skill
// returns a wrapped fs.ErrNotExist so callers can distinguish "not shipped"
// from "malformed".
func TestReadSkillMissingReturnsNotExist(t *testing.T) {
	_, err := ReadSkill("definitely-not-a-real-skill")
	if err == nil {
		t.Fatalf("expected an error for a non-existent skill; got nil")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected error to wrap fs.ErrNotExist; got %v", err)
	}
}

// TestFrontmatterRejectionsViaOverlay drives the frontmatter shape check
// against a handcrafted overlay embed.FS so the production embed.FS stays
// valid. Each case exercises one missing-required-thing path.
func TestFrontmatterRejectionsViaOverlay(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{
			name: "missing-frontmatter-delimiter",
			content: "name: bad\ndescription: bad\n\nbody\n",
		},
		{
			name: "missing-name-key",
			content: "---\ndescription: only-description\n---\n\nbody\n",
		},
		{
			name: "missing-description-key",
			content: "---\nname: only-name\n---\n\nbody\n",
		},
		{
			name: "unterminated-frontmatter",
			content: "---\nname: bad\ndescription: bad\nbody-without-closer\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			overlay := fstest.MapFS{
				"skills/bad.skill.md": &fstest.MapFile{Data: []byte(c.content)},
			}
			_, err := readSkill(overlay, "bad")
			if err == nil {
				t.Fatalf("expected an error; got nil")
			}
			if !errors.Is(err, ErrFrontmatterInvalid) {
				t.Fatalf("expected error to wrap ErrFrontmatterInvalid; got %v", err)
			}
			// The stable code string MUST appear in the error chain so
			// operators grepping for studio-embedded-skill-frontmatter-invalid
			// find the failure.
			if !strings.Contains(err.Error(), "studio-embedded-skill-frontmatter-invalid") {
				t.Fatalf("expected error message to mention the stable code; got %q", err.Error())
			}
		})
	}
}

// TestFrontmatterAcceptanceViaOverlay asserts a well-formed handcrafted
// source passes the shape check. The overlay path mirrors a production
// build cycle's read of a freshly-relocated source.
func TestFrontmatterAcceptanceViaOverlay(t *testing.T) {
	overlay := fstest.MapFS{
		"skills/good.skill.md": &fstest.MapFile{
			Data: []byte("---\nname: good\ndescription: a good skill\n---\n\nbody\n"),
		},
	}
	content, err := readSkill(overlay, "good")
	if err != nil {
		t.Fatalf("readSkill(overlay, good): %v", err)
	}
	if !strings.HasPrefix(string(content), "---") {
		t.Fatalf("readSkill returned content not starting with '---'; got %q", content[:5])
	}
}
