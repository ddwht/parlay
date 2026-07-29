package deployer

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
}

// shouldPruneSkill reports whether a deployed parlay-<slug> skill directory
// should be removed: it must be a slug core owns (currently or historically)
// and must not be in the currently-wanted set.
func shouldPruneSkill(slug string, wanted map[string]bool) bool {
	if wanted[slug] {
		return false
	}
	return retiredCoreSkills[slug]
}
