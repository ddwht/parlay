package deployer

import (
	"testing"

	"github.com/ddwht/parlay/core/internal/embedded"
)

// TestShouldPruneSkill_OnlyPrunesCoreOwnedSlugs guards a data-loss defect
// verified in practice: after retiring five core skills, `parlay upgrade`
// deleted parlay-design-loop — a skill parlay-studio had installed into the
// same .claude/skills/ directory. The prune matched on the parlay- prefix
// alone, so it could not tell "a core skill I retired" from "another tool's
// skill". The next design-loop invocation would have found no skill, with
// nothing reporting why.
func TestShouldPruneSkill_OnlyPrunesCoreOwnedSlugs(t *testing.T) {
	wanted := map[string]bool{"loop": true, "build-feature": true}

	if !shouldPruneSkill("repair", wanted) {
		t.Error("a retired core skill must be pruned — that is the point of the pass")
	}
	if shouldPruneSkill("loop", wanted) {
		t.Error("a currently-shipped skill must never be pruned")
	}
	if shouldPruneSkill("design-loop", wanted) {
		t.Error("parlay-design-loop is deployed by parlay-studio into the shared agent surface and must survive a core upgrade")
	}
	if shouldPruneSkill("some-third-party-thing", wanted) {
		t.Error("an unknown parlay-prefixed skill belongs to someone else and must be left alone")
	}
}

// TestRetiredCoreSkills_DisjointFromShipped keeps the list honest: a slug
// cannot be both retired and currently shipped.
func TestRetiredCoreSkills_DisjointFromShipped(t *testing.T) {
	skills, err := embedded.ReadAllSkills()
	if err != nil {
		t.Skipf("ReadAllSkills unavailable: %v", err)
	}
	for _, s := range skills {
		if retiredCoreSkills[s.Name] {
			t.Errorf("%q is in retiredCoreSkills but is still shipped — remove it from the retired list", s.Name)
		}
	}
}
