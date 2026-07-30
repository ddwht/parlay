package agent

// parlay-feature: parlay-tool/schema-consolidation
// parlay-component: validate-go-split-project-pass
//
// Project-pass validation, split out of validate.go (Phase 6.3): walking
// every buildfile under a root, cross-feature plannedCreates/cycle
// detection, cross-cutting entry classification and target-pattern
// resolution, and the plan: section validator shared by both
// single-feature and project-pass callers.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// FeatureVerdict is the per-feature result emitted by
// ValidateBuildfilesProjectStructured. Each verdict carries the buildfile
// path, the feature slug (extracted from the buildfile), and the structured
// errors produced by the deep validator.
//
// parlay-extends: parlay-tool/cross-cutting-target-paths/project-pass-validation-and-cli-flag
type FeatureVerdict struct {
	Feature       string            `json:"feature"`
	BuildfilePath string            `json:"buildfile_path"`
	Errors        []ValidationError `json:"errors,omitempty"`
}

// ValidateBuildfilesProjectStructured validates every buildfile under
// rootDir/.parlay/build/**/buildfile.yaml in project-pass mode. The cross-
// feature plannedCreates map is built once from the union of every
// feature's plan.creates rows; each feature is then validated with the
// union-minus-self map threaded into validatePlanSection. The pass also
// detects create-modify cycles between features and emits one
// plan-create-modify-cycle error per cycle edge.
//
// Returns one FeatureVerdict per feature, in lexicographic feature-slug
// order for determinism. The aggregate exit-code semantics are owned by
// the caller (commands/validate.go).
//
// parlay-extends: parlay-tool/cross-cutting-target-paths/project-pass-validation-and-cli-flag
func ValidateBuildfilesProjectStructured(rootDir string) ([]FeatureVerdict, error) {
	buildRoot := filepath.Join(rootDir, ".parlay", "build")
	info, statErr := os.Stat(buildRoot)
	if statErr != nil || !info.IsDir() {
		// Empty project — no buildfiles. Caller decides how to surface.
		return nil, nil
	}

	// Walk for buildfile.yaml under the build root. Skip the project-
	// internal ".parlay/build/_project" directory (no feature slug).
	type loaded struct {
		path string
		bf   deepBuildfile
		slug string
	}
	var loadedFeatures []loaded
	walkErr := filepath.Walk(buildRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(path) != "buildfile.yaml" {
			return nil
		}
		// Ignore _project internal sidecar directory.
		rel, relErr := filepath.Rel(buildRoot, path)
		if relErr == nil {
			parts := strings.Split(rel, string(filepath.Separator))
			if len(parts) > 0 && parts[0] == "_project" {
				return nil
			}
		}
		data, rdErr := os.ReadFile(path)
		if rdErr != nil {
			return nil
		}
		var bf deepBuildfile
		if yamlErr := yaml.Unmarshal(data, &bf); yamlErr != nil {
			// Malformed buildfile becomes a feature with a single
			// invalid-yaml error.
			loadedFeatures = append(loadedFeatures, loaded{path: path, slug: featureSlugFromPath(path, buildRoot), bf: deepBuildfile{}})
			return nil
		}
		slug := bf.Feature
		if slug == "" {
			slug = featureSlugFromPath(path, buildRoot)
		}
		loadedFeatures = append(loadedFeatures, loaded{path: path, bf: bf, slug: slug})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	// Sort for determinism.
	sort.Slice(loadedFeatures, func(i, j int) bool {
		return loadedFeatures[i].slug < loadedFeatures[j].slug
	})

	// Build the project-wide plannedCreates map (path -> "feature/<slug>").
	plannedCreatesAll := map[string]string{}
	creatorOf := map[string]string{} // path -> feature slug
	for _, lf := range loadedFeatures {
		if lf.bf.Plan == nil {
			continue
		}
		for _, c := range lf.bf.Plan.Creates {
			if c.Path == "" {
				continue
			}
			plannedCreatesAll[c.Path] = "feature/" + lf.slug
			creatorOf[c.Path] = lf.slug
		}
	}

	// Detect create-modify cycles between features. An edge runs from
	// feature A to feature B when A's plan.modifies path matches B's
	// plan.creates path: A "depends on" B for the file's existence.
	// Cycles in this graph mean two features each wait on the other.
	deps := map[string]map[string]bool{} // a -> set of b
	// edgePath records, for each (modifier, creator) edge, one
	// representative path the modifier names that the creator promises.
	edgePath := map[string]map[string]string{}
	for _, lf := range loadedFeatures {
		if lf.bf.Plan == nil {
			continue
		}
		for _, m := range lf.bf.Plan.Modifies {
			if m.Path == "" {
				continue
			}
			creator, ok := creatorOf[m.Path]
			if !ok || creator == lf.slug {
				continue
			}
			if deps[lf.slug] == nil {
				deps[lf.slug] = map[string]bool{}
			}
			deps[lf.slug][creator] = true
			if edgePath[lf.slug] == nil {
				edgePath[lf.slug] = map[string]string{}
			}
			if _, exists := edgePath[lf.slug][creator]; !exists {
				edgePath[lf.slug][creator] = m.Path
			}
		}
	}
	cycleEdges := detectCycleEdges(deps)
	// Fill cycle paths from edgePath.
	for i := range cycleEdges {
		e := &cycleEdges[i]
		if p, ok := edgePath[e.From][e.To]; ok {
			e.Path = p
		}
		if p, ok := edgePath[e.To][e.From]; ok {
			e.OtherPath = p
		}
	}

	verdicts := make([]FeatureVerdict, 0, len(loadedFeatures))
	for _, lf := range loadedFeatures {
		// Build plannedCreates as union-minus-self.
		minusSelf := map[string]string{}
		for path, src := range plannedCreatesAll {
			if creatorOf[path] == lf.slug {
				continue
			}
			minusSelf[path] = src
		}
		errs := validateBuildfileDeepCore(lf.path, "", minusSelf)
		// Append cycle errors that name this feature.
		for _, e := range cycleEdges {
			if e.From == lf.slug {
				errs = append(errs, ValidationError{
					Code:    "plan-create-modify-cycle",
					Message: fmt.Sprintf("feature %q's plan.modifies %q is created by feature %q, but %q's plan.modifies %q is created by %q — create/modify cycle", e.From, e.Path, e.To, e.To, e.OtherPath, e.From),
					Context: fmt.Sprintf("plan / cross-feature[%s <-> %s]", e.From, e.To),
					Fix:     "split one cross-cutting concern across two passes, or merge the two features so the create/modify edge stays within one feature",
				})
			}
		}
		// Which fixture this feature contributes to the composed seed. It is
		// a project-pass check because that is the only pass that knows a
		// project has more than one feature: one feature booting from
		// whichever fixture it likes is harmless, but as soon as features
		// share a runtime, each has to say which of its fixtures is the real
		// one, and the union is undefined until they do.
		if _, ambiguity := ComposingFixture(filepath.Dir(lf.path)); ambiguity != "" {
			errs = append(errs, ValidationError{
				Code:    "composition-seed-ambiguous",
				Message: fmt.Sprintf("feature %q: %s", lf.slug, ambiguity),
				Context: "fixtures",
				Fix:     "mark exactly one fixture `composes: true` — the one whose data the running prototype boots with",
			})
		}

		// Stamp severity from the shared table, exactly as
		// ValidateBuildfileDeepStructured does for the single-feature path.
		//
		// This call was missing, and its absence is what made `validate
		// --project` fail permanently on any project whose code has been
		// generated. Every finding came back with Severity == "", the command
		// counted all of them as errors, and plan-create-collision — which
		// the shared table grades a warning in both modes, for the reason
		// recorded there ("grading it blocking makes a correct buildfile
		// un-revalidatable the moment codegen runs") — failed the whole
		// project pass. It grew too: one finding per planned file per
		// generated feature, and the fix it suggested ("move the entry to
		// plan.modifies") would have misdescribed the plan.
		//
		// check-buildfile already read the same findings correctly, so the
		// two commands returned opposite verdicts on the same buildfile at
		// the same moment. Applied after the cycle errors so those are graded
		// too.
		errs = ApplyBuildfileSeverity(errs)
		verdicts = append(verdicts, FeatureVerdict{
			Feature:       lf.slug,
			BuildfilePath: lf.path,
			Errors:        errs,
		})
	}
	return verdicts, nil
}

// featureSlugFromPath derives "feature-slug" or "initiative/feature-slug"
// from a buildfile path inside .parlay/build/. Used as a fallback when the
// buildfile lacks a `feature:` field; ValidateBuildfilesProjectStructured
// prefers the in-file feature when present.
func featureSlugFromPath(buildfilePath, buildRoot string) string {
	rel, err := filepath.Rel(buildRoot, buildfilePath)
	if err != nil {
		return ""
	}
	dir := filepath.Dir(rel)
	if dir == "." {
		return ""
	}
	return filepath.ToSlash(dir)
}

// cycleEdge represents a single create-modify cycle between two features.
type cycleEdge struct {
	From      string // feature whose plan.modifies points at To's plan.creates
	To        string
	Path      string // the From->To path
	OtherPath string // the To->From path that closes the cycle
}

// detectCycleEdges finds 2-cycles (A modifies B's create, B modifies A's
// create) in the cross-feature dependency map. A 2-cycle is the simplest
// pathology and the most likely in practice; longer cycles fall through
// without an error, which is acceptable for this iteration — the
// signalling intent is to flag obvious authoring mistakes, not to resolve
// arbitrary dependency graphs.
//
// Each cycle is reported once per feature (the From end), so two
// FeatureVerdicts each carry one cycle error naming the same pair.
func detectCycleEdges(deps map[string]map[string]bool) []cycleEdge {
	var edges []cycleEdge
	seenPair := map[string]bool{}
	for from, tos := range deps {
		for to := range tos {
			// Self-edges are impossible by construction (creator == self
			// is filtered upstream). Pair key is order-independent so we
			// emit two records, one per side.
			pair := from + "->" + to
			if seenPair[pair] {
				continue
			}
			if deps[to] != nil && deps[to][from] {
				// We need to attach paths. The caller passes only edge
				// existence here; that's enough to flag the cycle. The
				// per-side path attribution is recovered when emitting
				// errors by re-walking the buildfiles in the caller.
				// For now, leave Path/OtherPath empty — the caller
				// fills them.
				edges = append(edges, cycleEdge{From: from, To: to})
				edges = append(edges, cycleEdge{From: to, To: from})
				seenPair[from+"->"+to] = true
				seenPair[to+"->"+from] = true
			}
		}
	}
	return edges
}

// classifyCrossCuttingEntry returns the entry's kind based on the explicit
// shape (target-creates declared) or, falling back, on per-path on-disk
// presence. The classification drives the kind-aware routing in
// validatePlanSection: "purely-introducing" entries route to plan.creates,
// "modifies-only" route to plan.modifies, "two-kinded" entries split
// target-files (modifies) from target-creates (creates), and "mixed"
// (heuristic-derived ambiguity) is rejected.
//
// rootDir is the project root used to stat target-files paths; pass "" to
// skip the on-disk check (in which case classification falls back to
// "modifies-only" for non-empty target-files entries — the legacy default).
//
// parlay-extends: parlay-tool/cross-cutting-target-paths/validator-classify-entry-kind-and-route
func classifyCrossCuttingEntry(entry deepCrossCuttingEntry, rootDir string) string {
	return classifyCrossCuttingEntryWithPlan(entry, rootDir, nil)
}

// classifyCrossCuttingEntryWithPlan is classifyCrossCuttingEntry with the
// entry's own authored plan.creates paths supplied, which makes the
// classification stable across the build→codegen boundary.
//
// The on-disk heuristic below asks "do these target files exist?" — a
// question whose answer codegen itself changes. An entry authored as
// purely-introducing (target absent → routed to plan.creates) was
// reclassified modifies-only the moment codegen created that very file, and
// then demanded a plan.modifies row that was never appropriate. The
// buildfile did not change; the world did. That made a correct buildfile
// fail re-validation immediately after its own code was generated, breaking
// CI re-checks, pre-commit hooks, and any regeneration pass.
//
// plannedCreatesForEntry is the set of paths this entry's own plan.creates
// rows claim. A path listed there was authored as "this entry introduces
// it", so its later presence on disk is the expected result of following
// the plan, not evidence of misclassification. The plan is the recorded
// intent; disk is only the current state.
func classifyCrossCuttingEntryWithPlan(entry deepCrossCuttingEntry, rootDir string, plannedCreatesForEntry map[string]bool) string {
	// Explicit two-kinded shape wins over the heuristic.
	if len(entry.TargetCreates) > 0 {
		return "two-kinded"
	}
	// With no rootDir, we cannot stat — preserve legacy modifies-only routing
	// so existing callers (single-feature without a resolvable root) keep
	// their byte-for-byte behavior.
	if rootDir == "" {
		return "modifies-only"
	}
	if len(entry.TargetFiles) == 0 {
		// No explicit files; pattern resolution is handled separately.
		return "modifies-only"
	}
	// Authored intent wins over current disk state: if every target file is
	// one this entry's own plan.creates claims, it is purely-introducing
	// regardless of whether codegen has since created those files.
	if len(plannedCreatesForEntry) > 0 {
		allPlannedCreates := true
		for _, p := range entry.TargetFiles {
			if !plannedCreatesForEntry[p] {
				allPlannedCreates = false
				break
			}
		}
		if allPlannedCreates {
			return "purely-introducing"
		}
	}
	allMissing := true
	allPresent := true
	for _, p := range entry.TargetFiles {
		abs := filepath.Join(rootDir, p)
		if _, err := os.Stat(abs); err == nil {
			allMissing = false
		} else if os.IsNotExist(err) {
			allPresent = false
		} else {
			// Any other stat error: treat as present to avoid spurious
			// purely-introducing classification on a permissions glitch.
			allMissing = false
		}
	}
	switch {
	case allPresent && !allMissing:
		return "modifies-only"
	case allMissing && !allPresent:
		return "purely-introducing"
	default:
		return "mixed"
	}
}

// resolveTargetPattern walks rootDir and the optional plannedCreates set,
// collecting paths satisfying filepath.Match(pattern, p). Returns a sorted
// (lexicographic) deduplicated list. Pure, deterministic, and shell-free.
//
// plannedCreates is the cross-pass set of paths the project-pass validator
// has already discovered as plan.creates rows in sibling features (key:
// path, value: producing-feature attribution). Single-feature callers pass
// nil to limit resolution to on-disk files.
//
// Patterns using filepath.Match-unsupported features (recursive `**`, brace
// expansion) typically resolve to zero paths; the caller surfaces
// "cross-cutting-pattern-empty" with the same message either way.
//
// parlay-extends: parlay-tool/cross-cutting-target-paths/validator-resolve-target-pattern-at-validation-time
func resolveTargetPattern(pattern string, rootDir string, plannedCreates map[string]string) []string {
	if pattern == "" {
		return nil
	}
	seen := map[string]bool{}
	if rootDir != "" {
		_ = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return nil
			}
			rel, relErr := filepath.Rel(rootDir, path)
			if relErr != nil {
				return nil
			}
			ok, mErr := filepath.Match(pattern, rel)
			if mErr == nil && ok {
				seen[rel] = true
			}
			return nil
		})
	}
	for p := range plannedCreates {
		ok, err := filepath.Match(pattern, p)
		if err == nil && ok {
			seen[p] = true
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	// Lexicographic sort for determinism.
	sort.Strings(out)
	return out
}

// validatePlanSection enforces the executable contract of the plan:
// section. Every components: entry must produce at least one plan row;
// every cross-cutting entry's target-files / target-pattern / target-creates
// must route to the correct plan section by entry kind; modify-paths must
// exist on disk (or appear in another feature's plan.creates in project-pass
// mode); create-paths must NOT exist.
//
// The function resolves paths relative to the directory containing the
// buildfile's source root, derived from the buildfile's location:
// .parlay/build/<feature>/buildfile.yaml lives at <root>/.parlay/build/<feature>/.
// Source-tree paths are interpreted relative to <root>.
//
// plannedCreates is the cross-feature set of paths that sibling features'
// plan.creates rows promise to produce. In project-pass mode (driven by
// ValidateBuildfilesProjectStructured), this map is the union-minus-self of
// every other feature's plan.creates paths and lets a plan.modifies path
// satisfy its existence check via a sibling create. In single-feature mode,
// plannedCreates is nil and behavior is byte-identical to the pre-feature
// outcome.
//
// parlay-extends: parlay-tool/cross-cutting-target-paths/validator-classify-entry-kind-and-route
// parlay-extends: parlay-tool/cross-cutting-target-paths/validator-resolve-target-pattern-at-validation-time
// parlay-extends: parlay-tool/cross-cutting-target-paths/validator-target-creates-and-two-kinded-entries
// parlay-extends: parlay-tool/cross-cutting-target-paths/project-pass-validation-and-cli-flag
func validatePlanSection(bf deepBuildfile, buildfilePath string, plannedCreates map[string]string) []ValidationError {
	var errors []ValidationError
	if bf.Plan == nil {
		errors = append(errors, ValidationError{
			Code:    "missing-plan",
			Message: "buildfile has no plan: section",
			Context: "plan",
			Fix:     "regenerate the buildfile via /parlay-build-feature so the plan: section is emitted",
		})
		return errors
	}
	rootDir := planRootDirFromBuildfilePath(buildfilePath)

	// Index plan entries by source for cross-checks. Also build a
	// path-keyed index so the kind-aware target check can look up which
	// plan section a path lives in regardless of which entry sourced it.
	type planEntryKind struct {
		kind string // "modify" | "create" | "delete"
		path string
	}
	bySource := map[string][]planEntryKind{}
	addEntries := func(kind string, entries []deepPlanEntry, ctxPrefix string) {
		for i, e := range entries {
			ctx := fmt.Sprintf("%s[%d]", ctxPrefix, i)
			if e.Path == "" {
				errors = append(errors, ValidationError{
					Code:    "plan-entry-missing-path",
					Message: fmt.Sprintf("plan.%s entry at index %d has no path", kind, i),
					Context: ctx,
					Fix:     "add path: <file path> to the plan entry",
				})
				continue
			}
			if len(e.Sources) == 0 {
				errors = append(errors, ValidationError{
					Code:    "plan-entry-missing-sources",
					Message: fmt.Sprintf("plan.%s entry %q has no sources", kind, e.Path),
					Context: ctx,
					Fix:     "add sources: [component/<name> or cross-cutting/<id>] linking this entry to the buildfile entry that produced it",
				})
			}
			for _, src := range e.Sources {
				bySource[src] = append(bySource[src], planEntryKind{kind: kind, path: e.Path})
			}
		}
	}
	addEntries("modify", bf.Plan.Modifies, "plan.modifies")
	addEntries("create", bf.Plan.Creates, "plan.creates")
	addEntries("delete", bf.Plan.Deletes, "plan.deletes")

	// Every components: entry must appear in plan via component/<name> source.
	for compName := range bf.Components {
		key := "component/" + compName
		if _, ok := bySource[key]; !ok {
			errors = append(errors, ValidationError{
				Code:    "component-not-in-plan",
				Message: fmt.Sprintf("component %q has no entry in plan: (no row sources include %q)", compName, key),
				Context: fmt.Sprintf("plan / components.%s", compName),
				Fix:     "add a plan.creates or plan.modifies entry whose sources references this component",
			})
		}
	}

	// Every cross-cutting: entry's target-files / target-pattern / target-creates
	// paths must route to the correct plan section by entry kind.
	for _, cc := range bf.CrossCutting {
		key := "cross-cutting/" + cc.ID
		entries := bySource[key]
		if cc.ID != "" && len(entries) == 0 {
			errors = append(errors, ValidationError{
				Code:    "cross-cutting-not-in-plan",
				Message: fmt.Sprintf("cross-cutting %q has no entry in plan:", cc.ID),
				Context: fmt.Sprintf("plan / cross-cutting[%s]", cc.ID),
				Fix:     "add plan.creates entries for purely-introducing entries, plan.modifies for modifies-only entries, or both for two-kinded entries",
			})
		}

		// Detect within-entry double-listing first: same path in
		// target-files and target-creates is an authoring error.
		if len(cc.TargetCreates) > 0 && len(cc.TargetFiles) > 0 {
			tcSet := map[string]bool{}
			for _, p := range cc.TargetCreates {
				tcSet[p] = true
			}
			for _, p := range cc.TargetFiles {
				if tcSet[p] {
					errors = append(errors, ValidationError{
						Code:    "cross-cutting-target-double-listed",
						Message: fmt.Sprintf("cross-cutting %q lists path %q in both target-files and target-creates", cc.ID, p),
						Context: fmt.Sprintf("cross-cutting[%s].target-files / cross-cutting[%s].target-creates", cc.ID, cc.ID),
						Fix:     fmt.Sprintf("pick one — list %s in target-files: if it exists on disk and is being modified, or in target-creates: if it is being introduced", p),
					})
				}
			}
		}

		// Paths this entry's own plan.creates rows claim. Supplying these
		// keeps the classification stable once codegen has created them —
		// see classifyCrossCuttingEntryWithPlan.
		plannedByThisEntry := map[string]bool{}
		for _, e := range entries {
			if e.kind == "create" {
				plannedByThisEntry[e.path] = true
			}
		}
		kind := classifyCrossCuttingEntryWithPlan(cc, rootDir, plannedByThisEntry)

		// Mixed heuristic outcomes are rejected — author must split or
		// declare target-creates: explicitly. Two-kinded entries do not
		// fall through this gate (kind == "two-kinded").
		if kind == "mixed" {
			parts := []string{}
			for _, p := range cc.TargetFiles {
				abs := filepath.Join(rootDir, p)
				state := "missing"
				if _, err := os.Stat(abs); err == nil {
					state = "exists"
				}
				parts = append(parts, fmt.Sprintf("%s=%s", p, state))
			}
			errors = append(errors, ValidationError{
				Code:    "cross-cutting-mixed-target-kinds",
				Message: fmt.Sprintf("cross-cutting %q has target-files of mixed kinds (%s) — every path is either on disk (modifies-only) or not yet on disk (purely-introducing); a single entry cannot be both", cc.ID, strings.Join(parts, ", ")),
				Context: fmt.Sprintf("cross-cutting[%s].target-files", cc.ID),
				Fix:     "split into two cross-cutting entries (one per kind), or use the both-kinds shape: declare existing-on-disk paths in target-files: and not-yet-existing paths in target-creates:",
			})
			// Skip per-path routing for a mixed entry — the author needs
			// to fix the shape before any per-path errors are useful.
			continue
		}

		// Resolve the target-files set into the kind-specific routing.
		// For two-kinded entries, target-files is treated strictly as
		// modifies-only (heuristic bypassed) and target-creates strictly
		// as creates.
		targetFilesKind := kind
		if kind == "two-kinded" {
			targetFilesKind = "modifies-only"
		}
		for _, target := range cc.TargetFiles {
			switch targetFilesKind {
			case "modifies-only":
				found := false
				for _, e := range entries {
					if e.kind == "modify" && e.path == target {
						found = true
						break
					}
				}
				if !found {
					// Distinguish: if no plan rows at all are sourced
					// by this entry, the legacy entry-level error fires
					// in addition; the per-target error names
					// plan.modifies as the right destination.
					errors = append(errors, ValidationError{
						Code:    "cross-cutting-target-not-in-modifies",
						Message: fmt.Sprintf("cross-cutting %q names target-files: %q but plan.modifies has no matching entry sourced from %q", cc.ID, target, key),
						Context: fmt.Sprintf("plan.modifies / cross-cutting[%s].target-files", cc.ID),
						Fix:     fmt.Sprintf("add plan.modifies entry { path: %s, sources: [%s] }", target, key),
					})
				}
			case "purely-introducing":
				found := false
				for _, e := range entries {
					if e.kind == "create" && e.path == target {
						found = true
						break
					}
				}
				if !found {
					errors = append(errors, ValidationError{
						Code:    "cross-cutting-target-not-in-creates",
						Message: fmt.Sprintf("cross-cutting %q names target-files: %q but plan.creates has no matching entry sourced from %q (the path is absent on disk, so this is a purely-introducing entry)", cc.ID, target, key),
						Context: fmt.Sprintf("plan.creates / cross-cutting[%s].target-files", cc.ID),
						Fix:     fmt.Sprintf("add plan.creates entry { path: %s, sources: [%s] }", target, key),
					})
				}
			}
		}

		// target-creates paths route strictly to plan.creates (only
		// applies to two-kinded entries).
		for _, target := range cc.TargetCreates {
			found := false
			for _, e := range entries {
				if e.kind == "create" && e.path == target {
					found = true
					break
				}
			}
			if !found {
				errors = append(errors, ValidationError{
					Code:    "cross-cutting-target-creates-not-in-plan",
					Message: fmt.Sprintf("cross-cutting %q names target-creates: %q but plan.creates has no matching entry sourced from %q", cc.ID, target, key),
					Context: fmt.Sprintf("plan.creates / cross-cutting[%s].target-creates", cc.ID),
					Fix:     fmt.Sprintf("add plan.creates entry { path: %s, sources: [%s] }", target, key),
				})
			}
		}

		// target-pattern resolves to a concrete set; route by per-path
		// classification (on-disk -> plan.modifies; cross-pass-create
		// -> plan.creates). Empty resolution is a hard error.
		if cc.TargetPattern != "" {
			resolved := resolveTargetPattern(cc.TargetPattern, rootDir, plannedCreates)
			if len(resolved) == 0 {
				createdHint := "no plannedCreates available (single-feature mode)"
				if plannedCreates != nil {
					createdHint = fmt.Sprintf("plannedCreates count: %d", len(plannedCreates))
				}
				errors = append(errors, ValidationError{
					Code:    "cross-cutting-pattern-empty",
					Message: fmt.Sprintf("cross-cutting %q target-pattern %q resolved to zero paths under %s (%s)", cc.ID, cc.TargetPattern, rootDir, createdHint),
					Context: fmt.Sprintf("cross-cutting[%s].target-pattern", cc.ID),
					Fix:     "fix the glob or remove the entry; if a sibling feature is expected to create matching files, run `parlay validate --project` instead",
				})
			} else {
				for _, p := range resolved {
					abs := filepath.Join(rootDir, p)
					_, statErr := os.Stat(abs)
					onDisk := statErr == nil
					_, plannedHere := plannedCreates[p]
					switch {
					case onDisk:
						// route to plan.modifies
						found := false
						for _, e := range entries {
							if e.kind == "modify" && e.path == p {
								found = true
								break
							}
						}
						if !found {
							errors = append(errors, ValidationError{
								Code:    "cross-cutting-target-not-in-modifies",
								Message: fmt.Sprintf("cross-cutting %q target-pattern resolved %q (on disk) but plan.modifies has no matching entry sourced from %q", cc.ID, p, key),
								Context: fmt.Sprintf("plan.modifies / cross-cutting[%s].target-pattern", cc.ID),
								Fix:     fmt.Sprintf("add plan.modifies entry { path: %s, sources: [%s] }", p, key),
							})
						}
					case plannedHere:
						// route to plan.creates
						found := false
						for _, e := range entries {
							if e.kind == "create" && e.path == p {
								found = true
								break
							}
						}
						if !found {
							errors = append(errors, ValidationError{
								Code:    "cross-cutting-target-not-in-creates",
								Message: fmt.Sprintf("cross-cutting %q target-pattern resolved %q (planned by sibling) but plan.creates has no matching entry sourced from %q", cc.ID, p, key),
								Context: fmt.Sprintf("plan.creates / cross-cutting[%s].target-pattern", cc.ID),
								Fix:     fmt.Sprintf("add plan.creates entry { path: %s, sources: [%s] }", p, key),
							})
						}
					}
				}
			}
		}
	}

	// Disk-shape checks (only when we can resolve a sensible root).
	if rootDir != "" {
		for _, e := range bf.Plan.Modifies {
			if e.Path == "" {
				continue
			}
			abs := filepath.Join(rootDir, e.Path)
			if _, err := os.Stat(abs); err != nil && os.IsNotExist(err) {
				// Project-pass relaxation: a sibling feature's
				// plan.creates row promising the same path satisfies
				// the existence check without changing provenance.
				if _, ok := plannedCreates[e.Path]; ok {
					continue
				}
				errors = append(errors, ValidationError{
					Code:    "plan-modify-target-missing",
					Message: fmt.Sprintf("plan.modifies %q does not exist in source root %s", e.Path, rootDir),
					Context: "plan.modifies",
					Fix:     "either fix the path, or move the entry to plan.creates if this feature genuinely introduces the file",
				})
			}
		}
		for _, e := range bf.Plan.Creates {
			if e.Path == "" {
				continue
			}
			abs := filepath.Join(rootDir, e.Path)
			if info, err := os.Stat(abs); err == nil && !info.IsDir() {
				errors = append(errors, ValidationError{
					Code:    "plan-create-collision",
					Message: fmt.Sprintf("plan.creates %q already exists at %s", e.Path, abs),
					Context: "plan.creates",
					Fix:     "either move the entry to plan.modifies (to merge into the existing file) or pick a different path",
				})
			}
		}
	}

	return errors
}

// planRootDirFromBuildfilePath derives the project root directory from
// a buildfile's path. A buildfile at <root>/.parlay/build/<feature>/buildfile.yaml
// — or, for initiative-nested features, <root>/.parlay/build/<initiative>/<feature>/buildfile.yaml
// — belongs to root <root>. Returns "" when the path doesn't match the
// expected layout, signaling the disk-shape checks should be skipped.
func planRootDirFromBuildfilePath(buildfilePath string) string {
	abs, err := filepath.Abs(buildfilePath)
	if err != nil {
		return ""
	}
	// Walk up from the buildfile's directory until we land on
	// <root>/.parlay/build/, then return <root>. Feature slugs may be
	// qualified (e.g., "initiative/feature"), so the depth between the
	// buildfile and .parlay/build is not fixed.
	dir := filepath.Dir(abs)
	for dir != "/" && dir != "." && dir != "" {
		parent := filepath.Dir(dir)
		if filepath.Base(dir) == "build" && filepath.Base(parent) == ".parlay" {
			return filepath.Dir(parent)
		}
		dir = parent
	}
	return ""
}
