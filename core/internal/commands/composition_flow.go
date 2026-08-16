package commands

// Cross-feature flow assertions and whether the prototype can actually
// satisfy them.
//
// A `scope: flow` suite that walks from one feature's route to another's and
// then asserts on domain state is asking a question about a SHARED runtime:
// approve a report on /review, then see it read "approved" on /expenses. When
// every feature hydrates its own fixture, nothing carries the write across
// the boundary, and the assertion is unsatisfiable no matter how the code is
// written.
//
// That was not caught before, and the way it failed is worth recording: the
// generating agent, faced with an assertion it could not satisfy, weakened it
// and wrote a ten-line comment explaining why. The suite went green. Nothing
// upstream ever learned that a cross-feature journey did not work, because
// the only artifact saying so was a comment in generated code.
//
// So this fires on the assertion, before code is written.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"gopkg.in/yaml.v3"
)

// flowStep is the subset of a testcase step this analysis reads.
type flowStep struct {
	Action string `yaml:"action"`
	Target string `yaml:"target"`
	Verify string `yaml:"verify"`
	// Expected carries the route on `verify: route` steps, where `target:`
	// names an element rather than a path. A flow that clicks through instead
	// of navigating explicitly establishes its new route only here.
	Expected string `yaml:"expected"`
}

type flowSuites struct {
	Suites []struct {
		Scope string   `yaml:"scope"`
		Name  string   `yaml:"name"`
		Flow  []string `yaml:"flow"`
		Cases []struct {
			Name  string     `yaml:"name"`
			Steps []flowStep `yaml:"steps"`
		} `yaml:"cases"`
	} `yaml:"suites"`
}

// flowPlan decodes only the plan: block; routes are resolved separately
// through agent.ResolveBuildfileRoutes so the v2 relocation of routes: under
// targets.presentation is honored (a private top-level routes: decode saw
// nothing for a multi-target buildfile, so no route in the project had an
// owner and every cross-feature flow read as same-feature).
type flowPlan struct {
	Plan struct {
		Creates []struct {
			Path string `yaml:"path"`
		} `yaml:"creates"`
		Modifies []struct {
			Path string `yaml:"path"`
		} `yaml:"modifies"`
	} `yaml:"plan"`
}

// findUnsatisfiableFlows reports cross-feature flow assertions the project
// has no mechanism to satisfy.
//
// storePath is the adapter's declared shared-store path, source-root-relative,
// or "" when the adapter declares none. The two cases are genuinely different
// and are reported differently:
//
//   - No store declared: a warning naming the fact. A CLI has no shared
//     runtime between invocations and a static generator has none at all;
//     demanding a store the framework cannot have would be parlay asserting
//     framework knowledge that adapters exist to hold.
//   - Store declared but a participating feature's plan neither creates nor
//     modifies it: an error. The mechanism exists and this feature is not
//     using it, so its writes stay local and the assertion cannot pass.
//
// A store declared and used by every participant produces no finding — that
// is the state in which the journey actually works.
func findUnsatisfiableFlows(cfg *config.Context, features []string, storePath string) []compositionFinding {
	// Route → owning feature. Ownership comes from the buildfile that
	// declares the route, which is the same table the router is generated
	// from, so "crosses a feature boundary" means the same thing here as it
	// does at runtime.
	owner := map[string]string{}
	planned := map[string]map[string]bool{} // feature → set of planned paths
	for _, slug := range features {
		data, err := os.ReadFile(filepath.Join(cfg.BuildPath(slug), "buildfile.yaml"))
		if err != nil {
			continue
		}
		var bf flowPlan
		if yaml.Unmarshal(data, &bf) != nil {
			continue
		}
		routes, _ := agent.ResolveBuildfileRoutes(data)
		for _, r := range routes {
			if r.Path != "" {
				owner[r.Path] = slug
			}
		}
		paths := map[string]bool{}
		for _, c := range bf.Plan.Creates {
			paths[c.Path] = true
		}
		for _, m := range bf.Plan.Modifies {
			paths[m.Path] = true
		}
		planned[slug] = paths
	}

	var findings []compositionFinding
	for _, slug := range features {
		data, err := os.ReadFile(filepath.Join(cfg.BuildPath(slug), "testcases.yaml"))
		if err != nil {
			continue
		}
		var tc flowSuites
		if yaml.Unmarshal(data, &tc) != nil {
			continue
		}

		for _, suite := range tc.Suites {
			if suite.Scope != "flow" || len(suite.Flow) < 2 {
				continue
			}

			for _, c := range suite.Cases {
				crossed, participants := stateAssertedAfterCrossing(c.Steps, owner)
				if !crossed {
					continue
				}

				// Which participants cannot reach the shared runtime.
				var unwired []string
				if storePath != "" {
					for _, f := range participants {
						if !planned[f][storePath] {
							unwired = append(unwired, f)
						}
					}
					sort.Strings(unwired)
					if len(unwired) == 0 {
						continue
					}
				}

				sites := append([]string{}, participants...)
				sort.Strings(sites)
				msg := ""
				if storePath == "" {
					msg = fmt.Sprintf(
						"flow %q asserts on domain state after crossing from one feature's route into another's (%s), "+
							"but the adapter declares no file-conventions.paths.store — there is nothing to carry the write across the boundary, "+
							"so each feature will read its own fixture and the assertion cannot hold end to end",
						suite.Name, strings.Join(sites, " → "))
				} else {
					msg = fmt.Sprintf(
						"flow %q asserts on domain state after crossing between features (%s), and the adapter declares a shared store at %q, "+
							"but %s does not plan to create or modify it — that feature's writes stay local, so the assertion cannot hold",
						suite.Name, strings.Join(sites, " → "), storePath, strings.Join(unwired, ", "))
				}
				findings = append(findings, compositionFinding{
					Code:    "composition-flow-unsatisfiable",
					Message: msg,
					Sites:   sites,
				})
			}
		}
	}

	sort.Slice(findings, func(i, j int) bool { return findings[i].Message < findings[j].Message })
	return findings
}

// stateAssertedAfterCrossing reports whether a case's steps navigate out of
// one feature into another and then assert on domain state, and names the
// features involved.
//
// The discriminator is `verify: state` AFTER the crossing, and it is chosen
// deliberately over "the flow spans two features". Plenty of cross-feature
// flows are pure navigation — click through from the expense list into the
// submit wizard — and those are satisfiable without any shared runtime,
// because nothing written in the first feature has to be visible in the
// second. Flagging them would bury the one case that matters under two that
// do not, which is how the real finding went unnoticed.
func stateAssertedAfterCrossing(steps []flowStep, owner map[string]string) (bool, []string) {
	current := ""
	seen := map[string]bool{}
	var participants []string
	crossed := false

	for _, s := range steps {
		// Both `action: navigate` and `verify: route` establish where we are;
		// a suite may assert its arrival rather than perform it.
		target := ""
		if s.Action == "navigate" {
			target = s.Target
		}
		if s.Verify == "route" && s.Expected != "" {
			target = s.Expected
		}
		if target != "" {
			if f, ok := owner[target]; ok {
				if current != "" && f != current {
					crossed = true
				}
				current = f
				if !seen[f] {
					seen[f] = true
					participants = append(participants, f)
				}
			} else {
				// A route no feature declares — the dashboard case. We cannot
				// say the flow crossed INTO a feature, so treat it as leaving
				// the current one without establishing a new owner.
				current = ""
			}
			continue
		}
		if crossed && s.Verify == "state" {
			return true, participants
		}
	}
	return false, nil
}
