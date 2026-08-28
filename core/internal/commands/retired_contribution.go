// parlay-feature: parlay-tool/active-page-composition
// parlay-component: retired-contribution-check
//
// The deletion half of cross-feature supersession.
//
// The composition signature marks a feature stale when a sibling retires one
// of its fragments. Staleness alone only forces a REBUILD; it does not say
// what the rebuild owes. Without this check a rebuilt buildfile can keep a
// component whose source fragment is no longer on any page, keep its path in
// plan.creates, and go on emitting and routing output for a slot another
// feature now owns — the racing pair supersedes: exists to prevent, arriving
// one build later instead of immediately.
//
// Two distinctions this makes deliberately, because collapsing either turns a
// correct refusal into data loss:
//
//   - "inactive in the composed page" is NOT "the contract was deleted". The
//     retired fragment stays live in its owner's surface; it is history the
//     feature still declares. So what has to go is the generated MOUNTING and
//     ROUTING, never the source contract.
//   - removal is keyed by page contribution, not by fragment name. A
//     component file whose path is also produced for a still-active fragment
//     must not be deleted merely because one page slot was superseded; there
//     the fix is to stop routing it, and deleting the file would break the
//     other caller.
package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// retiredPlan reads ONLY plan:, which stays top-level in v2 (per-target rows
// aggregate into it). Components deliberately do NOT come from here — they are
// a section v2 RELOCATED under targets.presentation, so a local struct would
// silently see zero components on every multi-target buildfile and report a
// clean bill for exactly the projects this check matters most to. They come
// from agent.ResolveBuildfileComponents, the canonical v1/v2-aware reader.
type retiredPlan struct {
	Plan *struct {
		Modifies []retiredPlanEntry `yaml:"modifies"`
		Creates  []retiredPlanEntry `yaml:"creates"`
		Deletes  []retiredPlanEntry `yaml:"deletes"`
		Targets  map[string]struct {
			Modifies []retiredPlanEntry `yaml:"modifies"`
			Creates  []retiredPlanEntry `yaml:"creates"`
			Deletes  []retiredPlanEntry `yaml:"deletes"`
		} `yaml:"targets"`
	} `yaml:"plan"`
}

type retiredPlanEntry struct {
	Path    string   `yaml:"path"`
	Sources []string `yaml:"sources"`
}

// RetiredContributionFinding is one component still emitting for a fragment
// that no longer reaches any page.
type RetiredContributionFinding struct {
	Code      string
	Component string
	Ref       string
	RetiredBy string
	Path      string
	Message   string
	Fix       string
	Severity  string
}

// checkRetiredContributions reports components whose source fragment has been
// retired by supersession but whose output the plan still writes.
//
// Fail-visible on unreadable inputs in the same direction as the rest of this
// release: a buildfile that cannot be read yields no findings here and is
// reported by check-buildfile, rather than being silently treated as clean.
// cannotEstablishRetirement builds the finding that says this claim could not
// establish its own subject.
//
// Named and coded to match the pattern the registry already uses —
// branchSubjectUnreadable, alongside coverage-decisions, testcases-readiness,
// composition and generated-state, each of which already reports its own
// subject being unreadable. There was never a missing vocabulary to invent;
// deferring on that basis was wrong.
//
// Every early return below used to hand back nil, which the boundary read as
// "this feature retires nothing" — an unreadable buildfile and a clean one
// produced the identical verdict. Deferring to another claim to notice some of
// those failures is not evidence THIS claim checked anything, and a claim that
// cannot say whether it looked is the exact shape the release is removing.
//
// Absence is still absence: a feature with no surfaces or no buildfile has
// genuinely nothing to check, and is distinguished from one whose artifacts
// exist and cannot be read.
func cannotEstablishRetirement(format string, args ...any) []RetiredContributionFinding {
	return []RetiredContributionFinding{{
		Code:     "retired-contribution-unresolvable",
		Message:  fmt.Sprintf(format, args...),
		Fix:      "make the named artifact readable, then re-run; until it can be read, whether a superseded fragment is still being emitted cannot be established",
		Severity: "error",
	}}
}

func checkRetiredContributions(cfg *config.Context, slug string) []RetiredContributionFinding {
	specDir := filepath.Join(cfg.Root.Path, config.SpecDir)
	fragments, err := parser.ScanAllSurfaces(specDir)
	if err != nil {
		if os.IsNotExist(err) {
			// No spec/intents at all: nothing declares a fragment, so nothing
			// can have been retired.
			return nil
		}
		return cannotEstablishRetirement("surfaces for %s cannot be scanned: %v — whether supersession retired any of this feature's fragments is unknown", slug, err)
	}
	view := agent.ResolveActiveView(fragments)

	// A refused composition is not a resolved one. While a fork, a cycle or a
	// duplicate ref stands, the retirement set is not the answer — it is what
	// the resolver produced while declining to compose.
	for _, e := range view.Errors {
		if e.Severity == "error" {
			return cannotEstablishRetirement("[%s] %s — the composition is unresolved, so which of %s's fragments are retired cannot be established", e.Code, e.Message, slug)
		}
	}

	retired := view.RetiredOwnedBy(slug)
	if len(retired) == 0 {
		return nil
	}

	buildfilePath := filepath.Join(cfg.BuildPath(slug), "buildfile.yaml")
	data, err := os.ReadFile(buildfilePath)
	if err != nil {
		if os.IsNotExist(err) {
			// NOT treated as "nothing to check".
			//
			// The tempting reading — no buildfile means the feature was never
			// built, so there is no emitted output to be stale — does not
			// follow. A buildfile can be deleted or lost AFTER code was
			// generated, which leaves on disk exactly the superseded output
			// this claim exists to rule out, with the one artifact that could
			// have named it now gone. Absence of the evidence is not evidence
			// of absence, and another claim blocking a missing buildfile is
			// not this claim having established its own subject.
			//
			// Distinct from the no-spec/intents case above, which is genuinely
			// clean: with no surfaces there is no supersession graph at all,
			// so nothing could have been retired in the first place.
			return []RetiredContributionFinding{{
				Code:     "retired-contribution-subject-missing",
				Message:  fmt.Sprintf("%s has fragments retired by supersession and no buildfile — whether previously generated output for them is still on disk cannot be established, and a deleted buildfile leaves exactly that output unaccounted for", slug),
				Fix:      "rebuild the feature so its plan states what is created and deleted; a first build blocks here harmlessly, a lost buildfile blocks here for cause",
				Severity: "error",
			}}
		}
		return cannotEstablishRetirement("%s has retired fragments and its buildfile cannot be read: %v — whether their output is still emitted is unknown", slug, err)
	}
	var bf retiredPlan
	if err := yaml.Unmarshal(data, &bf); err != nil {
		return cannotEstablishRetirement("%s has retired fragments and its buildfile cannot be parsed: %v — whether their output is still emitted is unknown", slug, err)
	}
	components, err := agent.ResolveBuildfileComponents(data)
	if err != nil {
		return cannotEstablishRetirement("%s has retired fragments and its buildfile components cannot be resolved: %v — whether their output is still emitted is unknown", slug, err)
	}

	// Which paths the plan still writes, and which it has already retired.
	writes := map[string]bool{}
	deletes := map[string]bool{}
	if bf.Plan != nil {
		for _, e := range append(append([]retiredPlanEntry{}, bf.Plan.Modifies...), bf.Plan.Creates...) {
			writes[e.Path] = true
		}
		for _, e := range bf.Plan.Deletes {
			deletes[e.Path] = true
		}
		for _, t := range bf.Plan.Targets {
			for _, e := range append(append([]retiredPlanEntry{}, t.Modifies...), t.Creates...) {
				writes[e.Path] = true
			}
			for _, e := range t.Deletes {
				deletes[e.Path] = true
			}
		}
	}

	// A path is SHARED when a component whose source is still active also
	// produces it. Shared paths are never deletion candidates — the other
	// contributor still needs the file — so the remedy there is to drop the
	// retired mount, not to remove the output.
	sharedPaths := map[string]bool{}
	componentPath := map[string]string{}
	for name, c := range components {
		ref := canonicalFragmentRef(c.Source)
		if ref == "" {
			continue
		}
		p := planPathForComponent(bf, name)
		if p == "" {
			continue
		}
		componentPath[name] = p
		if _, isRetired := retired[ref]; !isRetired {
			sharedPaths[p] = true
		}
	}

	names := make([]string, 0, len(components))
	for name := range components {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []RetiredContributionFinding
	for _, name := range names {
		ref := canonicalFragmentRef(components[name].Source)
		by, isRetired := retired[ref]
		if !isRetired {
			continue
		}
		path := componentPath[name]

		switch {
		case path != "" && sharedPaths[path]:
			// The file survives; only this component's contribution to the
			// page is gone. Deleting the path would take an active
			// contributor's output with it.
			out = append(out, RetiredContributionFinding{
				Code:      "retired-contribution-still-mounted",
				Component: name, Ref: ref, RetiredBy: by, Path: path,
				Message: fmt.Sprintf("component %q sources %s, which %s retired, but %s is also produced for a still-active fragment — the file must stay while this component's mount and route must go",
					name, ref, by, path),
				Fix:      "remove this component's route/mount wiring and its entry from plan.creates/modifies; do NOT add the shared path to plan.deletes",
				Severity: "error",
			})
		case writes[path]:
			out = append(out, RetiredContributionFinding{
				Code:      "retired-contribution-still-emitted",
				Component: name, Ref: ref, RetiredBy: by, Path: path,
				Message: fmt.Sprintf("component %q sources %s, which %s retired, yet plan still writes %s — the superseded output would be emitted and routed alongside its replacement",
					name, ref, by, path),
				Fix:      fmt.Sprintf("move %s to plan.deletes and drop the component, or remove the supersedes: annotation if the retirement was not intended", path),
				Severity: "error",
			})
		case path != "" && !deletes[path]:
			// Neither written nor scheduled for removal: the output is
			// orphaned on disk with nothing accounting for it.
			out = append(out, RetiredContributionFinding{
				Code:      "retired-contribution-unaccounted",
				Component: name, Ref: ref, RetiredBy: by, Path: path,
				Message: fmt.Sprintf("component %q sources %s, which %s retired; %s is neither written nor listed in plan.deletes, so previously generated output is left behind",
					name, ref, by, path),
				Fix: fmt.Sprintf("add %s to plan.deletes so the rebuild removes the retired output", path),
				// ERROR, not warning. This is precisely the rebuilt-plan shape
				// the deletion half exists to stop: the component is gone from
				// the plan, the path is not shared with an active contributor,
				// and nothing removes the file — so the previously generated
				// output stays on disk and keeps being routed. Letting the code
				// stage advance here leaves exactly the artifact this claim
				// promises to have removed.
				Severity: "error",
			})
		}
	}
	return out
}

// canonicalFragmentRef normalizes the spellings of a fragment reference onto
// the one supersedes: uses.
//
// A buildfile component says `source: "@graded/fragment:Customer Detail"` —
// kind-qualified, display name. A supersedes: target says
// "@graded/customer-detail" — bare, slugified. Comparing them raw silently
// matches nothing, which would make this check pass on every project while
// looking like it ran.
//
// Parsed from the RIGHT-hand `/fragment:` discriminator, never by splitting at
// the first slash. Fragment.Feature is a relative slug that may itself contain
// a slash: an initiative-nested feature is "parlay-tool/intent-supersession"
// (parser.scanNestedSurfaces qualifies it), so a first-slash split read the
// initiative as the feature and slugified "intent-supersession/fragment:Name"
// as the fragment. This repository's own features are initiative-nested, so
// that was the common case here, not an edge one.
func canonicalFragmentRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	body := strings.TrimPrefix(ref, "@")

	// Kind-qualified: everything left of the LAST "/<kind>:" is the feature.
	if i := strings.LastIndex(body, "/fragment:"); i >= 0 {
		feature := body[:i]
		name := body[i+len("/fragment:"):]
		return "@" + feature + "/" + parser.Slugify(strings.TrimSpace(name))
	}
	// A different kind (operation:, etc.) names another subject entirely and
	// must not be folded into a fragment ref.
	if strings.Contains(body, ":") {
		return ref
	}
	// Already bare "@feature/slug", where feature may contain slashes. The
	// fragment slug is the final segment; the rest is the feature.
	if i := strings.LastIndex(body, "/"); i >= 0 {
		return "@" + body[:i] + "/" + parser.Slugify(strings.TrimSpace(body[i+1:]))
	}
	return ref
}

// planSourceNames a plan row cites carry a kind prefix ("component/x"); the
// component map is keyed on the bare name.
func planSourceMatches(source, component string) bool {
	s := strings.TrimSpace(source)
	return s == component || strings.TrimPrefix(s, "component/") == component
}

// planPathForComponent finds the path the plan attributes to a component, by
// the sources: back-reference the plan rows already carry.
func planPathForComponent(bf retiredPlan, component string) string {
	if bf.Plan == nil {
		return ""
	}
	rows := append(append([]retiredPlanEntry{}, bf.Plan.Modifies...), bf.Plan.Creates...)
	rows = append(rows, bf.Plan.Deletes...)
	for _, t := range bf.Plan.Targets {
		rows = append(rows, t.Modifies...)
		rows = append(rows, t.Creates...)
		rows = append(rows, t.Deletes...)
	}
	for _, r := range rows {
		for _, s := range r.Sources {
			if planSourceMatches(s, component) {
				return r.Path
			}
		}
	}
	return ""
}
