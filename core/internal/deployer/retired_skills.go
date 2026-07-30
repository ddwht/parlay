package deployer

import "github.com/ddwht/parlay/core/internal/embedded"

// retiredCoreSkills lists skill slugs that parlay core has shipped in the
// past and no longer ships. The stale-skill prune removes a deployed
// .claude/skills/parlay-<slug>/ (or .cursor equivalent) only when its slug
// is either currently wanted or listed here.
//
// Why an explicit list rather than "delete anything parlay-prefixed that we
// don't currently ship": the agent surface is shared. parlay-studio deploys
// parlay-design-loop into the same .claude/skills/ directory, and other
// tools may add their own parlay-prefixed skills. A blanket prune deletes
// them — verified: after retiring five core skills, `parlay upgrade`
// silently removed parlay-design-loop, which `parlay-studio init` had
// installed moments earlier. The user's next design-loop invocation would
// simply have found no skill.
//
// Cost of this design: retiring a skill means adding its slug here in the
// same change. That is one line, and it makes every deletion the deployer
// performs auditable — which a wildcard never can be.
var retiredCoreSkills = map[string]bool{
	// Retired in 0.2.0 — thin wrappers whose entire Steps section was one
	// or two CLI calls. The commands remain; only the skill indirection is
	// gone. See `parlay new-initiative`, `parlay register-adapter`,
	// `parlay lock-page`, `parlay repair`, `parlay view-page`.
	"new-initiative":   true,
	"register-adapter": true,
	"lock-page":        true,
	"repair":           true,
	"view-page":        true,

	// Retired in 0.2.0 — folded into the doctor skill, which decides which
	// of these apply by inspecting the project rather than requiring the
	// designer to know which of five migrations fits their situation.
	// Every underlying CLI command still exists.
	"sync":                      true,
	"collect-questions":         true,
	"review-coverage":           true,
	"migrate-spec":              true,
	"migrate-config":            true,
	"migrate-capabilities":      true,
	"migrate-domain-model":      true,
	"migrate-domain-operations": true,

	// Retired in 0.2.0 with the editor's own deployer. design-loop was the only
	// skill that deployer ever embedded, and the round-trip it drove — push a
	// canonical layout to Figma via the host agent's MCP connection, read it
	// back, classify the designer's edits — has no replacement on the parlay
	// surface. Retiring it is what leaves that deployer with no work.
	//
	// Note the comment above about why this list exists at all: the case it
	// describes is this skill. parlay-studio installed parlay-design-loop into
	// the same .claude/skills/ a blanket prune would have swept. Core now owns
	// the slug's retirement, which is the only way the prune can remove it
	// deliberately rather than as collateral.
	"design-loop": true,
}

// shouldPruneSkill reports whether a deployed parlay-<slug> skill directory
// should be removed: it must be a slug core owns (currently or historically)
// and must not be in the currently-wanted set.
// "Owns" covers three cases. A slug in retiredCoreSkills is one we shipped
// and dropped. A slug still in the embedded set but no longer command-
// surface — a phase module — was on the menu in an earlier version and must
// come off it now. Anything else belongs to another tool and is left alone.
func shouldPruneSkill(slug string, wanted map[string]bool) bool {
	if wanted[slug] {
		return false
	}
	if retiredCoreSkills[slug] {
		return true
	}
	return isDemotedCoreSkill(slug)
}

// isDemotedCoreSkill reports whether the slug is still shipped by core but
// has moved off the agent menu to the module surface. Such a slug needs
// pruning from .claude/skills/ for the same reason a retired one does —
// otherwise last version's copy stays on the menu, and a designer invoking
// it gets stale instructions with nothing saying so.
func isDemotedCoreSkill(slug string) bool {
	all, err := embedded.ReadAllSkills()
	if err != nil {
		// Cannot prove ownership — leave the directory alone. Deleting on a
		// read error risks removing another tool's skill.
		return false
	}
	for _, s := range all {
		if s.Name == slug {
			return s.Surface == embedded.SurfaceModule
		}
	}
	return false
}
