// parlay-feature: parlay-tool/active-page-composition
// parlay-component: active-page-resolver
//
// The project-wide resolver that turns every feature's surface into ONE
// composed active view per page.
//
// `supersedes:` was parsed onto parser.Fragment, validated for two-headed
// forks, and then read by nothing that composes anything: view-page had zero
// references to it and codegen never saw the field at all. So the schema's
// promise — "Codegen routes the composed result (the superseding fragment's
// output in the shared slot), not two parallel files that race to mount" —
// was documentation over an unimplemented mechanism. Both fragments kept
// mounting.
//
// Two properties of the field force this to be project-wide rather than a
// per-feature filter, and getting either wrong reintroduces the bug:
//
//   - supersedes: is CROSS-FEATURE by construction. Feature B may retire a
//     fragment declared only in feature A. A resolver that folds over one
//     feature's own surface cannot see the edge that retires its own output,
//     so it would keep the retired fragment and demand a mount assertion for
//     a component deliberately gone.
//   - the page manifest is OPTIONAL (page.schema.md: "By default, pages are
//     derived views assembled on demand from feature surfaces"). A derivation
//     specified only against a manifest is undefined on the common shape.
//
// Hence the fixed order below: resolve over EVERY surface first, filter by
// owner last. Callers wanting one feature's contribution take the resolved
// active view and filter it; they must never resolve a filtered set.
package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ddwht/parlay/core/internal/parser"
)

// ActiveView is the resolved composition of every surface in a project.
//
// Active carries the fragments that survive supersession, in scan order.
// Retired maps a retired fragment's canonical ref to the ref that retired it,
// which is what lets a consumer distinguish "gone from this page" from "never
// existed" — the distinction deletion semantics depend on.
type ActiveView struct {
	Active  []parser.Fragment
	Retired map[string]string
	Errors  []ValidationError
}

// FragmentRef is the canonical identity of a fragment: "@feature/slug".
// Every comparison in this file goes through it, because a fragment's Name is
// display text and its slug is what a supersedes: target names.
func FragmentRef(f parser.Fragment) string {
	return fmt.Sprintf("@%s/%s", f.Feature, parser.Slugify(f.Name))
}

// normalizeRegion collapses the empty region to "main", matching
// assembleRegions and sharedRegionWarnings. Kept here so the three agree by
// construction rather than by coincidence.
func normalizeRegion(region string) string {
	r := strings.ToLower(strings.TrimSpace(region))
	if r == "" {
		return "main"
	}
	return r
}

// ResolveActiveView resolves supersession across every fragment in a project.
//
// The input must be the WHOLE project's fragments (parser.ScanAllSurfaces over
// the spec dir), never one feature's. Passing a filtered set silently produces
// a different answer, which is the defect this function exists to remove.
//
// Refusals are returned as errors and the offending fragments are left ACTIVE.
// Fail-visible, not fail-quiet: a fork or a cycle means nobody can say which
// fragment owns the slot, and dropping both would make a page silently lose
// content on the strength of a contradiction.
func ResolveActiveView(all []parser.Fragment) ActiveView {
	view := ActiveView{Retired: map[string]string{}}

	// Duplicate canonical refs are REJECTED, not last-one-wins.
	//
	// This map used to be built with a plain assignment, so two fragments
	// resolving to the same @feature/slug silently collapsed to whichever was
	// scanned last. That matters more here than in an ordinary index: this
	// function is the truth several callers now derive from — the composition
	// signature, the assembly derivation, the retirement join — and a
	// supersedes: target that names an ambiguous ref would retire one of two
	// fragments with nothing recording which. Callers must not have to assume
	// an earlier boundary de-duplicated for them.
	byRef := make(map[string]parser.Fragment, len(all))
	dupes := map[string]bool{}
	for _, f := range all {
		ref := FragmentRef(f)
		if _, seen := byRef[ref]; seen {
			dupes[ref] = true
			continue
		}
		byRef[ref] = f
	}
	for _, ref := range sortedBoolKeys(dupes) {
		view.Errors = append(view.Errors, ValidationError{
			Code:     "surface-fragment-ref-duplicated",
			Message:  fmt.Sprintf("more than one fragment resolves to %s, so a supersedes: naming it cannot say which one it replaces", ref),
			Context:  "surface",
			Fix:      "rename one of the fragments; a fragment reference must identify exactly one fragment across the project",
			Severity: "error",
		})
	}

	// supersedes: edges, keyed by target. Sorted iteration everywhere below —
	// this feeds a composition signature, and a map-order-dependent result
	// would make the signature flap between runs on an unchanged tree.
	type edge struct {
		superseder parser.Fragment
		ref        string
	}
	byTarget := map[string][]edge{}
	var targets []string
	for _, f := range all {
		target := strings.TrimSpace(f.Supersedes)
		if target == "" {
			continue
		}
		if _, seen := byTarget[target]; !seen {
			targets = append(targets, target)
		}
		byTarget[target] = append(byTarget[target], edge{f, FragmentRef(f)})
	}
	sort.Strings(targets)

	forked := map[string]bool{}
	for _, target := range targets {
		heads := byTarget[target]
		sort.Slice(heads, func(i, j int) bool { return heads[i].ref < heads[j].ref })

		refs := make([]string, 0, len(heads))
		for _, h := range heads {
			refs = append(refs, h.ref)
		}

		// The fork is checked FIRST, before the target is looked up, because
		// it is a fact about the superseders and holds whether or not the
		// target exists. Checking existence first would report a missing
		// target on a forked pair and swallow the fork — the more actionable
		// of the two, and the one that has to be resolved before the chain
		// can mean anything.
		if len(heads) > 1 {
			forked[target] = true
			view.Errors = append(view.Errors, ValidationError{
				Code:     "surface-supersedes-conflict",
				Message:  fmt.Sprintf("%d fragments (%s) all supersede %s — a two-headed chain the composition cannot resolve to one winner", len(heads), strings.Join(refs, ", "), target),
				Context:  "supersedes",
				Fix:      "keep one superseding fragment, or chain them (A supersedes B, B supersedes the target) so a single head remains",
				Severity: "error",
			})
			continue
		}

		// A target nothing declares. Only visible once you try to APPLY the
		// edge: a detector that never resolves has no reason to look the
		// target up, which is why this shape sat undetected in the tree.
		targetFrag, exists := byRef[target]
		if !exists {
			view.Errors = append(view.Errors, ValidationError{
				Code:     "surface-supersedes-target-unknown",
				Message:  fmt.Sprintf("%s supersedes %s, which no surface in this project declares", strings.Join(refs, ", "), target),
				Context:  "supersedes",
				Fix:      "correct the target ref, or remove the supersedes: annotation if the fragment it named is gone",
				Severity: "error",
			})
			continue
		}

		h := heads[0]

		// Same-slot constraint. supersedes: means "replaces that fragment
		// where the two occupy the same (page, region)". Across different
		// slots there is nothing to replace, and honouring it would retire a
		// fragment on a page the superseder never appears on.
		if h.superseder.Page != targetFrag.Page || normalizeRegion(h.superseder.Region) != normalizeRegion(targetFrag.Region) {
			view.Errors = append(view.Errors, ValidationError{
				Code: "surface-supersedes-slot-mismatch",
				Message: fmt.Sprintf("%s supersedes %s but they occupy different slots (%s/%s vs %s/%s) — supersedes: replaces a fragment in the SAME (page, region)",
					h.ref, target,
					h.superseder.Page, normalizeRegion(h.superseder.Region),
					targetFrag.Page, normalizeRegion(targetFrag.Region)),
				Context:  "supersedes",
				Fix:      "put the superseding fragment on the same page and region as its target, or drop the supersedes: annotation and let both stand",
				Severity: "error",
			})
			continue
		}

		view.Retired[target] = h.ref
	}

	// Cycles. A chain that closes on itself has no head, so every member
	// would be retired and the slot would empty — the same content-deleting
	// contradiction as a fork, and equally not something to resolve silently.
	for _, start := range sortedKeys(view.Retired) {
		seen := map[string]bool{start: true}
		cur := start
		for {
			next, ok := view.Retired[cur]
			if !ok {
				break
			}
			if seen[next] {
				members := sortedBoolKeys(seen)
				view.Errors = append(view.Errors, ValidationError{
					Code:     "surface-supersedes-cycle",
					Message:  fmt.Sprintf("supersedes: forms a cycle (%s) — the chain has no head, so no fragment owns the slot", strings.Join(members, " -> ")),
					Context:  "supersedes",
					Fix:      "break the cycle so exactly one fragment supersedes the others and is itself superseded by none",
					Severity: "error",
				})
				for m := range seen {
					forked[m] = true
				}
				break
			}
			seen[next] = true
			cur = next
		}
	}

	// A forked or cyclic target keeps its place: the refusal is reported, and
	// the tree is left as it was rather than half-composed.
	for ref := range forked {
		delete(view.Retired, ref)
	}

	for _, f := range all {
		if _, retired := view.Retired[FragmentRef(f)]; retired {
			continue
		}
		view.Active = append(view.Active, f)
	}

	// Deduplicate the cycle diagnostics: walking every start ref in a 3-cycle
	// finds the same cycle three times, and reporting one defect three times
	// reads as three defects.
	view.Errors = dedupeErrors(view.Errors)
	return view
}

// ActiveOnPage returns the resolved active fragments targeting one page.
//
// Takes an already-resolved view rather than raw fragments, so the
// project-wide-then-filter order cannot be inverted at a call site.
func (v ActiveView) ActiveOnPage(page string) []parser.Fragment {
	var out []parser.Fragment
	for _, f := range v.Active {
		if f.Page == page {
			out = append(out, f)
		}
	}
	return out
}

// OwnedBy narrows a fragment list to one feature's contribution. This is the
// LAST step in the pipeline, never the first: the caller must have resolved
// over every surface before reaching here.
func OwnedBy(fragments []parser.Fragment, feature string) []parser.Fragment {
	var out []parser.Fragment
	for _, f := range fragments {
		if f.Feature == feature {
			out = append(out, f)
		}
	}
	return out
}

// RetiredOwnedBy lists the refs this feature contributed that are no longer
// active, each with the ref that retired it.
//
// This is what makes cross-feature deletion expressible. A feature whose
// fragment was retired by ANOTHER feature has an unchanged surface, so nothing
// in its own source signature moves; without this list a rebuild has no way to
// learn that its contribution left the page.
func (v ActiveView) RetiredOwnedBy(feature string) map[string]string {
	out := map[string]string{}
	prefix := "@" + feature + "/"
	for ref, by := range v.Retired {
		if strings.HasPrefix(ref, prefix) {
			out[ref] = by
		}
	}
	return out
}

func sortedBoolKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func dedupeErrors(errs []ValidationError) []ValidationError {
	seen := map[string]bool{}
	var out []ValidationError
	for _, e := range errs {
		key := e.Code + "|" + e.Message
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}
	return out
}
