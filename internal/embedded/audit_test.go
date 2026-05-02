package embedded

import (
	"strings"
	"testing"
)

// TestSkillSourceAudit fails the build when an embedded skill file
// contains hardcoded `.parlay/...` or `spec/...` path references but is
// missing the `<!-- parlay:active-root-aware -->` marker that signals
// the file's prose has been reviewed for multi-root awareness.
//
// The marker convention: every skill that mentions parlay-managed paths
// must include the marker AND a "## Active root" section explaining
// that paths are relative to the resolver-chosen active root. Adding a
// new skill that mentions paths without the marker fails this test.
func TestSkillSourceAudit(t *testing.T) {
	skills, err := ReadAllSkills()
	if err != nil {
		t.Fatalf("ReadAllSkills: %v", err)
	}

	// Path tokens that imply multi-root awareness is required. We match
	// the literal directory prefix; a skill that talks abstractly about
	// "the active root's spec/intents/" still contains the substring,
	// which is fine — the marker must accompany it either way.
	pathTokens := []string{
		".parlay/",
		"spec/intents/",
		"spec/handoff/",
		"spec/pages/",
	}

	for _, skill := range skills {
		body := string(skill.Content)
		hasPath := false
		for _, tok := range pathTokens {
			if strings.Contains(body, tok) {
				hasPath = true
				break
			}
		}
		if !hasPath {
			continue
		}
		if !strings.Contains(body, "parlay:active-root-aware") {
			t.Errorf("skill %s mentions parlay-managed paths but lacks the `<!-- parlay:active-root-aware -->` marker. Add an `## Active root` section explaining that paths resolve against the active root.",
				skill.Name)
		}
		if !strings.Contains(body, "## Active root") {
			t.Errorf("skill %s has the active-root marker but no `## Active root` section. Add one.",
				skill.Name)
		}
	}
}
