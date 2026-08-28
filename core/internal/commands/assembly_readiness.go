// parlay-feature: parlay-tool/page-assembly-derivation
// parlay-component: assembly-readiness
//
// Wires the assembly derivation to the boundary that can act on it.
//
// The lesson this release keeps relearning is that a checker nobody calls is
// documentation. The criterion walkers were graduated to error and reachable
// from nothing; the coverage review was applied and proved nothing; supersedes:
// was parsed and composed nothing. So the derivation lands with its caller
// attached: readiness computes the expected assembly suite and diffs the
// authored one against it, on the path the code boundary already consults.
package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// authoredTestcases is the assembly-relevant read of testcases.yaml. Suites
// and cases only — no criterion accounting, which resolveCases already owns.
type authoredTestcases struct {
	SchemaVersion int `yaml:"schema_version"`
	Suites        []struct {
		Name   string `yaml:"name"`
		Kind   string `yaml:"kind"`
		Scope  string `yaml:"scope"`
		Origin string `yaml:"origin"`
		Page   string `yaml:"page"`
		Route  string `yaml:"route"`
		Cases  []struct {
			Name      string `yaml:"name"`
			Coverage  string `yaml:"coverage"`
			Criterion struct {
				Ref  string `yaml:"ref"`
				Text string `yaml:"text"`
			} `yaml:"criterion"`
			Derivation *struct {
				Kind      string `yaml:"kind"`
				Page      string `yaml:"page"`
				Subject   string `yaml:"subject"`
				Assertion string `yaml:"assertion"`
			} `yaml:"derivation"`
			Steps []struct {
				Action   string    `yaml:"action"`
				Verify   string    `yaml:"verify"`
				Target   string    `yaml:"target"`
				Value    string    `yaml:"value"`
				Expected yaml.Node `yaml:"expected"`
			} `yaml:"steps"`
		} `yaml:"cases"`
		PendingAssertions []struct {
			Page      string `yaml:"page"`
			Subject   string `yaml:"subject"`
			Assertion string `yaml:"assertion"`
			Needs     string `yaml:"needs_capability"`
		} `yaml:"pending_assertions"`
	} `yaml:"suites"`
}

// parseAuthoredSuites also returns the file's declared schema_version. The
// revision decides whether a missing assembly suite warns or blocks: released
// v2 artifacts predate `origin:` and get a rebuild window, while a v3 file —
// the shape build-feature now writes — is one where the fact could have been
// recorded, so omitting the whole mechanism must not advance forever.
func parseAuthoredSuites(content []byte) ([]agent.AuthoredSuite, int, error) {
	var doc authoredTestcases
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, 0, err
	}
	out := make([]agent.AuthoredSuite, 0, len(doc.Suites))
	for _, s := range doc.Suites {
		page := s.Page
		if page == "" {
			// A derived suite is per-route; the route is the page it composes
			// when no explicit page: is carried.
			page = s.Route
		}
		as := agent.AuthoredSuite{Name: s.Name, Kind: s.Kind, Scope: s.Scope, Origin: s.Origin, Page: page}
		for _, p := range s.PendingAssertions {
			as.PendingAssertions = append(as.PendingAssertions, agent.AuthoredPending{
				Page: p.Page, Subject: p.Subject, Assertion: p.Assertion, Needs: p.Needs,
			})
		}
		for _, c := range s.Cases {
			ac := agent.AuthoredCase{
				Name:          c.Name,
				Coverage:      c.Coverage,
				CriterionRef:  c.Criterion.Ref,
				CriterionText: c.Criterion.Text,
			}
			for _, st := range c.Steps {
				ac.Steps = append(ac.Steps, agent.AssemblyStep{
					Action: st.Action, Verify: st.Verify, Target: st.Target,
					Value: st.Value, Expected: st.Expected.Value,
				})
			}
			if c.Derivation != nil {
				ac.Derivation = &agent.AuthoredDerivation{
					Kind:      c.Derivation.Kind,
					Page:      c.Derivation.Page,
					Subject:   c.Derivation.Subject,
					Assertion: c.Derivation.Assertion,
				}
			}
			as.Cases = append(as.Cases, ac)
		}
		out = append(out, as)
	}
	return out, doc.SchemaVersion, nil
}

// expectedAssemblySuites derives what this feature's assembly suites must
// contain, from the RESOLVED composition rather than from its own surface.
//
// The order is the one the resolver's doc comment fixes: resolve project-wide,
// filter to the page, then to the owner. A feature-local derivation would
// demand a mount assertion for a fragment another feature has retired.
//
// Returns blockers rather than swallowing failures. Every early return here
// used to hand back a nil map, which readiness then read as "this feature
// expects no assembly suites" — turning "the composition cannot be
// established" into "there is nothing to check", which is the fail-open shape
// this whole release exists to remove.
func expectedAssemblySuites(cfg *config.Context, slug string) (map[string]agent.AssemblySuite, []string) {
	specDir := filepath.Join(cfg.Root.Path, config.SpecDir)
	fragments, err := parser.ScanAllSurfaces(specDir)
	if err != nil {
		if os.IsNotExist(err) {
			// A project with no spec/intents at all derives nothing, and that
			// is a real answer rather than an unreadable one.
			return map[string]agent.AssemblySuite{}, nil
		}
		return nil, []string{fmt.Sprintf(
			"surfaces for %s cannot be scanned: %v — the composed page cannot be established, so the assembly suites it requires are unknown", slug, err)}
	}

	view := agent.ResolveActiveView(fragments)

	// A refused composition is not a composed page. While a fork, a cycle, a
	// duplicate ref or a dangling supersedes: target stands, nobody can say
	// which fragment owns the slot — so the derived suite would be asserting
	// mounts for a page the tool has explicitly declined to resolve.
	var blockers []string
	for _, e := range view.Errors {
		if e.Severity == "error" {
			blockers = append(blockers, fmt.Sprintf(
				"[%s] %s — the composition is unresolved, so the assembly suite for the affected page cannot be derived", e.Code, e.Message))
		}
	}
	if len(blockers) > 0 {
		return nil, blockers
	}

	caps, capErr := adapterAssemblyCapabilities(cfg)
	if capErr != nil {
		return nil, []string{fmt.Sprintf(
			"the presentation adapter for %s cannot be read: %v — whether an assembly assertion is executable or capability debt cannot be decided", slug, capErr)}
	}

	mine := agent.OwnedBy(view.Active, slug)
	out := map[string]agent.AssemblySuite{}
	for _, page := range agent.AssemblyPagesFor(mine) {
		onPage := agent.OwnedBy(view.ActiveOnPage(page), slug)
		out[page] = agent.DeriveAssemblySuite(page, onPage, caps)
	}
	return out, nil
}

// adapterAssemblyCapabilities reads what the PRESENTATION adapter declares it
// can assert.
//
// Resolved through presentationAdapterFile — the adapter-set's presentation
// slot, with multi-root parent inheritance and a refusal when several adapters
// are present and none is pinned. An earlier version unioned `render-support:`
// across every file in .parlay/adapters, which let a backend adapter's
// declaration make a presentation assertion look executable, and read an
// unreadable active adapter as "declares nothing" — silently converting a real
// capability into debt.
//
// Absence of a declaration is still absence: no adapter in the tree declares
// either term today, so every assembly assertion currently lands in Pending.
// That is reported as a warning, never a blocker — see ValidateAssemblySuites.
func adapterAssemblyCapabilities(cfg *config.Context) (agent.AdapterCapabilities, error) {
	caps := agent.AdapterCapabilities{RenderSupport: map[string]bool{}}

	path := presentationAdapterFile(cfg)
	if path == "" {
		// No adapter pinned and none resolvable. Nothing is executable, which
		// is the same answer as an adapter declaring nothing — and unlike an
		// unreadable adapter, it is a state the project is legitimately in
		// before onboarding.
		return caps, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return caps, fmt.Errorf("read %s: %w", path, err)
	}
	var doc struct {
		Kind          string   `yaml:"kind"`
		RenderSupport []string `yaml:"render-support"`
		HitTesting    *bool    `yaml:"hit-testing-support"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return caps, fmt.Errorf("parse %s: %w", path, err)
	}
	for _, level := range doc.RenderSupport {
		caps.RenderSupport[level] = true
	}
	if doc.HitTesting != nil && *doc.HitTesting {
		caps.HitTesting = true
	}
	return caps, nil
}

// checkAssemblyReadiness is the readiness-facing entry point: derive, diff,
// and report. Blockers and warnings are separated by the severity the
// validators assign, so the capability-debt warning cannot become a blocker by
// accident.
func checkAssemblyReadiness(cfg *config.Context, slug string) (blockers, warnings []string) {
	tcPath := filepath.Join(cfg.BuildPath(slug), "testcases.yaml")
	content, err := os.ReadFile(tcPath)
	if err != nil {
		// A missing or unreadable testcases file is already reported by
		// CheckTestcasesReadiness; saying it twice adds noise, not safety.
		return nil, nil
	}
	authored, revision, err := parseAuthoredSuites(content)
	if err != nil {
		return []string{fmt.Sprintf(
			"testcases for %s cannot be read for assembly validation: %v", slug, err)}, nil
	}

	expected, expBlockers := expectedAssemblySuites(cfg, slug)
	if len(expBlockers) > 0 {
		return expBlockers, nil
	}
	if len(expected) == 0 && len(authored) == 0 {
		return nil, nil
	}

	findings := agent.ValidateAssemblySuites(expected, authored, revision)
	findings = append(findings, agent.FindAssemblyAssertionsInContractSuites(expected, authored)...)

	for _, f := range findings {
		line := "[" + f.Code + "] " + f.Message + " — " + f.Fix
		if f.Severity == "warning" {
			warnings = append(warnings, line)
			continue
		}
		blockers = append(blockers, line)
	}
	return blockers, warnings
}
