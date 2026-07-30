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
//
// design-loop has since changed sides, which is worth being precise about
// because it looks like the invariant reversing. It has not. The rule is still
// "prune only what core owns"; what changed is that core now owns design-loop's
// retirement — the editor's deployer that installed it is deleted, and the slug
// is in retiredCoreSkills. Pruning it is now the deliberate act the list exists
// to enable, rather than the collateral damage it once was. The ownership
// distinction the original defect taught is unchanged, and the third-party case
// below is what still carries it.
func TestShouldPruneSkill_OnlyPrunesCoreOwnedSlugs(t *testing.T) {
	wanted := map[string]bool{"loop": true, "build-feature": true}

	if !shouldPruneSkill("repair", wanted) {
		t.Error("a retired core skill must be pruned — that is the point of the pass")
	}
	if shouldPruneSkill("loop", wanted) {
		t.Error("a currently-shipped skill must never be pruned")
	}
	if !shouldPruneSkill("design-loop", wanted) {
		t.Error("design-loop is retired by core now; upgrade must prune the stale copy from existing projects")
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
		// Fatal, not Skip. ReadAllSkills reads an embedded FS compiled into the
		// test binary — it cannot fail for an environmental reason, so a failure
		// here means the embedded skill set is broken, which would also break
		// every deploy. Skipping reported that as a yellow line nobody reads,
		// and the check it gates is the one that stops a slug being both retired
		// and shipped.
		t.Fatalf("ReadAllSkills failed: %v", err)
	}
	for _, s := range skills {
		if retiredCoreSkills[s.Name] {
			t.Errorf("%q is in retiredCoreSkills but is still shipped — remove it from the retired list", s.Name)
		}
	}
}
